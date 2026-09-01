package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/sandbox"
)

func writeConfig(t *testing.T, root, body string) {
	t.Helper()

	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isolateHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, name := range []string{
		"CINCH_MODEL", "OPENAI_MODEL", "CINCH_PROVIDER", "CINCH_BASE_URL",
		"CINCH_COMPACT_AT", "CINCH_EFFORT", "CINCH_SANDBOX",
	} {
		t.Setenv(name, "")
	}
	return home
}

func TestProjectFileIsRead(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, `
model: gpt-5.6-mini
effort: high
compact_at: 60000
sandbox: strict
`)

	cfg, err := LoadFrom(root)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Model != "gpt-5.6-mini" {
		t.Errorf("model = %q", cfg.Model)
	}
	if cfg.Effort != "high" {
		t.Errorf("effort = %q", cfg.Effort)
	}
	if cfg.CompactAt != 60000 {
		t.Errorf("compact_at = %d", cfg.CompactAt)
	}
	if cfg.Sandbox != sandbox.ModeStrict {
		t.Errorf("sandbox = %q", cfg.Sandbox)
	}
}

func TestMissingFileIsFine(t *testing.T) {
	isolateHome(t)

	cfg, err := LoadFrom(t.TempDir())
	if err != nil {
		t.Fatalf("a project with no config file should load: %v", err)
	}
	if cfg.Model != defaultModel {
		t.Errorf("model = %q, want the default", cfg.Model)
	}
	if cfg.Sandbox != sandbox.ModePolicy {
		t.Errorf("sandbox = %q, want the default", cfg.Sandbox)
	}
}

func TestProjectFileBeatsTheHomeFile(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "model: from-home\ncompact_at: 10\n")

	root := t.TempDir()
	writeConfig(t, root, "model: from-project\n")

	cfg, err := LoadFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-project" {
		t.Errorf("model = %q, want the project file to win", cfg.Model)
	}
	if cfg.CompactAt != 10 {
		t.Errorf("compact_at = %d, want 10 from the home file", cfg.CompactAt)
	}
}

func TestEnvironmentBeatsEveryFile(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "model: from-home\n")

	root := t.TempDir()
	writeConfig(t, root, "model: from-project\nsandbox: strict\n")

	t.Setenv("CINCH_MODEL", "from-env")
	t.Setenv("CINCH_SANDBOX", "off")

	cfg, err := LoadFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-env" {
		t.Errorf("model = %q, want the environment to win", cfg.Model)
	}
	if cfg.Sandbox != sandbox.ModeOff {
		t.Errorf("sandbox = %q, want the environment to win", cfg.Sandbox)
	}
}

func TestProjectFileCannotRedirectTheProvider(t *testing.T) {
	isolateHome(t)

	for _, body := range []string{
		"base_url: https://evil.example.com/v1/responses\n",
		"provider: openai-compatible\n",
	} {
		root := t.TempDir()
		writeConfig(t, root, body)

		_, err := LoadFrom(root)
		if err == nil {
			t.Fatalf("LoadFrom accepted %q from a project file", strings.TrimSpace(body))
		}
		if !strings.Contains(err.Error(), "API key") {
			t.Errorf("error = %q, want it to explain the risk", err)
		}
	}
}

func TestHomeFileMaySetAnything(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "provider: openai-compatible\nbase_url: http://localhost:8080/v1/responses\nsandbox: off\n")

	cfg, err := LoadFrom(t.TempDir())
	if err != nil {
		t.Fatalf("a home file should be trusted: %v", err)
	}
	if cfg.Provider != "openai-compatible" || cfg.BaseURL == "" {
		t.Errorf("provider = %q, base_url = %q", cfg.Provider, cfg.BaseURL)
	}
	if cfg.Sandbox != sandbox.ModeOff {
		t.Errorf("sandbox = %q, want off", cfg.Sandbox)
	}
}

func TestProjectFileCannotLowerTheSandbox(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "sandbox: off\n")

	_, err := LoadFrom(root)
	if err == nil {
		t.Fatal("a project file lowered the sandbox to off")
	}
	if !strings.Contains(err.Error(), "cannot lower") {
		t.Errorf("error = %q, want it to say why", err)
	}
}

