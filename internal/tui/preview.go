package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

const previewDuration = 2 * time.Second

type PreviewState int

const (
	PreviewHidden  PreviewState = iota
	PreviewVisible              // showing compressed preview, countdown active
)

type PreviewModel struct {
	State      PreviewState
	Original   string
	Compressed string
	Engine     string
	Deadline   time.Time
	Cancelled  bool
}

type PreviewTickMsg struct{}
type PreviewSendMsg struct{ Text string }
type PreviewCancelMsg struct{}

func (p PreviewModel) Init() tea.Cmd {
	return nil
}

func (p PreviewModel) Show(original, compressed, engine string) (PreviewModel, tea.Cmd) {
	p.State = PreviewVisible
	p.Original = original
	p.Compressed = compressed
	p.Engine = engine
	p.Deadline = time.Now().Add(previewDuration)
	p.Cancelled = false
	return p, tickPreview()
}

func (p PreviewModel) Update(msg tea.Msg) (PreviewModel, tea.Cmd) {
	if p.State != PreviewVisible {
		return p, nil
	}
	switch msg.(type) {
	case PreviewTickMsg:
		if time.Now().After(p.Deadline) {
			p.State = PreviewHidden
			return p, func() tea.Msg { return PreviewSendMsg{Text: p.Compressed} }
		}
		return p, tickPreview()

	case tea.KeyPressMsg:
		k := msg.(tea.KeyPressMsg).Key()
		if k.Code == tea.KeyEsc {
			p.State = PreviewHidden
			p.Cancelled = true
			return p, func() tea.Msg { return PreviewSendMsg{Text: p.Original} }
		}
	}
	return p, nil
}

func (p PreviewModel) View(width int) string {
	if p.State != PreviewVisible {
		return ""
	}
	remaining := time.Until(p.Deadline)
	secs := remaining.Seconds()
	if secs < 0 {
		secs = 0
	}

	ratio := 0
	if len(p.Original) > 0 {
		ratio = 100 - (len([]rune(p.Compressed))*100)/len([]rune(p.Original))
	}
	preview := p.Compressed
	maxLen := width - 25
	if len(preview) > maxLen && maxLen > 3 {
		preview = preview[:maxLen] + "..."
	}

	line1 := fmt.Sprintf("  Compressed -%d%%: %s", ratio, preview)
	line2 := fmt.Sprintf("  Sending in %.1fs...  [Esc: send original]  [%s]", secs, p.Engine)

	style := lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252"))

	return style.Render(padRight(line1, width)) + "\n" +
		style.Render(padRight(line2, width))
}

func tickPreview() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return PreviewTickMsg{}
	})
}

func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
