package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/compact"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/session"
)

type mode int

const (
	modeIdle mode = iota
	modeWorking
	modeApproving
	modePicking
)

type Model struct {
	agent    *agent.Agent
	provider llm.Provider
	cfg      config.Config
	sess     *session.Session
	saved    *approval.Store
	root     string

	ctx    context.Context
	events chan tea.Msg

	// The turn in flight, if any.
	cancelTurn context.CancelFunc
	work       working
	turnTokens int

	mode      mode
	pending   *approvalReq
	picker    *picker
	cmdCursor int // highlighted row in the slash popup
	entries   []entry
	streaming bool // an assistant entry is open and receiving text

	viewport viewport.Model
	input    textarea.Model
	glamour  *glamour.TermRenderer
	theme    Theme
	dark     bool

	width  int
	height int

	sessionApprovals map[string]bool
	quitting         bool
	err              error
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		textarea.Blink,
		waitForEvent(m.events),
	)
}

func waitForEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.theme = darkTheme()
		if !m.dark {
			m.theme = lightTheme()
		}
		m.applyTheme()
		return m, nil

	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)

	case textMsg:
		m.appendText(string(msg))
		return m, waitForEvent(m.events)

	case toolCallMsg:
		m.entries = append(m.entries, entry{
			kind: entryTool, tool: msg.name, summary: msg.summary,
		})
		m.streaming = false
		m.refresh()
		return m, waitForEvent(m.events)

	case toolResultMsg:
		m.finishTool(msg)
		return m, waitForEvent(m.events)

	case usageMsg:
		m.turnTokens = llm.Usage(msg).InputTokens
		return m, waitForEvent(m.events)

	case *approvalReq:
		m.pending = msg
		m.mode = modeApproving
		m.refresh()
		return m, waitForEvent(m.events)

	case turnDoneMsg:
		return m, m.finishTurn(msg.err)

	case tickMsg:
		if m.mode != modeWorking {
			return m, nil
		}
		m.work.tick()
		return m, tick()

	case modelsMsg:
		m.picker = newPicker("Model", msg, m.agent.Model())
		m.mode = modePicking
		m.refresh()
		return m, nil

	case noticeMsg:
		m.note(string(msg))
		return m, nil

	case errorMsg:
		m.fail(msg.error)
		return m, nil
	}

	return m, nil
}

func (m *Model) View() tea.View {
	var v tea.View
	v.WindowTitle = "cinch"

	if m.quitting {
		return v
	}

	parts := []string{m.viewport.View(), m.status()}

	switch {
	case m.mode == modeWorking:
		parts = append(parts, m.work.line(m.theme, m.turnTokens))
	case m.mode == modeApproving:
		parts = append(parts, m.approvalPrompt())
	case m.mode == modePicking:
		parts = append(parts, m.picker.view(m.theme))
	}

	if m.mode == modeIdle || m.mode == modeWorking {
		parts = append(parts, m.input.View())
		if popup := m.commandPopup(); popup != "" {
			parts = append(parts, popup)
		}
	}

	v.Content = lipgloss.JoinVertical(lipgloss.Left, parts...)
	return v
}

func (m *Model) status() string {
	model := m.agent.Model()
	if model == "" {
		model = m.cfg.Model
	}

	effort := m.agent.Effort()
	if effort == "" {
		effort = "default"
	}

	fields := []string{
		model,
		effort,
		compactTokens(m.agent.Usage().InputTokens) + " ctx",
		filepath.Base(m.root),
	}
	return m.theme.Status.Render("  " + strings.Join(fields, " · "))
}

// suggestions returns the commands the popup is offering, and where the
// highlight sits within them. The cursor is clamped here rather than on every
// keystroke, so shrinking the list as you type can never leave it out of range.
func (m *Model) suggestions() ([]command, int) {
	matches := matchCommands(m.input.Value())
	if len(matches) == 0 {
		return nil, 0
	}
	return matches, min(m.cmdCursor, len(matches)-1)
}

