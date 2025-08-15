package llm

import (
	"fmt"
	"strings"

	"ai-chatter/internal/config"
)

const (
	ProviderOpenAI = "openai"
	ProviderYandex = "yandex"
)

var AllowedModels = map[string]bool{
	"openai/gpt-5-nano":              true,
	"openai/gpt-oss-20b:free":        true,
	"qwen/qwen3-coder":               true,
	"z-ai/glm-4.5-air:free":          true,
	"qwen/qwen3-coder:free":          true,
	"google/gemini-2.5-flash-lite":   true,
	"deepseek/deepseek-r1-0528:free": true,
}

// Factory creates LLM clients with consistent logic
type Factory struct {
	OpenaiAPIKey       string
	OpenaiBaseURL      string
	OpenRouterReferrer string
	OpenRouterTitle    string
	YandexOAuthToken   string
	YandexFolderID     string
}

func NewFactory(cfg *config.Config) *Factory {
	return &Factory{
		OpenaiAPIKey:       cfg.OpenAIAPIKey,
		OpenaiBaseURL:      cfg.OpenAIBaseURL,
		OpenRouterReferrer: cfg.OpenRouterReferrer,
		OpenRouterTitle:    cfg.OpenRouterTitle,
		YandexOAuthToken:   cfg.YandexOAuthToken,
		YandexFolderID:     cfg.YandexFolderID,
	}
}

func (f *Factory) CreateClient(provider, model string) (Client, error) {
	switch strings.ToLower(provider) {
	case ProviderOpenAI:
		return NewOpenAI(f.OpenaiAPIKey, f.OpenaiBaseURL, model, f.OpenRouterReferrer, f.OpenRouterTitle), nil
	case ProviderYandex:
		return NewYandex(f.YandexOAuthToken, f.YandexFolderID)
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", provider)
	}
}

func IsModelAllowed(model string) bool {
	return AllowedModels[model]
}

func GetAllowedModels() []string {
	models := make([]string, 0, len(AllowedModels))
	for model := range AllowedModels {
		models = append(models, model)
	}
	return models
}
