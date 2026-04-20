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

// Chain queries providers in order (Grok → Gemini → local Ollama) and returns
// the first successful response along with the engine name and token count.
// Local Ollama is always available so Chain only returns an error if all three fail.
func Chain(prompt string) (string, string, int, error) {
	if result, tokens, err := QueryGrok(prompt); err == nil {
		return result, "Grok fast", tokens, nil
	}
	if result, tokens, err := QueryGemini(prompt); err == nil {
		return result, "Gemini Flash", tokens, nil
	}
	result, err := QueryOllamaFallback(prompt)
	return result, "Ollama local", 0, err
}
