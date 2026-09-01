package tools

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/skills"
)

func catalogWith(t *testing.T, name, body string) skills.Catalog {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	dir := filepath.Join(root, config.DirName, skills.DirName, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skills.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := skills.Load(root)
	if len(catalog.Problems) > 0 {
		t.Fatalf("problems: %v", catalog.Problems)
	}
	return catalog
}

func toolNames(t *testing.T, ts *Tools) []string {
	t.Helper()

	var out []string
	for _, d := range ts.Definitions() {
		out = append(out, d.Name)
	}
	return out
}

func TestSkillToolIsAbsentWithoutSkills(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if slices.Contains(toolNames(t, ts), "skill") {
		t.Error("the skill tool was offered with no skills to read")
	}
}

func TestSkillToolAppearsWithSkills(t *testing.T) {
	catalog := catalogWith(t, "deploy", "---\ndescription: how to ship\n---\n\nRun the pipeline.\n")

	ts, err := New(t.TempDir(), WithSkills(catalog))
	if err != nil {
		t.Fatal(err)
	}

	var def struct{ found bool }
	for _, d := range ts.Definitions() {
		if d.Name != "skill" {
			continue
		}
		def.found = true

		props, _ := d.Schema["properties"].(map[string]any)
		name, _ := props["name"].(map[string]any)

		enum, ok := name["enum"].([]string)
		if !ok || !slices.Equal(enum, []string{"deploy"}) {
			t.Errorf("enum = %v, want the skill names", name["enum"])
		}
	}
	if !def.found {
		t.Fatal("the skill tool was not offered")
	}
}

func TestSkillToolReturnsTheBody(t *testing.T) {
	catalog := catalogWith(t, "deploy", "---\ndescription: how to ship\n---\n\nRun the pipeline.\n")

	ts, err := New(t.TempDir(), WithSkills(catalog))
	if err != nil {
		t.Fatal(err)
	}

	got := ts.Run(context.Background(), "skill", `{"name":"deploy"}`)
	if !strings.Contains(got, "Run the pipeline") {
		t.Errorf("Run = %q, want the skill body", got)
	}
	if strings.Contains(got, "description:") {
		t.Errorf("front matter leaked: %q", got)
	}
}

func TestSkillToolExplainsAnUnknownName(t *testing.T) {
	catalog := catalogWith(t, "deploy", "---\ndescription: how to ship\n---\n\nRun the pipeline.\n")

	ts, err := New(t.TempDir(), WithSkills(catalog))
	if err != nil {
		t.Fatal(err)
	}

	got := ts.Run(context.Background(), "skill", `{"name":"nope"}`)
	if !strings.HasPrefix(got, "error:") {
		t.Fatalf("Run = %q, want an error", got)
	}
	if !strings.Contains(got, "deploy") {
		t.Errorf("Run = %q, want it to list what is available", got)
	}
}

func TestSkillToolNeedsNoApproval(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if ts.NeedsApproval("skill") {
		t.Error("the skill tool should not ask for approval")
	}
}

func TestSkillSummary(t *testing.T) {
	if got := Summary("skill", `{"name":"deploy"}`); got != "skill: deploy" {
		t.Errorf("Summary = %q", got)
	}
}
