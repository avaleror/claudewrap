package tui

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
	"github.com/taigrr/bubbleterm"
)

// TermWidget wraps bubbleterm and manages the PTY for the claude subprocess.
type TermWidget struct {
	model     *bubbleterm.Model
	ptmx      *os.File
	width     int
	height    int
	lastByte  time.Time
	idleTimer *time.Timer
}

type IdleMsg struct{}
type PTYExitMsg struct{ Err error }

func NewTermWidget(width, height int, args []string) (*TermWidget, error) {
	cmd := exec.Command("claude", args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	model, err := bubbleterm.NewWithCommand(width, height, cmd)
	if err != nil {
		return nil, err
	}

	return &TermWidget{
		model:    model,
		width:    width,
		height:   height,
		lastByte: time.Now(),
	}, nil
}

func (t *TermWidget) Init() tea.Cmd {
	return t.model.Init()
}

// IsIdle returns true if no PTY output has arrived in the last 500ms.
func (t *TermWidget) IsIdle() bool {
	return time.Since(t.lastByte) > 500*time.Millisecond
}

// SendText writes text + newline to the PTY — equivalent to the user submitting input.
func (t *TermWidget) SendText(text string) tea.Cmd {
	t.lastByte = time.Now()
	return t.model.SendInput(text + "\n")
}

// SendRaw forwards a single keystroke directly to the PTY (for confirmations).
func (t *TermWidget) SendRaw(s string) tea.Cmd {
	t.lastByte = time.Now()
	return t.model.SendInput(s)
}

func (t *TermWidget) Resize(w, h int) tea.Cmd {
	t.width = w
	t.height = h
	return t.model.Resize(w, h)
}

func (t *TermWidget) Update(msg tea.Msg) (*TermWidget, tea.Cmd) {
	m, cmd := t.model.Update(msg)
	t.model = m.(*bubbleterm.Model)
	// Track output activity
	switch msg.(type) {
	case tea.WindowSizeMsg:
		// handled via Resize
	}
	return t, cmd
}

func (t *TermWidget) View() string {
	v := t.model.View()
	return v.Content
}

func (t *TermWidget) Close() {
	t.model.Close()
}

// WatchSIGWINCH installs a single SIGWINCH handler that resizes both the PTY and
// notifies BubbleTea via the program. Must be called after p.Run() starts.
// Returns a cancel func to stop watching.
func WatchSIGWINCH(p *tea.Program, ptmx *os.File) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ch:
				if ptmx != nil {
					pty.InheritSize(os.Stdin, ptmx)
				}
				ws, err := pty.GetsizeFull(os.Stdin)
				if err == nil {
					p.Send(tea.WindowSizeMsg{Width: int(ws.Cols), Height: int(ws.Rows)})
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
