# ClaudeWrap — Architecture

ClaudeWrap is two programs: a Go CLI that wraps Claude Code in a TUI, and a Swift menubar companion that reads session data independently. They share no runtime state — the menubar polls files on disk.

---

## System overview

```mermaid
graph TD
    User["User (terminal)"]
    CW["claudewrap (Go TUI)"]
    Claude["claude subprocess (PTY)"]
    Ollama["Ollama\nclaudewrap-compressor\nqwen2.5-coder:3b"]
    FB["AI Fallback Chain\nGrok → Gemini → Ollama"]
    HookBin["Hook subprocesses\n(claudewrap --hook-*)"]
    JSONL["~/.claude/projects/.../session.jsonl"]
    Socket["~/.claudewrap/daemon-pid-<PID>.sock"]
    SessionFile["~/.claudewrap/sessions/<id>.json"]
    Queue["~/.claudewrap/queue.json"]
    Menubar["claudewrap-menubar (Swift)"]
    Notif["macOS Notifications\nOSC 9 / alerter / osascript"]

    User -->|"types prompt"| CW
    CW -->|"PTY I/O"| Claude
    Claude -->|"writes"| JSONL
    Claude -->|"spawns"| HookBin
    HookBin -->|"CLAUDEWRAP_SOCKET"| Socket
    Socket -->|"MsgSessionStart\nMsgRateLimit\nMsgPreCompact"| CW
    CW -->|"compress prompt"| Ollama
    CW -->|"rate limited"| FB
    JSONL -->|"fsnotify watch"| CW
    HookBin -->|"session info"| SessionFile
    HookBin -->|"pending prompt"| Queue
    CW --> Notif
    Menubar -->|"polls every 2s"| JSONL
    Menubar -->|"reads"| SessionFile
```

---

## Component 1 — CLI TUI (Go)

### Internal package map

```mermaid
graph LR
    subgraph cmd
        root["root.go\nCLI entry + TUI launch"]
        hooks["hooks.go\nhook handlers"]
        setup["setup.go\nhook install + winsize"]
    end

    subgraph tui
        app["app.go\nApp (BubbleTea root)"]
        terminal["terminal.go\nTermWidget + SIGWINCH"]
        input["input.go\nInputModel + history"]
        preview["preview.go\nPreviewModel 2s window"]
        tokenpanel["tokenpanel.go\nright sidebar"]
        state["state.go\nClaudeState + styles"]
    end

    subgraph compress
        pipeline["pipeline.go\nbypass rules + dispatch"]
        ollama_c["ollama.go\nHTTP /v1/chat/completions"]
    end

    subgraph fallback
        grok["grok.go"]
        gemini["gemini.go"]
        ollama_f["ollama.go + Chain()"]
    end

    subgraph monitor
        jsonl["jsonl.go\nWatcher (fsnotify)"]
        session["session.go\nState + Snapshot + estimates"]
    end

    subgraph other
        compact["compact/counter.go"]
        notify["notify/notify.go"]
        schedule["schedule/queue.go"]
        daemon["daemon/socket.go"]
        context["context/terminal.go"]
    end

    root --> tui
    root --> compress
    root --> fallback
    root --> monitor
    root --> daemon
    root --> schedule
    root --> context
    hooks --> daemon
    hooks --> monitor
    hooks --> notify
    hooks --> schedule
    app --> terminal
    app --> input
    app --> preview
    app --> tokenpanel
    app --> state
    app --> notify
    app --> schedule
    app --> compact
    pipeline --> ollama_c
    fallback --> grok
    fallback --> gemini
    fallback --> ollama_f
    monitor --> jsonl
    monitor --> session
    session --> context
```

---

## Prompt flow

Every prompt the user types goes through this pipeline before reaching Claude.

