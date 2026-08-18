package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError is a failure reported by the provider.
//
// It is a struct rather than a formatted string so the caller can decide what
// to do with it. A wrong API key must stop the program; a rate limit should
// simply be tried again. As one string, those two are indistinguishable.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RetryAfter time.Duration // from the Retry-After header, 0 when absent
}

func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("%s (http %d, %s)", e.Message, e.StatusCode, e.Type)
	}
	return fmt.Sprintf("%s (http %d)", e.Message, e.StatusCode)
}

// Retryable reports whether sending the same request again could succeed.
//
// A 4xx code means the request itself is wrong, so repeating it cannot help.
// The three below are the exceptions: they mean "not now", not "never".
func (e *APIError) Retryable() bool {
	switch e.StatusCode {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return true
	}
	return e.StatusCode >= 500
}

// decodeError reads the provider's JSON error body:
//
//	{"error": {"message": "Rate limit reached", "type": "rate_limit_error"}}
//
// A proxy or gateway may return HTML instead, so the raw body is kept as a
// fallback rather than losing the message entirely.
func decodeError(status int, body []byte, header http.Header) *APIError {
	e := &APIError{
		StatusCode: status,
		Message:    strings.TrimSpace(string(body)),
		RetryAfter: retryAfter(header),
	}

	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		e.Message = envelope.Error.Message
		e.Type = envelope.Error.Type
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}

// retryAfter reads the Retry-After header, which is a number of seconds. When
// the server tells us how long to wait, that is better than any guess we make.
func retryAfter(h http.Header) time.Duration {
	value := h.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
