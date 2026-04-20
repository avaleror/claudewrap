// InputModel is the single-line prompt input with history, Ctrl+W, and Ctrl+U.
// It emits SubmitMsg on Enter and CancelMsg on Esc.
package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// InputModel is a simple single-line input component for bubbletea v2.
// Supports history (↑↓), basic editing, and emits SubmitMsg on Enter.
type InputModel struct {
	buffer     []rune
	cursor     int
	width      int
	history    []string // newest first
	historyIdx int      // -1 = live input; 0+ = navigating history
	saved      string   // preserves live input while browsing history
}

type SubmitMsg struct{ Text string }
type CancelMsg struct{}

func NewInput(width int) InputModel {
	return InputModel{width: width, historyIdx: -1}
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		k := msg.Key()
		switch k.Code {
		case tea.KeyEnter:
			text := string(m.buffer)
			if text != "" {
				m.history = append([]string{text}, m.history...)
				if len(m.history) > 100 {
					m.history = m.history[:100]
				}
			}
			m.buffer = nil
			m.cursor = 0
			m.historyIdx = -1
			m.saved = ""
			return m, func() tea.Msg { return SubmitMsg{Text: text} }

		case tea.KeyEsc:
			m.buffer = nil
			m.cursor = 0
			m.historyIdx = -1
			m.saved = ""
			return m, func() tea.Msg { return CancelMsg{} }

		case tea.KeyUp:
			if len(m.history) > 0 && m.historyIdx < len(m.history)-1 {
				if m.historyIdx == -1 {
					m.saved = string(m.buffer)
				}
				m.historyIdx++
				m.buffer = []rune(m.history[m.historyIdx])
				m.cursor = len(m.buffer)
			}

		case tea.KeyDown:
			if m.historyIdx > 0 {
				m.historyIdx--
				m.buffer = []rune(m.history[m.historyIdx])
				m.cursor = len(m.buffer)
			} else if m.historyIdx == 0 {
				m.historyIdx = -1
				m.buffer = []rune(m.saved)
				m.cursor = len(m.buffer)
			}

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

// IsConfirmationInput returns true if input looks like a tool approval response.
// These bypass compression and go straight to the PTY.
func IsConfirmationInput(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "y", "n", "yes", "no":
		return true
	}
	// Single digit — tool/option selection (1, 2, 3...)
	return len(s) == 1 && s[0] >= '0' && s[0] <= '9'
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
