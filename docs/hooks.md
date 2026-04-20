# Hook Integration

ClaudeWrap uses three Claude Code lifecycle hooks. All are handled by the same binary (`claudewrap`) dispatched via hidden flags.

---

## Overview

```
~/.claude/settings.json

{
  "hooks": {
    "SessionStart":  [{ "command": "claudewrap --hook-session-start" }],
    "StopFailure":   [{ "matcher": "rate_limit",
                        "command": "claudewrap --hook-rate-limit" }],
    "PreCompact":    [{ "command": "claudewrap --hook-pre-compact" }]
  }
}
```

These are auto-merged by `configureClaudeHooks()` on first run. If `SessionStart` already exists in the config, the function returns early — it never overwrites custom hooks.

---

## SessionStart

**When:** Claude Code starts a new session.

**Stdin payload:**
```json
{
  "session_id": "abc123",
  "transcript_path": "/Users/me/.claude/projects/…/sessions/abc123.jsonl"
}
```

**What the hook does:**
1. Writes `~/.claudewrap/sessions/<session_id>.json` with `transcript_path` and `started_at`
2. Writes `~/.claudewrap/current-session` with the session ID (plain text)
3. Sends `MsgSessionStart` to the TUI via `CLAUDEWRAP_SOCKET`
4. Exits 0

**What the TUI does on receipt:**
1. Reads `SessionInfo` from disk (gets `transcript_path`)
2. Starts the JSONL watcher with a 30-second retry loop (file may not exist yet)
3. From this point, every JSONL write triggers a `StateUpdateMsg` → panel re-render

---

## StopFailure (rate limit)

**When:** Claude Code fails to get a response because the session is rate-limited. Matcher: `"rate_limit"`.

**Stdin payload:**
```json
{
  "prompt": "the pending prompt, if any"
}
```

**What the hook does:**
1. If `payload.Prompt != ""`, appends it to `~/.claudewrap/queue.json`
2. Reads current session ID from `~/.claudewrap/current-session`
3. Sends `MsgRateLimit` to the TUI
4. Sends a native macOS notification
5. Exits 0

**What the TUI does on receipt:**
1. Switches to `StateRateLimit` — red border
2. Saves a context snapshot to `~/.claudewrap/context_<ts>.json`
3. All subsequent input is routed to the fallback chain

---

## PreCompact

**When:** Claude Code is about to run `/compact`.

**Stdin payload:** empty (hook receives no structured data).

**What the hook does:**
1. Reads current session ID
2. Sends `MsgPreCompact` to the TUI
3. Exits 0

**What the TUI does on receipt:**
1. Increments `panel.CompactionCount`
2. Persists the new count to the session JSON file (menubar reads it)
3. Refreshes `gitBranch` (post-compact is a good time to re-check the branch)

---

## Socket path resolution

Hooks resolve the socket path with this priority:

```
1. CLAUDEWRAP_SOCKET env var   ← set by TUI before starting claude
2. daemon.SocketPath(sessionID) ← ~/.claudewrap/daemon-<session_id>.sock
```

The env var approach works reliably when the hook is a direct subprocess of the TUI process (normal case). The session-ID fallback handles edge cases where the TUI was started by a different process or the env var was not inherited.

---

## Timeout behaviour

All hook-to-TUI messages use a **2-second timeout**. If the TUI is not running, `daemon.Send()` returns an error, the hook logs nothing, and exits 0. Claude Code never sees a failed hook.

This means hooks are fire-and-forget from Claude's perspective. The TUI is purely additive — removing it never breaks the underlying `claude` workflow.

---

## Adding a new hook

1. Add a `MsgType` constant in `internal/daemon/socket.go`
2. Add a handler flag in `cmd/root.go` (`rootCmd.Flags().BoolVar(...)`)
3. Write the handler function in `cmd/hooks.go` following the existing pattern
4. Handle the new `Msg*` type in `handleConn()` in `cmd/root.go`
5. Update the `jqExpr` in `cmd/setup.go` to include the new hook
