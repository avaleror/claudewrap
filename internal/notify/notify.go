package notify

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Send emits a notification via OSC 9 (cmux/Ghostty), alerter, or osascript.
// OSC 9 is always emitted — silently ignored by terminals that don't support it.
func Send(title, body string) {
	// OSC 9 — works in Ghostty and cmux sidebar
	fmt.Printf("\033]9;%s: %s\033\\", title, body)

	if os.Getenv("SSH_TTY") != "" {
		return
	}

	if isInstalled("alerter") {
		runAlerter(title, body)
	} else {
		runOsascript(title, body)
	}
}

// SendWithActions sends a notification with Schedule/Dismiss action buttons via alerter.
// Falls back to basic notification if alerter is not available.
func SendWithActions(title, body string) {
	fmt.Printf("\033]9;%s: %s\033\\", title, body)

	if os.Getenv("SSH_TTY") != "" {
		return
	}

	if isInstalled("alerter") {
		args := []string{
			"-title", title,
			"-message", body,
			"-actions", "Schedule,Dismiss",
			"-timeout", "30",
		}
		exec.Command("alerter", args...).Start()
	} else {
		runOsascript(title, body)
	}
}

func isInstalled(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func runAlerter(title, body string) {
	exec.Command("alerter", "-title", title, "-message", body, "-timeout", "10").Start()
}

func runOsascript(title, body string) {
	script := fmt.Sprintf(
		`display notification %q with title %q`,
		sanitize(body), sanitize(title),
	)
	exec.Command("osascript", "-e", script).Start()
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
