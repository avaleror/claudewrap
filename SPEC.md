# ClaudeWrap — Implementation Spec

ClaudeWrap has two components: (1) CLI TUI wrapper in Go, (2) Swift menubar companion app.

---

## Component 1: CLI TUI Wrapper (Go)

**Dependencies:** BubbleTea (github.com/charmbracelet/bubbletea), bubbleterm (github.com/taigrr/bubbleterm), creack/pty, cobra, fsnotify, openai-go SDK

**Layout:**
- Left pane: bubbleterm PTY widget running Claude Code transparently
- Right pane: token panel reading ~/.claude/projects/*.jsonl in real-time
- Bottom bar: compression status indicator (Ollama / bypass / pass-through)

### Prompt Compression (local Ollama only)

- Intercept via UserPromptSubmit hook communicating over `~/.claudewrap/daemon.sock`
- Call `http://localhost:11434/v1/chat/completions` with model `claudewrap-compressor`
- No cloud fallback for compression — Ollama runs as a launchd service, always available
- After compression: show 1-line preview at bottom of TUI + 2s countdown timer
  - Esc within 2s = send original prompt
  - After 2s = auto-send compressed prompt
- Show indicator: which engine ran (Ollama / bypass / pass-through)

**Bypass mode (skip compression):**
- Prompt starts with `!!` (two exclamation marks)
- Prompt contains 3+ consecutive code lines (backtick block or consistent indentation)
- Prompt is under 80 characters

### Auto-/compact

- Monitor `context_window.used_percentage` from JSONL on every assistant entry
- When `used_percentage >= 60%`: write `/compact\n` to PTY master FD
- Show TUI status: `Auto-compacting context...`

### Token Monitoring

- `SessionStart` hook stdin payload has `transcript_path` — watch this JSONL file with fsnotify
- Parse `remaining_percentage` and `usage` fields from each `assistant` entry
- Session reset time = first JSONL entry `timestamp` + 5 hours
- When `remaining_percentage <= 11`: red TUI banner + macOS notification
  - Use osascript by default; use alerter (if installed) for actionable Schedule/Dismiss buttons
- Two separate counters:
  1. Claude session tokens (from JSONL `usage` fields)
  2. AI fallback daily cost counter (only when Grok/Gemini used)

**Token panel display:**

```
Claude Session
  Used:      45,230 tokens
  Remaining: 73% [=========-]
  Resets in: 3h 42m

AI Fallback Cost
  Today:     12,400 tokens / $0.002
  Engine:    Grok 4.1 Fast
```

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

All hooks communicate with TUI daemon via Unix socket `~/.claudewrap/daemon.sock`.

### Scheduling on Token Exhaustion

When `StopFailure` hook fires with `rate_limit`:
1. Compute reset time: first JSONL entry timestamp + 5 hours
2. Save pending prompts to `~/.claudewrap/queue.json`: `[{"prompt": "...", "bypass": false, "added_at": "..."}]`
3. Save context snapshot to `~/.claudewrap/context_<timestamp>.json`
4. Send notification: "Claude locked. Resumes at HH:MM." with Schedule/Dismiss buttons
5. On next `claudewrap` run: detect queue file, offer to replay queued prompts

### AI Fallback (when Claude exhausted — NOT for compression)

Route prompts to fallback chain: Grok 4.1 Fast → Gemini 2.5 Flash → local Ollama

- **Grok:** `base_url=https://api.x.ai/v1`, model `grok-4.1-fast`, auth `GROK_API_KEY` env var
- **Gemini:** `base_url=https://generativelanguage.googleapis.com/v1beta/openai/`, model `gemini-2.5-flash`, auth `GEMINI_API_KEY` env var
- Both use openai-go SDK with base_url override
- TUI shows: `⚠ Claude unavailable — routing to Grok`
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

---

## Component 2: Swift Menubar Companion App

~200 lines SwiftUI. Thin app — no PTY, no Claude interaction.

- Persistent menubar icon showing token remaining% (color-coded: green >50%, yellow >11%, red <=11%)
- Click opens popover:
  - Progress bar: tokens used / remaining
  - Reset countdown: "Resets in 3h 42m"
  - AI fallback cost today
  - Active compression engine
- Reads `~/.claude/projects/**/*.jsonl` with FSEvents or polling
- Communicates with CLI wrapper via `~/.claudewrap/daemon.sock`
- Sends native macOS notifications via `UNUserNotificationCenter` with action buttons
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

# 3. Ollama
command -v ollama >/dev/null 2>&1 || curl -fsSL https://ollama.com/install.sh | sh

# 4. Write Modelfile
cat > /tmp/Modelfile << 'EOF'
FROM qwen2.5:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite user prompts to be 40-60% shorter while preserving every instruction, constraint, file name, and technical term exactly. Output ONLY the rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
EOF

# 5-6. Pull and create model (start ollama temporarily if needed)
ollama serve &>/dev/null & OLLAMA_PID=$!
sleep 2
ollama pull qwen2.5:3b
ollama create claudewrap-compressor -f /tmp/Modelfile
kill $OLLAMA_PID 2>/dev/null || true

# 7. Install Ollama as launchd service
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

# 8. Build claudewrap Go binary
go build -o /usr/local/bin/claudewrap ./...

# 9. Build Swift menubar app
cd menubar && swift build -c release
cp -r .build/release/ClaudeWrap.app /Applications/ClaudeWrap.app 2>/dev/null || true
cd ..

# 10. Merge Claude Code hooks into ~/.claude/settings.json
SETTINGS=~/.claude/settings.json
if [ ! -f "$SETTINGS" ]; then echo '{}' > "$SETTINGS"; fi
jq '.hooks.UserPromptSubmit = [{"hooks": [{"type": "command", "command": "claudewrap --hook-prompt"}]}]
  | .hooks.StopFailure = [{"matcher": "rate_limit", "hooks": [{"type": "command", "command": "claudewrap --hook-rate-limit"}]}]
  | .hooks.SessionStart = [{"hooks": [{"type": "command", "command": "claudewrap --hook-session-start"}]}]
  | .hooks.PreCompact = [{"hooks": [{"type": "command", "command": "claudewrap --hook-pre-compact"}]}]' \
  "$SETTINGS" > /tmp/settings_merged.json && mv /tmp/settings_merged.json "$SETTINGS"

# 11. PATH
grep -q 'claudewrap' ~/.zshrc 2>/dev/null || echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.zshrc
grep -q 'claudewrap' ~/.bashrc 2>/dev/null || echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc

# 12-13. Menubar app launchd
cat > ~/Library/LaunchAgents/com.claudewrap.menubar.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.claudewrap.menubar</string>
  <key>ProgramArguments</key><array><string>/Applications/ClaudeWrap.app/Contents/MacOS/ClaudeWrap</string></array>
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
  internal/tui/terminal.go
  internal/tui/tokenpanel.go
  internal/tui/preview.go
  internal/compress/pipeline.go
  internal/compress/ollama.go
  internal/fallback/grok.go
  internal/fallback/gemini.go
  internal/monitor/jsonl.go
  internal/monitor/session.go
  internal/compact/auto.go
  internal/notify/notify.go
  internal/schedule/queue.go
  internal/daemon/socket.go
  menubar/Package.swift
  menubar/Sources/ClaudeWrapMenuBar/ClaudeWrapApp.swift
  menubar/Sources/ClaudeWrapMenuBar/MenuBarView.swift
  menubar/Sources/ClaudeWrapMenuBar/TokenMonitor.swift
  contrib/vim-floaterm.vim
  Modelfile
  install.sh
  go.mod
  README.md
```

---

## Ollama Modelfile (also at root as `Modelfile`)

```
FROM qwen2.5:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite user prompts to be 40-60% shorter while preserving every instruction, constraint, file name, and technical term exactly. Output ONLY the rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
```

---

## Environment Variables

- `GROK_API_KEY` — xAI Grok API key (for AI fallback when Claude exhausted)
- `GEMINI_API_KEY` — Google Gemini API key (secondary AI fallback)
- `OLLAMA_HOST` — optional, defaults to localhost:11434

---

## README

Must cover:
- Prerequisites and install steps (`curl ... | bash install.sh` one-liner)
- Required env vars and how to set them
- Usage: run `claudewrap` instead of `claude`
- Bypass mode: prefix prompt with `!!` to skip compression
- Auto-compact: automatic at 60% context usage
- Vim integration with vim-floaterm
- Optional: Orin Nano LLMLingua-2 microservice for alternative compression
