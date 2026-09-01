package skills

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/config"
)

func isolateHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func writeSkill(t *testing.T, root, name, body string) {
	t.Helper()

	dir := filepath.Join(root, config.DirName, DirName, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const deploySkill = `---
name: deploy
description: How to ship this service. Use when asked to deploy or release.
---

Run the pipeline, then check the dashboard.
`

func TestLoadReadsASkill(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "deploy", deploySkill)

	catalog := Load(root)
	if len(catalog.Problems) > 0 {
		t.Fatalf("problems: %v", catalog.Problems)
	}
	if catalog.Len() != 1 {
		t.Fatalf("loaded %d skills, want 1", catalog.Len())
	}

	s := catalog.Skills[0]
	if s.Name != "deploy" {
		t.Errorf("name = %q", s.Name)
	}
	if !strings.HasPrefix(s.Description, "How to ship") {
		t.Errorf("description = %q", s.Description)
	}
}

func TestBodyIsReadOnlyOnDemand(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "deploy", deploySkill)

	catalog := Load(root)
	if got := catalog.Wrap("BASE"); strings.Contains(got, "Run the pipeline") {
		t.Error("the body leaked into the system prompt")
	}

	body, err := catalog.Body("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Run the pipeline") {
		t.Errorf("body = %q", body)
	}
	if strings.Contains(body, "description:") {
		t.Errorf("front matter leaked into the body: %q", body)
	}
}

func TestNoSkillsIsNotAnError(t *testing.T) {
	isolateHome(t)

	catalog := Load(t.TempDir())
	if catalog.Len() != 0 || len(catalog.Problems) != 0 {
		t.Errorf("catalog = %+v, want empty", catalog)
	}
	if got := catalog.Wrap("BASE"); got != "BASE" {
		t.Errorf("an empty catalog changed the prompt: %q", got)
	}
}

func TestDirectoryNameIsTheFallback(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "writing-tests", "---\ndescription: How tests work here.\n---\n\nBody.\n")

	catalog := Load(root)
	if catalog.Len() != 1 || catalog.Skills[0].Name != "writing-tests" {
		t.Fatalf("catalog = %+v, want the directory name used", catalog)
	}
}

func TestFrontMatterNameWins(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "folder-name", "---\nname: real-name\ndescription: x\n---\n\nBody.\n")

	catalog := Load(root)
	if catalog.Len() != 1 || catalog.Skills[0].Name != "real-name" {
		t.Fatalf("catalog = %+v, want real-name", catalog)
	}
}

func TestBrokenSkillsAreReportedNotFatal(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()

	writeSkill(t, root, "good", deploySkill)
	writeSkill(t, root, "no-description", "---\nname: x\n---\n\nBody.\n")
	writeSkill(t, root, "no-front-matter", "Just a body, no markers.\n")
	writeSkill(t, root, "unclosed", "---\nname: y\ndescription: z\n\nBody.\n")

	catalog := Load(root)

	if catalog.Len() != 1 {
		t.Errorf("loaded %d skills, want only the good one", catalog.Len())
	}
	if len(catalog.Problems) != 3 {
		t.Errorf("problems = %v, want 3", catalog.Problems)
	}
	joined := strings.Join(catalog.Problems, "\n")
	if !strings.Contains(joined, "no description") {
		t.Errorf("problems should explain the missing description: %v", catalog.Problems)
	}
}

func TestFolderWithoutASkillFileIsIgnored(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, config.DirName, DirName, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	catalog := Load(root)
	if catalog.Len() != 0 || len(catalog.Problems) != 0 {
		t.Errorf("catalog = %+v, want it quietly ignored", catalog)
	}
}

func TestDescriptionIsCapped(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "wordy", "---\ndescription: "+strings.Repeat("x", maxDescription+1)+"\n---\n\nBody.\n")

	catalog := Load(root)
	if catalog.Len() != 0 {
		t.Error("an oversized description was accepted")
	}
	if len(catalog.Problems) != 1 || !strings.Contains(catalog.Problems[0], "every turn") {
		t.Errorf("problems = %v, want it to explain the cost", catalog.Problems)
	}
}

func TestDescriptionIsFlattened(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "multi", "---\ndescription: >\n  one two\n  three\n---\n\nBody.\n")

	catalog := Load(root)
	if catalog.Len() != 1 {
		t.Fatalf("problems: %v", catalog.Problems)
	}
	if got := catalog.Skills[0].Description; got != "one two three" {
		t.Errorf("description = %q, want it on one line", got)
	}
}

