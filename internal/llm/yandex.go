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

func (c *YandexClient) Generate(ctx context.Context, messages []Message) (Response, error) {
	return c.GenerateWithTools(ctx, messages, nil)
}

func (c *YandexClient) GenerateWithTools(ctx context.Context, messages []Message, tools []Tool) (Response, error) {
	// YandexGPT пока не поддерживает function calling
	// Игнорируем tools и делаем обычный запрос
	var yaMsgs []yagpt.Message
	for _, m := range messages {
		yaMsgs = append(yaMsgs, yagpt.Message{Role: m.Role, Content: m.Content})
	}

	resp, err := c.ya.CompletionWithCtx(ctx, c.iamToken, yaMsgs)
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
	// YandexGPT не поддерживает tool calls
	out.ToolCalls = nil
	return out, nil
}
