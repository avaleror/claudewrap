package fallback

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const grokModel = "grok-4-1-fast"

func QueryGrok(prompt string) (string, int, error) {
	key := os.Getenv("GROK_API_KEY")
	if key == "" {
		return "", 0, fmt.Errorf("GROK_API_KEY not set")
	}

	client := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL("https://api.x.ai/v1"),
	)

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: grokModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("grok: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("grok: no choices")
	}

	tokens := int(resp.Usage.TotalTokens)
	return resp.Choices[0].Message.Content, tokens, nil
}