func TestProjectSkillBeatsHome(t *testing.T) {
	home := isolateHome(t)
	root := t.TempDir()

	writeSkill(t, home, "deploy", "---\ndescription: the home version\n---\n\nHome body.\n")
	writeSkill(t, root, "deploy", "---\ndescription: the project version\n---\n\nProject body.\n")

	catalog := Load(root)
	if catalog.Len() != 1 {
		t.Fatalf("loaded %d, want the two to merge into one", catalog.Len())
	}
	if got := catalog.Skills[0].Description; got != "the project version" {
		t.Errorf("description = %q, want the project to win", got)
	}

	body, err := catalog.Body("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Project body") {
		t.Errorf("body = %q, want the project's", body)
	}
}

func TestHomeSkillsAreAvailableToo(t *testing.T) {
	home := isolateHome(t)
	writeSkill(t, home, "personal", "---\ndescription: mine\n---\n\nBody.\n")

	catalog := Load(t.TempDir())
	if catalog.Len() != 1 || catalog.Skills[0].Name != "personal" {
		t.Errorf("catalog = %+v, want the home skill", catalog)
	}
}

func TestSkillsAreSorted(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()

	for _, name := range []string{"zebra", "alpha", "middle"} {
		writeSkill(t, root, name, "---\ndescription: d\n---\n\nBody.\n")
	}

	if got := Load(root).Names(); !slices.Equal(got, []string{"alpha", "middle", "zebra"}) {
		t.Errorf("Names = %v, want them sorted", got)
	}
}

func TestBodyRejectsAnUnknownName(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "deploy", deploySkill)

	catalog := Load(root)

	_, err := catalog.Body("nope")
	if err == nil {
		t.Fatal("an unknown skill returned no error")
	}
	if !strings.Contains(err.Error(), "deploy") {
		t.Errorf("error = %q, want it to list what is available", err)
	}
}

func TestBodyWithNoSkillsAtAll(t *testing.T) {
	isolateHome(t)

	_, err := Load(t.TempDir()).Body("anything")
	if err == nil || !strings.Contains(err.Error(), "no skills") {
		t.Errorf("error = %v, want it to say there are none", err)
	}
}

func TestBodyRejectsAnEmptyBody(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "hollow", "---\ndescription: d\n---\n")

	catalog := Load(root)
	if catalog.Len() != 1 {
		t.Fatalf("problems: %v", catalog.Problems)
	}

	if _, err := catalog.Body("hollow"); err == nil {
		t.Error("a skill with no instructions returned no error")
	}
}

func TestBodyIsNotCached(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "deploy", deploySkill)

	catalog := Load(root)
	if _, err := catalog.Body("deploy"); err != nil {
		t.Fatal(err)
	}

	writeSkill(t, root, "deploy", "---\ndescription: d\n---\n\nRewritten.\n")

	body, err := catalog.Body("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Rewritten") {
		t.Errorf("body = %q, want the new text", body)
	}
}

func TestWrapListsNamesAndDescriptions(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "deploy", deploySkill)
	writeSkill(t, root, "testing", "---\ndescription: How tests work.\n---\n\nBody.\n")

	got := Load(root).Wrap("BASE PROMPT")

	if !strings.HasPrefix(got, "BASE PROMPT") {
		t.Error("Wrap should build on the prompt it was given")
	}
	for _, want := range []string{"deploy", "testing", "How tests work.", "skill tool"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q:\n%s", want, got)
		}
	}
}

func TestTruncateIsRuneSafe(t *testing.T) {
	long := strings.Repeat("é", maxBody) 

	got := truncate(long, "x/SKILL.md")
	if !strings.Contains(got, "truncated") {
		t.Fatal("an oversized body was not truncated")
	}
	if strings.Contains(got, "\uFFFD") {
		t.Error("truncation split a character in half")
	}
}

func TestSplitHandlesCarriageReturns(t *testing.T) {
	meta, body, err := split("---\r\nname: x\r\ndescription: d\r\n---\r\n\r\nBody.\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "x" || meta.Description != "d" {
		t.Errorf("meta = %+v", meta)
	}
	if !strings.Contains(body, "Body.") {
		t.Errorf("body = %q", body)
	}
}
