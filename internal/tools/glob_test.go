package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func globWorkspace(t *testing.T, paths ...string) *Tools {
	t.Helper()

	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func runGlob(t *testing.T, ts *Tools, pattern, path string) []string {
	t.Helper()

	args := `{"pattern":"` + pattern + `"`
	if path != "" {
		args += `,"path":"` + path + `"`
	}
	args += `}`

	out := ts.Run(context.Background(), "glob", args)
	if strings.HasPrefix(out, "no files match") {
		return nil
	}
	if strings.HasPrefix(out, "error:") {
		t.Fatalf("glob %q: %s", pattern, out)
	}
	return strings.Split(out, "\n")
}

func TestGlobStarStaysWithinOneName(t *testing.T) {
	ts := globWorkspace(t, "main.go", "readme.md", "internal/agent/agent.go")

	got := runGlob(t, ts, "*.go", "")
	want := []string{"main.go"}

	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v: * must not cross directories", got, want)
	}
}

func TestGlobDoubleStarCrossesDirectories(t *testing.T) {
	ts := globWorkspace(t,
		"main.go",
		"internal/agent/agent.go",
		"internal/tools/glob.go",
		"readme.md",
	)

	got := runGlob(t, ts, "**/*.go", "")
	want := []string{"internal/agent/agent.go", "internal/tools/glob.go", "main.go"}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// "**/" spans zero directories too, so a file in the root must still match.
func TestGlobDoubleStarMatchesRootFiles(t *testing.T) {
	ts := globWorkspace(t, "main.go", "sub/other.go")

	got := runGlob(t, ts, "**/*.go", "")
	if len(got) != 2 {
		t.Errorf("got %v, want both files", got)
	}
}

func TestGlobQuestionMark(t *testing.T) {
	ts := globWorkspace(t, "a.go", "ab.go", "b.go")

	got := runGlob(t, ts, "?.go", "")
	if len(got) != 2 {
		t.Errorf("got %v, want a.go and b.go", got)
	}
}

func TestGlobResultsAreSorted(t *testing.T) {
	ts := globWorkspace(t, "z.go", "a.go", "m.go")

	got := runGlob(t, ts, "*.go", "")
	if !sortedStrings(got) {
		t.Errorf("results are not sorted: %v", got)
	}
}

func TestGlobSearchesUnderPath(t *testing.T) {
	ts := globWorkspace(t, "main.go", "internal/agent/agent.go", "internal/tools/glob.go")

	got := runGlob(t, ts, "*.go", "internal/tools")
	if len(got) != 1 || got[0] != "internal/tools/glob.go" {
		t.Errorf("got %v, want only the file under internal/tools", got)
	}
}

func TestGlobSkipsDependencyDirectories(t *testing.T) {
	ts := globWorkspace(t, "main.go", "node_modules/pkg/index.go", "vendor/lib/x.go")

	got := runGlob(t, ts, "**/*.go", "")
	if len(got) != 1 || got[0] != "main.go" {
		t.Errorf("got %v, want dependency directories skipped", got)
	}
}

func TestGlobSkipsSecretFiles(t *testing.T) {
	ts := globWorkspace(t, ".env", ".env.local", "main.go")

	got := runGlob(t, ts, "**/*", "")
	for _, p := range got {
		if strings.Contains(p, ".env") {
			t.Errorf("a secret file was listed: %v", got)
		}
	}
}

func TestGlobRejectsEscapes(t *testing.T) {
	ts := globWorkspace(t, "main.go")

	out := ts.Run(context.Background(), "glob", `{"pattern":"*.go","path":"../.."}`)
	if !strings.HasPrefix(out, "error:") {
		t.Errorf("got %q, want a refusal", out)
	}
}

func TestGlobNoMatches(t *testing.T) {
	ts := globWorkspace(t, "main.go")

	out := ts.Run(context.Background(), "glob", `{"pattern":"*.rs"}`)
	if !strings.HasPrefix(out, "no files match") {
		t.Errorf("got %q, want a no-match message", out)
	}
}

func TestGlobNeedsAPattern(t *testing.T) {
	ts := globWorkspace(t, "main.go")

	out := ts.Run(context.Background(), "glob", `{}`)
	if !strings.HasPrefix(out, "error:") {
		t.Errorf("got %q, want an error", out)
	}
}

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		{"**/*.go", "sub/main.go", true},
		{"**/*.go", "a/b/c/main.go", true},
		{"**/*.go", "main.go", true},
		{"internal/**/*.go", "internal/tools/glob.go", true},
		{"internal/**/*.go", "cmd/main.go", false},
		{"?.go", "a.go", true},
		{"?.go", "ab.go", false},
		{"*_test.go", "glob_test.go", true},
		{"*_test.go", "glob.go", false},
		// A dot is a regex metacharacter and must be matched literally.
		{"a.go", "axgo", false},
	}

	for _, tc := range cases {
		re, err := globToRegexp(tc.pattern)
		if err != nil {
			t.Fatalf("%q: %v", tc.pattern, err)
		}
		if got := re.MatchString(tc.path); got != tc.want {
			t.Errorf("%q against %q = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
