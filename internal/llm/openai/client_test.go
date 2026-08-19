package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastBackoff shrinks the retry delay so a test finishes in milliseconds
// instead of seconds. The original value is restored when the test ends.
func fastBackoff(t *testing.T) {
	t.Helper()
	original := backoffBase
	backoffBase = time.Millisecond
	t.Cleanup(func() { backoffBase = original })
}

// A temporary failure must be retried until it succeeds.
func TestRetriesUntilSuccess(t *testing.T) {
	fastBackoff(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error"}}`))
			return
		}
		sse(w, `{"type":"response.completed","response":{
			"output":[],"usage":{"input_tokens":7,"output_tokens":2}
		}}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	resp, err := client.Call(context.Background(), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d requests, want 3", got)
	}
	if resp.Usage.InputTokens != 7 {
		t.Errorf("got %d input tokens, want 7", resp.Usage.InputTokens)
	}
}

// A wrong API key can never succeed, so it must not be retried.
func TestDoesNotRetryPermanentFailure(t *testing.T) {
	fastBackoff(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	client := New("wrong-key", "test-model", WithBaseURL(server.URL))

	_, err := client.Call(context.Background(), "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error from a 401")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1: a 401 must not be retried", got)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", apiErr.StatusCode)
	}
	// The message must come from the JSON field, not the whole raw body.
	if apiErr.Message != "Incorrect API key provided" {
		t.Errorf("got message %q, want the message field from the body", apiErr.Message)
	}
	if apiErr.Type != "invalid_request_error" {
		t.Errorf("got type %q, want invalid_request_error", apiErr.Type)
	}
}

// A gateway can return HTML instead of JSON. The text must survive rather than
// being replaced by an empty message.
func TestNonJSONErrorBodyIsKept(t *testing.T) {
	fastBackoff(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	_, err := client.Call(context.Background(), "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error from a 502")
	}
	if !strings.Contains(err.Error(), "Bad Gateway") {
		t.Errorf("error lost the body text: %q", err)
	}
}

// Ctrl-C during a retry wait must stop immediately.
func TestCancelledContextStopsImmediately(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	_, err := client.Call(ctx, "", nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if got := attempts.Load(); got != 0 {
		t.Errorf("server saw %d requests, want 0 with an already cancelled context", got)
	}
}

func TestBackoffPrefersRetryAfter(t *testing.T) {
	withHeader := &APIError{StatusCode: 429, RetryAfter: 20 * time.Second}
	if got := backoff(2, withHeader); got != 20*time.Second {
		t.Errorf("got %v, want the server's Retry-After of 20s", got)
	}

	// Without the header, each wait doubles.
	plain := &APIError{StatusCode: 500}
	if got := backoff(2, plain); got != backoffBase {
		t.Errorf("second attempt waits %v, want %v", got, backoffBase)
	}
	if got := backoff(3, plain); got != 2*backoffBase {
		t.Errorf("third attempt waits %v, want %v", got, 2*backoffBase)
	}
}

func TestRetryable(t *testing.T) {
	cases := map[int]bool{
		400: false, // malformed request
		401: false, // wrong key
		404: false,
		408: true, // request timeout
		409: true, // conflict
		429: true, // rate limited
		500: true,
		503: true,
	}
	for status, want := range cases {
		if got := (&APIError{StatusCode: status}).Retryable(); got != want {
			t.Errorf("status %d: Retryable() = %v, want %v", status, got, want)
		}
	}
}

// Wrong JSON tags fail silently: they simply give zero. These two values live
// inside nested objects, so they are the ones most likely to be lost.
func TestUsageFlattensDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, `{"type":"response.completed","response":{
			"output": [],
			"usage": {
				"input_tokens": 1240,
				"input_tokens_details":  {"cached_tokens": 980},
				"output_tokens": 310,
				"output_tokens_details": {"reasoning_tokens": 256}
			}
		}}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	resp, err := client.Call(context.Background(), "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := Usage{InputTokens: 1240, OutputTokens: 310, CachedTokens: 980, ReasoningTokens: 256}
	if resp.Usage != want {
		t.Errorf("got %+v, want %+v", resp.Usage, want)
	}
}

func TestUsageAdd(t *testing.T) {
	total := Usage{InputTokens: 100, OutputTokens: 20}
	total.Add(Usage{InputTokens: 50, OutputTokens: 10, ReasoningTokens: 8})

	want := Usage{InputTokens: 150, OutputTokens: 30, ReasoningTokens: 8}
	if total != want {
		t.Errorf("got %+v, want %+v", total, want)
	}
}

// sse writes events in the wire format the API uses, flushing each one so the
// client really does receive them one at a time.
func sse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		// A data value must be one line. The payloads below are written across
		// several lines to stay readable, so compact them first. This also
		// fails loudly if a test contains malformed JSON.
		var oneLine bytes.Buffer
		if err := json.Compact(&oneLine, []byte(e)); err != nil {
			panic("bad test event JSON: " + err.Error())
		}
		fmt.Fprintf(w, "data: %s\n\n", oneLine.String())
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func TestStreamingDeltasAndFinalResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"type":"response.created"}`,
			`{"type":"response.output_text.delta","delta":"Hello"}`,
			`{"type":"response.output_text.delta","delta":" there"}`,
			`{"type":"response.completed","response":{
				"output":[{"type":"message","content":[{"type":"output_text","text":"Hello there"}]}],
				"usage":{"input_tokens":42,"output_tokens":3}
			}}`,
		)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	var streamed strings.Builder
	resp, err := client.Call(context.Background(), "", nil, nil, func(s string) {
		streamed.WriteString(s)
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := streamed.String(); got != "Hello there" {
		t.Errorf("streamed %q, want %q", got, "Hello there")
	}
	// The final object, not the deltas, is what the agent works from.
	if texts := resp.Texts(); len(texts) != 1 || texts[0] != "Hello there" {
		t.Errorf("final response text = %v", texts)
	}
	if resp.Usage.InputTokens != 42 {
		t.Errorf("got %d input tokens, want 42", resp.Usage.InputTokens)
	}
}

// A dropped connection after a 200 must not look like an empty answer.
func TestStreamWithoutCompletedFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, `{"type":"response.output_text.delta","delta":"half a sen"}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	if _, err := client.Call(context.Background(), "", nil, nil, func(string) {}); err == nil {
		t.Fatal("expected an error when the stream ends early")
	}
}

// Once text has reached the screen, retrying would print the answer twice.
func TestNoRetryOnceTextHasBeenWritten(t *testing.T) {
	fastBackoff(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		sse(w, `{"type":"response.output_text.delta","delta":"partial"}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	if _, err := client.Call(context.Background(), "", nil, nil, func(string) {}); err == nil {
		t.Fatal("expected an error")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1: no retry after output began", got)
	}
}

// With no display callback there is nothing to duplicate, so a broken stream is
// still retried.
func TestRetriesWhenNobodyIsWatching(t *testing.T) {
	fastBackoff(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 2 {
			sse(w, `{"type":"response.output_text.delta","delta":"partial"}`)
			return
		}
		sse(w, `{"type":"response.completed","response":{"output":[],"usage":{}}}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	if _, err := client.Call(context.Background(), "", nil, nil, nil); err != nil {
		t.Fatalf("expected the broken stream to be retried, got %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("server saw %d requests, want 2", got)
	}
}

func TestStreamErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, `{"type":"error","message":"model overloaded"}`)
	}))
	defer server.Close()

	client := New("test-key", "test-model", WithBaseURL(server.URL))

	_, err := client.Call(context.Background(), "", nil, nil, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("got %v, want the message from the error event", err)
	}
}
