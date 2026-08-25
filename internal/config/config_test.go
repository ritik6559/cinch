package config

import (
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	for _, name := range []string{
		"CINCH_PROVIDER", "CINCH_MODEL", "OPENAI_MODEL", "CINCH_BASE_URL",
		"CINCH_COMPACT_AT", "CINCH_EFFORT", "CINCH_API_KEY", "OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	} {
		t.Setenv(name, "")
	}
}

func TestDefaults(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Provider != defaultProvider {
		t.Errorf("provider = %q, want %q", cfg.Provider, defaultProvider)
	}
	if cfg.Model != defaultModel {
		t.Errorf("model = %q, want %q", cfg.Model, defaultModel)
	}
	if cfg.CompactAt != defaultCompactAt {
		t.Errorf("compactAt = %d, want %d", cfg.CompactAt, defaultCompactAt)
	}
	if cfg.Effort != "" {
		t.Errorf("effort = %q, want empty so the provider decides", cfg.Effort)
	}
}

func TestCompactAtIsReturned(t *testing.T) {
	isolate(t)
	t.Setenv("CINCH_COMPACT_AT", "4242")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CompactAt != 4242 {
		t.Errorf("compactAt = %d, want 4242", cfg.CompactAt)
	}
}

func TestCompactAtRejectsNonsense(t *testing.T) {
	isolate(t)
	t.Setenv("CINCH_COMPACT_AT", "lots")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a non-numeric CINCH_COMPACT_AT")
	}
}

func TestEffortIsValidated(t *testing.T) {
	isolate(t)

	for _, effort := range llm.Efforts {
		t.Setenv("CINCH_EFFORT", effort)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("%q should be accepted: %v", effort, err)
		}
		if cfg.Effort != effort {
			t.Errorf("effort = %q, want %q", cfg.Effort, effort)
		}
	}

	t.Setenv("CINCH_EFFORT", "hgh")
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error for a misspelled effort")
	}
	if !strings.Contains(err.Error(), "medium") {
		t.Errorf("the error should list the valid values, got: %v", err)
	}
}

func TestModelAliases(t *testing.T) {
	isolate(t)

	t.Setenv("OPENAI_MODEL", "from-openai-var")
	cfg, _ := Load()
	if cfg.Model != "from-openai-var" {
		t.Errorf("model = %q, want the OPENAI_MODEL value", cfg.Model)
	}

	t.Setenv("CINCH_MODEL", "from-cinch-var")
	cfg, _ = Load()
	if cfg.Model != "from-cinch-var" {
		t.Errorf("model = %q, want CINCH_MODEL to win", cfg.Model)
	}
}

func TestKeyLookupIsPerProvider(t *testing.T) {
	isolate(t)
	t.Setenv("CINCH_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "anthropic-key" {
		t.Errorf("apiKey = %q, want the provider's own variable", cfg.APIKey)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidateNeedsAKey(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error with no key set")
	}
}