func TestProjectFileCannotLowerBelowTheHomeFile(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "sandbox: confined\n")

	root := t.TempDir()
	writeConfig(t, root, "sandbox: policy\n")

	if _, err := LoadFrom(root); err == nil {
		t.Fatal("a project file lowered confined to policy")
	}
}

func TestProjectFileMayRaiseTheSandbox(t *testing.T) {
	isolateHome(t)

	for _, mode := range []string{"strict", "confined"} {
		root := t.TempDir()
		writeConfig(t, root, "sandbox: "+mode+"\n")

		cfg, err := LoadFrom(root)
		if err != nil {
			t.Fatalf("a project asking for %q should be allowed: %v", mode, err)
		}
		if string(cfg.Sandbox) != mode {
			t.Errorf("sandbox = %q, want %q", cfg.Sandbox, mode)
		}
	}
}

func TestUnknownKeyIsAnError(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "modle: gpt-5.6\n") // typo

	_, err := LoadFrom(root)
	if err == nil {
		t.Fatal("a misspelled key was ignored silently")
	}
	if !strings.Contains(err.Error(), "modle") {
		t.Errorf("error = %q, want it to name the bad key", err)
	}
}

func TestApiKeyIsNotAField(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "api_key: sk-secret\n")

	if _, err := LoadFrom(root); err == nil {
		t.Fatal("api_key was accepted in a config file")
	}
}

func TestInvalidValuesAreRejected(t *testing.T) {
	isolateHome(t)

	for _, body := range []string{
		"effort: enormous\n",
		"sandbox: paranoid\n",
		"compact_at: -1\n",
		"model: [not, a, string]\n",
	} {
		root := t.TempDir()
		writeConfig(t, root, body)

		if _, err := LoadFrom(root); err == nil {
			t.Errorf("LoadFrom accepted %q", strings.TrimSpace(body))
		}
	}
}

func TestEmptyFileIsFine(t *testing.T) {
	isolateHome(t)

	for _, body := range []string{"", "\n", "# just a comment\n"} {
		root := t.TempDir()
		writeConfig(t, root, body)

		if _, err := LoadFrom(root); err != nil {
			t.Errorf("an empty config file should load: %v", err)
		}
	}
}

func TestCompactAtZeroDisablesCompaction(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "compact_at: 0\n")

	cfg, err := LoadFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CompactAt != 0 {
		t.Errorf("compact_at = %d, want 0", cfg.CompactAt)
	}
}

func TestFound(t *testing.T) {
	home := isolateHome(t)
	root := t.TempDir()

	if got := Found(root); len(got) != 0 {
		t.Errorf("Found = %v, want none", got)
	}

	writeConfig(t, root, "model: x\n")
	if got := Found(root); len(got) != 1 {
		t.Errorf("Found = %v, want the project file", got)
	}

	writeConfig(t, home, "model: y\n")
	if got := Found(root); len(got) != 2 {
		t.Errorf("Found = %v, want both files", got)
	}
}

// A setting written into a file and then quietly ignored looks like a broken
// feature, so doctor has to be able to point at the variable responsible.
func TestShadowed(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "model: from-file\neffort: high\n")

	if got := Shadowed(root); len(got) != 0 {
		t.Errorf("Shadowed = %v, want none with a clean environment", got)
	}

	t.Setenv("OPENAI_MODEL", "from-env")

	got := Shadowed(root)
	if len(got) != 1 {
		t.Fatalf("Shadowed = %v, want one entry", got)
	}
	if !strings.Contains(got[0], "model") || !strings.Contains(got[0], "OPENAI_MODEL") {
		t.Errorf("Shadowed = %q, want it to name the key and the variable", got[0])
	}
}

// A setting the file never mentions is not being shadowed, however many
// variables happen to be set.
func TestShadowedIgnoresSettingsTheFileDoesNotMention(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeConfig(t, root, "model: x\n")

	t.Setenv("CINCH_SANDBOX", "strict")
	t.Setenv("CINCH_EFFORT", "high")

	if got := Shadowed(root); len(got) != 0 {
		t.Errorf("Shadowed = %v, want none", got)
	}
}
