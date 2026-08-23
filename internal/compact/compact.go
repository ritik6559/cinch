package compact

import (
	"fmt"
	"strings"

	"github.com/ritik6559/cinch/internal/llm"
)

const placeholderPrefix = "[cleared to save context"

type Options struct {
	// how many of the newest tool results are left untouchec.
	KeepRecent int

	// least a pass must free to be worth doing
	MinBytes int
}

func DefaultOptions() Options {
	return Options{KeepRecent: 6, MinBytes: 4000}
}

type Result struct {
	Cleared    int
	FreedBytes int
}

func (r Result) EstimatedTokens() int {
	return r.FreedBytes / 4
}

type position struct {
	message, block int
}

// ToolResults returns a copy of messages with old tool output cleared.
func ToolResults(messages []llm.Message, opts Options) ([]llm.Message, Result) {
	if opts.KeepRecent < 0 {
		opts.KeepRecent = 0
	}

	var all []position
	for i, m := range messages {
		for j, b := range m.Blocks {
			r, ok := b.(llm.ToolResult)
			if ok && r.Content != "" && !isPlaceholder(r.Content) {
				all = append(all, position{i, j})
			}
		}
	}

	if len(all) <= opts.KeepRecent {
		return messages, Result{}
	}
	targets := all[:len(all)-opts.KeepRecent]

	clearing := make(map[position]bool, len(targets))
	freed := 0
	for _, p := range targets {
		clearing[p] = true
		freed += len(messages[p.message].Blocks[p.block].(llm.ToolResult).Content)
	}

	if freed < opts.MinBytes {
		return messages, Result{}
	}
	out := make([]llm.Message, len(messages))
	for i, m := range messages {
		changed := false
		blocks := make([]llm.Block, len(m.Blocks))

		for j, b := range m.Blocks {
			if clearing[position{i, j}] {
				r := b.(llm.ToolResult)
				r.Content = placeholder(r.Content)
				blocks[j] = r
				changed = true
				continue
			}
			blocks[j] = b
		}

		if changed {
			out[i] = llm.Message{Role: m.Role, Blocks: blocks}
		} else {
			out[i] = m
		}
	}

	return out, Result{Cleared: len(targets), FreedBytes: freed}
}

func placeholder(content string) string {
	return fmt.Sprintf("%s: %d lines. Run the tool again if you need this.]",
		placeholderPrefix, strings.Count(content, "\n")+1)
}

func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, placeholderPrefix)
}
