# ClaudeWrap — Implementation Spec

ClaudeWrap has two components: (1) CLI TUI wrapper in Go, (2) Swift menubar companion app.

---

## Component 1: CLI TUI Wrapper (Go)

**Dependencies:** BubbleTea (github.com/charmbracelet/bubbletea), bubbleterm (github.com/taigrr/bubbleterm), creack/pty, cobra, fsnotify, openai-go SDK, lipgloss

**Layout:**
- Left pane: bubbleterm PTY widget running Claude Code transparently
- Right pane: token panel reading session JSONL in real-time
- Bottom bar: compression status + git branch + terminal context (cmux/ghostty/plain)
- Border color reflects Claude state (see section: Visual State Indicators)

### Terminal Context Detection

Detect at startup and store globally:

```go
type TerminalContext int
const (
    TermPlain TerminalContext = iota
    TermGhostty
    TermCmux
)

func detectTerminalContext() TerminalContext {
    switch os.Getenv("TERM_PROGRAM") {
    case "cmux":    return TermCmux
    case "ghostty": return TermGhostty
    default:        return TermPlain
    }
}
```

Also detect SSH: `os.Getenv("SSH_TTY") != ""` — disable macOS-only features (osascript,
UNUserNotificationCenter) when running over SSH.

### Prompt Compression (local Ollama only)

- Intercept via UserPromptSubmit hook communicating over session-scoped socket (see Daemon Socket)
- Call `http://localhost:11434/v1/chat/completions` with model `claudewrap-compressor`
- No cloud fallback for compression — Ollama runs as a launchd service, always available
- After compression: show 1-line preview at bottom of TUI + 2s countdown timer
  - Esc within 2s = send original prompt
  - After 2s = auto-send compressed prompt
- Show indicator: which engine ran (Ollama / bypass / pass-through)

**CRITICAL — Hook return mechanism:**
The UserPromptSubmit hook binary must:
1. Send prompt to TUI daemon via session-scoped socket (triggers preview + 2s window)
2. Block waiting for daemon response: `{"action": "send", "prompt": "..."}` or `{"action": "cancel"}`
3. Write the result to **stdout** (not socket) — Claude reads the hook's stdout
4. Timeout: if daemon does not respond in 3s, write original prompt to stdout and exit 0
Hook must never hang. Claude Code will block waiting for hook to exit.

**Bypass mode (skip compression):**
- Prompt starts with `!!` (two exclamation marks)
- Prompt contains 3+ consecutive code lines (backtick block or consistent indentation)
- Prompt is under 80 characters

### Visual State Indicators

Use lipgloss border styling on the terminal pane to reflect Claude's current state.
This mirrors cmux's notification ring concept without requiring cmux:

```go
const (
    StateRunning   = "running"    // neutral border (default)
    StateWaiting   = "waiting"    // blue border — Claude needs input
    StateRateLimit = "ratelimit"  // red border — session exhausted
    StateCompact   = "compacting" // yellow border — compaction in progress
)
```

Detect idle/waiting state: no PTY output bytes for 500ms after last assistant chunk.
This same idle detection gates the auto-/compact injection (see Auto-/compact section).

### Auto-/compact

- Monitor `context_window.used_percentage` from JSONL on every assistant entry
- When `used_percentage >= 60%`: wait for idle state (no PTY output for 500ms), then write `/compact\n` to PTY master FD
- **Never inject /compact mid-response** — doing so corrupts the session
- Show TUI status: `Auto-compacting context...` (yellow border during compaction)
- Increment compaction counter per session (tracked in daemon state)
- After 2 compactions: show persistent warning in token panel: "Compacted 2x — quality degrading"
- After 3 compactions: show banner: "Compacted 3x — consider restarting session"

### Token Monitoring

- `SessionStart` hook stdin payload has `transcript_path` — use this path directly (do NOT glob)
  - Real path format: `~/.claude/projects/<url-encoded-project-path>/sessions/<uuid>.jsonl`
  - Watch with fsnotify on the exact file, not a glob pattern
