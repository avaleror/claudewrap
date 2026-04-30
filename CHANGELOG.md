# Changelog

All notable changes to ClaudeWrap are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

### Added
- Operation log (`~/.claudewrap/logs/claudewrap-YYYY-MM-DD.jsonl`) — every compress, filter, and fallback call is logged with tokens, ratio, duration, and engine
- Compression dedup cache (`~/.claudewrap/compress-cache.json`) — identical prompts and pastes skip Ollama and reuse previous results across sessions
- Secret scrubber — strips API keys, Bearer tokens, and known key prefixes (`sk-`, `xai-`, `AIza`, `AKIA`) from any text before it reaches local Ollama
- Exponential backoff in fallback chain — Grok and Gemini each get 3 attempts (1 s, 2 s, 4 s delays) before falling through to the next provider

### Changed
- Auto-compact threshold lowered from 60 % → 30 % context used (research shows model quality degrades at 20–40 %, not at the limit)
- Neovim integration replaces Vim: passthrough mode now triggers on `$NVIM` (Neovim ≥ 0.5 socket) and `$NVIM_LISTEN_ADDRESS` (legacy); `$VIM` check removed
- `contrib/vim-floaterm.vim` replaced by `contrib/neovim.lua` (toggleterm.nvim, with floaterm fallback comment)
- `install.sh` now installs Neovim via Homebrew if not present
- `Result` struct gains `CacheHit` and `Redacted` fields

---

## [0.5.0] — 2026-04-25

### Added
- Paste filter: bracketed paste (`tea.PasteMsg`) is intercepted and routed through a dedicated Ollama filter (`qwen2.5:3b` with content-type-aware system prompt) instead of the compressor
  - Handles errors/logs, code, docs, JSON/YAML differently
  - Teal preview overlay (vs grey for compression)
  - Any text already typed is prepended before filtering
- `!!` prefix bypasses both compression and paste filter
- `PasteFilterResult` / `SetPasteFilterFunc` extension points in TUI (same pattern as `SetCompressFunc`)

---

## [0.4.0] — 2026-04-23

### Added
- Comprehensive architecture docs with Mermaid diagrams (`docs/architecture.md`)
- Package-level GoDoc and exported symbol comments across all packages

---

## [0.3.0] — 2026-04-20

### Added
- UX: prompt history (↑↓, last 100 entries), `Ctrl+U` clear, `Ctrl+W` delete word
- Compression indicator in status bar showing active engine
- Confirmation bypass: single-key responses (`y`, `n`, digits) skip compression and go straight to PTY
- macOS desktop notifications via `osascript` / `alerter` / OSC 9

### Fixed
- Swift 6 strict concurrency errors in menubar app
- `watchJSONL`: 30 s retry loop — JSONL file may not exist when `SessionStart` fires
- PID-based socket (`daemon-pid-<pid>.sock`) + `CLAUDEWRAP_SOCKET` env var so hooks always find the right TUI instance
- `setup.go`: spurious `UserPromptSubmit` hook removed; early-return fixed
- `compress/pipeline.go`: 4-space indent detection corrected
- Queue replay wired into `NewApp`; tick reschedule fixed after injection
- `SessionInfo.CompactionCount` added and persisted on `PreCompact`

---

## [0.2.0] — 2026-04-19

### Added
- AI fallback chain: Grok (`grok-4-1-fast`) → Gemini (`gemini-2.5-flash`) → local Ollama, activated on rate limit
- `--resume` flag to reopen the most recent Claude session
- Context snapshot saved on rate limit; queue offered for replay on next start
- Git branch display in status bar (refreshed on each compaction)
- Token count formatting (K/M suffixes)

---

## [0.1.0] — 2026-04-19

### Added
- BubbleTea TUI with left PTY pane (bubbleterm + creack/pty) and right token panel
- Prompt compression via local Ollama (`claudewrap-compressor` / `qwen2.5-coder:3b`), 40–60 % savings
- 2-second compression preview overlay with Esc cancellation
- Live token monitoring: reads `~/.claude/projects/<hash>/<session>.jsonl` in real-time via fsnotify
- Auto-compact: injects `/compact` at context threshold, never mid-response
- Low-token alert at 11 % remaining
- Session queue: saves pending prompts on rate limit, offers replay on restart
- Unix socket daemon (`~/.claudewrap/daemon-pid-<pid>.sock`) for hook communication
- Claude Code hooks: `SessionStart`, `StopFailure` (rate limit), `PreCompact`
- `install.sh`: full install automation (Homebrew, Ollama, model pull, launchd services, hook config)
- Swift menubar app: token % in menubar, aggregates all active sessions, polls JSONL every 2 s
- Passthrough mode when running inside Neovim (`$NVIM` / `$NVIM_LISTEN_ADDRESS`)
- `contrib/neovim.lua`: toggleterm.nvim integration (`<leader>cc`)
