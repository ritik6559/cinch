package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/llm"
)

func (m *Model) onKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeApproving:
		return m, m.approvalKey(key)
	case modePicking:
		return m, m.pickerKey(key)
	}

	switch key.String() {
	case "ctrl+c":
		if m.mode == modeWorking && m.cancelTurn != nil {
			m.cancelTurn()
			return m, nil
		}
		return m.quit()

	case "esc":
		if m.mode == modeWorking && m.cancelTurn != nil {
			m.cancelTurn()
			return m, nil
		}
		if matches, _ := m.suggestions(); len(matches) > 0 {
			m.input.Reset()
			m.cmdCursor = 0
		}
		return m, nil

	case "ctrl+d":
		return m.quit()

	case "tab":
		m.complete()
		return m, nil

	case "enter":
		return m.submit()

	case "up", "down":
		if matches, cursor := m.suggestions(); len(matches) > 0 {
			if key.String() == "up" {
				m.cmdCursor = max(0, cursor-1)
			} else {
				m.cmdCursor = min(len(matches)-1, cursor+1)
			}
			return m, nil
		}

	case "pgup":
		m.scroll(m.viewport.HalfPageUp)
		return m, nil

	case "pgdown":
		m.scroll(m.viewport.HalfPageDown)
		return m, nil

	case "shift+up", "ctrl+up":
		m.scroll(func() { m.viewport.ScrollUp(1) })
		return m, nil

	case "shift+down", "ctrl+down":
		m.scroll(func() { m.viewport.ScrollDown(1) })
		return m, nil

	// Only steal end while scrolled up, where it means "back to live". At the
	// bottom that would be a no-op, so leave it to the textarea for editing.
	case "end":
		if !m.follow {
			m.scroll(func() { m.viewport.GotoBottom() })
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)

	m.cmdCursor = 0
	return m, cmd
}

func (m *Model) complete() bool {
	matches, cursor := m.suggestions()
	if len(matches) == 0 {
		return false
	}

	m.input.SetValue(completion(matches[cursor]))
	m.input.CursorEnd()
	m.cmdCursor = 0
	return true
}

func (m *Model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	m.saveSession()
	return m, tea.Quit
}

func (m *Model) submit() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return m, nil
	}
	if m.mode == modeWorking {
		return m, nil
	}
	// Enter on a half-typed command takes the highlighted row rather than
	// running it, so `/c` never silently fires `/clear`. A second Enter runs it.
	if matches, cursor := m.suggestions(); len(matches) > 0 && "/"+matches[cursor].Name != line {
		m.complete()
		return m, nil
	}

	m.input.Reset()
	m.cmdCursor = 0

	if isCommand(line) {
		return m, m.runCommand(line)
	}

	m.entries = append(m.entries, entry{kind: entryUser, text: line})
	m.sess.SetTitle(line)
	m.streaming = false
	m.follow = true // sending a message means you want to watch the answer
	m.refresh()

	return m, m.startTurn(line)
}

func (m *Model) startTurn(prompt string) tea.Cmd {
	turnCtx, cancel := context.WithCancel(m.ctx)
	m.cancelTurn = cancel
	m.mode = modeWorking
	m.work = newWorking()
	m.turnTokens = 0

	run := func() tea.Msg {
		err := m.agent.Run(turnCtx, prompt)
		cancel()
		return turnDoneMsg{err: err}
	}

	return tea.Batch(run, tick())
}

func (m *Model) hooks() agentHooks {
	return agentHooks{
		text: func(s string) { m.events <- textMsg(s) },
		call: func(name, summary string) {
			m.events <- toolCallMsg{name: name, summary: summary}
		},
		result: func(name, result string, failed bool) {
			m.events <- toolResultMsg{name: name, result: result, failed: failed}
		},
		usage: func(u llm.Usage) { m.events <- usageMsg(u) },
	}
}

func newInput() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "ask about this repository, or / for commands"
	ta.Prompt = "› "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(inputHeight)
	ta.Focus()
	return ta
}
