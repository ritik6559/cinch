package tools

import (
	"context"
	"strings"
	"testing"
)

func requireShell(t *testing.T) {
	t.Helper()
	if _, err := posixShell(); err != nil {
		t.Skip("no POSIX shell on PATH")
	}
}

func runBash(t *testing.T, ts *Tools, command string) string {
	t.Helper()
	return ts.Run(context.Background(), "bash", `{"command":`+quote(command)+`}`)
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestBashReturnsOutput(t *testing.T) {
	requireShell(t)
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if got := runBash(t, ts, "echo hello"); !strings.Contains(got, "hello") {
		t.Errorf("got %q, want it to contain hello", got)
	}
}

// A failing command is information the model must see, not a tool error.
func TestBashReportsExitStatus(t *testing.T) {
	requireShell(t)
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got := runBash(t, ts, "echo problem >&2; exit 3")
	if !strings.Contains(got, "exit status 3") {
		t.Errorf("got %q, want the exit status", got)
	}
	// Combined output: stderr must be included, or build errors are invisible.
	if !strings.Contains(got, "problem") {
		t.Errorf("got %q, want stderr included", got)
	}
}

// Blocking .env is pointless if a shell can read the same value from its own
// environment.
func TestBashHidesSecretsFromTheEnvironment(t *testing.T) {
	requireShell(t)
	t.Setenv("TOTALLY_FAKE_API_KEY", "supersecretvalue")

	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got := runBash(t, ts, "env")
	if strings.Contains(got, "supersecretvalue") {
		t.Error("the API key reached the command and would now be in the transcript")
	}
}

func TestBashRunsInTheWorkspace(t *testing.T) {
	requireShell(t)
	root := t.TempDir()
	ts, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	runBash(t, ts, "echo marker > made-here.txt")
	if out := runBash(t, ts, "ls"); !strings.Contains(out, "made-here.txt") {
		t.Errorf("command did not run in the workspace root: %q", out)
	}
}

func TestBashTimesOut(t *testing.T) {
	requireShell(t)
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got := ts.Run(context.Background(), "bash", `{"command":"sleep 5","timeout":1}`)
	if !strings.Contains(got, "timed out") {
		t.Errorf("got %q, want a timeout", got)
	}
}

func TestBashEmptyCommand(t *testing.T) {
	ts, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := ts.Run(context.Background(), "bash", `{"command":"  "}`); !strings.HasPrefix(got, "error:") {
		t.Errorf("got %q, want an error", got)
	}
}