```mermaid
flowchart TD
    A["User presses Enter"] --> B{"IsConfirmationInput?\ny/n/yes/no/digit"}
    B -->|yes| C["Send directly to PTY\nno compression"]
    B -->|no| D{"ShouldBypass?"}
    D -->|"!! prefix"| E["Strip !! prefix\nSend to PTY"]
    D -->|"< 80 chars"| E
    D -->|"3+ code lines"| E
    D -->|no| F["Ollama compress\n8s timeout"]
    F -->|error / unavailable| G["Passthrough to PTY"]
    F -->|success| H["PreviewModel\nShows 2s countdown"]
    H -->|"Esc within 2s"| I["Send ORIGINAL to PTY"]
    H -->|"2s timeout"| J["Send COMPRESSED to PTY"]
    C --> K["Claude processes"]
    E --> K
    G --> K
    I --> K
    J --> K
```

---

## State machine

The `ClaudeState` enum drives border color, input availability, and compaction timing.

```mermaid
stateDiagram-v2
    [*] --> Running : startup

    Running --> Waiting : PTY idle 500ms
    Waiting --> Running : PTY output arrives

    Waiting --> Compacting : context >= 60%\nor Ctrl+K
    Compacting --> Running : /compact sent\nPTY resumes

    Running --> RateLimit : MsgRateLimit\nfrom hook
    Waiting --> RateLimit : MsgRateLimit\nfrom hook

    RateLimit --> [*] : claudewrap exits

    note right of Waiting
        Border: blue
        Input box active
        b: toggle breakdown
    end note

    note right of Running
        Border: neutral
        Input box hidden
        Keys forwarded to PTY
    end note

    note right of Compacting
        Border: yellow
        /compact injected to PTY
    end note

    note right of RateLimit
        Border: red
        Input box active
        Routes to fallback chain
    end note
```

---

## Hook system

Claude Code calls hooks as subprocesses. ClaudeWrap hooks are the same binary (`claudewrap`) dispatched by hidden flags.

```mermaid
sequenceDiagram
    participant CC as Claude Code
    participant Hook as claudewrap --hook-*
    participant Socket as Unix socket
    participant TUI as ClaudeWrap TUI

    Note over CC,TUI: Session start
    CC->>Hook: SessionStart\nstdin: {session_id, transcript_path}
    Hook->>Hook: WriteSessionInfo to disk
    Hook->>Hook: WriteCurrentSessionID to disk
    Hook->>Socket: MsgSessionStart {session_id}
    Socket->>TUI: SessionStartMsg
    TUI->>TUI: Start JSONL watcher\n(30s retry loop)
    Hook-->>CC: exit 0

    Note over CC,TUI: Rate limit
    CC->>Hook: StopFailure (matcher: rate_limit)\nstdin: {prompt?}
    Hook->>Hook: Append prompt to queue.json
    Hook->>Socket: MsgRateLimit
    Socket->>TUI: RateLimitMsg
    TUI->>TUI: StateRateLimit\nSave context snapshot
    Hook-->>CC: exit 0

    Note over CC,TUI: Pre-compact
    CC->>Hook: PreCompact
    Hook->>Socket: MsgPreCompact
    Socket->>TUI: PreCompactMsg
    TUI->>TUI: Increment compaction count\nRefresh git branch
    Hook-->>CC: exit 0
```

### Socket rendezvous

The TUI needs hooks to find the right socket when multiple claudewrap instances are running (git worktrees, cmux claude-teams).

```
TUI process (PID 12345)
  └─ sets CLAUDEWRAP_SOCKET = ~/.claudewrap/daemon-pid-12345.sock
  └─ claude subprocess inherits the env var
       └─ hook subprocess also inherits it
            └─ hook reads CLAUDEWRAP_SOCKET → sends message to correct TUI
```

If `CLAUDEWRAP_SOCKET` is not set (out-of-process invocation), hooks fall back to reading `~/.claudewrap/current-session` and constructing the path from the stored session ID.

---

## JSONL data flow

Claude Code writes one JSON object per line to a session transcript file. ClaudeWrap tails it in real time.

