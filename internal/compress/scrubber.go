// Secret scrubber — strips likely API keys and credentials from text before
// it is sent to local Ollama. Targets assignment patterns (KEY=value) and
// known key prefixes (sk-, xai-, AIza, Bearer). Legitimate prose that
// mentions "API key" without an attached value is left untouched.
package compress

import (
	"regexp"
)

var secretPatterns = []*regexp.Regexp{
	// Assignment: SOME_KEY=<token> or SOME_KEY: <token>
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|auth|credential)[_a-z]*\s*[=:]\s*\S+`),
	// Bearer / Authorization header values
	regexp.MustCompile(`(?i)(bearer|authorization:\s*bearer)\s+[A-Za-z0-9\-._~+/]+=*`),
	// Known key prefixes: Anthropic (sk-ant-), OpenAI (sk-), xAI (xai-), Google (AIza)
	regexp.MustCompile(`\b(sk-ant-|sk-|xai-|AIza)[A-Za-z0-9\-_]{16,}`),
	// AWS-style access keys
	regexp.MustCompile(`\b(AKIA|ASIA|AROA|AIDA)[A-Z0-9]{16}\b`),
	// Generic long hex/base64 secrets after common field names
	regexp.MustCompile(`(?i)"(key|secret|token|password)"\s*:\s*"[A-Za-z0-9+/\-_]{20,}=*"`),
}

// ScrubSecrets replaces any detected secrets with [REDACTED] and returns
// the cleaned text plus whether any substitutions occurred.
func ScrubSecrets(text string) (cleaned string, redacted bool) {
	cleaned = text
	for _, re := range secretPatterns {
		replaced := re.ReplaceAllStringFunc(cleaned, func(match string) string {
			// Preserve the field name portion if we can find the separator
			for i, ch := range match {
				if ch == '=' || ch == ':' {
					return match[:i+1] + " [REDACTED]"
				}
			}
			return "[REDACTED]"
		})
		if replaced != cleaned {
			redacted = true
			cleaned = replaced
		}
	}
	return cleaned, redacted
}
