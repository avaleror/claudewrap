package tui

import "github.com/charmbracelet/lipgloss"

type ClaudeState int

const (
	StateRunning   ClaudeState = iota // Claude processing
	StateWaiting                      // Claude idle, waiting for input
	StateCompacting                   // /compact in progress
	StateRateLimit                    // Session exhausted
)

var (
	colorNeutral  = lipgloss.Color("240")
	colorWaiting  = lipgloss.Color("39")  // blue
	colorCompact  = lipgloss.Color("214") // yellow/orange
	colorLimit    = lipgloss.Color("196") // red
	colorGood     = lipgloss.Color("82")  // green
	colorWarn     = lipgloss.Color("214")
	colorDanger   = lipgloss.Color("196")
	colorDim      = lipgloss.Color("240")
	colorHeader   = lipgloss.Color("111")
)

func borderColor(state ClaudeState) lipgloss.Color {
	switch state {
	case StateWaiting:
		return colorWaiting
	case StateCompacting:
		return colorCompact
	case StateRateLimit:
		return colorLimit
	default:
		return colorNeutral
	}
}

var panelStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	Padding(0, 1)

var headerStyle = lipgloss.NewStyle().
	Foreground(colorHeader).
	Bold(true)

var dimStyle = lipgloss.NewStyle().Foreground(colorDim)

var statusBarStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("236")).
	Foreground(lipgloss.Color("250")).
	Padding(0, 1)
