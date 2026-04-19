# ClaudeWrap

A macOS TUI wrapper for Claude Code. Run `claudewrap` instead of `claude` and get:

- Prompt compression via local Ollama (40–60% shorter, same meaning)
- Token panel with live usage, reset countdown, and per-category breakdown
- Auto-/compact at 60% context with idle-state guard
- AI fallback chain (Grok → Gemini → local Ollama) when session is rate-limited
- Visual state indicators: border color shows what Claude is doing
- Native notifications + cmux/Ghostty sidebar integration via OSC 9
- Session queue: prompts saved when rate-limited, replayed after reset

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/avaleror/claudewrap/main/install.sh | bash
```

Requires: macOS 14+, Go 1.26+, Swift 6+, Homebrew.

## API keys (for AI fallback)

Add to `~/.zshrc`:

```bash
export GROK_API_KEY=your-xai-key
export GEMINI_API_KEY=your-google-key
```

## Usage

```bash
claudewrap          # start with current directory
claudewrap --resume # resume last session
```

### Bypass compression

Prefix your prompt with `!!` to send it as-is:

```
!! explain this regex without summarizing
```

Compression is also skipped automatically for short prompts (<80 chars) and prompts with 3+ consecutive code lines.

### Token panel

The right panel shows:

```
Claude Session             [b: breakdown]
  Used:  45,230 tokens
  Rem:   73% [=========-]
  Reset: ~3h 42m (est.)    Peak active

Compacted: 1x
```

Press `b` to toggle per-category token breakdown (CLAUDE.md, tool call I/O, @-files, etc.).

### Visual state indicators

| Border color | Meaning |
|---|---|
| Neutral | Claude processing |
| Blue | Waiting for your input |
| Yellow | Auto-compacting |
| Red | Rate limited |

### Auto-compact

ClaudeWrap injects `/compact` when context usage hits 60%. It waits for Claude to finish responding before injecting — never mid-stream.

After 2 compactions: warning shown. After 3: restart recommendation.

### cmux / Ghostty

Notifications appear in cmux's sidebar and notification ring automatically. No setup needed — ClaudeWrap emits OSC 9 sequences that cmux and Ghostty pick up natively.

### Peak hours

Anthropic throttles sessions 5am–11am Pacific. ClaudeWrap detects this and shows "Peak active" with an adjusted reset estimate.

### Multi-agent / worktrees

Each claudewrap instance gets a session-scoped socket (`~/.claudewrap/daemon-<id>.sock`). Running inside `cmux claude-teams` or multiple git worktrees works without conflicts.

### Vim integration

```vim
" Add to init.vim / .vimrc (requires vim-floaterm)
source /path/to/claudewrap/contrib/vim-floaterm.vim
" <leader>cc opens ClaudeWrap in a floating window
```

---

## Architecture

Two components:

1. **CLI TUI** (Go) — `claudewrap` binary. Wraps `claude` in a PTY, intercepts input at Enter for compression, renders a split view with terminal output + token panel.

2. **Menubar app** (Swift) — `claudewrap-menubar`. Persistent icon showing token %, aggregates all active sessions, sends macOS notifications with action buttons.

Both communicate via session-scoped Unix sockets at `~/.claudewrap/daemon-<session_id>.sock`.

---

## Environment variables

| Variable | Purpose |
|---|---|
| `GROK_API_KEY` | xAI Grok (AI fallback tier 1) |
| `GEMINI_API_KEY` | Google Gemini (AI fallback tier 2) |
| `OLLAMA_HOST` | Ollama endpoint (default: http://localhost:11434) |

`TERM_PROGRAM` and `SSH_TTY` are read automatically — no config needed.
