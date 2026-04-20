package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"

	appctx "github.com/avaleror/claudewrap/internal/context"
	"github.com/avaleror/claudewrap/internal/monitor"
	"github.com/avaleror/claudewrap/internal/notify"
	"github.com/avaleror/claudewrap/internal/schedule"
)

const (
	panelWidth    = 30
	statusHeight  = 1
	inputHeight   = 1
	previewHeight = 2
)

// App is the root BubbleTea model for ClaudeWrap.
type App struct {
	term            *TermWidget
	panel           monitor.StateSnapshot
	input           InputModel
	preview         PreviewModel
	state           ClaudeState
	width           int
	height          int
	showBreakdown   bool
	engine          string // compression engine label
	termCtx         appctx.TerminalContext
	gitBranch       string
	lowTokenAlerted bool
	compactArmed    bool     // /compact ready to fire on next idle
	replayQueue     []string // prompts to inject on first StateWaiting
	fallbackLog     string   // accumulated Q&A shown when rate limited
}

// StateUpdateMsg carries fresh token state from the JSONL monitor.
type StateUpdateMsg struct{ Snap monitor.StateSnapshot }

// SessionReadyMsg signals the JSONL watcher is live.
type SessionReadyMsg struct {
	TranscriptPath string
	SessionID      string
}

// CompactArmedMsg means context hit 60% — fire /compact on next idle.
type CompactArmedMsg struct{}

// TickMsg drives idle detection.
type TickMsg struct{}