func (m *Model) commandPopup() string {
	matches, cursor := m.suggestions()
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder
	for i, c := range matches {
		name := "/" + c.Name
		if c.Args != "" {
			name += " " + c.Args
		}

		style := m.theme.CmdName
		marker := "  "
		if i == cursor {
			style = m.theme.Selected
			marker = m.theme.Prompt.Render("› ")
		}
		fmt.Fprintf(&b, "%s%-22s %s\n", marker, style.Render(name), m.theme.CmdDesc.Render(c.Desc))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height

	m.input.SetWidth(width - 2)
	m.glamour = newGlamour(m.dark, max(width-4, 20))

	const chrome = 9
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(max(height-chrome, 3))

	m.refresh()
}

func (m *Model) applyTheme() {
	styles := m.input.Styles()
	styles.Focused.Prompt = styles.Focused.Prompt.Foreground(m.theme.Accent)
	styles.Blurred.Prompt = styles.Blurred.Prompt.Foreground(m.theme.Faint)
	m.input.SetStyles(styles)
	m.glamour = newGlamour(m.dark, max(m.width-4, 20))
	m.refresh()
}

func (m *Model) refresh() {
	blocks := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		if rendered := m.render(e); rendered != "" {
			blocks = append(blocks, rendered)
		}
	}

	m.viewport.SetContent(strings.Join(blocks, "\n\n"))
	m.viewport.GotoBottom()
}

func (m *Model) appendText(text string) {
	if !m.streaming {
		m.entries = append(m.entries, entry{kind: entryAssistant})
		m.streaming = true
	}
	m.entries[len(m.entries)-1].text += text
	m.refresh()
}

func (m *Model) finishTool(msg toolResultMsg) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind == entryTool && !m.entries[i].done && m.entries[i].tool == msg.name {
			m.entries[i].result = msg.result
			m.entries[i].failed = msg.failed
			m.entries[i].done = true
			m.refresh()
			return
		}
	}

	m.entries = append(m.entries, entry{
		kind: entryTool, tool: msg.name, summary: "",
		result: msg.result, failed: msg.failed, done: true,
	})
	m.refresh()
}

func (m *Model) note(text string) {
	m.entries = append(m.entries, entry{kind: entryNotice, text: text})
	m.streaming = false
	m.refresh()
}

func (m *Model) fail(err error) {
	m.entries = append(m.entries, entry{kind: entryError, text: err.Error()})
	m.streaming = false
	m.refresh()
}

func (m *Model) finishTurn(err error) tea.Cmd {
	m.mode = modeIdle
	m.streaming = false
	m.cancelTurn = nil

	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		m.note("interrupted")
	default:
		m.fail(err)
	}

	m.saveSession()

	if m.cfg.CompactAt > 0 && m.agent.TurnUsage().InputTokens > m.cfg.CompactAt {
		m.compactNow()
	}

	m.refresh()
	return nil
}

func (m *Model) saveSession() {
	m.sess.Messages = m.agent.Messages()
	m.sess.Usage = m.agent.Usage()
	if model := m.agent.Model(); model != "" {
		m.sess.Model = model
	}
	if err := m.sess.Save(); err != nil {
		m.fail(fmt.Errorf("could not save session: %w", err))
	}
}

func (m *Model) compactNow() {
	messages, cleared := compact.ToolResults(m.agent.Messages(), compact.DefaultOptions())

	remaining := m.agent.TurnUsage().InputTokens - cleared.EstimatedTokens()
	opts := compact.DefaultSummarizeOptions()
	var summarized compact.SummaryResult

	if m.cfg.CompactAt > 0 && remaining > m.cfg.CompactAt && compact.CanSummarize(messages, opts) {
		next, result, err := compact.Summarize(m.ctx, m.provider, messages, opts)
		if err != nil {
			m.fail(fmt.Errorf("could not summarize: %w", err))
		} else {
			messages, summarized = next, result
		}
	}

	if cleared.Cleared == 0 && summarized.Summarized == 0 {
		m.note("nothing more to compact")
		return
	}

	total := m.agent.Usage()
	total.Add(summarized.Usage)
	m.agent.Restore(messages, total)
	m.saveSession()

	var parts []string
	if cleared.Cleared > 0 {
		parts = append(parts, fmt.Sprintf("cleared %d tool results (~%s tokens)",
			cleared.Cleared, comma(cleared.EstimatedTokens())))
	}
	if summarized.Summarized > 0 {
		parts = append(parts, fmt.Sprintf("summarized %d messages", summarized.Summarized))
	}
	m.note("compacted: " + strings.Join(parts, ", "))
}
