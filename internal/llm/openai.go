package llm

import (
	"context"
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

func NewOpenAI(apiKey, baseURL, model string, referrer, title string) *OpenAIClient {
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