- Parse `remaining_percentage` and `usage` fields from each `assistant` entry
- Parse `context_window.used_percentage` for auto-compact threshold
- Session reset time = first JSONL entry `timestamp` + 5 hours (estimate only — server-side throttling applies)
- Peak hours: 05:00–11:00 Pacific Time. During peak, apply 1.75x multiplier to elapsed usage:
  ```go
  func estimatedResetTime(firstEntryTime time.Time) time.Time {
      if isPeakHour(time.Now()) {
          // Usage drains ~1.75x faster during peak hours
          return firstEntryTime.Add(5*time.Hour / 1.75)
      }
      return firstEntryTime.Add(5 * time.Hour)
  }
  ```
  Always display as "~Xh remaining (est.)" and show "Peak hours — throttle active" during peak
- When `remaining_percentage <= 11`: red TUI banner + notification (see Notifications section)
- Two separate counters:
  1. Claude session tokens (from JSONL `usage` fields)
  2. AI fallback daily cost counter (only when Grok/Gemini used)

**Token panel display:**

```
Claude Session                         [b: breakdown]
  Used:      45,230 tokens
  Remaining: 73% [=========-]
  Resets in: ~3h 42m (est.)            ← "Peak active" badge if applicable

Compaction: 1x                         ← yellow at 2x, red at 3x+

AI Fallback Cost
  Today:     12,400 tokens / $0.002
  Engine:    Grok fast
```

**Breakdown view (toggle with `b`):**

```
Token Breakdown (this turn)
  CLAUDE.md:          8,200
  Tool call I/O:      4,100
  @-mentioned files:  3,400
  Extended thinking:  1,200
  Conversation:      28,000
  Skills:               330
  User text:             20
  ──────────────────────────
  Total used:        45,250
```

Per-turn breakdown is in the JSONL `usage` fields as of 2026 (7 categories). Parse and
display on each assistant entry update.

### Notifications

**Rules:**
- DO NOT use the `Notification` lifecycle hook — it fires on every response and causes alert fatigue
- Emit notifications only on meaningful events: token warning (<=11%), rate limit, compaction warning

**Notification strategy by terminal context:**

```go
func notify(title, body string) {
    // Always emit OSC 9 — silently ignored in terminals that don't support it
    // Shows notification in cmux sidebar + notification panel automatically
    fmt.Printf("\033]9;%s: %s\033\\", title, body)

    if os.Getenv("SSH_TTY") != "" {
        return // SSH — no macOS-native notifications
    }

    if isAlertInstalled() {
        // alerter supports action buttons (Schedule / Dismiss)
        runAlerter(title, body)
    } else {
        runOsascript(title, body)
    }
}
```

Using `alerter` (if installed) enables actionable "Schedule / Dismiss" buttons on the
rate-limit notification. Install check: `command -v alerter`.

### Claude Code Hooks

Auto-merge into `~/.claude/settings.json` on first run using `jq` (never overwrite existing settings):

```json
{
  "hooks": {
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "claudewrap --hook-prompt"}]}],
    "StopFailure": [{"matcher": "rate_limit", "hooks": [{"type": "command", "command": "claudewrap --hook-rate-limit"}]}],
    "SessionStart": [{"hooks": [{"type": "command", "command": "claudewrap --hook-session-start"}]}],
    "PreCompact": [{"hooks": [{"type": "command", "command": "claudewrap --hook-pre-compact"}]}]
  }
}
```

**Note on StopFailure matcher:** Verify exact match string against a rate-limited session at
first implementation — may be `"rate_limit"`, `"rate_limit_error"`, or `"rate limit"`.

All hooks communicate with TUI daemon via **session-scoped** Unix socket.

### Daemon Socket

Single flat socket path conflicts when multiple claudewrap instances run simultaneously
(cmux claude-teams, git worktrees). Use session-scoped sockets:

