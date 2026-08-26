package tui

import (
	"time"

	"charm.land/bubbletea/v2"

	"github.com/ritik6559/cinch/internal/llm"
)

type (
	textMsg     string
	usageMsg    llm.Usage
	noticeMsg   string
	modelsMsg   []string
	tickMsg     struct{}
	errorMsg    struct{ error }
	turnDoneMsg struct{ err error }

	toolCallMsg struct {
		name    string
		summary string
	}

	toolResultMsg struct {
		name   string
		result string
		failed bool
	}
)

type agentHooks struct {
	text   func(string)
	call   func(name, summary string)
	result func(name, result string, failed bool)
	usage  func(llm.Usage)
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}
