package llm

import (
	"context"
	"fmt"

	"github.com/Morwran/yagpt"
)

type YandexClient struct {
	ya       yagpt.YaGPTFace
	iamToken string
}

func NewYandex(oauthToken, folderID string) (*YandexClient, error) {
	// Create IAM token from OAuth token
	iam, err := yagpt.NewYaIam(oauthToken)
	if err != nil {
		return nil, fmt.Errorf("failed to init yandex iam: %w", err)
	}
	resp, err := iam.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create iam token: %w", err)
	}

	// Create YaGPT client for a folder
	ya, err := yagpt.NewYagpt(folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to init yagpt: %w", err)
	}

	return &YandexClient{
		ya:       ya,
		iamToken: resp.IamToken,
	}, nil
}

func (c *YandexClient) GenerateText(ctx context.Context, systemPrompt string, prompt string) (Response, error) {
	var messages []yagpt.Message
	if systemPrompt != "" {
		messages = append(messages, yagpt.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, yagpt.Message{Role: "user", Content: prompt})

	resp, err := c.ya.CompletionWithCtx(ctx, c.iamToken, messages)
	if err != nil {
		return Response{}, fmt.Errorf("yagpt completion failed: %w", err)
	}
	if resp == nil || len(resp.Alternatives) == 0 {
		return Response{}, fmt.Errorf("yagpt returned empty response")
	}
	out := Response{Content: resp.Alternatives[0].Message.Content, Model: yagpt.YaModelLite}
	out.PromptTokens = int(resp.Usage.InputTextTokens)
	out.CompletionTokens = int(resp.Usage.CompletionTokens)
	out.TotalTokens = int(resp.Usage.TotalTokens)
	return out, nil
}
