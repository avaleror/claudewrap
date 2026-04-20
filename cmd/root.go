// Package cmd wires together all ClaudeWrap subsystems.
// root.go launches the TUI or dispatches to hook handlers based on CLI flags.
// Hook subcommands (--hook-session-start etc.) are hidden from help output and
// are intended to be called only from Claude Code's hook configuration.
package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/avaleror/claudewrap/internal/compress"
	appctx "github.com/avaleror/claudewrap/internal/context"
	"github.com/avaleror/claudewrap/internal/daemon"
	"github.com/avaleror/claudewrap/internal/fallback"
	"github.com/avaleror/claudewrap/internal/monitor"
	"github.com/avaleror/claudewrap/internal/schedule"
	"github.com/avaleror/claudewrap/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "claudewrap [claude args...]",
	Short: "ClaudeWrap — TUI wrapper for Claude Code with compression and token monitoring",
	Args:  cobra.ArbitraryArgs,
	RunE:  runRoot,
}

var (
	hookSessionStartFlag bool
	hookRateLimitFlag    bool
	hookPreCompactFlag   bool
	resumeFlag           bool
)

func init() {
	rootCmd.Flags().BoolVar(&hookSessionStartFlag, "hook-session-start", false, "")
	rootCmd.Flags().BoolVar(&hookRateLimitFlag, "hook-rate-limit", false, "")
	rootCmd.Flags().BoolVar(&hookPreCompactFlag, "hook-pre-compact", false, "")
	rootCmd.Flags().BoolVar(&resumeFlag, "resume", false, "Resume the most recent Claude session")
	rootCmd.Flags().MarkHidden("hook-session-start")
	rootCmd.Flags().MarkHidden("hook-rate-limit")
	rootCmd.Flags().MarkHidden("hook-pre-compact")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	if hookSessionStartFlag {
		hookSessionStart(cmd, args)
		return nil
	}
	if hookRateLimitFlag {
		hookRateLimit(cmd, args)
		return nil
	}
	if hookPreCompactFlag {
		hookPreCompact(cmd, args)
		return nil
	}

	if isVimEnv() {
		return runPassthrough(args)
	}

	if resumeFlag {
		args = append([]string{"--resume"}, args...)
	}

	var replayQueue []string
	if schedule.HasQueue() {
		replayQueue = offerQueueReplay()
	}

	if err := configureClaudeHooks(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not configure hooks: %v\n", err)
	}

	return runTUI(args, replayQueue)
}

func isVimEnv() bool {
	return os.Getenv("VIM") != "" || os.Getenv("NVIM_LISTEN_ADDRESS") != ""
}

func runPassthrough(args []string) error {
	c := exec.Command("claude", args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runTUI(args []string, replayQueue []string) error {
	termCtx := appctx.Detect()
	gitBranch := tui.GitBranch()

	// Use PID-based socket so hooks can find us via the inherited env var.
	socketPath := daemon.SocketPath(fmt.Sprintf("pid-%d", os.Getpid()))
	os.Setenv("CLAUDEWRAP_SOCKET", socketPath)

	ollamaOK := compress.OllamaAvailable()

	w, h := termSize()
	app, err := tui.NewApp(termCtx, gitBranch, args, w, h, replayQueue, ollamaOK)
	if err != nil {
		return fmt.Errorf("failed to start TUI: %w", err)
	}

	tui.SetCompressFunc(func(text string) tea.Msg {
		r := compress.Compress(text)
		return tui.CompressResult(text, r.Text, r.Engine, r.Skipped)
	})

	tui.SetFallbackFunc(func(text string) tea.Msg {
		result, engine, tokens, err := fallback.Chain(text)
		return tui.FallbackResult(result, engine, tokens, err)
	})

	p := tea.NewProgram(app)

	go listenDaemon(socketPath, p)

	cancel := tui.WatchSIGWINCH(p, nil)
	defer cancel()

	_, err = p.Run()
	return err
}

// listenDaemon accepts connections on the Unix socket and injects messages into p.
func listenDaemon(socketPath string, p *tea.Program) {
	ln, err := daemon.Listen(socketPath)
	if err != nil {
		return
	}
	defer ln.Close()

	state := monitor.NewState()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn, p, state)
	}
}

func handleConn(conn net.Conn, p *tea.Program, state *monitor.State) {
	defer conn.Close()

	var msg daemon.Message
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		return
	}

	// Send ack
	json.NewEncoder(conn).Encode(daemon.Message{Type: daemon.MsgState, State: "ok"})

	switch msg.Type {
	case daemon.MsgSessionStart:
		if msg.SessionID == "" {
			return
		}
		info, err := monitor.ReadSessionInfo(msg.SessionID)
		if err != nil {
			return
		}
		p.Send(tui.SessionStartMsg{
			SessionID:      msg.SessionID,
			TranscriptPath: info.TranscriptPath,
		})
		// Start JSONL watcher
		go watchJSONL(info, p, state)

	case daemon.MsgRateLimit:
		p.Send(tui.RateLimitMsg{})

	case daemon.MsgPreCompact:
		p.Send(tui.PreCompactMsg{})
		state.IncrCompaction()
		// Persist compaction count so Swift menubar can read it
		if info, err := monitor.ReadSessionInfo(msg.SessionID); err == nil {
			info.CompactionCount = state.Snapshot().CompactionCount
			monitor.WriteSessionInfo(*info)
		}
	}
}

func watchJSONL(info *monitor.SessionInfo, p *tea.Program, state *monitor.State) {
	var (
		w   *monitor.Watcher
		err error
	)
	// The JSONL file may not exist yet when SessionStart fires — retry for up to 30s.
	for i := 0; i < 30; i++ {
		w, err = monitor.NewWatcher(info.TranscriptPath, func(e monitor.Entry) {
			state.Update(e)
			snap := state.Snapshot()
			p.Send(tui.StateUpdateMsg{Snap: snap})
		})
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "claudewrap: JSONL watcher: %v\n", err)
		return
	}
	_ = w // watcher's internal goroutine holds the reference; runs for process lifetime
}

func offerQueueReplay() []string {
	q, err := schedule.Load()
	if err != nil || len(q) == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\n%d queued prompt(s) from last session. Replay? [y/N] ", len(q))
	var resp string
	fmt.Scanln(&resp)
	if resp != "y" && resp != "Y" {
		schedule.Clear()
		return nil
	}
	schedule.Clear()
	texts := make([]string, len(q))
	for i, p := range q {
		texts[i] = p.Prompt
	}
	return texts
}

func termSize() (int, int) {
	ws, err := getWinsize()
	if err != nil {
		return 120, 40
	}
	return ws[0], ws[1]
}

