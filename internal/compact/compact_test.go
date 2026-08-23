package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ritik6559/cinch/internal/llm"
)

func conversation(toolResults, lines int) []llm.Message {
	body := strings.Repeat("output line\n", lines)

	messages := []llm.Message{llm.UserText("start")}
	for i := range toolResults {
		id := fmt.Sprintf("call_%d", i)
		messages = append(messages,
			llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{
				llm.ToolUse{ID: id, Name: "bash", Input: json.RawMessage(`{}`)},
			}},
			llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{
				llm.ToolResult{ToolUseID: id, Content: body},
			}},
		)
	}
	return messages
}

func toolResults(messages []llm.Message) []llm.ToolResult {
	var out []llm.ToolResult
	for _, m := range messages {
		for _, b := range m.Blocks {
			if r, ok := b.(llm.ToolResult); ok {
				out = append(out, r)
			}
		}
	}
	return out
}

func TestClearsOldKeepsRecent(t *testing.T) {
	out, result := ToolResults(conversation(10, 50), Options{KeepRecent: 3, MinBytes: 100})
	if result.Cleared != 7 {
		t.Fatalf("cleared %d, want 7", result.Cleared)
	}

	for i, r := range toolResults(out) {
		cleared := isPlaceholder(r.Content)
		if i < 7 && !cleared {
			t.Errorf("result %d should be cleared", i)
		}
		if i >= 7 && cleared {
			t.Errorf("result %d is recent and should be untouched", i)
		}
	}
}

func TestPairingSurvives(t *testing.T) {
	messages := conversation(8, 50)
	before := toolResults(messages)

	out, _ := ToolResults(messages, Options{KeepRecent: 2, MinBytes: 100})
	after := toolResults(out)

	if len(after) != len(before) {
		t.Fatalf("got %d results, want %d: a block was dropped", len(after), len(before))
	}
	for i := range before {
		if after[i].ToolUseID != before[i].ToolUseID {
			t.Errorf("result %d id changed from %q to %q", i, before[i].ToolUseID, after[i].ToolUseID)
		}
	}
}

func TestIdempotent(t *testing.T) {
	once, first := ToolResults(conversation(10, 50), Options{KeepRecent: 2, MinBytes: 100})
	if first.Cleared == 0 {
		t.Fatal("first pass cleared nothing")
	}

	if _, second := ToolResults(once, Options{KeepRecent: 2, MinBytes: 100}); second.Cleared != 0 {
		t.Errorf("second pass cleared %d, want 0", second.Cleared)
	}
}

func TestInputIsNotModified(t *testing.T) {
	messages := conversation(10, 50)
	original := toolResults(messages)[0].Content

	ToolResults(messages, Options{KeepRecent: 2, MinBytes: 100})

	if toolResults(messages)[0].Content != original {
		t.Error("the input conversation was modified in place")
	}
}

func TestSkipsWhenSavingIsTooSmall(t *testing.T) {
	_, result := ToolResults(conversation(10, 1), Options{KeepRecent: 2, MinBytes: 100_000})
	if result.Cleared != 0 {
		t.Errorf("cleared %d, want 0 when the saving is below MinBytes", result.Cleared)
	}
}

func TestNothingToDoWithFewResults(t *testing.T) {
	_, result := ToolResults(conversation(3, 50), Options{KeepRecent: 6, MinBytes: 100})
	if result.Cleared != 0 {
		t.Errorf("cleared %d, want 0", result.Cleared)
	}
}

func TestPlaceholderTellsTheModelWhatHappened(t *testing.T) {
	out, _ := ToolResults(conversation(5, 40), Options{KeepRecent: 1, MinBytes: 100})
	got := toolResults(out)[0].Content

	if !strings.Contains(got, "41 lines") {
		t.Errorf("placeholder should say how much was cleared: %q", got)
	}
	if !strings.Contains(got, "again") {
		t.Errorf("placeholder should say the tool can be re-run: %q", got)
	}
}
