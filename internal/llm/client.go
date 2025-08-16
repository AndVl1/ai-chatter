package llm

import "context"

type Message struct {
	Role    string
	Content string
}

// FunctionCall представляет вызов функции от LLM
type FunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCall представляет вызов инструмента
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// Function описывает доступную функцию для LLM
type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Tool представляет инструмент для LLM
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Response struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// Function calling support
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type Client interface {
	Generate(ctx context.Context, messages []Message) (Response, error)
	GenerateWithTools(ctx context.Context, messages []Message, tools []Tool) (Response, error)
}
