package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/sandbox"
)

func bashCall(id, command string) llm.Response {
	input, _ := json.Marshal(map[string]string{"command": command})
	return llm.Response{Message: llm.Message{
		Role:   llm.RoleAssistant,
		Blocks: []llm.Block{llm.ToolUse{ID: id, Name: "bash", Input: input}},
	}}
}

// recorder answers every request and remembers what it was asked.
type recorder struct {
	seen   []ApprovalRequest
	answer bool
}

func (r *recorder) approve(req ApprovalRequest) bool {
	r.seen = append(r.seen, req)
	return r.answer
}

// runOne drives a single bash call to completion and returns the result the
// model was given for it.
func runOne(t *testing.T, mode sandbox.Mode, command string, rec *recorder) string {
	t.Helper()

	provider := &fakeProvider{responses: []llm.Response{
		bashCall("call_1", command),
		assistantText("done"),
	}}

	a, _ := newTestAgent(t, provider, rec.approve)
	a.SetSandbox(mode)

	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, msg := range a.Messages() {
		for _, block := range msg.Blocks {
			if r, ok := block.(llm.ToolResult); ok && r.ToolUseID == "call_1" {
				return r.Content
			}
		}
	}
	t.Fatal("no tool result for call_1")
	return ""
}

func TestDeniedCommandNeverReachesTheShell(t *testing.T) {
	rec := &recorder{answer: true}
	result := runOne(t, sandbox.ModePolicy, "rm -rf /", rec)

	if len(rec.seen) != 0 {
		t.Errorf("the user was asked about a refused command: %+v", rec.seen)
	}
	if !strings.HasPrefix(result, "refused:") {
		t.Errorf("result = %q, want a refusal", result)
	}
	if !strings.Contains(result, "delete") {
		t.Errorf("result = %q, want it to say what was wrong", result)
	}
	// The model must not be invited to try again.
	if !strings.Contains(result, "will not change that") {
		t.Errorf("result = %q, want it to close the door on a retry", result)
	}
}

func TestReadOnlyCommandRunsWithoutAsking(t *testing.T) {
	rec := &recorder{answer: false} // would refuse if asked
	result := runOne(t, sandbox.ModePolicy, "echo hello", rec)

	if len(rec.seen) != 0 {
		t.Fatalf("the user was asked about a read-only command: %+v", rec.seen)
	}
	if strings.Contains(result, "denied") {
		t.Errorf("result = %q, want the command to have run", result)
	}
}

func TestAskingCarriesTheReason(t *testing.T) {
	rec := &recorder{answer: true}
	runOne(t, sandbox.ModePolicy, "curl https://example.com", rec)

	if len(rec.seen) != 1 {
		t.Fatalf("asked %d times, want 1", len(rec.seen))
	}
	if got := rec.seen[0].Reason; !strings.Contains(got, "network") {
		t.Errorf("reason = %q, want it to mention the network", got)
	}
}

func TestRefusalWhenTheUserSaysNo(t *testing.T) {
	rec := &recorder{answer: false}
	result := runOne(t, sandbox.ModePolicy, "curl https://example.com", rec)

	if len(rec.seen) != 1 {
		t.Fatalf("asked %d times, want 1", len(rec.seen))
	}
	if !strings.HasPrefix(result, "denied:") {
		t.Errorf("result = %q, want a denial", result)
	}
}

func TestStrictModeAsksAboutEverything(t *testing.T) {
	rec := &recorder{answer: true}
	runOne(t, sandbox.ModeStrict, "echo hello", rec)

	if len(rec.seen) != 1 {
		t.Errorf("strict mode asked %d times about `echo hello`, want 1", len(rec.seen))
	}
}

func TestStrictModeStillRefuses(t *testing.T) {
	rec := &recorder{answer: true}
	result := runOne(t, sandbox.ModeStrict, "rm -rf /", rec)

	if len(rec.seen) != 0 {
		t.Error("strict mode should still refuse outright, not ask")
	}
	if !strings.HasPrefix(result, "refused:") {
		t.Errorf("result = %q, want a refusal", result)
	}
}

func TestOffModeRestoresTheOldBehaviour(t *testing.T) {
	rec := &recorder{answer: true}

	// Everything asks...
	runOne(t, sandbox.ModeOff, "echo hello", rec)
	if len(rec.seen) != 1 {
		t.Errorf("off mode asked %d times about `echo hello`, want 1", len(rec.seen))
	}

	// ...and nothing is refused without a person saying so.
	rec = &recorder{answer: true}
	result := runOne(t, sandbox.ModeOff, "rm -rf /", rec)
	if len(rec.seen) != 1 {
		t.Errorf("off mode asked %d times about `rm -rf /`, want 1", len(rec.seen))
	}
	if strings.HasPrefix(result, "refused:") {
		t.Error("off mode should not refuse anything on its own")
	}
}

// An unset mode must be the default, not the weakest setting.
func TestZeroModeIsThePolicy(t *testing.T) {
	rec := &recorder{answer: true}
	result := runOne(t, "", "rm -rf /", rec)

	if !strings.HasPrefix(result, "refused:") {
		t.Errorf("result = %q, want the default policy to apply", result)
	}
}

func TestSetSandboxIgnoresAnEmptyMode(t *testing.T) {
	a, _ := newTestAgent(t, &fakeProvider{}, nil)

	a.SetSandbox(sandbox.ModeStrict)
	a.SetSandbox("")

	if got := a.Sandbox(); got != sandbox.ModeStrict {
		t.Errorf("Sandbox() = %q, want the mode to survive an empty set", got)
	}
}

// The file tools keep their own confinement and must not be routed through the
// shell policy, which knows nothing about them.
func TestFileToolsStillAsk(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"path": "x.txt", "content": "hi"})
	provider := &fakeProvider{responses: []llm.Response{
		{Message: llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.ToolUse{ID: "call_1", Name: "write_file", Input: input},
		}}},
		assistantText("done"),
	}}

	rec := &recorder{answer: true}
	a, _ := newTestAgent(t, provider, rec.approve)

	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rec.seen) != 1 || rec.seen[0].Tool != "write_file" {
		t.Errorf("write_file was not put to the user: %+v", rec.seen)
	}
	if rec.seen[0].Reason != "" {
		t.Errorf("reason = %q, want none for a tool that always asks", rec.seen[0].Reason)
	}
}

func TestReadOnlyToolsNeverAsk(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"dir": "."})
	provider := &fakeProvider{responses: []llm.Response{
		{Message: llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.ToolUse{ID: "call_1", Name: "list_files", Input: input},
		}}},
		assistantText("done"),
	}}

	rec := &recorder{answer: false}
	a, _ := newTestAgent(t, provider, rec.approve)

	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rec.seen) != 0 {
		t.Errorf("list_files should never ask: %+v", rec.seen)
	}
}
