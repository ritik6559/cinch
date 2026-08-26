package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/session"
)

func (m *Model) runCommand(line string) tea.Cmd {
	name, args := splitCommand(line)

	if _, ok := lookupCommand(name); !ok {
		m.fail(fmt.Errorf("unknown command /%s — press / to see them", name))
		return nil
	}

	switch name {
	case "help":
		m.showHelp()
	case "model":
		return m.cmdModel(args)
	case "effort":
		m.cmdEffort(args)
	case "compact":
		m.compactNow()
	case "cost":
		m.showCost()
	case "clear":
		m.cmdClear()
	case "sessions":
		m.cmdSessions()
	case "resume":
		m.cmdResume(args)
	case "approvals":
		m.cmdApprovals()
	case "quit":
		m.quitting = true
		m.saveSession()
		return tea.Quit
	}
	return nil
}

func (m *Model) showHelp() {
	var b strings.Builder
	for _, c := range commands {
		name := "/" + c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		fmt.Fprintf(&b, "%-20s %s\n", name, c.Desc)
	}
	m.note(strings.TrimRight(b.String(), "\n"))
}

func (m *Model) cmdModel(args string) tea.Cmd {
	if args != "" {
		m.agent.SetModel(args)
		m.sess.Model = args
		m.note("model: " + args)
		return nil
	}

	lister, ok := m.provider.(llm.ModelLister)
	if !ok {
		m.fail(fmt.Errorf("this provider cannot list models — use /model <id>"))
		return nil
	}

	// Fetched off the update loop: the call is over the network and would
	// otherwise freeze the interface.
	return func() tea.Msg {
		models, err := lister.Models(m.ctx)
		if err != nil && len(models) == 0 {
			return errorMsg{err}
		}
		return modelsMsg(models)
	}
}

func (m *Model) cmdEffort(args string) {
	if args != "" {
		if !llm.ValidEffort(args) {
			m.fail(fmt.Errorf("effort must be one of %s", strings.Join(llm.Efforts, ", ")))
			return
		}
		m.agent.SetEffort(args)
		m.note("effort: " + args)
		return
	}

	p := newPicker("Reasoning effort", llm.Efforts, m.agent.Effort())
	p.onChoose = func(i int) tea.Cmd {
		m.agent.SetEffort(llm.Efforts[i])
		m.note("effort: " + llm.Efforts[i])
		return nil
	}
	m.picker = p
	m.mode = modePicking
	m.refresh()
}

func (m *Model) showCost() {
	u := m.agent.Usage()
	m.note(fmt.Sprintf(
		"session: %s in · %s cached · %s out · %s thinking",
		comma(u.InputTokens), comma(u.CachedTokens),
		comma(u.OutputTokens), comma(u.ReasoningTokens),
	))
}

func (m *Model) cmdClear() {
	m.sess = session.New(m.root, m.cfg.Provider, m.agentModel())
	m.agent.Restore(nil, llm.Usage{})
	m.entries = nil
	m.streaming = false
	m.note("started a new conversation")
}

func (m *Model) agentModel() string {
	if model := m.agent.Model(); model != "" {
		return model
	}
	return m.cfg.Model
}

func (m *Model) cmdSessions() {
	all, err := session.List()
	if err != nil {
		m.fail(err)
		return
	}
	if len(all) == 0 {
		m.note("no saved sessions yet")
		return
	}

	ids := make([]string, len(all))
	labels := make([]string, len(all))
	for i, s := range all {
		ids[i] = s.ID
		labels[i] = fmt.Sprintf("%-24s %2d turns  %s", s.ID, s.Turns(), clip(s.Title, 40))
	}

	p := newPicker("Sessions", ids, m.sess.ID)
	p.labels = labels
	p.onChoose = func(i int) tea.Cmd {
		m.loadSession(ids[i])
		return nil
	}
	m.picker = p
	m.mode = modePicking
	m.refresh()
}

func (m *Model) cmdResume(id string) {
	if id == "" {
		m.fail(fmt.Errorf("/resume needs a session id — try /sessions"))
		return
	}
	m.loadSession(id)
}

func (m *Model) loadSession(id string) {
	s, err := session.Load(id)
	if err != nil {
		m.fail(err)
		return
	}

	m.sess = s
	m.agent.Restore(s.Messages, s.Usage)
	m.entries = transcript(s.Messages)
	m.streaming = false

	if s.Model != "" {
		m.agent.SetModel(s.Model)
	}
	m.note(fmt.Sprintf("resumed %s · %d turns", s.ID, s.Turns()))
}

func transcript(messages []llm.Message) []entry {
	var out []entry

	for _, msg := range messages {
		for _, block := range msg.Blocks {
			switch b := block.(type) {
			case llm.Text:
				kind := entryAssistant
				if msg.Role == llm.RoleUser {
					kind = entryUser
				}
				out = append(out, entry{kind: kind, text: b.Text})

			case llm.ToolUse:
				out = append(out, entry{
					kind: entryTool, tool: b.Name, summary: string(b.Input), done: true,
					result: "(from a saved session)",
				})
			}
		}
	}
	return out
}

func (m *Model) cmdApprovals() {
	saved, err := approval.Load()
	if err != nil {
		m.fail(err)
		return
	}
	if len(saved.Rules) == 0 {
		m.note("no saved approvals")
		return
	}

	keys := make([]string, len(saved.Rules))
	labels := make([]string, len(saved.Rules))
	for i, r := range saved.Rules {
		keys[i] = r.Prefix
		if r.Prefix == "" {
			keys[i] = r.Tool
		}
		labels[i] = fmt.Sprintf("%-12s %s", r.Tool, approval.Describe(r.Tool, r.Prefix))
	}

	p := newPicker("Approvals — enter removes", keys, "")
	p.labels = labels
	p.onChoose = func(i int) tea.Cmd {
		if n := saved.Remove(keys[i]); n > 0 {
			if err := saved.Save(); err != nil {
				m.fail(err)
				return nil
			}
			m.saved = saved
			m.note("removed " + keys[i])
		}
		return nil
	}
	m.picker = p
	m.mode = modePicking
	m.refresh()
}
