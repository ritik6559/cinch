package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/tools"
)

type fakeProvider struct {
	responses []llm.Response
	calls     int
	requests  []llm.Request
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(ctx context.Context, req llm.Request, onText func(string)) (*llm.Response, error) {
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		return nil, errors.New("fake provider ran out of responses")
	}
	resp := f.responses[f.calls]
	f.calls++

	if onText != nil {
		if text := resp.Message.TextContent(); text != "" {
			onText(text)
		}
	}
	return &resp, nil
}

func newTestAgent(t *testing.T, p llm.Provider, approve Approver) (*Agent, *bytes.Buffer) {
	t.Helper()
	ts, err := tools.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	hooks := Hooks{
		OnText:     func(text string) { out.WriteString(text) },
		OnToolCall: func(name, summary string) { fmt.Fprintf(out, "\n -> %s\n", summary) },
	}
	return New(p, ts, approve, hooks), out
}

func assistantText(s string) llm.Response {
	return llm.Response{Message: llm.Message{
		Role:   llm.RoleAssistant,
		Blocks: []llm.Block{llm.Text{Text: s}},
	}}
}

func toolCall(id, name, args string) llm.Response {
	return llm.Response{Message: llm.Message{
		Role: llm.RoleAssistant,
		Blocks: []llm.Block{
			llm.ToolUse{ID: id, Name: name, Input: json.RawMessage(args)},
		},
	}}
}

func TestStopsWhenNoToolsRequested(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Response{assistantText("done")}}
	a, out := newTestAgent(t, provider, nil)

	if err := a.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Errorf("made %d calls, want 1", provider.calls)
	}
	if !strings.Contains(out.String(), "done") {
		t.Errorf("text was not printed: %q", out)
	}
}

// A denied call must still be answered, with the same id. Leaving it
// unanswered makes the provider reject the whole next request.
func TestDeniedToolStillProducesAResult(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Response{
		toolCall("call_1", "write_file", `{"path":"x.txt","content":"hi"}`),
		assistantText("understood"),
	}}

	denyAll := func(ApprovalRequest) bool { return false }
	a, _ := newTestAgent(t, provider, denyAll)

	if err := a.Run(context.Background(), "write a file"); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 {
		t.Fatalf("made %d calls, want 2", provider.calls)
	}

	// The second request must carry a result for call_1.
	second := provider.requests[1]
	last := second.Messages[len(second.Messages)-1]

	var found bool
	for _, b := range last.Blocks {
		if r, ok := b.(llm.ToolResult); ok && r.ToolUseID == "call_1" {
			found = true
			if !r.IsError || !strings.Contains(r.Content, "denied") {
				t.Errorf("result does not report the denial: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("no result for call_1: the provider would reject this request")
	}
}

// An approved call runs the tool and returns its output.
func TestApprovedToolRuns(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Response{
		toolCall("call_1", "write_file", `{"path":"note.txt","content":"hello"}`),
		assistantText("written"),
	}}

	allowAll := func(ApprovalRequest) bool { return true }
	a, _ := newTestAgent(t, provider, allowAll)

	if err := a.Run(context.Background(), "write a file"); err != nil {
		t.Fatal(err)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	for _, b := range last.Blocks {
		if r, ok := b.(llm.ToolResult); ok {
			if r.IsError || !strings.Contains(r.Content, "wrote") {
				t.Errorf("unexpected tool result: %+v", r)
			}
			return
		}
	}
	t.Fatal("no tool result found")
}

// A model that never stops asking for tools must be stopped by the step limit.
func TestMaxStepsFires(t *testing.T) {
	responses := make([]llm.Response, maxSteps+5)
	for i := range responses {
		responses[i] = toolCall("call_1", "list_files", `{}`)
	}
	provider := &fakeProvider{responses: responses}
	a, _ := newTestAgent(t, provider, nil)

	err := a.Run(context.Background(), "loop forever")
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("got %v, want ErrMaxSteps", err)
	}
	if provider.calls > maxSteps {
		t.Errorf("made %d calls, want at most %d", provider.calls, maxSteps)
	}
}

// Reasoning must be sent back on the next request or the model loses its own
// chain of thought between tool calls.
func TestReasoningIsSentBack(t *testing.T) {
	const opaque = `{"type":"reasoning","id":"rs_1","encrypted_content":"SECRET"}`

	provider := &fakeProvider{responses: []llm.Response{
		{Message: llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.Thinking{ID: "rs_1", Opaque: json.RawMessage(opaque)},
			llm.ToolUse{ID: "call_1", Name: "list_files", Input: json.RawMessage(`{}`)},
		}}},
		assistantText("done"),
	}}
	a, _ := newTestAgent(t, provider, nil)

	if err := a.Run(context.Background(), "look around"); err != nil {
		t.Fatal(err)
	}

	var seen bool
	for _, m := range provider.requests[1].Messages {
		for _, b := range m.Blocks {
			if th, ok := b.(llm.Thinking); ok && string(th.Opaque) == opaque {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("the reasoning block was not carried into the second request")
	}
}

// One Run can make several API calls. Both counters must add them all up.
func TestUsageAccumulates(t *testing.T) {
	first := toolCall("c1", "list_files", `{}`)
	first.Usage = llm.Usage{InputTokens: 100, OutputTokens: 10}

	second := assistantText("ok")
	second.Usage = llm.Usage{InputTokens: 150, OutputTokens: 5, ReasoningTokens: 12}

	provider := &fakeProvider{responses: []llm.Response{first, second}}
	a, _ := newTestAgent(t, provider, nil)

	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := a.TurnUsage().InputTokens; got != 250 {
		t.Errorf("turn usage = %d, want 250 across both calls", got)
	}
	if got := a.Usage().ReasoningTokens; got != 12 {
		t.Errorf("session reasoning tokens = %d, want 12", got)
	}
}

// The system prompt and the tool list must be on every request, not only the
// first: the API carries neither from one call to the next.
func TestSystemPromptAndToolsSentEveryTime(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Response{
		toolCall("c1", "list_files", `{}`),
		assistantText("ok"),
	}}
	a, _ := newTestAgent(t, provider, nil)

	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}

	for i, req := range provider.requests {
		if req.System == "" {
			t.Errorf("request %d has no system prompt", i)
		}
		if len(req.Tools) == 0 {
			t.Errorf("request %d has no tools", i)
		}
	}
}

// A turn that stops before its tools run leaves a tool call with no result.
// The next request would carry that unanswered call and be rejected outright,
// so one Ctrl-C would break the rest of the session.
func TestAbandonedToolCallsAreAnswered(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Response{
		toolCall("call_1", "list_files", `{}`),
	}}
	a, _ := newTestAgent(t, provider, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Run(ctx, "look around")

	provider.responses = append(provider.responses, assistantText("ok"))
	if err := a.Run(context.Background(), "never mind, what is 2+2"); err != nil {
		t.Fatalf("the next turn failed: %v", err)
	}

	var answered bool
	for _, m := range provider.requests[len(provider.requests)-1].Messages {
		for _, b := range m.Blocks {
			if r, ok := b.(llm.ToolResult); ok && r.ToolUseID == "call_1" {
				answered = true
			}
		}
	}
	if !answered {
		t.Fatal("call_1 was never answered: the provider would reject this request")
	}
}
