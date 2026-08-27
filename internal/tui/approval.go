package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/tools"
)

type approvalReq struct {
	tool      string
	summary   string
	arguments string
	reply     chan bool
}

func (m *Model) approver() func(tool, summary, arguments string) bool {
	return func(tool, summary, arguments string) bool {
		command := tools.CommandOf(arguments)

		if m.sessionApprovals[tool] || m.saved.Allows(tool, command) {
			return true
		}

		req := &approvalReq{
			tool: tool, summary: summary, arguments: arguments,
			reply: make(chan bool, 1),
		}
		m.events <- req

		select {
		case answer := <-req.reply:
			return answer
		case <-m.ctx.Done():
			return false
		}
	}
}

func (m *Model) approvalPrompt() string {
	var b strings.Builder

	fmt.Fprintln(&b, m.theme.Prompt.Render("  allow ")+m.theme.CmdName.Render(m.pending.summary)+"?")

	if m.pending.tool == "edit_file" {
		if d := m.diff(m.pending.arguments); d != "" {
			fmt.Fprintln(&b, d)
		}
	}

	options := "  [y] yes   [n] no   [s] save permanently"
	if m.pending.tool != "bash" {
		options = "  [y] yes   [n] no   [a] all this session   [s] save permanently"
	}
	fmt.Fprint(&b, m.theme.Hint.Render(options))

	return b.String()
}

func (m *Model) approvalKey(key tea.KeyPressMsg) tea.Cmd {
	answer := func(ok bool) tea.Cmd {
		m.pending.reply <- ok
		m.pending = nil
		m.mode = modeWorking
		m.refresh()
		return nil
	}

	switch key.String() {
	case "y":
		return answer(true)

	case "a":
		if m.pending.tool == "bash" {
			m.note("not offered for bash — press s to save a command prefix")
			return nil
		}
		m.sessionApprovals[m.pending.tool] = true
		return answer(true)

	case "s":
		m.remember(m.pending.tool, tools.CommandOf(m.pending.arguments))
		return answer(true)

	case "n", "esc", "ctrl+c":
		return answer(false)
	}

	return nil
}

func (m *Model) remember(tool, command string) {
	prefix := ""
	if tool == "bash" {
		p, ok := approval.PrefixFor(command)
		if !ok {
			m.note("allowed once — cannot save a rule because " + approval.WhyUnsafe(command))
			return
		}
		prefix = p
	}

	if m.saved.Add(tool, prefix) {
		if err := m.saved.Save(); err != nil {
			m.fail(fmt.Errorf("could not save approval: %w", err))
			return
		}
	}
	m.note("saved: " + approval.Describe(tool, prefix))
}

var _ = func(m *Model) func(string, string, string) bool { return m.approver() }

var _ = context.Background
