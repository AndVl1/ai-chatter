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

func (c *OpenAIClient) GenerateText(ctx context.Context, systemPrompt string, prompt string) (Response, error) {
	messages := []openai.ChatCompletionMessage{}
	if systemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    c.model,
			Messages: messages,
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
