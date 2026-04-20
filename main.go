// ClaudeWrap is a macOS TUI wrapper for Claude Code.
//
// It adds prompt compression via local Ollama, live token monitoring, auto-/compact
// at 60% context usage, and an AI fallback chain (Grok → Gemini → Ollama) when the
// Claude session is rate-limited. A companion Swift menubar app shows token state and
// sends native macOS notifications.
//
// Usage: claudewrap [claude flags...]  (replaces `claude` in your shell)
package main

import "github.com/avaleror/claudewrap/cmd"

func main() {
	cmd.Execute()
}
