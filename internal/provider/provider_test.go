package provider

import (
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/config"
)

func TestNewKnownProvider(t *testing.T) {
	p, err := New(config.Config{Provider: "openai", APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "openai" {
		t.Errorf("got %q, want openai", p.Name())
	}
}

func TestProviderNameIsCaseInsensitive(t *testing.T) {
	if _, err := New(config.Config{Provider: "OpenAI", APIKey: "k", Model: "m"}); err != nil {
		t.Errorf("provider names should not be case sensitive: %v", err)
	}
}

// The error must say what the valid names are. A bare "unknown provider" leaves
// the user guessing at the spelling.
func TestNewUnknownProviderListsOptions(t *testing.T) {
	_, err := New(config.Config{Provider: "gpt", APIKey: "k"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Errorf("error does not list the known providers: %v", err)
	}
}

func TestCompatibleProviderNeedsBaseURL(t *testing.T) {
	if _, err := New(config.Config{Provider: "openai-compatible", APIKey: "k"}); err == nil {
		t.Fatal("expected an error when CINCH_BASE_URL is missing")
	}

	if _, err := New(config.Config{
		Provider: "openai-compatible",
		APIKey:   "k",
		Model:    "m",
		BaseURL:  "http://localhost:8080/v1/responses",
	}); err != nil {
		t.Errorf("unexpected error with a base URL set: %v", err)
	}
}

func TestNamesIsSorted(t *testing.T) {
	names := Names()
	if len(names) < 2 {
		t.Fatalf("expected at least two providers, got %v", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names() is not sorted: %v", names)
		}
	}
}
