// Package context detects the runtime environment (terminal emulator, SSH) and
// provides time-of-day helpers used to adjust token reset estimates during
// Anthropic's peak throttling window.
package context

import (
	"os"
	"time"
)

// TerminalContext identifies the host terminal emulator.
type TerminalContext int

const (
	TermPlain   TerminalContext = iota // generic terminal or unknown
	TermGhostty                        // Ghostty — supports OSC 9 notifications
	TermCmux                           // cmux — Ghostty-based, adds sidebar + notification ring
)

// Detect reads TERM_PROGRAM to identify the terminal emulator at startup.
func Detect() TerminalContext {
	switch os.Getenv("TERM_PROGRAM") {
	case "cmux":
		return TermCmux
	case "ghostty":
		return TermGhostty
	default:
		return TermPlain
	}
}

// IsSSH returns true when running inside an SSH session.
// Used to disable macOS-native notifications (osascript, UNUserNotificationCenter).
func IsSSH() bool {
	return os.Getenv("SSH_TTY") != ""
}

// IsPeakHour returns true during 05:00–11:00 Pacific Time.
// Anthropic throttles Claude Pro sessions roughly 1.75× faster during this window,
// so the reset estimate is shortened proportionally.
func IsPeakHour(t time.Time) bool {
	// Fixed -7h offset approximates PDT; off by 1h during PST (Nov–Mar) — acceptable.
	pt := t.UTC().Add(-7 * time.Hour)
	h := pt.Hour()
	return h >= 5 && h < 11
}
