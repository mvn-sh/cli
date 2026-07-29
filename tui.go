package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type choice struct{ label, value string }
type selectModel struct {
	title     string
	choices   []choice
	cursor    int
	selected  string
	cancelled bool
}

func (m selectModel) Init() tea.Cmd { return nil }
func (m selectModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = m.choices[m.cursor].value
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m selectModel) View() string {
	var b strings.Builder
	b.WriteString("\n  " + m.title + "\n\n")
	for i, item := range m.choices {
		cursor := "  "
		if i == m.cursor {
			cursor = "› "
		}
		fmt.Fprintf(&b, "  %s%s\n", cursor, item.label)
	}
	b.WriteString("\n  ↑/↓ move • enter select • esc cancel\n")
	return b.String()
}
func selectChoice(title string, choices []choice) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no choices available")
	}
	if len(choices) == 1 {
		return choices[0].value, nil
	}
	result, err := tea.NewProgram(selectModel{title: title, choices: choices}).Run()
	if err != nil {
		return "", err
	}
	model := result.(selectModel)
	if model.cancelled || model.selected == "" {
		return "", fmt.Errorf("selection cancelled")
	}
	return model.selected, nil
}
