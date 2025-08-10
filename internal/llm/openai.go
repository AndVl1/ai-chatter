package llm

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAI(apiKey, baseURL, model string) *OpenAIClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &OpenAIClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (c *OpenAIClient) Generate(ctx context.Context, messages []Message) (Response, error) {
	var oaMsgs []openai.ChatCompletionMessage
	for _, m := range messages {
		oaMsgs = append(oaMsgs, openai.ChatCompletionMessage{Role: m.Role, Content: m.Content})
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    c.model,
			Messages: oaMsgs,
		},
	)
	if err != nil {
		return Response{}, fmt.Errorf("failed to create chat completion: %w", err)
	}

	out := Response{Content: resp.Choices[0].Message.Content, Model: c.model}
	out.PromptTokens = resp.Usage.PromptTokens
	out.CompletionTokens = resp.Usage.CompletionTokens
	out.TotalTokens = resp.Usage.TotalTokens
	return out, nil
}
