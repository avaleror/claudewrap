package context

import (
	"os"
	"time"
)

type TerminalContext int

const (
	TermPlain TerminalContext = iota
	TermGhostty
	TermCmux
)

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

func IsSSH() bool {
	return os.Getenv("SSH_TTY") != ""
}

// IsPeakHour returns true during 05:00–11:00 Pacific Time.
// During peak, Anthropic throttles usage ~1.75x faster.
func IsPeakHour(t time.Time) bool {
	// Convert to Pacific Time (UTC-7 PDT / UTC-8 PST)
	pt := t.UTC().Add(-7 * time.Hour) // approximate PDT
	h := pt.Hour()
	return h >= 5 && h < 11
}
