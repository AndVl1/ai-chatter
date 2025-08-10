package llm

import "context"

type Response struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Client interface {
	GenerateText(ctx context.Context, systemPrompt string, prompt string) (Response, error)
}
