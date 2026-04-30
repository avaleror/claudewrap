package fallback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func queryOllamaChat(prompt, model string) (string, error) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
		Stream   bool   `json:"stream"`
	}
	type choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type resp struct {
		Choices []choice `json:"choices"`
	}

	body, _ := json.Marshal(req{
		Model:    model,
		Messages: []msg{{Role: "user", Content: prompt}},
		Stream:   false,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		host+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	r, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama fallback: status %d", r.StatusCode)
	}

	var result resp
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama fallback: no choices")
	}
	return result.Choices[0].Message.Content, nil
}

// withBackoff retries fn up to maxAttempts times with exponential backoff.
// Delays: 1s, 2s, 4s (capped at maxAttempts-1 retries after the first try).
func withBackoff(maxAttempts int, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second)
		}
	}
	return err
}

// Chain queries providers in order (Grok → Gemini → local Ollama) with
// exponential backoff on transient failures. Returns the first successful
// response along with the engine name and token count.
func Chain(prompt string) (string, string, int, error) {
	var (
		result string
		tokens int
		err    error
	)

	err = withBackoff(3, func() error {
		result, tokens, err = QueryGrok(prompt)
		return err
	})
	if err == nil {
		return result, "Grok fast", tokens, nil
	}

	err = withBackoff(3, func() error {
		result, tokens, err = QueryGemini(prompt)
		return err
	})
	if err == nil {
		return result, "Gemini Flash", tokens, nil
	}

	result, err = QueryOllamaFallback(prompt)
	return result, "Ollama local", 0, err
}
