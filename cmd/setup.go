package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// configureClaudeHooks merges ClaudeWrap hooks into ~/.claude/settings.json using jq.
func configureClaudeHooks() error {
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	if _, err := os.Stat(settings); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(settings), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(settings, []byte("{}"), 0644); err != nil {
			return err
		}
	}

	// Check if hooks already configured
	data, err := os.ReadFile(settings)
	if err != nil {
		return err
	}
	var cfg struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		if _, ok := cfg.Hooks["SessionStart"]; ok {
			return nil // already configured
		}
	}

	jqExpr := `
.hooks.SessionStart = [{"hooks": [{"type": "command", "command": "claudewrap --hook-session-start"}]}]
| .hooks.StopFailure = [{"matcher": "rate_limit", "hooks": [{"type": "command", "command": "claudewrap --hook-rate-limit"}]}]
| .hooks.PreCompact = [{"hooks": [{"type": "command", "command": "claudewrap --hook-pre-compact"}]}]`

	tmp := settings + ".tmp"
	cmd := exec.Command("jq", jqExpr, settings)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, settings)
}

// getWinsize returns (cols, rows) using TIOCGWINSZ.
func getWinsize() ([2]int, error) {
	type winsize struct {
		Rows, Cols, Xpixel, Ypixel uint16
	}
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return [2]int{120, 40}, errno
	}
	if ws.Cols == 0 || ws.Rows == 0 {
		return [2]int{120, 40}, nil
	}
	return [2]int{int(ws.Cols), int(ws.Rows)}, nil
}
