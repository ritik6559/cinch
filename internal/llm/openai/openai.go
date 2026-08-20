package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/version"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/responses"
	maxAttempts    = 3
	maxEventBytes  = 8 * 1024 * 1024
)

// backoffBase is the first retry delay. It is a variable rather than a constant
// so tests can shrink it: otherwise every retry test would take seconds.
var backoffBase = time.Second

type Client struct {
	apikey  string
	model   string
	baseURL string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.baseURL = url
		}
	}
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

func New(apiKey, model string, opts ...Option) *Client {
	c := &Client{
		apikey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Client implements llm.Provider against the OpenAI Responses API.
var _ llm.Provider = (*Client)(nil)

func (c *Client) Name() string { return "openai" }

func (c *Client) Complete(ctx context.Context, req llm.Request, onText func(string)) (*llm.Response, error) {
	payload, err := c.lowerRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	raw, err := c.post(ctx, body, onText)
	if err != nil {
		return nil, err
	}
	return raiseResponse(raw)
}

func (c *Client) post(ctx context.Context, body []byte, onText func(string)) ([]byte, error) {
	var lastErr error
	var written bool

	emit := onText
	if onText != nil {
		emit = func(s string) {
			written = true
			onText(s)
		}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if err := wait(ctx, backoff(attempt, lastErr)); err != nil {
				return nil, err
			}
		}

		raw, err := c.do(ctx, body, emit)
		if err == nil {
			return raw, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if written {
			return nil, fmt.Errorf("stream failed after output began: %w", err)
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) && !apiErr.Retryable() {
			return nil, err
		}
	}

	return nil, fmt.Errorf("gave up after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) do(ctx context.Context, body []byte, onText func(string)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apikey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "text/event-stream")

	httpResp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		raw, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, decodeError(httpResp.StatusCode, raw, httpResp.Header)
	}

	return readStream(httpResp.Body, onText)
}

func readStream(body io.Reader, onText func(string)) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventBytes)

	var completed []byte

	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}

		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Message  string          `json:"message"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.output_text.delta":
			if onText != nil && event.Delta != "" {
				onText(event.Delta)
			}

		case "response.completed":
			completed = event.Response

		case "error", "response.failed", "response.incomplete":
			return nil, streamError(event.Message, event.Response)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	if completed == nil {
		return nil, errors.New("stream ended without a completed event")
	}
	return completed, nil
}

func streamError(message string, response json.RawMessage) error {
	if message != "" {
		return fmt.Errorf("stream failed: %s", message)
	}

	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response, &body); err == nil && body.Error.Message != "" {
		return fmt.Errorf("stream failed: %s", body.Error.Message)
	}
	return errors.New("stream failed")
}

func backoff(attempt int, err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	return time.Duration(1<<(attempt-2)) * backoffBase
}

func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
