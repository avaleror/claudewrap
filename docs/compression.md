# Prompt Compression

ClaudeWrap compresses prompts locally before they reach Claude, reducing token usage by 40–60% on average. Compression is synchronous from the user's perspective but asynchronous in the BubbleTea model — the TUI stays responsive during the Ollama call.

---

## Model

The compressor uses a custom Modelfile wrapping `qwen2.5-coder:3b`:

```
FROM qwen2.5-coder:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite
       user prompts to be 40-60% shorter while preserving every instruction,
       constraint, file name, and technical term exactly. Output ONLY the
       rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
```

`qwen2.5-coder:3b` is preferred over the general `qwen2.5:3b` because:
- Same parameter count (~2GB on disk)
- Better handling of code-adjacent vocabulary (file paths, function names, flags)
- Lower hallucination rate on technical terms at temperature 0.1

---

## Bypass rules

Compression is skipped when any of these match:

| Rule | Reason |
|---|---|
| Prompt starts with `!!` | Explicit user override — send verbatim |
| Prompt < 80 characters | Overhead of Ollama round-trip exceeds savings |
| 3+ consecutive code lines | Compressing code risks corrupting syntax |

Code detection looks for:
- Lines inside a fenced block (` ``` ... ``` `)
- 3+ consecutive lines starting with a tab or 4 spaces

---

## Preview window

```
┌──────────────────────────────────────────────────────────────────────┐
│  Compressed -52%: refactor auth middleware session token per comply…  │
│  Sending in 1.8s...  [Esc: send original]  [ollama]                  │
└──────────────────────────────────────────────────────────────────────┘
```

- Shows for **2 seconds** after a successful compression
- Displays: compression ratio, truncated preview, countdown, engine name
- `Esc` before countdown: sends original prompt
- Countdown expires: sends compressed prompt automatically
- Compression ratio = `100 - (len(compressed) * 100 / len(original))`

---

## Engine labels

The status bar and preview show which engine handled the last prompt:

| Label | Meaning |
|---|---|
| `ollama` | Compressed via local Ollama |
| `bypass` | Bypassed (!! prefix or short/code prompt) |
| `passthrough` | Ollama unavailable or returned error |

---

## Availability check

`OllamaAvailable()` runs at startup:

```
GET http://localhost:11434/api/tags
timeout: 2s
```

If it fails, `ollamaOK = false` is stored in the App struct. The status bar shows `ollama: off` and all prompts use passthrough. The check runs once — a restart is needed to re-enable compression if Ollama comes up later.

Override the endpoint with `OLLAMA_HOST=http://host:port`.

---

## Async flow in BubbleTea

```
SubmitMsg received
  └─ a.compressing = true
  └─ return compressAsync(text)   ← BubbleTea runs this in a goroutine

        ↓  (Ollama call in progress)

  Input box shows "Compressing with Ollama..."
  New keypresses are blocked (compressing guard)

        ↓  (goroutine returns)

compressResultMsg received
  └─ a.compressing = false
  └─ if skipped or unchanged → SendText to PTY
  └─ else → PreviewModel.Show()
```

The `compressing` flag prevents the user from submitting a second prompt while the first is in-flight, which would cause two concurrent Ollama requests racing to the same PTY.

---

## Customising the model

To use a different base model, edit the Modelfile at the repo root and recreate:

```bash
ollama create claudewrap-compressor -f Modelfile
```

The model name `claudewrap-compressor` is hardcoded in `internal/compress/ollama.go`. The endpoint is read from `OLLAMA_HOST` at runtime.