```mermaid
flowchart LR
    CC["Claude Code\nwrites JSONL entries"]
    File["~/.claude/projects/.../\nsession.jsonl"]
    FN["fsnotify.Watcher\ndetects Write events"]
    ReadFrom["readFrom(offset)\nscans new bytes only"]
    State["monitor.State\nthread-safe accumulator"]
    Snap["StateSnapshot\nimmutable read copy"]
    TUI["BubbleTea\nStateUpdateMsg"]
    Panel["Token Panel\nre-renders"]

    CC -->|"append"| File
    File -->|"inotify event"| FN
    FN --> ReadFrom
    ReadFrom -->|"Entry{Type,Usage,...}"| State
    State -->|"Snapshot()"| Snap
    Snap -->|"StateUpdateMsg"| TUI
    TUI --> Panel
```

Each JSONL entry has this shape (simplified):

```json
{
  "type": "assistant",
  "timestamp": "2026-04-21T14:32:01Z",
  "message": {
    "usage": {
      "input_tokens": 41200,
      "output_tokens": 4030,
      "cache_read_input_tokens": 12400,
      "cache_creation_input_tokens": 800,
      "remaining_percentage": 72.4,
      "context_window": {
        "used_percentage": 61.2,
        "total_tokens": 200000,
        "used_tokens": 122400
      },
      "breakdown": {
        "claude_md": 8200,
        "tool_call_io": 4100,
        "mentioned_files": 3400,
        "extended_thinking": 1200,
        "conversation": 28000,
        "skill_activations": 330,
        "team_overhead": 450,
        "user_text": 20
      }
    }
  }
}
```

---

## Compression pipeline

```mermaid
flowchart TD
    A["OllamaAvailable()\n2s GET /api/tags at startup"]
    A -->|"false"| B["ollamaOK = false\nstatus bar: 'ollama: off'"]
    A -->|"true"| C["ollamaOK = true"]

    D["User submits prompt"] --> E{"ShouldBypass?"}
    E -->|"!! prefix\n< 80 chars\n3+ code lines"| F["bypass\nEngine label: 'bypass'"]
    E -->|"no"| G{"ollamaOK?"}
    G -->|"false"| H["passthrough\nEngine label: 'passthrough'"]
    G -->|"true"| I["POST /v1/chat/completions\nmodel: claudewrap-compressor\n8s timeout"]
    I -->|"error"| H
    I -->|"ok"| J["compressed text\nEngine label: 'ollama'"]

    F --> K["PreviewModel skipped\nsend immediately"]
    H --> K
    J --> L["PreviewModel shown\n2s countdown"]
    L -->|"Esc"| K
    L -->|"auto-send"| M["send compressed\nto PTY"]
    K --> N["send original\nto PTY"]
```

---

## AI fallback chain

Activated when `StateRateLimit` — user prompts go to fallback instead of PTY.

```mermaid
flowchart TD
    A["User submits in RateLimit state"]
    A --> B["fallbackAsync(text)\ngoroutine"]
    B --> C{"GROK_API_KEY set?"}
    C -->|"yes"| D["POST api.x.ai/v1\ngrok-4-1-fast"]
    D -->|"ok"| E["response\nengine: Grok fast"]
    D -->|"error"| F{"GEMINI_API_KEY set?"}
    C -->|"no"| F
    F -->|"yes"| G["POST generativelanguage.googleapis.com\ngemini-2.5-flash"]
    G -->|"ok"| H["response\nengine: Gemini Flash"]
    G -->|"error"| I["POST localhost:11434\nqwen2.5-coder:3b\n30s timeout"]
    F -->|"no"| I
    I -->|"ok"| J["response\nengine: Ollama local"]
    I -->|"error"| K["fallbackResultMsg\nerr: all providers failed"]
    E --> L["fallbackLog += engine: response\nupdate panel cost/tokens"]
    H --> L
    J --> L
    K --> M["fallbackLog += error message"]
```