```go
// Session ID from SessionStart hook payload
socketPath := fmt.Sprintf("%s/.claudewrap/daemon-%s.sock", os.Getenv("HOME"), sessionID)
```

`SessionStart` hook extracts `session_id` from stdin JSON and stores it in
`~/.claudewrap/current-session` so the TUI knows which socket to listen on.

If session ID is unavailable at startup (TUI launched before first session), fall back to
`daemon-default.sock` and migrate when SessionStart fires.

### Scheduling on Token Exhaustion

When `StopFailure` hook fires with `rate_limit`:
1. Compute reset time using peak-aware `estimatedResetTime()`
2. Save pending prompts to `~/.claudewrap/queue.json`: `[{"prompt": "...", "bypass": false, "added_at": "..."}]`
3. Save context snapshot to `~/.claudewrap/context_<timestamp>.json`
4. Send notification: "Claude locked. Resumes ~HH:MM (est.)." with Schedule/Dismiss buttons
5. Set terminal pane border to red (StateRateLimit)
6. On next `claudewrap` run: detect queue file, offer to replay queued prompts

### AI Fallback (when Claude exhausted — NOT for compression)

Route prompts to fallback chain: Grok (fast) → Gemini (flash) → local Ollama

- **Grok:** `base_url=https://api.x.ai/v1`, auth `GROK_API_KEY` env var
- **Gemini:** `base_url=https://generativelanguage.googleapis.com/v1beta/openai/`, auth `GEMINI_API_KEY` env var
- Both use openai-go SDK with base_url override
- **Verify model IDs at implementation time** — use latest available fast/flash models
- TUI shows: `⚠ Claude unavailable — routing to Grok` + yellow warning in bottom bar
- Track fallback tokens/cost in separate daily counter

### Vim Integration

```go
if os.Getenv("VIM") != "" || os.Getenv("NVIM_LISTEN_ADDRESS") != "" {
    runPassthroughMode() // transparent stdin/stdout, no TUI
} else {
    runTUIMode()
}
```

Include `contrib/vim-floaterm.vim`:

```vim
let g:floaterm_width = 0.9
let g:floaterm_height = 0.9
noremap <silent> <leader>cc :FloatermNew --title=ClaudeWrap claudewrap<CR>
```

### SIGWINCH Handling

