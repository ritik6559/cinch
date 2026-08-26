package tui

import (
	"context"

	"charm.land/bubbles/v2/viewport"
	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/agent"
	"github.com/ritik6559/cinch/internal/approval"
	"github.com/ritik6559/cinch/internal/config"
	"github.com/ritik6559/cinch/internal/llm"
	"github.com/ritik6559/cinch/internal/session"
	"github.com/ritik6559/cinch/internal/tools"
)

type Deps struct {
	Provider llm.Provider
	Tools    *tools.Tools
	Config   config.Config
	Session  *session.Session
	Root     string
	System   string
}

func Run(ctx context.Context, d Deps) error {
	saved, err := approval.Load()
	if err != nil {
		saved = &approval.Store{}
	}

	m := &Model{
		provider:         d.Provider,
		cfg:              d.Config,
		sess:             d.Session,
		saved:            saved,
		root:             d.Root,
		ctx:              ctx,
		events:           make(chan tea.Msg, 64),
		theme:            darkTheme(),
		dark:             true,
		follow:           true,
		width:            80,
		height:           24,
		sessionApprovals: map[string]bool{},
	}

	m.viewport = viewport.New()
	m.input = newInput()

	h := m.hooks()
	m.agent = agent.New(d.Provider, d.Tools, m.approver(), agent.Hooks{
		OnText:       h.text,
		OnToolCall:   h.call,
		OnToolResult: h.result,
		OnUsage:      h.usage,
	})

	if d.System != "" {
		m.agent.SetSystemPrompt(d.System)
	}
	m.agent.SetModel(d.Config.Model)
	m.agent.SetEffort(d.Config.Effort)

	if len(d.Session.Messages) > 0 {
		m.agent.Restore(d.Session.Messages, d.Session.Usage)
		m.entries = transcript(d.Session.Messages)
		if d.Session.Model != "" {
			m.agent.SetModel(d.Session.Model)
		}
	}

	_, err = tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}
