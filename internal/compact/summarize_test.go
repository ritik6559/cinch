package compact

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
)

type fakeProvider struct {
	summary  string
	err      error
	requests []llm.Request
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(ctx context.Context, req llm.Request, onText func(string)) (*llm.Response, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{llm.Text{Text: f.summary}}},
		Usage:   llm.Usage{InputTokens: 900, OutputTokens: 120},
	}, nil
}

// turns builds n user prompts, each followed by a tool call and its result.
func turns(n int) []llm.Message {
	var messages []llm.Message
	for i := range n {
		id := "call_" + string(rune('a'+i))
		messages = append(messages,
			llm.UserText("prompt "+string(rune('a'+i))),
			llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{
				llm.ToolUse{ID: id, Name: "bash", Input: json.RawMessage(`{}`)},
			}},
			llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{
				llm.ToolResult{ToolUseID: id, Content: "output"},
			}},
			llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{llm.Text{Text: "answer"}}},
		)
	}
	return messages
}

// Splitting between a tool call and its result produces a transcript the
// provider rejects outright.
func TestSafeSplitPointNeverSeparatesToolPairs(t *testing.T) {
	messages := turns(10)

	for desired := range len(messages) {
		split := SafeSplitPoint(messages, desired)
		if split == 0 || split == len(messages) {
			continue
		}

		// Every tool result kept must have its call kept too.
		kept := messages[split:]
		calls := map[string]bool{}
		for _, m := range kept {
			for _, b := range m.Blocks {
				if u, ok := b.(llm.ToolUse); ok {
					calls[u.ID] = true
				}
			}
		}
		for _, m := range kept {
			for _, b := range m.Blocks {
				if r, ok := b.(llm.ToolResult); ok && !calls[r.ToolUseID] {
					t.Fatalf("desired %d split at %d: result %q kept without its call",
						desired, split, r.ToolUseID)
				}
			}
		}
	}
}

func TestSafeSplitPointEdges(t *testing.T) {
	messages := turns(3)

	if got := SafeSplitPoint(messages, len(messages)+5); got != len(messages) {
		t.Errorf("beyond the end: got %d, want %d", got, len(messages))
	}
	if got := SafeSplitPoint(messages, -1); got != 0 {
		t.Errorf("negative: got %d, want 0", got)
	}
	if got := SafeSplitPoint(nil, 5); got != 0 {
		t.Errorf("empty: got %d, want 0", got)
	}
}

func TestSummarizeReplacesOldKeepsRecent(t *testing.T) {
	messages := turns(10) // 40 messages
	p := &fakeProvider{summary: "the user asked about X and we changed Y"}

	out, result, err := Summarize(context.Background(), p, messages, SummarizeOptions{KeepRecent: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summarized == 0 {
		t.Fatal("nothing was summarized")
	}

	if len(out) >= len(messages) {
		t.Errorf("got %d messages, want fewer than %d", len(out), len(messages))
	}

	first := out[0].TextContent()
	if !strings.Contains(first, "compacted") || !strings.Contains(first, "asked about X") {
		t.Errorf("first message should be the summary, got %q", first)
	}

	// The tail must be untouched.
	tail := messages[len(messages)-(len(out)-1):]
	for i, m := range tail {
		if m.TextContent() != out[i+1].TextContent() {
			t.Errorf("recent message %d was changed", i)
		}
	}
}

func TestSummarizeUsesTheTranscriptNotTheRawMessages(t *testing.T) {
	messages := turns(10)
	p := &fakeProvider{summary: "ok"}

	if _, _, err := Summarize(context.Background(), p, messages, SummarizeOptions{KeepRecent: 8}); err != nil {
		t.Fatal(err)
	}

	req := p.requests[0]
	if len(req.Messages) != 1 {
		t.Fatalf("sent %d messages, want 1 rendered transcript", len(req.Messages))
	}
	// No tools are defined on this call, so no tool blocks may be sent.
	for _, b := range req.Messages[0].Blocks {
		if _, ok := b.(llm.Text); !ok {
			t.Errorf("the summarize request carried a %s block", b.BlockType())
		}
	}
	if len(req.Tools) != 0 {
		t.Errorf("the summarize request should define no tools, got %d", len(req.Tools))
	}
	if req.System == "" {
		t.Error("the summarize request has no system prompt")
	}
}

func TestSummarizeReportsUsage(t *testing.T) {
	p := &fakeProvider{summary: "ok"}

	_, result, err := Summarize(context.Background(), p, turns(10), SummarizeOptions{KeepRecent: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage.InputTokens != 900 {
		t.Errorf("usage not reported: %+v", result.Usage)
	}
}

// An empty summary would silently erase the conversation.
func TestSummarizeRejectsEmptySummary(t *testing.T) {
	p := &fakeProvider{summary: "   "}

	out, _, err := Summarize(context.Background(), p, turns(10), SummarizeOptions{KeepRecent: 8})
	if err == nil {
		t.Fatal("expected an error for an empty summary")
	}
	if len(out) != len(turns(10)) {
		t.Error("the conversation should be returned unchanged when summarizing fails")
	}
}

func TestSummarizeReturnsOriginalOnProviderError(t *testing.T) {
	messages := turns(10)
	p := &fakeProvider{err: errors.New("rate limited")}

	out, result, err := Summarize(context.Background(), p, messages, SummarizeOptions{KeepRecent: 8})
	if err == nil {
		t.Fatal("expected the provider error")
	}
	if result.Summarized != 0 || len(out) != len(messages) {
		t.Error("the conversation must be unchanged when the provider fails")
	}
}

// Too short to be worth splitting.
func TestSummarizeSkipsShortConversations(t *testing.T) {
	messages := turns(2)
	p := &fakeProvider{summary: "ok"}

	out, result, err := Summarize(context.Background(), p, messages, SummarizeOptions{KeepRecent: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summarized != 0 {
		t.Errorf("summarized %d, want 0", result.Summarized)
	}
	if len(out) != len(messages) {
		t.Error("the conversation should be unchanged")
	}
	if len(p.requests) != 0 {
		t.Error("no provider call should be made when there is nothing to summarize")
	}
}

func TestRenderSkipsThinkingAndClipsResults(t *testing.T) {
	long := strings.Repeat("x", maxRenderedResult+500)

	got := render([]llm.Message{
		{Role: llm.RoleAssistant, Blocks: []llm.Block{
			llm.Thinking{ID: "rs_1", Opaque: json.RawMessage(`{"encrypted_content":"SECRET"}`)},
			llm.Text{Text: "hello"},
		}},
		{Role: llm.RoleUser, Blocks: []llm.Block{
			llm.ToolResult{ToolUseID: "c", Content: long},
		}},
	})

	if strings.Contains(got, "SECRET") {
		t.Error("encrypted reasoning must not be rendered")
	}
	if !strings.Contains(got, "hello") {
		t.Error("text was not rendered")
	}
	if len(got) > maxRenderedResult+500 {
		t.Errorf("tool result was not clipped: %d bytes", len(got))
	}
}
