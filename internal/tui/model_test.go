package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/session"
)

func testModel(t *testing.T) *Model {
	t.Helper()

	root := t.TempDir()
	m := &Model{
		cfg:              config.Config{Provider: "openai", Model: "gpt-5.6"},
		sess:             session.New(root, "openai", "gpt-5.6"),
		saved:            &approval.Store{},
		root:             root,
		ctx:              context.Background(),
		events:           make(chan tea.Msg, 64),
		theme:            darkTheme(),
		dark:             true,
		sessionApprovals: map[string]bool{},
	}
	m.viewport = viewport.New()
	m.input = newInput()
	m.agent = agent.New(nil, nil, nil, agent.Hooks{})
	m.agent.SetModel("gpt-5.6")
	m.resize(100, 30)

	return m
}

func press(t *testing.T, m *Model, keys ...string) {
	t.Helper()

	for _, k := range keys {
		var msg tea.KeyPressMsg
		if r := []rune(k); len(r) == 1 {
			msg = tea.KeyPressMsg{Code: r[0], Text: k}
		} else {
			msg = tea.KeyPressMsg{Code: keyCode(t, k)}
		}
		m.Update(msg)
	}
}

func keyCode(t *testing.T, name string) rune {
	t.Helper()

	switch name {
	case "up":
		return tea.KeyUp
	case "down":
		return tea.KeyDown
	case "tab":
		return tea.KeyTab
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEscape
	}
	t.Fatalf("unknown key %q", name)
	return 0
}

func TestViewDoesNotPanic(t *testing.T) {
	m := testModel(t)

	m.entries = []entry{
		{kind: entryUser, text: "where is the step limit?"},
		{kind: entryAssistant, text: "It is in `agent.go`.\n\n- one\n- two"},
		{kind: entryTool, tool: "grep", summary: "maxSteps", result: "a\nb", done: true},
		{kind: entryTool, tool: "bash", summary: "run: go test", result: "boom", done: true, failed: true},
		{kind: entryTool, tool: "read_file", summary: "main.go"}, // still running
		{kind: entryNotice, text: "compacted"},
		{kind: entryError, text: "something broke"},
	}
	m.refresh()

	for _, md := range []mode{modeIdle, modeWorking, modePicking} {
		m.mode = md
		if md == modePicking {
			m.picker = newPicker("Model", []string{"a", "b"}, "a")
		}
		if got := m.View().Content; got == "" {
			t.Errorf("View() was empty in mode %d", md)
		}
	}
}

func TestPopupArrowsMoveTheHighlight(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "c") // compact, cost, clear

	matches, cursor := m.suggestions()
	if len(matches) != 3 || cursor != 0 {
		t.Fatalf("after typing /c: %d matches, cursor %d; want 3 and 0", len(matches), cursor)
	}

	press(t, m, "down", "down")
	if _, cursor = m.suggestions(); cursor != 2 {
		t.Errorf("cursor after two downs = %d, want 2", cursor)
	}

	press(t, m, "down") // already at the end
	if _, cursor = m.suggestions(); cursor != 2 {
		t.Errorf("cursor should stop at the last row, got %d", cursor)
	}

	press(t, m, "up")
	if _, cursor = m.suggestions(); cursor != 1 {
		t.Errorf("cursor after up = %d, want 1", cursor)
	}
}

func TestTypingResetsTheHighlight(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "c", "down", "down")
	if _, cursor := m.suggestions(); cursor != 2 {
		t.Fatalf("setup: cursor = %d, want 2", cursor)
	}

	// "/co" narrows to compact and cost, so row 2 no longer exists.
	press(t, m, "o")
	matches, cursor := m.suggestions()
	if len(matches) != 2 || cursor != 0 {
		t.Errorf("after typing: %d matches, cursor %d; want 2 and 0", len(matches), cursor)
	}
}

func TestTabCompletesTheHighlightedCommand(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "c", "down", "tab") // second row is /cost
	if got := m.input.Value(); got != "/cost" {
		t.Errorf("input = %q, want /cost", got)
	}
}

func TestTabAddsASpaceForCommandsThatTakeArguments(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "m", "tab")
	if got := m.input.Value(); got != "/model " {
		t.Errorf("input = %q, want %q", got, "/model ")
	}
	if matches, _ := m.suggestions(); len(matches) != 0 {
		t.Error("the popup should close once the command is complete")
	}
}

// Enter on an ambiguous prefix must complete, not run: otherwise /c could fire
// /clear and throw the conversation away.
func TestEnterOnAPrefixCompletesInstead(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "c", "down", "down", "enter") // third row is /clear
	if got := m.input.Value(); got != "/clear" {
		t.Fatalf("input = %q, want /clear — Enter should have completed", got)
	}
	if len(m.entries) != 0 {
		t.Errorf("nothing should have run yet, got %d entries", len(m.entries))
	}

	press(t, m, "enter") // now it runs
	if len(m.entries) == 0 {
		t.Fatal("the second Enter should have run /clear")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "new conversation") {
		t.Errorf("last entry = %q, want the /clear notice", last.text)
	}
}

func TestEscClosesThePopup(t *testing.T) {
	m := testModel(t)

	press(t, m, "/", "m", "esc")
	if got := m.input.Value(); got != "" {
		t.Errorf("input = %q, want it cleared", got)
	}
}

func TestUnknownCommandReportsItself(t *testing.T) {
	m := testModel(t)

	m.input.SetValue("/zzz")
	press(t, m, "enter")

	if len(m.entries) != 1 || m.entries[0].kind != entryError {
		t.Fatalf("want one error entry, got %+v", m.entries)
	}
	if !strings.Contains(m.entries[0].text, "zzz") {
		t.Errorf("error = %q, want it to name the command", m.entries[0].text)
	}
}

func TestFinishToolMatchesTheOpenCall(t *testing.T) {
	m := testModel(t)

	m.entries = []entry{
		{kind: entryTool, tool: "grep", summary: "a", done: true, result: "old"},
		{kind: entryTool, tool: "grep", summary: "b"},
	}

	m.finishTool(toolResultMsg{name: "grep", result: "x\ny"})

	if m.entries[0].result != "old" {
		t.Error("a finished call was overwritten")
	}
	if !m.entries[1].done || m.entries[1].result != "x\ny" {
		t.Errorf("the open call was not closed: %+v", m.entries[1])
	}
}

// A result with no matching call still has to appear, or output vanishes.
func TestFinishToolWithoutACallStillShows(t *testing.T) {
	m := testModel(t)

	m.finishTool(toolResultMsg{name: "bash", result: "boom", failed: true})

	if len(m.entries) != 1 || !m.entries[0].failed {
		t.Fatalf("want one failed tool entry, got %+v", m.entries)
	}
}

func TestAppendTextOpensOneEntryPerRun(t *testing.T) {
	m := testModel(t)

	m.appendText("Hel")
	m.appendText("lo")

	if len(m.entries) != 1 || m.entries[0].text != "Hello" {
		t.Fatalf("want one entry saying Hello, got %+v", m.entries)
	}

	// A tool call interrupts the prose, so the next text starts a new entry.
	m.streaming = false
	m.appendText("more")

	if len(m.entries) != 2 {
		t.Errorf("want a second entry after the break, got %d", len(m.entries))
	}
}
