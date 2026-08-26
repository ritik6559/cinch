package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func modelServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want Bearer k", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestModelsFiltersAndSorts(t *testing.T) {
	srv := modelServer(t, http.StatusOK, `{"data":[
		{"id":"gpt-5.6-mini"},
		{"id":"text-embedding-3-large"},
		{"id":"gpt-4o-realtime-preview"},
		{"id":"gpt-5.6"},
		{"id":"whisper-1"},
		{"id":"gpt-4o-2024-08-06"},
		{"id":"o3"},
		{"id":"stable-diffusion"}
	]}`)

	c := New("k", "gpt-5.6", WithBaseURL(srv.URL+"/v1/responses"))

	got, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}

	want := []string{"gpt-5.6", "gpt-5.6-mini", "o3"}
	if !slices.Equal(got, want) {
		t.Errorf("Models = %v, want %v", got, want)
	}
}

func TestModelsFallsBackOnError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusInternalServerError, `nope`},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"bad key"}}`},
		{"broken json", http.StatusOK, `{"data":[`},
		{"nothing usable", http.StatusOK, `{"data":[{"id":"whisper-1"}]}`},
		{"empty list", http.StatusOK, `{"data":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := modelServer(t, tt.status, tt.body)
			c := New("k", "gpt-5.6", WithBaseURL(srv.URL+"/v1/responses"))

			got, _ := c.Models(context.Background())
			if !slices.Equal(got, fallbackModels) {
				t.Errorf("Models = %v, want the fallback list %v", got, fallbackModels)
			}
		})
	}
}

func TestUsableModel(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"gpt-5.6", true},
		{"gpt-5.6-mini", true},
		{"o3", true},
		{"o4-mini", true},
		{"claude-opus-4-1", true},
		{"gemini-2.5-pro", true},
		{"text-embedding-3-small", false},
		{"whisper-1", false},
		{"tts-1-hd", false},
		{"dall-e-3", false},
		{"omni-moderation-latest", false},
		{"gpt-4o-realtime-preview", false},
		{"gpt-4o-audio-preview", false},
		{"gpt-4o-transcribe", false},
		{"gpt-4o-search-preview", false},
		{"codex-mini-latest", false},
		{"gpt-4o-2024-08-06", false}, // dated snapshot
		{"davinci-002", false},       // not a chat family we know
	}

	for _, tt := range tests {
		if got := usableModel(tt.id); got != tt.want {
			t.Errorf("usableModel(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}
