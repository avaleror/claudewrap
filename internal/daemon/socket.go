// Package daemon manages the Unix socket used for hook → TUI communication.
//
// Each claudewrap process listens on a PID-based socket and exports its path via
// the CLAUDEWRAP_SOCKET environment variable. Claude Code hook binaries (spawned as
// subprocesses) inherit that variable and use it to send events back to the running TUI
// without needing to know the session ID in advance.
//
// Socket path scheme: ~/.claudewrap/daemon-pid-<pid>.sock
package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// MsgType identifies the event being sent over the socket.
type MsgType string

const (
	MsgPrompt       MsgType = "prompt"        // reserved for future use
	MsgCompressed   MsgType = "compressed"    // reserved for future use
	MsgCancel       MsgType = "cancel"        // reserved for future use
	MsgSessionStart MsgType = "session_start" // Claude session started; payload has session_id
	MsgRateLimit    MsgType = "rate_limit"    // Claude session exhausted
	MsgPreCompact   MsgType = "pre_compact"   // /compact is about to run
	MsgState        MsgType = "state"         // generic state acknowledgement
)

// Message is the JSON envelope exchanged over the Unix socket.
type Message struct {
	Type      MsgType `json:"type"`
	SessionID string  `json:"session_id,omitempty"`
	Prompt    string  `json:"prompt,omitempty"`
	State     string  `json:"state,omitempty"`
}

// SocketPath returns the Unix socket path for the given session identifier.
// If sessionID is empty it falls back to "default".
func SocketPath(sessionID string) string {
	home := os.Getenv("HOME")
	if sessionID == "" {
		sessionID = "default"
	}
	return filepath.Join(home, ".claudewrap", fmt.Sprintf("daemon-%s.sock", sessionID))
}

// CurrentSessionPath returns the file that stores the most-recently-started session ID.
// Written by the SessionStart hook; read by StopFailure / PreCompact hooks when
// CLAUDEWRAP_SOCKET is not set (legacy / out-of-process fallback).
func CurrentSessionPath() string {
	return filepath.Join(os.Getenv("HOME"), ".claudewrap", "current-session")
}

// ReadCurrentSessionID reads the current session ID written by the SessionStart hook.
// Returns an empty string if the file does not exist.
func ReadCurrentSessionID() string {
	data, err := os.ReadFile(CurrentSessionPath())
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteCurrentSessionID persists the session ID so subsequent hook calls can look it up.
func WriteCurrentSessionID(id string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".claudewrap")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(CurrentSessionPath(), []byte(id), 0644)
}

// Send delivers msg to the TUI daemon over a Unix socket with the given timeout.
// It waits for a single acknowledgement response before returning.
// Errors are non-fatal — if the TUI is not running the hook should still exit 0.
func Send(socketPath string, msg Message, timeout time.Duration) (*Message, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return nil, err
	}

	var resp Message
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Listen creates the Unix socket directory, removes any stale socket file, and
// returns a net.Listener. Called once at TUI startup.
func Listen(socketPath string) (net.Listener, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	os.Remove(socketPath) // remove stale socket from a previous run
	return net.Listen("unix", socketPath)
}
