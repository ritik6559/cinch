package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
)

type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryTool
	entryNotice
	entryError
)

type entry struct {
	kind entryKind
	text string

	tool    string
	summary string
	result  string
	failed  bool
	done    bool
}

func (m *Model) render(e entry) string {
	switch e.kind {
	case entryUser:
		return m.theme.Prompt.Render("› ") + m.theme.User.Render(e.text)

	case entryAssistant:
		return m.markdown(e.text)

	case entryTool:
		return m.renderTool(e)

	case entryNotice:
		return m.theme.Notice.Render("  " + e.text)

	case entryError:
		return m.theme.Error.Render("  " + e.text)
	}
	return ""
}

func (m *Model) renderTool(e entry) string {
	head := m.theme.ToolName.Render("⏺ "+e.tool) +
		m.theme.ToolArgs.Render(" "+toolArgs(e.summary, e.tool))

	if !e.done {
		return head
	}

	style := m.theme.ToolOK
	if e.failed {
		style = m.theme.ToolFail
	}
	return head + "\n" + style.Render("  ⎿ "+summarise(e.tool, e.result, e.failed))
}

func toolArgs(summary, tool string) string {
	for _, prefix := range []string{tool + ": ", tool + " ", "run: ", "write file ", "edit file ", "list files in "} {
		if strings.HasPrefix(summary, prefix) {
			return strings.TrimPrefix(summary, prefix)
		}
	}
	return summary
}

func summarise(tool, result string, failed bool) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return "no output"
	}

	if failed {
		first, _, more := strings.Cut(result, "\n")
		first = strings.TrimPrefix(first, "error: ")
		if more {
			first += " …"
		}
		return clip(first, 100)
	}

	lines := strings.Count(result, "\n") + 1

	switch tool {
	case "grep":
		if result == "no matches" {
			return "no matches"
		}
		return plural(lines, "match", "matches")

	case "glob":
		if strings.HasPrefix(result, "no files") {
			return "no files"
		}
		return plural(lines, "file", "files")

	case "read_file":
		return plural(lines, "line", "lines")

	case "list_files":
		return plural(lines, "entry", "entries")

	case "bash":
		if status, rest, ok := strings.Cut(result, "\n"); ok && strings.HasPrefix(status, "exit status") {
			return status + " · " + plural(strings.Count(rest, "\n")+1, "line", "lines")
		}
		return plural(lines, "line", "lines")
	}

	return clip(strings.ReplaceAll(result, "\n", " "), 100)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func (m *Model) markdown(text string) string {
	if m.glamour == nil {
		return m.theme.Assistant.Render(text)
	}

	out, err := m.glamour.Render(text)
	if err != nil {
		return m.theme.Assistant.Render(text)
	}
	return strings.TrimRight(out, "\n")
}

func newGlamour(dark bool, width int) *glamour.TermRenderer {
	style := glamour.WithStandardStyle("light")
	if dark {
		style = glamour.WithStandardStyle("dark")
	}

	r, err := glamour.NewTermRenderer(style, glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}
	return r
}

func (m *Model) diff(arguments string) string {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintln(&b, m.theme.ToolName.Render(args.Path))

	for line := range strings.SplitSeq(args.OldString, "\n") {
		fmt.Fprintln(&b, m.theme.DiffDel.Render("  - "+clip(line, m.width-6)))
	}
	for line := range strings.SplitSeq(args.NewString, "\n") {
		fmt.Fprintln(&b, m.theme.DiffAdd.Render("  + "+clip(line, m.width-6)))
	}

	return strings.TrimRight(b.String(), "\n")
}

func comma(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func compactTokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}
