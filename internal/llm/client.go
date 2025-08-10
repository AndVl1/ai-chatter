package llm

import "context"

type Message struct {
	Role    string
	Content string
}

type Response struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Client interface {
	Generate(ctx context.Context, messages []Message) (Response, error)
}
