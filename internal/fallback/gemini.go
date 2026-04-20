package fallback

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const geminiModel = "gemini-2.5-flash"

// QueryGemini sends prompt to Google's Gemini API via the OpenAI-compatible endpoint.
// Requires GEMINI_API_KEY.
func QueryGemini(prompt string) (string, int, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return "", 0, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL("https://generativelanguage.googleapis.com/v1beta/openai/"),
	)

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: geminiModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("gemini: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("gemini: no choices")
	}

	tokens := int(resp.Usage.TotalTokens)
	return resp.Choices[0].Message.Content, tokens, nil
}

// QueryOllamaFallback is the last resort using local Ollama with the default model.
func QueryOllamaFallback(prompt string) (string, error) {
	// Reuse a simple model available locally — not the compressor
	result, err := queryOllamaChat(prompt, "qwen2.5-coder:3b")
	return result, err
}
