// Ollama HTTP client for prompt compression.
// Uses the OpenAI-compatible /v1/chat/completions endpoint so the same model
// (claudewrap-compressor) can be swapped for any Ollama-hosted chat model.
package compress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	defaultOllamaHost = "http://localhost:11434"
	compressorModel   = "claudewrap-compressor"
	filterModel       = "qwen2.5:3b"
	compressTimeout   = 8 * time.Second
	filterTimeout     = 12 * time.Second
)

const filterSystem = `You are a content filter for an AI coding assistant. The user pasted content into their prompt. Extract ONLY what the AI needs to understand and act on. Rules:
1. Error/log: keep the error message, stack trace, and key context. Drop timestamps, repeated lines, and verbose debug output.
2. Code: keep as-is if under 200 lines. If longer, keep key functions and replace large filler blocks with a one-line summary comment.
3. Docs/text: extract requirements, constraints, and key facts. Drop redundant examples and filler.
4. JSON/YAML/CSV: keep structure, truncate repeated records to 3 examples with a "...N more" comment.
5. If the content is already concise, output it unchanged.
Output ONLY the filtered content. No explanation, no preamble.`

type ollamaRequest struct {
	Model    string    `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func ollamaFilterPaste(content string) (string, error) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = defaultOllamaHost
	}
	url := host + "/v1/chat/completions"

	reqBody := ollamaRequest{
		Model: filterModel,
		Messages: []ollamaMsg{
			{Role: "system", Content: filterSystem},
			{Role: "user", Content: content},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), filterTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: status %d", resp.StatusCode)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama: no choices")
	}
	return result.Choices[0].Message.Content, nil
}

func ollamaCompress(prompt string) (string, error) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = defaultOllamaHost
	}
	url := host + "/v1/chat/completions"

	reqBody := ollamaRequest{
		Model: compressorModel,
		Messages: []ollamaMsg{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), compressTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: status %d", resp.StatusCode)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama: no choices")
	}
	return result.Choices[0].Message.Content, nil
}
