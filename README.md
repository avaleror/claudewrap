# ClaudeWrap

A macOS TUI wrapper for [Claude Code](https://claude.ai/code) that adds prompt compression, paste filtering, live token monitoring, auto-compaction, and an AI fallback chain — without changing how you use Claude.

Run `claudewrap` instead of `claude`. Everything else works the same.

---

## What it does

| Feature | How |
|---|---|
| **Prompt compression** | Rewrites prompts 40–60% shorter via local Ollama before they reach Claude |
| **Paste filter** | Detects pasted content (logs, code, docs, JSON) and extracts only what Claude needs to act on |
| **Live token panel** | Right sidebar shows usage %, reset countdown, cache hits, and per-category breakdown |
| **Auto-compact** | Injects `/compact` at 30% context usage — never mid-response |
| **AI fallback chain** | When rate-limited, routes prompts to Grok → Gemini → local Ollama with exponential backoff |
| **Session queue** | Saves prompts on rate limit; replays them after reset |
| **Operation log** | Every compress, filter, and fallback call is logged to `~/.claudewrap/logs/` as JSONL |
| **Native notifications** | OSC 9 (cmux/Ghostty), alerter (actionable), or osascript |
| **Menubar app** | Persistent token % in the macOS menubar, aggregates all active sessions |

---

## Requirements

- macOS 14+
- Go 1.26+
- Swift 6+ (Xcode or Swift toolchain, for the menubar app)
- [Ollama](https://ollama.com) (for compression and paste filter — optional but recommended)
- Neovim (installed automatically via Homebrew if missing)
- `jq` (for hook auto-install)
- `alerter` (optional — enables actionable notification buttons)

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/avaleror/claudewrap/main/install.sh | bash
```

The install script:
1. Installs Go, jq, Neovim, alerter, and Ollama via Homebrew
2. Pulls `qwen2.5-coder:3b` and creates the `claudewrap-compressor` model
3. Registers Ollama and the menubar app as launchd services
4. Builds and places `claudewrap` + `claudewrap-menubar` in `/usr/local/bin`
5. Auto-configures Claude Code hooks in `~/.claude/settings.json`

### Manual build

```bash
git clone https://github.com/avaleror/claudewrap
cd claudewrap
go build -o /usr/local/bin/claudewrap .

cd menubar
swift build -c release
cp .build/release/ClaudeWrapMenuBar /usr/local/bin/claudewrap-menubar
```

---

## API keys (for AI fallback)

Add to `~/.zshrc`:

```bash
export GROK_API_KEY=your-xai-key       # tier 1 fallback
export GEMINI_API_KEY=your-google-key  # tier 2 fallback
```

If neither key is set, fallback routes to local Ollama instead.

---

## Usage

```bash
claudewrap            # start in current directory
claudewrap --resume   # resume most recent session
claudewrap --help     # pass-through to claude --help
```

Any flag not recognized by claudewrap is forwarded to `claude` directly.

---

## TUI layout

```
┌──────────────────────────────────────┬──────────────────────────────┐
│                                      │  Claude Session  [b: brkdwn] │
│                                      │    Used:  45,230 tokens      │
│   Claude Code PTY                    │    Rem:   [=======-    ]      │
│                                      │    Cache: ↓12,400 / 800 new  │
│   (full terminal output)             │    Reset: ~3h 42m (est.)     │
│                                      │           Peak active        │
│                                      │    Compacted: 1x             │
│                                      │                              │
│                                      │  AI Fallback                 │
│                                      │    Today: 4,100 / $0.0002    │
│                                      │    Engine: Grok fast         │
├──────────────────────────────────────┴──────────────────────────────┤
│  > your prompt here█                                                 │
├──────────────────────────────────────────────────────────────────────┤
│  ollama   main  ghostty  [b: breakdown] [Ctrl+K: compact] [!! bypass]│
└──────────────────────────────────────────────────────────────────────┘
```

### Border colors

| Color | State | Meaning |
|---|---|---|
| Neutral grey | Running | Claude is processing |
| Blue | Waiting | Idle — ready for your input |
| Yellow | Compacting | `/compact` injection in progress |
| Red | Rate limited | Session exhausted — fallback active |

---

## Keyboard shortcuts

| Key | Action |
|---|---|
| `Enter` | Submit prompt (runs through compression) |
| `!!` prefix | Bypass compression and paste filter — send as-is |
| `Esc` | During preview: send original instead of compressed/filtered |
| `↑` / `↓` | Navigate prompt history (last 100 entries) |
| `b` | Toggle per-category token breakdown in panel |
| `Ctrl+K` | Manual `/compact` (only when idle) |
| `Ctrl+U` | Clear current input line |
| `Ctrl+W` | Delete word backward |
| `Ctrl+C` | Quit ClaudeWrap |

---

## Prompt compression

ClaudeWrap intercepts every prompt at Enter and rewrites it with a local Ollama model before sending it to Claude. Typical savings: 40–60% fewer tokens.

### When compression is skipped

- Prompt starts with `!!` (explicit bypass)
- Prompt is shorter than 80 characters
- Prompt contains 3+ consecutive code lines (fenced block or 4-space indent)

### Preview window

After compression or paste filtering, a 2-second preview appears:

```
  Compressed -52%: refactor auth middleware to use session tokens per compliance req...
  Sending in 1.8s...  [Esc: send original]  [ollama]
```

Paste filter previews use a teal background to distinguish them from compression. Press `Esc` within 2 seconds to send the original instead.

### Ollama model

The compressor uses a custom Modelfile wrapping `qwen2.5-coder:3b`:

```
FROM qwen2.5-coder:3b
SYSTEM You are a prompt compression engine. Rewrite prompts 40-60% shorter,
       preserving every instruction, constraint, file name, and technical term.
       Output ONLY the rewritten prompt.
PARAMETER temperature 0.1
```

If Ollama is not running, compression falls back to passthrough silently.

---

## Paste filter

When you paste content into the input box, ClaudeWrap detects it via bracketed paste mode and routes it through a dedicated filter (Ollama `qwen2.5:3b`) before sending to Claude. The filter extracts only what Claude needs to act on, removing noise based on content type:

| Content | What gets kept | What gets removed |
|---|---|---|
| Error / log | Error message, stack trace, key context | Timestamps, repeated lines, verbose debug output |
| Code | Full content if < 200 lines; key functions + summary comments if longer | Large filler blocks |
| Docs / text | Requirements, constraints, key facts | Redundant examples, filler |
| JSON / YAML | Structure + 3 example records | Remaining repeated records (noted with count) |

If you paste something already concise (< 80 characters), it bypasses the filter. Prefix with `!!` to skip entirely. Any text already typed in the input box is prepended to the paste before filtering.

### Secret scrubbing

Before any text reaches local Ollama (for either compression or paste filtering), ClaudeWrap scans for and redacts:
- Assignment patterns: `API_KEY=...`, `TOKEN=...`, `SECRET=...`
- Bearer / Authorization header values
- Known key prefixes: `sk-ant-`, `sk-`, `xai-`, `AIza`
- AWS access key patterns (`AKIA...`, `ASIA...`)

Redaction is noted in the operation log and in the `Redacted` field of the result.

---

## Token panel

The right sidebar updates live as Claude responds.

### Standard view

```
Claude Session        [b: breakdown]
  Used:  45,230 tokens
  Rem:   [=========-        ]
  Cache: ↓12,400 read / 800 new
  Reset: ~3h 42m (est.)
         Peak active

  Compacted: 2x         ← yellow warning at 2x
  Compacted 2x — quality degrading
```

### Breakdown view (press `b`)

```
Token Breakdown
  CLAUDE.md        8,200
  Tool call I/O    4,100
  @-files          3,400
  Thinking         1,200
  Conversation    28,000
  Skills             330
  Team               450
  User text           20
  ──────────────────────
  Total           45,700
```

### Reset estimate

ClaudeWrap estimates session reset as `first_entry_time + 5h`. During **peak hours** (05:00–11:00 Pacific) Anthropic throttles usage ~1.75x faster, so the estimate is shortened to `first_entry_time + 2h 51m`. Always shown as "est." — the actual reset is server-side.

---

## Auto-compact

When `context_window.used_percentage >= 30%`, ClaudeWrap waits for Claude to become idle (no PTY output for 500ms), then injects `/compact`.

Research shows model quality starts degrading at 20–40% context fill, so compacting at 30% keeps quality high throughout the session.

Rules:
- Never fires mid-response
- Manual trigger: `Ctrl+K` (only available when idle)
- Compaction counter shown in token panel
- Yellow warning at 2 compactions; red "restart session" warning at 3+

---

## AI fallback chain

When the Claude session hits a rate limit, ClaudeWrap switches to fallback mode. Your input box stays active; prompts are routed through:

```
Grok (grok-4-1-fast)  →  Gemini (gemini-2.5-flash)  →  local Ollama
```

Each cloud provider gets 3 attempts with exponential backoff (1 s, 2 s, 4 s) before falling through to the next. The terminal pane shows the Q&A log from fallback providers. The status bar shows `⚠ rate limited` in red. Daily token count and cost are tracked separately in the panel.

On the next `claudewrap` start, if a queue file exists you will be asked whether to replay the queued prompts.

---

## Operation log

Every compression, paste filter, and fallback operation is logged to `~/.claudewrap/logs/claudewrap-YYYY-MM-DD.jsonl`. One file per day.

Example entry:

```json
{"timestamp":"2026-04-30T18:45:12Z","operation":"compress","engine":"ollama","tokens_in":312,"tokens_out":141,"ratio":0.55,"duration_ms":820,"success":true}
{"timestamp":"2026-04-30T18:46:03Z","operation":"filter","engine":"filter","tokens_in":2104,"tokens_out":390,"ratio":0.81,"duration_ms":1340,"success":true,"redacted":true}
{"timestamp":"2026-04-30T19:01:22Z","operation":"fallback","engine":"Grok fast","tokens_out":540,"success":true}
```

Fields: `timestamp`, `operation`, `engine`, `tokens_in`, `tokens_out`, `ratio`, `duration_ms`, `success`, `skipped`, `cache_hit`, `redacted`, `error`.

---

## Notifications

| Event | Notification |
|---|---|
| Claude finishes responding | "ClaudeWrap — Ready" (silent after startup) |
| Tokens <= 11% | "Claude low: X% remaining. Resumes ~HH:MM" |
| Rate limit | "Session locked. Pending prompts saved." |
| Auto-compact | "Auto-compacting context..." |

Notifications use: OSC 9 (always, for cmux/Ghostty), then `alerter` if installed (gives Schedule/Dismiss buttons), then `osascript`. Disabled when running over SSH.

---

## Menubar app

`claudewrap-menubar` is a lightweight SwiftUI app that runs as a launchd service.

- Shows token % in the menubar: `CC 73%` (green), `CC 31%` (orange), `CC⚠` (red, <=11%)
- Tooltip shows active session count
- Click to open popover with full panel view
- Polls JSONL files every 2 seconds — no socket dependency
- Aggregates across all active sessions (modified in last 5h)

---

## cmux / Ghostty integration

No setup needed. ClaudeWrap always emits OSC 9 escape sequences:

```
\033]9;ClaudeWrap: Ready\033\\
```

cmux picks these up as sidebar notifications automatically. In Ghostty they appear as native notifications.

---

## Neovim

When `$NVIM` or `$NVIM_LISTEN_ADDRESS` is set, ClaudeWrap runs in passthrough mode (transparent stdio, no TUI). Use with [toggleterm.nvim](https://github.com/akinsho/toggleterm.nvim):

```lua
-- contrib/neovim.lua
local Terminal = require("toggleterm.terminal").Terminal

local claudewrap = Terminal:new({
  cmd = "claudewrap",
  direction = "float",
  float_opts = { border = "curved", width = math.floor(vim.o.columns * 0.9), height = math.floor(vim.o.lines * 0.9) },
  on_open = function() vim.cmd("startinsert!") end,
})

vim.keymap.set("n", "<leader>cc", function() claudewrap:toggle() end,
  { desc = "Toggle ClaudeWrap", noremap = true, silent = true })
```

Copy the full config from `contrib/neovim.lua`. vim-floaterm also works — see the comment at the bottom of that file.

---

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `GROK_API_KEY` | — | xAI Grok (AI fallback tier 1) |
| `GEMINI_API_KEY` | — | Google Gemini (AI fallback tier 2) |
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama endpoint |
| `CLAUDEWRAP_SOCKET` | set automatically | TUI socket path — inherited by hook subprocesses |
| `TERM_PROGRAM` | set by terminal | Detected at startup (`cmux`, `ghostty`, or plain) |
| `SSH_TTY` | set by SSH | Disables macOS-native notifications when set |

---

## File layout

```
~/.claudewrap/
  daemon-pid-<pid>.sock      # TUI Unix socket (PID-scoped, per process)
  current-session            # session ID of the most recently started session
  sessions/<id>.json         # session info written by SessionStart hook
  queue.json                 # pending prompts saved on rate limit
  context_<ts>.json          # token snapshot at time of rate limit
  compress-cache.json        # dedup cache: SHA-256 → compressed result
  logs/
    claudewrap-YYYY-MM-DD.jsonl  # structured operation log (one file per day)

~/.claude/
  settings.json              # Claude Code config — hooks auto-merged here
  projects/<path>/
    sessions/<uuid>.jsonl    # live session transcript (JSONL, append-only)
```

---

## Architecture overview

See [docs/architecture.md](docs/architecture.md) for detailed diagrams.

```
┌─────────────────────────────────────────────────────────────────────┐
│  claudewrap (Go CLI)                                                 │
│                                                                      │
│  ┌──────────┐  PTY   ┌────────────────┐  hooks  ┌────────────────┐  │
│  │  User    │───────▶│  claude (PTY)  │────────▶│  hook binary   │  │
│  │  input   │        └────────────────┘         │  (claudewrap   │  │
│  └──────────┘               │                   │  --hook-*)     │  │
│       │                JSONL file               └───────┬────────┘  │
│       │                     │                           │            │
│  ┌────▼──────┐    ┌─────────▼──────┐          Unix socket           │
│  │  Ollama   │    │  JSONL Watcher │         (CLAUDEWRAP_SOCKET)     │
│  │ compress  │    │  (fsnotify)    │                   │            │
│  │  + filter │    └────────┬───────┘          ┌────────▼────────┐  │
│  └────┬──────┘             │                   │  listenDaemon   │  │
│       │                    │                   └────────┬────────┘  │
│  ┌────▼────────────────────▼───────────────────▼──────────────────┐  │
│  │                     BubbleTea App                               │  │
│  │  TermWidget │ InputModel │ PreviewModel │ Token Panel            │  │
│  └────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────┐
│  claudewrap-menubar (Swift) │
│  Reads JSONL directly       │
│  Polls every 2s             │
│  Shows % in menubar         │
└─────────────────────────────┘
```

---

## Project structure

```
claudewrap/
├── main.go
├── CHANGELOG.md
├── cmd/
│   ├── root.go          # CLI entry point, TUI launch, queue replay, logger init
│   ├── hooks.go         # SessionStart / RateLimit / PreCompact handlers
│   └── setup.go         # hook auto-install into ~/.claude/settings.json
├── internal/
│   ├── tui/
│   │   ├── app.go       # root BubbleTea model
│   │   ├── terminal.go  # PTY widget (bubbleterm) + SIGWINCH handler
│   │   ├── input.go     # single-line input with history
│   │   ├── preview.go   # 2s compression/filter preview overlay
│   │   ├── tokenpanel.go # right-side token panel renderer
│   │   └── state.go     # ClaudeState enum, colors, lipgloss styles
│   ├── compress/
│   │   ├── pipeline.go  # bypass rules, cache, secret scrubbing, logging
│   │   ├── ollama.go    # HTTP client for /v1/chat/completions (compress + filter)
│   │   ├── cache.go     # SHA-256 dedup cache, persisted to disk
│   │   └── scrubber.go  # regex-based secret redaction before Ollama calls
│   ├── fallback/
│   │   ├── grok.go      # xAI Grok via openai-go SDK
│   │   ├── gemini.go    # Google Gemini via OpenAI-compat endpoint
│   │   └── ollama.go    # local Ollama fallback + Chain() with exponential backoff
│   ├── logger/
│   │   └── logger.go    # JSONL operation log to ~/.claudewrap/logs/
│   ├── monitor/
│   │   ├── jsonl.go     # fsnotify JSONL watcher, offset-based reads
│   │   └── session.go   # State, StateSnapshot, reset estimate, ProgressBar
│   ├── compact/
│   │   └── counter.go   # compaction warnings (2x quality, 3x restart)
│   ├── notify/
│   │   └── notify.go    # OSC 9 + alerter + osascript + SSH guard
│   ├── schedule/
│   │   └── queue.go     # prompt queue + context snapshot persistence
│   ├── daemon/
│   │   └── socket.go    # Unix socket helpers, PID-based path scheme
│   └── context/
│       └── terminal.go  # terminal detection, IsPeakHour
├── contrib/
│   └── neovim.lua       # toggleterm.nvim integration (<leader>cc)
└── menubar/
    ├── Package.swift
    └── Sources/ClaudeWrapMenuBar/
        ├── ClaudeWrapApp.swift   # NSStatusItem, popover, notifications
        ├── TokenMonitor.swift    # JSONL polling, multi-session aggregation
        └── MenuBarView.swift     # SwiftUI popover view
```

---

## License

MIT
