package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type MsgType string

const (
	MsgPrompt     MsgType = "prompt"
	MsgCompressed MsgType = "compressed"
	MsgCancel     MsgType = "cancel"
	MsgSessionStart MsgType = "session_start"
	MsgRateLimit  MsgType = "rate_limit"
	MsgPreCompact MsgType = "pre_compact"
	MsgState      MsgType = "state"
)

type Message struct {
	Type      MsgType `json:"type"`
	SessionID string  `json:"session_id,omitempty"`
	Prompt    string  `json:"prompt,omitempty"`
	State     string  `json:"state,omitempty"`
}

func SocketPath(sessionID string) string {
	home := os.Getenv("HOME")
	if sessionID == "" {
		sessionID = "default"
	}
	return filepath.Join(home, ".claudewrap", fmt.Sprintf("daemon-%s.sock", sessionID))
}

func CurrentSessionPath() string {
	return filepath.Join(os.Getenv("HOME"), ".claudewrap", "current-session")
}

func ReadCurrentSessionID() string {
	data, err := os.ReadFile(CurrentSessionPath())
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteCurrentSessionID(id string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".claudewrap")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(CurrentSessionPath(), []byte(id), 0644)
}

// Send sends a message to the daemon socket and returns a response if expected.
// Used by hook binaries to communicate with the running TUI.
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

// Listen creates a Unix domain socket and returns a listener.
func Listen(socketPath string) (net.Listener, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	os.Remove(socketPath)
	return net.Listen("unix", socketPath)
}
