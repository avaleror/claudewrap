package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/avaleror/claudewrap/internal/daemon"
	"github.com/avaleror/claudewrap/internal/monitor"
	"github.com/avaleror/claudewrap/internal/notify"
	"github.com/avaleror/claudewrap/internal/schedule"
)

// hookSessionStart handles the SessionStart hook.
// Reads JSON from stdin, writes session info to disk, notifies TUI via socket.
func hookSessionStart(cmd *cobra.Command, args []string) {
	var payload struct {
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		os.Exit(0)
	}

	info := monitor.SessionInfo{
		SessionID:      payload.SessionID,
		TranscriptPath: payload.TranscriptPath,
		StartedAt:      time.Now(),
	}
	monitor.WriteSessionInfo(info)
	daemon.WriteCurrentSessionID(payload.SessionID)

	// Notify running TUI via inherited socket path
	socketPath := hookSocketPath(payload.SessionID)
	daemon.Send(socketPath, daemon.Message{
		Type:      daemon.MsgSessionStart,
		SessionID: payload.SessionID,
	}, 2*time.Second)

	os.Exit(0)
}

// hookRateLimit handles StopFailure with rate_limit matcher.
func hookRateLimit(cmd *cobra.Command, args []string) {
	var payload struct {
		Prompt string `json:"prompt"`
	}
	json.NewDecoder(os.Stdin).Decode(&payload)

	// Save pending prompt if present
	if payload.Prompt != "" {
		schedule.Append(payload.Prompt, false)
	}

	// Notify TUI via inherited socket path
	sessionID := daemon.ReadCurrentSessionID()
	socketPath := hookSocketPath(sessionID)
	daemon.Send(socketPath, daemon.Message{
		Type:      daemon.MsgRateLimit,
		SessionID: sessionID,
	}, 2*time.Second)

	// Notification
	notify.SendWithActions("ClaudeWrap — Rate limited",
		fmt.Sprintf("Session locked. Pending prompts saved."))

	os.Exit(0)
}

// hookPreCompact handles the PreCompact hook.
func hookPreCompact(cmd *cobra.Command, args []string) {
	sessionID := daemon.ReadCurrentSessionID()
	socketPath := hookSocketPath(sessionID)
	daemon.Send(socketPath, daemon.Message{
		Type:      daemon.MsgPreCompact,
		SessionID: sessionID,
	}, 2*time.Second)
	os.Exit(0)
}

// hookSocketPath returns the socket path for hook → TUI communication.
// Prefers CLAUDEWRAP_SOCKET (inherited from TUI process) over session-based fallback.
func hookSocketPath(sessionID string) string {
	if s := os.Getenv("CLAUDEWRAP_SOCKET"); s != "" {
		return s
	}
	return daemon.SocketPath(sessionID)
}
