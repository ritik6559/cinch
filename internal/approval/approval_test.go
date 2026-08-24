package approval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestPrefixStopsAtAWordBoundary(t *testing.T) {
	s := &Store{}
	s.Add("bash", "go test")

	allowed := []string{
		"go test",
		"go test ./...",
		"go test -run TestFoo ./internal/agent",
		"go test\t./...",
	}
	for _, cmd := range allowed {
		if !s.Allows("bash", cmd) {
			t.Errorf("%q should be allowed", cmd)
		}
	}

	denied := []string{
		"go testify",
		"go testsomething",
		"gotest",
		"go build",
		"rm -rf /",
	}
	for _, cmd := range denied {
		if s.Allows("bash", cmd) {
			t.Errorf("%q must not be allowed by the prefix %q", cmd, "go test")
		}
	}
}

func TestRuleAppliesOnlyToItsTool(t *testing.T) {
	s := &Store{}
	s.Add("bash", "go test")

	if s.Allows("write_file", "go test") {
		t.Error("a bash rule must not allow write_file")
	}
}

func TestEmptyPrefixAllowsTheWholeTool(t *testing.T) {
	s := &Store{}
	s.Add("edit_file", "")

	if !s.Allows("edit_file", "") {
		t.Error("an empty prefix should allow the tool")
	}
	if s.Allows("bash", "anything") {
		t.Error("it must not leak to other tools")
	}
}

func TestPrefixFor(t *testing.T) {
	cases := map[string]string{
		"go test ./...":         "go test",
		"go test":               "go test",
		"git diff --stat":       "git diff",
		"make check":            "make check",
		"npm run build":         "npm run",
		"ls -la":                "ls",
		"cat internal/agent.go": "cat",
		"./scripts/deploy.sh":   "./scripts/deploy.sh",
		"go":                    "go",
		"":                      "",
		"  go   test   ./...  ": "go test",
	}

	for command, want := range cases {
		if got := PrefixFor(command); got != want {
			t.Errorf("PrefixFor(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestAddIsDeduplicated(t *testing.T) {
	s := &Store{}

	if !s.Add("bash", "go test") {
		t.Error("the first add should report true")
	}
	if s.Add("bash", "go test") {
		t.Error("a duplicate should report false")
	}
	if len(s.Rules) != 1 {
		t.Errorf("got %d rules, want 1", len(s.Rules))
	}
}

func TestRemove(t *testing.T) {
	s := &Store{}
	s.Add("bash", "go test")
	s.Add("bash", "git diff")
	s.Add("edit_file", "")

	if n := s.Remove("go test"); n != 1 {
		t.Errorf("removed %d, want 1", n)
	}
	if s.Allows("bash", "go test ./...") {
		t.Error("the rule should be gone")
	}

	if n := s.Remove("edit_file"); n != 1 {
		t.Errorf("removed %d, want 1", n)
	}
	if n := s.Remove("nothing here"); n != 0 {
		t.Errorf("removed %d, want 0", n)
	}
	if len(s.Rules) != 1 {
		t.Errorf("got %d rules, want 1 remaining", len(s.Rules))
	}
}

func TestSaveAndLoad(t *testing.T) {
	isolate(t)

	s := &Store{}
	s.Add("bash", "go test")
	s.Add("bash", "git diff")

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(loaded.Rules))
	}
	if !loaded.Allows("bash", "go test ./...") {
		t.Error("a saved rule did not survive")
	}
}

func TestLoadWithNoFile(t *testing.T) {
	isolate(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("a missing file should not be an error: %v", err)
	}
	if len(s.Rules) != 0 {
		t.Errorf("got %d rules, want 0", len(s.Rules))
	}
}

func TestLoadDamagedFile(t *testing.T) {
	isolate(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), fileMode); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("got %v, want a damaged-file error", err)
	}
}

func TestDescribe(t *testing.T) {
	if got := Describe("bash", "go test"); !strings.Contains(got, "go test") {
		t.Errorf("got %q, want the prefix named", got)
	}
	if got := Describe("edit_file", ""); !strings.Contains(got, "every") {
		t.Errorf("got %q, want it to say the whole tool is allowed", got)
	}
}
