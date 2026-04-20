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
)

type Result struct {
	Text    string
	Skipped bool   // true if bypass rules triggered
	Engine  string // "ollama", "bypass", "passthrough"
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
	if len([]rune(prompt)) < 80 {
		return true
	}
	if hasConsecutiveCodeLines(prompt, 3) {
		return true
	}
	return false
}

// Compress runs the prompt through Ollama compression.
// Returns the original prompt unchanged if Ollama is unavailable.
func Compress(prompt string) Result {
	if ShouldBypass(prompt) {
		text := strings.TrimPrefix(prompt, "!!")
		return Result{Text: strings.TrimSpace(text), Skipped: true, Engine: "bypass"}
	}

	compressed, err := ollamaCompress(prompt)
	if err != nil {
		return Result{Text: prompt, Skipped: true, Engine: "passthrough"}
	}
	return Result{Text: compressed, Engine: "ollama"}
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
		// Check consistent indentation (tab or 4+ leading spaces)
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
