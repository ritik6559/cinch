package tui

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func names(cs []command) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestMatchCommands(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"not a command", "hello", nil},
		{"empty", "", nil},
		{"slash alone lists everything", "/", names(commands)},
		{"prefix", "/mo", []string{"model"}},
		{"shared prefix", "/c", []string{"compact", "cost", "clear"}},
		{"case insensitive", "/MO", []string{"model"}},
		{"exact name still matches itself", "/quit", []string{"quit"}},
		{"no such command", "/zzz", []string{}},
		{"a space means the user moved on to arguments", "/model gpt-5.6", nil},
		{"trailing space too", "/model ", nil},
		{"a slash mid-line is not a command", "what is a/b", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(matchCommands(tt.line))
			if !slices.Equal(got, tt.want) {
				t.Errorf("matchCommands(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
		wantArgs string
	}{
		{"/help", "help", ""},
		{"/model gpt-5.6", "model", "gpt-5.6"},
		{"/MODEL GPT-5.6", "model", "GPT-5.6"},
		{"  /effort   high  ", "effort", "high"},
		{"/resume 2026-08-27-a1b2", "resume", "2026-08-27-a1b2"},
		{"/model  a  b", "model", "a  b"},
		{"/", "", ""},
	}

	for _, tt := range tests {
		name, args := splitCommand(tt.line)
		if name != tt.wantName || args != tt.wantArgs {
			t.Errorf("splitCommand(%q) = (%q, %q), want (%q, %q)",
				tt.line, name, args, tt.wantName, tt.wantArgs)
		}
	}
}

// Every command the popup can offer must be reachable by runCommand, or Tab
// would complete a name that then reports "unknown command".
func TestEveryCommandIsDispatchable(t *testing.T) {
	source, err := os.ReadFile("slash.go")
	if err != nil {
		t.Fatalf("read slash.go: %v", err)
	}

	for _, c := range commands {
		if !strings.Contains(string(source), `case "`+c.Name+`":`) {
			t.Errorf("/%s is listed but runCommand has no case for it", c.Name)
		}
	}
}

func TestCompletionLeavesRoomForArguments(t *testing.T) {
	for _, c := range commands {
		got := completion(c)

		if !strings.HasPrefix(got, "/"+c.Name) {
			t.Errorf("completion(%q) = %q, want it to start with /%s", c.Name, got, c.Name)
		}
		if wantSpace := c.Args != ""; wantSpace != strings.HasSuffix(got, " ") {
			t.Errorf("completion(%q) = %q, args=%q — trailing space is wrong", c.Name, got, c.Args)
		}
	}
}

func TestLookupCommand(t *testing.T) {
	if _, ok := lookupCommand("model"); !ok {
		t.Error("lookupCommand(model) should find it")
	}
	if _, ok := lookupCommand("mo"); ok {
		t.Error("lookupCommand should match whole names, not prefixes")
	}
	if _, ok := lookupCommand(""); ok {
		t.Error("lookupCommand should not match an empty name")
	}
}
