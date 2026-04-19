package compress

import (
	"strings"
	"unicode"
)

type Result struct {
	Text    string
	Skipped bool   // true if bypass rules triggered
	Engine  string // "ollama", "bypass", "passthrough"
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
		// Check consistent indentation (4+ spaces or tab)
		if len(line) > 0 && (line[0] == '\t' || (len(line) >= 4 && line[0] == ' ' && unicode.IsSpace(rune(line[0])))) {
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