func NewApp(termCtx appctx.TerminalContext, gitBranch string, claudeArgs []string, w, h int, replayQueue []string) (*App, error) {
	termH := h - statusHeight - inputHeight - previewHeight
	termW := w - panelWidth - 1

	term, err := NewTermWidget(termW, termH, claudeArgs)
	if err != nil {
		return nil, err
	}

	return &App{
		term:        term,
		input:       NewInput(w),
		state:       StateRunning,
		width:       w,
		height:      h,
		termCtx:     termCtx,
		gitBranch:   gitBranch,
		engine:      "passthrough",
		replayQueue: replayQueue,
	}, nil
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.term.Init(),
		tick(),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		termH := a.height - statusHeight - inputHeight - previewHeight
		termW := a.width - panelWidth - 1
		cmds = append(cmds, a.term.Resize(termW, termH))

	case TickMsg:
		// Idle detection
		if a.term.IsIdle() {
			if a.state == StateRunning {
				a.state = StateWaiting
				// Replay next queued prompt on first idle
				if len(a.replayQueue) > 0 {
					text := a.replayQueue[0]
					a.replayQueue = a.replayQueue[1:]
					cmds = append(cmds, compressAsync(text), tick())
					return a, tea.Batch(cmds...)
				}
			}
			// Fire /compact if armed and idle
			if a.compactArmed && a.state == StateWaiting {
				a.compactArmed = false
				a.state = StateCompacting
				cmds = append(cmds, a.term.SendText("/compact"))
				notify.Send("ClaudeWrap", "Auto-compacting context...")
			}
		} else {
			if a.state == StateWaiting {
				a.state = StateRunning
			}
		}
		cmds = append(cmds, tick())

	case RateLimitMsg:
		a.state = StateRateLimit
		a.fallbackLog = "[Rate limited — routing new prompts to fallback chain]\n\n"
		schedule.SaveContextSnapshot(schedule.ContextSnapshot{
			RemainingPct:    a.panel.RemainingPct,
			UsedTokens:      a.panel.UsedTokens,
			TotalTokens:     a.panel.TotalTokens,
			EstimatedReset:  a.panel.EstimatedReset,
			CompactionCount: a.panel.CompactionCount,
		})

	case PreCompactMsg:
		a.panel.CompactionCount++
		a.gitBranch = GitBranch()

	case SessionStartMsg:
		// nothing to do here — JSONL watcher started from daemon handler

	case StateUpdateMsg:
		snap := msg.Snap
		a.panel = snap
		a.engine = "ollama"
		// Arm /compact if context usage >= 60%
		if snap.ContextUsedPct >= 60 && !a.compactArmed && a.state != StateCompacting {
			a.compactArmed = true
			cmds = append(cmds, func() tea.Msg { return CompactArmedMsg{} })
		}
		// Low token warning
		if snap.RemainingPct <= 11 && !a.lowTokenAlerted {
			a.lowTokenAlerted = true
			notify.SendWithActions("ClaudeWrap", fmt.Sprintf("Claude low: %.0f%% remaining. Resumes %s", snap.RemainingPct, snap.ResetIn()))
		}

	case PreviewSendMsg:
		cmds = append(cmds, a.term.SendText(msg.Text))
		a.state = StateRunning

	case SubmitMsg:
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			break
		}
		if a.state == StateRateLimit {
			a.fallbackLog += fmt.Sprintf("You: %s\n\n", text)
			cmds = append(cmds, fallbackAsync(text))
		} else {
			cmds = append(cmds, compressAsync(text))
		}

	case compressResultMsg:
		a.engine = msg.engine
		if msg.skipped || msg.compressed == msg.original {
			// No meaningful compression — send directly
			cmds = append(cmds, a.term.SendText(msg.original))
			a.state = StateRunning
		} else {
			// Show 2s preview
			var cmd tea.Cmd
			a.preview, cmd = a.preview.Show(msg.original, msg.compressed, msg.engine)
			cmds = append(cmds, cmd)
		}

	case fallbackResultMsg:
		if msg.err != nil {
			a.fallbackLog += fmt.Sprintf("[All fallback providers failed: %v]\n\n", msg.err)
		} else {
			a.fallbackLog += fmt.Sprintf("%s:\n%s\n\n", msg.engine, msg.text)
			a.panel.FallbackEngine = msg.engine
			a.panel.FallbackDailyTokens += msg.tokens
			a.panel.FallbackDailyCost += fallbackCostPerToken(msg.engine) * float64(msg.tokens)
		}

	case PreviewTickMsg, PreviewCancelMsg:
		var cmd tea.Cmd
		a.preview, cmd = a.preview.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyPressMsg:
		k := msg.Key()

		// Global quit
		if k.Code == 'c' && k.Mod == tea.ModCtrl {
			a.term.Close()
			return a, tea.Quit
		}

		// 'b' toggles breakdown view
		if k.Text == "b" && (a.state == StateWaiting || a.state == StateRateLimit) && a.preview.State == PreviewHidden {
			a.showBreakdown = !a.showBreakdown
			break
		}

		// Esc in preview
		if a.preview.State == PreviewVisible {
			var cmd tea.Cmd
			a.preview, cmd = a.preview.Update(msg)
			cmds = append(cmds, cmd)
			break
		}

		// Waiting or rate-limited: route keys to our input component
		if a.state == StateWaiting || a.state == StateRateLimit {
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			cmds = append(cmds, cmd)
			break
		}

		// When Claude is running: pass through to PTY (tool confirmations, ctrl+c, etc.)
		var tw *TermWidget
		var cmd tea.Cmd
		tw, cmd = a.term.Update(msg)
		a.term = tw
		cmds = append(cmds, cmd)

	default:
		// Forward all other messages to the terminal widget
		var cmd tea.Cmd
		a.term, cmd = a.term.Update(msg)
		cmds = append(cmds, cmd)

		// Also update preview tick
		if a.preview.State == PreviewVisible {
			var pcmd tea.Cmd
			a.preview, pcmd = a.preview.Update(msg)
			cmds = append(cmds, pcmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) View() tea.View {
	termH := a.height - statusHeight - inputHeight - previewHeight
	termW := a.width - panelWidth - 1

	// Left: terminal output (or fallback log when rate limited)
	termContent := a.term.View()
	if a.state == StateRateLimit && a.fallbackLog != "" {
		termContent = a.fallbackLog
	}

	// Right: token panel
	panelContent := renderTokenPanel(a.panel, panelWidth-2, a.showBreakdown)

	// Compose left+right side by side
	termLines := splitLines(termContent, termH)
	panelLines := splitLines(panelContent, termH)

	bc := borderColor(a.state)
	termStyle := panelStyle.
		BorderForeground(bc).
		Width(termW - 2).
		Height(termH - 2)
	rightStyle := panelStyle.
		BorderForeground(colorNeutral).
		Width(panelWidth - 2).
		Height(termH - 2)

	_ = termLines
	_ = panelLines

	main := lipgloss.JoinHorizontal(lipgloss.Top,
		termStyle.Render(strings.Join(splitLines(termContent, termH-2), "\n")),
		rightStyle.Render(panelContent),
	)

	// Input area
	var inputLine string
	if a.preview.State == PreviewVisible {
		inputLine = a.preview.View(a.width)
	} else if a.state == StateWaiting || a.state == StateRateLimit {
		inputLine = a.input.View()
	} else {
		inputLine = dimStyle.Render("  [Claude is thinking...]")
	}

	// Status bar
	statusLine := a.renderStatusBar()

	full := main + "\n" + inputLine + "\n" + statusLine
	v := tea.NewView(full)
	v.AltScreen = true
	return v
}

func (a *App) renderStatusBar() string {
	parts := []string{}

	engineLabel := a.engine
	if a.panel.FallbackEngine != "" {
		engineLabel = "⚠ " + a.panel.FallbackEngine
	}
	parts = append(parts, engineLabel)

	if a.gitBranch != "" {
		parts = append(parts, "  "+a.gitBranch)
	}

	ctx := "plain"
	switch a.termCtx {
	case appctx.TermCmux:
		ctx = "cmux"
	case appctx.TermGhostty:
		ctx = "ghostty"
	}
	parts = append(parts, "  "+ctx)

	if a.state == StateCompacting {
		parts = append(parts, "  "+lipgloss.NewStyle().Foreground(colorCompact).Render("compacting..."))
	}
	if a.state == StateRateLimit {
		parts = append(parts, "  "+lipgloss.NewStyle().Foreground(colorDanger).Render("rate limited"))
	}

	return statusBarStyle.Render(strings.Join(parts, ""))
}

// splitLines splits a string into at most n lines, padding shorter content.
func splitLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

// compressResultMsg carries the result of async compression.
type compressResultMsg struct {
	original   string
	compressed string
	engine     string
	skipped    bool
}

func compressAsync(text string) tea.Cmd {
	return func() tea.Msg { return runCompress(text) }
}

// fallbackResultMsg carries the result of an AI fallback query.
type fallbackResultMsg struct {
	text   string
	engine string
	tokens int
	err    error
}

func fallbackAsync(text string) tea.Cmd {
	return func() tea.Msg { return runFallback(text) }
}

var runFallback = func(text string) tea.Msg {
	return fallbackResultMsg{err: fmt.Errorf("fallback not configured")}
}

// SetFallbackFunc wires in the real fallback chain from cmd.
func SetFallbackFunc(fn func(string) tea.Msg) {
	runFallback = fn
}

// FallbackResult builds a fallbackResultMsg for use from cmd package.
func FallbackResult(text, engine string, tokens int, err error) tea.Msg {
	return fallbackResultMsg{text: text, engine: engine, tokens: tokens, err: err}
}

// fallbackCostPerToken returns a rough USD/token estimate for each engine.
func fallbackCostPerToken(engine string) float64 {
	switch {
	case strings.Contains(engine, "Grok"):
		return 5.0 / 1_000_000 // ~$5/1M tokens
	case strings.Contains(engine, "Gemini"):
		return 0.15 / 1_000_000 // ~$0.15/1M tokens
	default:
		return 0 // Ollama local
	}
}

// runCompress is the actual compression call (avoids import cycle in cmd).
var runCompress = func(text string) tea.Msg {
	return compressResultMsg{original: text, compressed: text, engine: "passthrough", skipped: true}
}

// SetCompressFunc wires in the real compress pipeline from main.
func SetCompressFunc(fn func(string) tea.Msg) {
	runCompress = fn
}

// GitBranch runs git branch --show-current and returns the result.
func GitBranch() string {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CompressResult builds a compressResultMsg for use from cmd package.
func CompressResult(original, compressed, engine string, skipped bool) tea.Msg {
	return compressResultMsg{
		original:   original,
		compressed: compressed,
		engine:     engine,
		skipped:    skipped,
	}
}

// RateLimitMsg signals that Claude's session is exhausted.
type RateLimitMsg struct{}

// PreCompactMsg signals that a /compact is about to happen.
type PreCompactMsg struct{}

// SessionStartMsg signals that a new session began and the JSONL path is known.
type SessionStartMsg struct {
	SessionID      string
	TranscriptPath string
}
