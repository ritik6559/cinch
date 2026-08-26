package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
)

const pickerWindow = 8

type picker struct {
	title    string
	items    []string
	labels   []string
	cursor   int
	onChoose func(index int) tea.Cmd
}

func newPicker(title string, items []string, current string) *picker {
	p := &picker{title: title, items: items}

	for i, item := range items {
		if item == current {
			p.cursor = i
			break
		}
	}
	return p
}

func (p *picker) label(i int) string {
	if i < len(p.labels) {
		return p.labels[i]
	}
	return p.items[i]
}

func (p *picker) view(t Theme) string {
	var b strings.Builder
	fmt.Fprintln(&b, t.Prompt.Render("  "+p.title)+t.Hint.Render("   ↑↓ choose · enter select · esc cancel"))

	start := max(0, min(p.cursor-pickerWindow/2, len(p.items)-pickerWindow))
	end := min(start+pickerWindow, len(p.items))

	for i := start; i < end; i++ {
		if i == p.cursor {
			fmt.Fprintf(&b, "%s%s\n", t.Prompt.Render("  › "), t.Selected.Render(p.label(i)))
			continue
		}
		fmt.Fprintf(&b, "    %s\n", t.CmdName.Render(p.label(i)))
	}

	if len(p.items) > pickerWindow {
		fmt.Fprint(&b, t.Hint.Render(fmt.Sprintf("    %d of %d", p.cursor+1, len(p.items))))
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) pickerKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "up", "ctrl+p":
		m.picker.cursor = max(0, m.picker.cursor-1)

	case "down", "ctrl+n":
		m.picker.cursor = min(len(m.picker.items)-1, m.picker.cursor+1)

	case "enter":
		cmd := m.picker.onChoose
		index := m.picker.cursor
		m.closePicker()
		if cmd != nil {
			return cmd(index)
		}

	case "esc", "ctrl+c", "q":
		m.closePicker()
	}
	return nil
}

func (m *Model) closePicker() {
	m.picker = nil
	m.mode = modeIdle
	m.refresh()
}
