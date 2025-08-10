package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/config"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/telegram"
)

func main() {
	// Try several common locations for .env
	if err := godotenv.Load(".env" /*, "../.env", "cmd/bot/.env"*/); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	cfg := config.New()

	authSvc := auth.New(cfg.AllowedUsers)

	llmClient, err := newLLMClient(cfg)
	if err != nil {
		log.Fatalf("failed to create llm client: %v", err)
	}

	systemPrompt := readSystemPrompt(cfg.SystemPromptPath)

	bot, err := telegram.New(cfg.TelegramBotToken, authSvc, llmClient, systemPrompt)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	bot.Start(context.Background())
}

func readSystemPrompt(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("system prompt file not found or unreadable at %s: %v", path, err)
		return ""
	}
	return string(data)
}

func newLLMClient(cfg *config.Config) (llm.Client, error) {
	switch cfg.LLMProvider {
	case config.ProviderOpenAI:
		return llm.NewOpenAI(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.OpenAIModel), nil
	case config.ProviderYandex:
		return llm.NewYandex(cfg.YandexOAuthToken, cfg.YandexFolderID)
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", cfg.LLMProvider)
	}
}
