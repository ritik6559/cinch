package projectctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workspace(t *testing.T, contents string) string {
	t.Helper()

	root := t.TempDir()
	if contents != "" {
		if err := os.WriteFile(filepath.Join(root, FileName), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMissingFileIsNotAnError(t *testing.T) {
	got, err := Load(workspace(t, ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLoadsInstructions(t *testing.T) {
	root := workspace(t, "  Run tests with: go test ./...\n\n")

	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Run tests with: go test ./..." {
		t.Errorf("got %q, want the trimmed contents", got)
	}
}

func TestWhitespaceOnlyFileIsIgnored(t *testing.T) {
	got, err := Load(workspace(t, "\n\n   \n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestOversizedFileIsTruncated(t *testing.T) {
	got, err := Load(workspace(t, strings.Repeat("x", maxBytes+5000)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > maxBytes+200 {
		t.Errorf("got %d bytes, want it truncated near %d", len(got), maxBytes)
	}
	if !strings.Contains(got, "truncated") {
		t.Error("truncation should be visible in the text")
	}
}

func TestWrapWithoutInstructionsIsUnchanged(t *testing.T) {
	if got := Wrap("base prompt", ""); got != "base prompt" {
		t.Errorf("got %q, want the base prompt unchanged", got)
	}
	if got := Wrap("base prompt", "   \n "); got != "base prompt" {
		t.Errorf("whitespace should count as nothing, got %q", got)
	}
}

func TestWrapKeepsBothAndOrdersThem(t *testing.T) {
	got := Wrap("BASE RULES", "PROJECT RULES")

	base := strings.Index(got, "BASE RULES")
	project := strings.Index(got, "PROJECT RULES")

	if base < 0 || project < 0 {
		t.Fatalf("both parts must survive: %q", got)
	}
	if base > project {
		t.Error("the base prompt must come first")
	}
	if !strings.Contains(got, "the rules above win") {
		t.Error("the precedence rule must be stated, or a repo could override safety rules")
	}
}
