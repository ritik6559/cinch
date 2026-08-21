package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ritik6559/cinch/internal/llm"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestSaveAndLoad(t *testing.T) {
	isolate(t)

	s := New("/work/cinch", "openai", "gpt-5.6")
	s.SetTitle("fix the timeout bug")
	s.Usage = llm.Usage{InputTokens: 100, ReasoningTokens: 12}
	s.Messages = []llm.Message{
		llm.UserText("fix the timeout bug"),
		{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.Thinking{ID: "rs_1", Opaque: json.RawMessage(`{"encrypted_content":"SECRET"}`)},
			llm.ToolUse{ID: "call_1", Name: "grep", Input: json.RawMessage(`{"pattern":"timeout"}`)},
		}},
		{Role: llm.RoleUser, Blocks: []llm.Block{
			llm.ToolResult{ToolUseID: "call_1", Content: "bash.go:31"},
		}},
	}

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Title != s.Title || loaded.Model != s.Model || loaded.Workspace != s.Workspace {
		t.Errorf("header changed: %+v", loaded)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(loaded.Messages))
	}
	if loaded.Usage.ReasoningTokens != 12 {
		t.Errorf("usage lost: %+v", loaded.Usage)
	}

	thinking, ok := loaded.Messages[1].Blocks[0].(llm.Thinking)
	if !ok {
		t.Fatalf("got %T, want Thinking", loaded.Messages[1].Blocks[0])
	}
	if !strings.Contains(string(thinking.Opaque), "SECRET") {
		t.Errorf("reasoning did not survive: %s", thinking.Opaque)
	}
}

func TestListIsNewestFirst(t *testing.T) {
	isolate(t)

	for _, title := range []string{"oldest", "middle", "newest"} {
		s := New("/work", "openai", "m")
		s.SetTitle(title)
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d sessions, want 3", len(all))
	}
	if all[0].Title != "newest" {
		t.Errorf("first is %q, want newest", all[0].Title)
	}

	latest, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if latest.Title != "newest" {
		t.Errorf("Latest() is %q, want newest", latest.Title)
	}
}

func TestListWithNoSessions(t *testing.T) {
	isolate(t)

	all, err := List()
	if err != nil {
		t.Fatalf("an empty session directory should not be an error: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("got %d sessions, want 0", len(all))
	}
}

func TestListSkipsDamagedFiles(t *testing.T) {
	isolate(t)

	good := New("/work", "openai", "m")
	good.SetTitle("fine")
	if err := good.Save(); err != nil {
		t.Fatal(err)
	}

	dir, _ := Dir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Title != "fine" {
		t.Errorf("got %d sessions, want only the good one", len(all))
	}
}

func TestLoadDamagedFileGivesAClearError(t *testing.T) {
	isolate(t)

	dir, _ := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	id := NewID(time.Now())
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(id)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "damaged") {
		t.Errorf("error should say the file is damaged, got: %v", err)
	}
}

func TestLoadRefusesNewerFormat(t *testing.T) {
	isolate(t)

	s := New("/work", "openai", "m")
	s.Version = Version + 1
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(s.ID); err == nil {
		t.Fatal("expected an error for a newer format")
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	isolate(t)

	for _, id := range []string{
		"../../../../etc/passwd",
		"..\\..\\windows\\system32\\config\\sam",
		"/etc/passwd",
		"not-an-id",
		"",
	} {
		if _, err := Load(id); err == nil {
			t.Errorf("Load(%q) succeeded, want an error", id)
		}
	}
}

func TestTitleIsShortenedAndKept(t *testing.T) {
	s := New("/work", "openai", "m")

	s.SetTitle("first\nsecond line")
	if strings.Contains(s.Title, "\n") {
		t.Errorf("title should be one line, got %q", s.Title)
	}

	s.SetTitle("a different prompt")
	if s.Title == "a different prompt" {
		t.Error("the title should stay as the first prompt")
	}

	long := New("/work", "openai", "m")
	long.SetTitle(strings.Repeat("x", 200))
	if len([]rune(long.Title)) > maxTitle+1 {
		t.Errorf("title is %d runes, want it shortened", len([]rune(long.Title)))
	}
}

func TestTurnsCountsUserPromptsOnly(t *testing.T) {
	s := New("/work", "openai", "m")
	s.Messages = []llm.Message{
		llm.UserText("first"),
		{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.ToolUse{ID: "c1", Name: "grep", Input: json.RawMessage(`{}`)},
		}},
		{Role: llm.RoleUser, Blocks: []llm.Block{
			llm.ToolResult{ToolUseID: "c1", Content: "match"},
		}},
		{Role: llm.RoleAssistant, Blocks: []llm.Block{llm.Text{Text: "done"}}},
		llm.UserText("second"),
	}

	if got := s.Turns(); got != 2 {
		t.Errorf("got %d turns, want 2", got)
	}
}

func TestIDsSortByTime(t *testing.T) {
	early := NewID(time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC))
	later := NewID(time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC))

	if early >= later {
		t.Errorf("ids do not sort by time: %q then %q", early, later)
	}
}