BubbleTea and creack/pty both attempt to handle SIGWINCH. Register a single handler manually
and disable BubbleTea's default SIGWINCH listener:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGWINCH)
go func() {
    for range sigCh {
        if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
            log.Printf("resize: %v", err)
        }
        // Send window size to BubbleTea program
        cols, rows := getTermSize()
        p.Send(tea.WindowSizeMsg{Width: cols, Height: rows})
    }
}()
```

Do not rely on BubbleTea's built-in SIGWINCH — override it.

### Git Branch Display

Run `git branch --show-current` at startup and on `SessionStart`. Display in bottom bar.
Refresh on every `PreCompact` hook.

---

## Component 2: Swift Menubar Companion App

~200 lines SwiftUI. Thin app — no PTY, no Claude interaction.

**Build note:** `swift build` does NOT produce a `.app` bundle. Ship as a raw executable
managed by launchd. launchd runs it directly from `.build/release/ClaudeWrap`. No Xcode
dependency needed.

- Persistent menubar icon showing token remaining% (color-coded: green >50%, yellow >11%, red <=11%)
- Click opens popover:
  - Progress bar: tokens used / remaining
  - Reset countdown: "~Xh remaining (est.)" — show "Peak active" badge during peak hours (05:00–11:00 PT)
  - Compaction count and quality warning
  - AI fallback cost today
  - Active compression engine
- **Multi-session aggregation:** watches ALL active JSONL files under `~/.claude/projects/**/sessions/*.jsonl`
  - Show total tokens used across all active sessions today (active = modified in last 5h)
  - Show count of active sessions in menubar icon tooltip
- Communicates with CLI wrapper via session-scoped sockets `~/.claudewrap/daemon-*.sock`
- Sends native macOS notifications via `UNUserNotificationCenter` with action buttons
- Detects SSH sessions (reads `~/.claudewrap/current-session` metadata) and skips notifications for those
- Starts on login via launchd plist
- **Include `Package.swift`** so it compiles with `swift build`

---

## install.sh (fully automated, idempotent)

```bash
#!/bin/bash
set -e

# 1. Homebrew
command -v brew >/dev/null 2>&1 || /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 2. Go + jq
brew list go >/dev/null 2>&1 || brew install go
brew list jq >/dev/null 2>&1 || brew install jq

# 3. alerter (optional — enables actionable notifications with Schedule/Dismiss buttons)
brew list alerter >/dev/null 2>&1 || brew install alerter || true

# 4. Ollama
command -v ollama >/dev/null 2>&1 || curl -fsSL https://ollama.com/install.sh | sh

# 5. Write Modelfile — uses qwen2.5-coder:3b (code-aware, same size as 3b general)
cat > /tmp/Modelfile << 'EOF'
FROM qwen2.5-coder:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite user prompts to be 40-60% shorter while preserving every instruction, constraint, file name, and technical term exactly. Output ONLY the rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
EOF

# 6-7. Pull and create model
ollama serve &>/dev/null & OLLAMA_PID=$!
sleep 2
ollama pull qwen2.5-coder:3b
ollama create claudewrap-compressor -f /tmp/Modelfile
kill $OLLAMA_PID 2>/dev/null || true

# 8. Install Ollama as launchd service
cat > ~/Library/LaunchAgents/com.ollama.ollama.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.ollama.ollama</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/ollama</string><string>serve</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/ollama.log</string>
  <key>StandardErrorPath</key><string>/tmp/ollama.err</string>
</dict>
</plist>
EOF
launchctl unload ~/Library/LaunchAgents/com.ollama.ollama.plist 2>/dev/null || true
launchctl load ~/Library/LaunchAgents/com.ollama.ollama.plist

# 9. Build claudewrap Go binary
go build -o /usr/local/bin/claudewrap ./...

# 10. Build Swift menubar app (raw executable, no .app bundle needed)
cd menubar && swift build -c release
cp .build/release/ClaudeWrap /usr/local/bin/claudewrap-menubar
cd ..

# 11. Merge Claude Code hooks into ~/.claude/settings.json
SETTINGS=~/.claude/settings.json
if [ ! -f "$SETTINGS" ]; then echo '{}' > "$SETTINGS"; fi
jq '.hooks.UserPromptSubmit = [{"hooks": [{"type": "command", "command": "claudewrap --hook-prompt"}]}]
  | .hooks.StopFailure = [{"matcher": "rate_limit", "hooks": [{"type": "command", "command": "claudewrap --hook-rate-limit"}]}]
  | .hooks.SessionStart = [{"hooks": [{"type": "command", "command": "claudewrap --hook-session-start"}]}]
  | .hooks.PreCompact = [{"hooks": [{"type": "command", "command": "claudewrap --hook-pre-compact"}]}]' \
  "$SETTINGS" > /tmp/settings_merged.json && mv /tmp/settings_merged.json "$SETTINGS"

# 12. PATH
grep -q '/usr/local/bin' ~/.zshrc 2>/dev/null || echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.zshrc
grep -q '/usr/local/bin' ~/.bashrc 2>/dev/null || echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc

# 13. Menubar app launchd (raw executable, not .app bundle)
cat > ~/Library/LaunchAgents/com.claudewrap.menubar.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.claudewrap.menubar</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/claudewrap-menubar</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
</dict>
</plist>
EOF
launchctl unload ~/Library/LaunchAgents/com.claudewrap.menubar.plist 2>/dev/null || true
launchctl load ~/Library/LaunchAgents/com.claudewrap.menubar.plist

echo ""
echo "ClaudeWrap installed successfully."
echo "Set environment variables: GROK_API_KEY and GEMINI_API_KEY"
echo "Run: claudewrap (instead of claude)"
```

---

## Project Structure

```
claudewrap/
  main.go
  cmd/root.go
  cmd/hooks.go
  internal/tui/app.go
  internal/tui/terminal.go           # bubbleterm PTY widget + SIGWINCH handler
  internal/tui/tokenpanel.go         # token panel + breakdown view (toggle b)
  internal/tui/preview.go            # 2s compression preview + cancel window
  internal/tui/state.go              # visual state (border colors, StateRunning etc.)
  internal/compress/pipeline.go
  internal/compress/ollama.go
  internal/fallback/grok.go
  internal/fallback/gemini.go
  internal/monitor/jsonl.go          # reads transcript_path directly (no glob)
  internal/monitor/session.go        # peak-hour aware reset estimate
  internal/monitor/breakdown.go      # per-category token breakdown parsing
  internal/compact/auto.go           # /compact injection with idle-state guard
  internal/compact/counter.go        # compaction count + quality warnings
  internal/notify/notify.go          # OSC 9 emit + osascript/alerter + SSH detection
  internal/schedule/queue.go
  internal/daemon/socket.go          # session-scoped socket path
  internal/context/terminal.go       # detectTerminalContext(), isPeakHour()
  menubar/Package.swift
  menubar/Sources/ClaudeWrapMenuBar/ClaudeWrapApp.swift
  menubar/Sources/ClaudeWrapMenuBar/MenuBarView.swift
  menubar/Sources/ClaudeWrapMenuBar/TokenMonitor.swift   # multi-session aggregation
  contrib/vim-floaterm.vim
  Modelfile
  install.sh
  go.mod
  README.md
```

---

## Ollama Modelfile (also at root as `Modelfile`)

```
FROM qwen2.5-coder:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite user prompts to be 40-60% shorter while preserving every instruction, constraint, file name, and technical term exactly. Output ONLY the rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
```

Uses `qwen2.5-coder:3b` — code-aware model handles code-adjacent prompts better while
staying the same size (~2GB) as the general qwen2.5:3b.

---

## Environment Variables

- `GROK_API_KEY` — xAI Grok API key (for AI fallback when Claude exhausted)
- `GEMINI_API_KEY` — Google Gemini API key (secondary AI fallback)
- `OLLAMA_HOST` — optional, defaults to localhost:11434
- `TERM_PROGRAM` — read at startup to detect cmux/ghostty (set automatically by those terminals)
- `SSH_TTY` — read at startup to detect SSH sessions (disables macOS-native notifications)

---

## README

Must cover:
- Prerequisites and install steps (`curl ... | bash install.sh` one-liner)
- Required env vars and how to set them
- Usage: run `claudewrap` instead of `claude`
- Bypass mode: prefix prompt with `!!` to skip compression
- Auto-compact: fires at 60% with idle-state guard; quality warnings at 2+/3+ compactions
- Visual state indicators: border colors and what they mean
- cmux/Ghostty users: notification ring integration works automatically via OSC 9
- Token panel breakdown view: press `b` to toggle per-category breakdown
- Peak hours: why reset estimates show "est." and what "Peak active" means
- Vim integration with vim-floaterm
- Multi-agent / worktrees: each session gets its own socket — no conflicts

---

## Implementation Notes (Pre-Build Checklist)

Verify before writing first line of code:

- [ ] Exact StopFailure `rate_limit` matcher string — test against a rate-limited session or check Claude Code source
- [ ] Current Grok fast model ID at `api.x.ai/v1/models`
- [ ] Current Gemini flash model ID at `generativelanguage.googleapis.com`
- [ ] `qwen2.5-coder:3b` available on Ollama registry: `ollama show qwen2.5-coder:3b`
- [ ] UserPromptSubmit hook stdout format — confirm JSON schema for returning modified prompt
- [ ] PreCompact hook payload format — confirm what fields are in stdin JSON
- [ ] JSONL `usage` per-category breakdown field names — confirm against live session transcript
