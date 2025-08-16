package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

type headerTransport struct {
	rt      http.RoundTripper
	headers http.Header
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating the original
	cl := req.Clone(req.Context())
	for k, vs := range t.headers {
		for _, v := range vs {
			cl.Header.Add(k, v)
		}
	}
	return t.rt.RoundTrip(cl)
}

func NewOpenAI(apiKey, baseURL, model, referrer, title string) *OpenAIClient {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	// Inject optional headers (useful for OpenRouter)
	if referrer != "" || title != "" {
		h := http.Header{}
		if referrer != "" {
			h.Set("HTTP-Referer", referrer)
		}
		if title != "" {
			h.Set("X-Title", title)
		}
		base := http.DefaultTransport
		config.HTTPClient = &http.Client{Transport: headerTransport{rt: base, headers: h}}
	}
	return &OpenAIClient{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (c *OpenAIClient) Generate(ctx context.Context, messages []Message) (Response, error) {
	return c.GenerateWithTools(ctx, messages, nil)
}

func (c *OpenAIClient) GenerateWithTools(ctx context.Context, messages []Message, tools []Tool) (Response, error) {
	var oaMsgs []openai.ChatCompletionMessage
	for _, m := range messages {
		oaMsgs = append(oaMsgs, openai.ChatCompletionMessage{Role: m.Role, Content: m.Content})
	}

	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: oaMsgs,
	}

	// Добавляем tools если они есть
	if len(tools) > 0 {
		var oaTools []openai.Tool
		for _, tool := range tools {
			oaTools = append(oaTools, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
		req.Tools = oaTools
		req.ToolChoice = "auto" // LLM решает сама когда вызывать функции
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return Response{}, fmt.Errorf("failed to create chat completion: %w", err)
	}

	out := Response{
		Content: resp.Choices[0].Message.Content,
		Model:   c.model,
	}
	out.PromptTokens = resp.Usage.PromptTokens
	out.CompletionTokens = resp.Usage.CompletionTokens
	out.TotalTokens = resp.Usage.TotalTokens

	// Обрабатываем tool calls если они есть
	if len(resp.Choices[0].Message.ToolCalls) > 0 {
		for _, tc := range resp.Choices[0].Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: parseJSONArgs(tc.Function.Arguments),
				},
			})
		}
	}

	return out, nil
}

// parseJSONArgs парсит аргументы функции из JSON строки
func parseJSONArgs(args string) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(args), &result); err != nil {
		return make(map[string]interface{})
	}
	return result
}
