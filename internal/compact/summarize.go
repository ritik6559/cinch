package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ritik6559/cinch/internal/llm"
)

const summarySystem = `You summarize coding agent transcripts. Produce a compact summary that preserves:
- the user's goal and any constraints they gave
- decisions made, and why
- files created or changed
- commands run and what they showed
- unresolved errors and open questions
- enough recent context to continue the work

Drop repeated logs, abandoned exploration, and large tool output once its
conclusion is captured. Write plain prose. Do not invent anything that is not
in the transcript.`

const summaryRequest = "\n\nSummarize the conversation above following your instructions. Reply with only the summary."

const summaryPreamble = "Earlier conversation was compacted to fit the context window. Summary of what happened so far:\n\n"

// maxRenderedResult caps each tool result when the transcript is rendered for
// summarizing. The transcript is being summarized because it is too large, so
// handing all of it back unbounded could exceed the window on the way in.
const maxRenderedResult = 2000

type SummarizeOptions struct {
	// KeepRecent is how many messages at the end stay.
	KeepRecent int
}

func DefaultSummarizeOptions() SummarizeOptions {
	return SummarizeOptions{KeepRecent: 20}
}

type SummaryResult struct {
	Summarized int // messages replaced by the summary
	Usage      llm.Usage
}

// Summarize replaces the older part of a conversation with a single summary
func Summarize(ctx context.Context, p llm.Provider, messages []llm.Message, opts SummarizeOptions) ([]llm.Message, SummaryResult, error) {
	split := SafeSplitPoint(messages, len(messages)-opts.KeepRecent)
	if split <= 0 {
		return messages, SummaryResult{}, nil
	}

	// The old turns are rendered as plain text rather than replayed as
	// messages. Replaying them would mean sending tool calls without their tool
	// definitions, and reasoning items out of position — both of which a
	// provider may reject. The summarizer needs the content, not the structure.
	transcript := render(messages[:split])

	resp, err := p.Complete(ctx, llm.Request{
		System:   summarySystem,
		Messages: []llm.Message{llm.UserText(transcript + summaryRequest)},
	}, nil)
	if err != nil {
		return messages, SummaryResult{}, err
	}

	summary := strings.TrimSpace(resp.Message.TextContent())
	if summary == "" {
		return messages, SummaryResult{}, errors.New("the model returned an empty summary")
	}

	out := make([]llm.Message, 0, len(messages)-split+1)
	out = append(out, llm.UserText(summaryPreamble+summary))
	out = append(out, messages[split:]...)

	return out, SummaryResult{Summarized: split, Usage: resp.Usage}, nil
}

// SafeSplitPoint returns an index at or before desired where the transcript can
// be cut without separating a tool call from its result.
func SafeSplitPoint(messages []llm.Message, desired int) int {
	if desired >= len(messages) {
		return len(messages)
	}
	if desired <= 0 {
		return 0
	}

	for i := desired; i > 0; i-- {
		if startsUserTurn(messages[i]) {
			return i
		}
	}
	return 0
}

// startsUserTurn reports whether a message is a prompt the person typed, rather
// than the tool results that also travel as user messages.
func startsUserTurn(m llm.Message) bool {
	if m.Role != llm.RoleUser {
		return false
	}
	for _, b := range m.Blocks {
		if _, ok := b.(llm.ToolResult); ok {
			return false
		}
	}
	return true
}

func render(messages []llm.Message) string {
	var b strings.Builder

	for _, m := range messages {
		for _, block := range m.Blocks {
			switch v := block.(type) {
			case llm.Text:
				fmt.Fprintf(&b, "%s: %s\n", m.Role, v.Text)
			case llm.ToolUse:
				fmt.Fprintf(&b, "tool call %s %s\n", v.Name, string(v.Input))
			case llm.ToolResult:
				fmt.Fprintf(&b, "tool result: %s\n", clip(v.Content, maxRenderedResult))
			}
			// Thinking blocks are skipped: they are encrypted and unreadable.
		}
	}
	return b.String()
}

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... [%d more bytes]", len(s)-max)
}
