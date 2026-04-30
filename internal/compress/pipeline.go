// Package compress runs user prompts through a local Ollama model to reduce
// their token footprint before they reach Claude. Short prompts, code blocks,
// and prompts prefixed with "!!" bypass compression entirely.
package compress

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/avaleror/claudewrap/internal/logger"
)

type Result struct {
	Text     string
	Skipped  bool   // true if bypass rules triggered
	Engine   string // "ollama", "bypass", "passthrough", "filter"
	CacheHit bool
	Redacted bool // true if secrets were stripped before sending to Ollama
}

// OllamaAvailable returns true if the Ollama server is reachable.
func OllamaAvailable() bool {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = defaultOllamaHost
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ShouldBypass returns true if the prompt should skip compression.
func ShouldBypass(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if strings.HasPrefix(prompt, "!!") {
		return true
	}
	if utf8.RuneCountInString(prompt) < 80 {
		return true
	}
	if hasConsecutiveCodeLines(prompt, 3) {
		return true
	}
	return false
}

// FilterPaste runs pasted content through the Ollama paste filter.
// Short content and !! prefix bypass filtering. Secrets are scrubbed first.
func FilterPaste(content string) Result {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "!!") {
		return Result{Text: strings.TrimSpace(content[2:]), Skipped: true, Engine: "bypass"}
	}
	if utf8.RuneCountInString(content) < 80 {
		return Result{Text: content, Skipped: true, Engine: "bypass"}
	}

	// Cache check
	if cached, ok := cacheGet(content); ok {
		logger.Log(logger.Entry{Operation: "filter", Engine: cached.Engine, CacheHit: true, Success: true})
		return Result{Text: cached.Compressed, Engine: cached.Engine, CacheHit: true}
	}

	// Secret scrubbing
	clean, redacted := ScrubSecrets(content)

	start := time.Now()
	filtered, err := ollamaFilterPaste(clean)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		logger.Log(logger.Entry{Operation: "filter", Engine: "passthrough", DurationMs: dur, Success: false, Error: err.Error(), Redacted: redacted})
		return Result{Text: content, Skipped: true, Engine: "passthrough", Redacted: redacted}
	}

	cacheSet(content, cacheEntry{Compressed: filtered, Engine: "filter"})
	logger.Log(logger.Entry{
		Operation:  "filter",
		Engine:     "filter",
		TokensIn:   utf8.RuneCountInString(content),
		TokensOut:  utf8.RuneCountInString(filtered),
		Ratio:      1 - float64(utf8.RuneCountInString(filtered))/float64(utf8.RuneCountInString(content)),
		DurationMs: dur,
		Success:    true,
		Redacted:   redacted,
	})
	return Result{Text: filtered, Engine: "filter", Redacted: redacted}
}

// Compress runs the prompt through Ollama compression.
// Returns the original prompt unchanged if Ollama is unavailable.
func Compress(prompt string) Result {
	if ShouldBypass(prompt) {
		text := strings.TrimPrefix(strings.TrimSpace(prompt), "!!")
		logger.Log(logger.Entry{Operation: "compress", Engine: "bypass", Skipped: true, Success: true})
		return Result{Text: strings.TrimSpace(text), Skipped: true, Engine: "bypass"}
	}

	// Cache check
	if cached, ok := cacheGet(prompt); ok {
		logger.Log(logger.Entry{Operation: "compress", Engine: cached.Engine, CacheHit: true, Success: true})
		return Result{Text: cached.Compressed, Engine: cached.Engine, CacheHit: true}
	}

	// Secret scrubbing
	clean, redacted := ScrubSecrets(prompt)

	start := time.Now()
	compressed, err := ollamaCompress(clean)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		logger.Log(logger.Entry{Operation: "compress", Engine: "passthrough", DurationMs: dur, Success: false, Error: err.Error(), Redacted: redacted})
		return Result{Text: prompt, Skipped: true, Engine: "passthrough", Redacted: redacted}
	}

	cacheSet(prompt, cacheEntry{Compressed: compressed, Engine: "ollama"})
	logger.Log(logger.Entry{
		Operation:  "compress",
		Engine:     "ollama",
		TokensIn:   utf8.RuneCountInString(prompt),
		TokensOut:  utf8.RuneCountInString(compressed),
		Ratio:      1 - float64(utf8.RuneCountInString(compressed))/float64(utf8.RuneCountInString(prompt)),
		DurationMs: dur,
		Success:    true,
		Redacted:   redacted,
	})
	return Result{Text: compressed, Engine: "ollama", Redacted: redacted}
}

// hasConsecutiveCodeLines checks for 3+ consecutive code lines (``` block or consistent indent).
func hasConsecutiveCodeLines(s string, threshold int) bool {
	lines := strings.Split(s, "\n")
	consecutive := 0
	inFenced := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFenced {
				inFenced = false
				consecutive = 0
			} else {
				inFenced = true
			}
			continue
		}
		if inFenced {
			consecutive++
			if consecutive >= threshold {
				return true
			}
			continue
		}
		if len(line) > 0 && (line[0] == '\t' || (len(line) >= 4 && line[0] == ' ' && line[1] == ' ' && line[2] == ' ' && line[3] == ' ')) {
			consecutive++
			if consecutive >= threshold {
				return true
			}
		} else {
			consecutive = 0
		}
	}
	return false
}
