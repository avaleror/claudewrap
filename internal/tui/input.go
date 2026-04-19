package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// InputModel is a simple multi-line input component for bubbletea v2.
// It buffers keystrokes, handles basic editing, and emits SubmitMsg on Enter.
type InputModel struct {
	buffer []rune
	cursor int
	width  int
}

type SubmitMsg struct{ Text string }
type CancelMsg struct{}

func NewInput(width int) InputModel {
	return InputModel{width: width}
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		k := msg.Key()
		switch k.Code {
		case tea.KeyEnter:
			text := string(m.buffer)
			m.buffer = nil
			m.cursor = 0
			return m, func() tea.Msg { return SubmitMsg{Text: text} }

		case tea.KeyEsc:
			m.buffer = nil
			m.cursor = 0
			return m, func() tea.Msg { return CancelMsg{} }

		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.buffer = append(m.buffer[:m.cursor-1], m.buffer[m.cursor:]...)
				m.cursor--
			}

		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyRight:
			if m.cursor < len(m.buffer) {
				m.cursor++
			}

		case tea.KeyHome:
			m.cursor = 0

		case tea.KeyEnd:
			m.cursor = len(m.buffer)

		default:
			// Ctrl+U — clear line
			if k.Code == 'u' && k.Mod == tea.ModCtrl {
				m.buffer = nil
				m.cursor = 0
				break
			}
			// Ctrl+W — delete word backward
			if k.Code == 'w' && k.Mod == tea.ModCtrl {
				m = deleteWordBackward(m)
				break
			}
			// Printable character
			if k.Text != "" {
				runes := []rune(k.Text)
				m.buffer = append(m.buffer[:m.cursor], append(runes, m.buffer[m.cursor:]...)...)
				m.cursor += len(runes)
			}
		}
	}
	return m, nil
}

func (m InputModel) View() string {
	text := string(m.buffer)
	cursor := "█"
	if m.cursor >= len(m.buffer) {
		return "> " + text + cursor
	}
	before := string(m.buffer[:m.cursor])
	at := string(m.buffer[m.cursor])
	after := string(m.buffer[m.cursor+1:])
	return "> " + before + cursor + at + after
}

func (m InputModel) Value() string {
	return string(m.buffer)
}

func (m InputModel) IsEmpty() bool {
	return len(m.buffer) == 0
}

// IsConfirmationInput returns true if the buffer looks like a y/n response.
// Used to detect when we should pass through directly to PTY.
func IsConfirmationInput(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "y" || s == "n" || s == "yes" || s == "no"
}

func deleteWordBackward(m InputModel) InputModel {
	if m.cursor == 0 {
		return m
	}
	end := m.cursor
	for end > 0 && unicode.IsSpace(m.buffer[end-1]) {
		end--
	}
	start := end
	for start > 0 && !unicode.IsSpace(m.buffer[start-1]) {
		start--
	}
	m.buffer = append(m.buffer[:start], m.buffer[end:]...)
	m.cursor = start
	return m
}