---

## Reset time estimate

```
                  first_entry_time
                        │
                        ▼
           ┌────────────────────────┐
           │  IsPeakHour(now)?      │
           │  05:00–11:00 Pacific   │
           └────────────────────────┘
                  │          │
                 yes         no
                  │          │
                  ▼          ▼
            +2h 51m        +5h 00m
            (5h / 1.75)
                  │          │
                  └────┬─────┘
                       │
                  EstimatedReset
                  shown as "~Xh Ym (est.)"
```

The Swift menubar uses the same formula with the proper Pacific timezone via `Calendar`.

---

## Component 2 — Menubar app (Swift)

```mermaid
flowchart TD
    subgraph SwiftUI["claudewrap-menubar (Swift)"]
        TM["TokenMonitor\n@MainActor ObservableObject\npolls every 2s"]
        AD["AppDelegate\n@MainActor\nNSStatusItem + popover"]
        MBV["MenuBarView\nSwiftUI popover"]
        SNAP["TokenSnapshot\nvalue type"]
    end

    JSONL["~/.claude/projects/**/\nsessions/*.jsonl\n(modified in last 5h)"]
    SFile["~/.claudewrap/sessions/<id>.json\ncompaction_count"]
    CSFile["~/.claudewrap/current-session\nactive session ID"]
    Notif["UNUserNotificationCenter\nnative macOS notifications"]
    Icon["NSStatusItem\nCC 73% (color-coded)"]

    TM -->|"findActiveJSONLFiles()\nparses last N lines"| JSONL
    TM -->|"readCompactionCount()"| CSFile
    CSFile -->|"session ID"| SFile
    TM -->|"@Published snapshot"| SNAP
    SNAP --> AD
    SNAP --> MBV
    AD -->|"Timer every 5s\nupdateIcon()"| Icon
    AD -->|"<= 11%"| Notif
```

The menubar app never connects to the Unix socket. It reads JSONL and session files directly, which means it keeps working even if the TUI is not running.

---

## Data directory layout

```
~/.claudewrap/
│
├── daemon-pid-<PID>.sock          # TUI socket — deleted when TUI exits
│                                  # one per running claudewrap process
│
├── current-session                # plaintext session ID
│                                  # written by SessionStart hook
│                                  # read by: hooks (fallback), menubar app
│
├── sessions/
│   └── <session-uuid>.json        # SessionInfo: transcript_path, started_at,
│                                  #   compaction_count
│
├── queue.json                     # [{prompt, bypass, added_at}, ...]
│                                  # written by rate-limit hook
│                                  # replayed on next claudewrap start
│
└── context_<timestamp>.json       # TokenSnapshot at moment of rate limit:
                                   #   remaining_pct, used_tokens, total_tokens,
                                   #   estimated_reset, compaction_count
```

---

## Key design decisions

### Why PID-based sockets?

Multiple claudewrap instances (git worktrees, cmux claude-teams) must not share a socket. Using the TUI's PID as the socket name guarantees uniqueness. The PID is exported as `CLAUDEWRAP_SOCKET` before `claude` is started, so hook subprocesses inherit it without any coordination.

### Why intercept at Enter instead of using UserPromptSubmit hook?

`UserPromptSubmit` hooks cannot modify the prompt they receive (Claude Code issue #13912 — stdout is not read back by the host process). Intercepting keystrokes at the TUI input layer gives full control over what reaches the PTY, with no hook round-trip latency.

### Why retry the JSONL watcher for 30 seconds?

The `SessionStart` hook fires before Claude creates the transcript file. The watcher would fail immediately if it tried to open the file at hook time. The 30-second retry loop (1 attempt/second) handles the race condition without requiring any changes to Claude Code's hook firing order.

### Why does the menubar not use the socket?

Polling JSONL files directly makes the menubar independent of whether a TUI is running. It also means it naturally aggregates multiple sessions without any coordination protocol.
