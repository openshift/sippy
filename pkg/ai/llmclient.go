package ai

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	log "github.com/sirupsen/logrus"
)

type LLMClient struct {
	client *openai.Client
	model  string
}

func NewLLMClient(url, model string) *LLMClient {
	options := []option.RequestOption{option.WithBaseURL(url)}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Info("OPENAI_API_KEY environment variable is not set, will try unauthenticated access")
	} else {
		options = append(options, option.WithAPIKey(apiKey))
	}

	client := openai.NewClient(options...)
	return &LLMClient{client: &client, model: model}
}

func (llm *LLMClient) Chat(ctx context.Context, instructions, data string) (string, error) {
	resp, err := llm.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(instructions),
			openai.UserMessage(data),
		},
		Model: llm.model,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("client didn't return any content choices")
	}

	return resp.Choices[0].Message.Content, nil
}
