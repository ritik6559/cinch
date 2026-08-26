package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	Accent  color.Color // prompt, active selection, the cinch name
	Text    color.Color // model prose
	Muted   color.Color // tool names, status bar
	Faint   color.Color // tool arguments, hints, results
	Success color.Color
	Danger  color.Color
	Border  color.Color

	User      lipgloss.Style
	Assistant lipgloss.Style
	ToolName  lipgloss.Style
	ToolArgs  lipgloss.Style
	ToolOK    lipgloss.Style
	ToolFail  lipgloss.Style
	Status    lipgloss.Style
	StatusKey lipgloss.Style
	Hint      lipgloss.Style
	Working   lipgloss.Style
	Prompt    lipgloss.Style
	Selected  lipgloss.Style
	CmdName   lipgloss.Style
	CmdDesc   lipgloss.Style
	Error     lipgloss.Style
	Notice    lipgloss.Style
	Box       lipgloss.Style
	DiffAdd   lipgloss.Style
	DiffDel   lipgloss.Style
}

// Warm neutral: terracotta accent, warm greys. Colour is used sparingly — for
// the prompt, for what is selected, and for whether a tool worked.
func darkTheme() Theme {
	return build(Theme{
		Accent:  lipgloss.Color("#d98f6a"),
		Text:    lipgloss.Color("#e8e3dd"),
		Muted:   lipgloss.Color("#a39a90"),
		Faint:   lipgloss.Color("#6b645d"),
		Success: lipgloss.Color("#8fae7a"),
		Danger:  lipgloss.Color("#d1746b"),
		Border:  lipgloss.Color("#3d3833"),
	})
}

func lightTheme() Theme {
	return build(Theme{
		Accent:  lipgloss.Color("#b05a30"),
		Text:    lipgloss.Color("#2b2724"),
		Muted:   lipgloss.Color("#6b645d"),
		Faint:   lipgloss.Color("#9a938b"),
		Success: lipgloss.Color("#4f7a3a"),
		Danger:  lipgloss.Color("#b03a30"),
		Border:  lipgloss.Color("#d6cfc6"),
	})
}

func build(t Theme) Theme {
	t.User = lipgloss.NewStyle().Foreground(t.Muted)
	t.Assistant = lipgloss.NewStyle().Foreground(t.Text)
	t.ToolName = lipgloss.NewStyle().Foreground(t.Muted)
	t.ToolArgs = lipgloss.NewStyle().Foreground(t.Faint)
	t.ToolOK = lipgloss.NewStyle().Foreground(t.Faint)
	t.ToolFail = lipgloss.NewStyle().Foreground(t.Danger)
	t.Status = lipgloss.NewStyle().Foreground(t.Faint)
	t.StatusKey = lipgloss.NewStyle().Foreground(t.Muted)
	t.Hint = lipgloss.NewStyle().Foreground(t.Faint)
	t.Working = lipgloss.NewStyle().Foreground(t.Accent)
	t.Prompt = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	t.Selected = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	t.CmdName = lipgloss.NewStyle().Foreground(t.Text)
	t.CmdDesc = lipgloss.NewStyle().Foreground(t.Faint)
	t.Error = lipgloss.NewStyle().Foreground(t.Danger)
	t.Notice = lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	t.Box = lipgloss.NewStyle().Foreground(t.Border)
	t.DiffAdd = lipgloss.NewStyle().Foreground(t.Success)
	t.DiffDel = lipgloss.NewStyle().Foreground(t.Danger)
	return t
}
