package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/config"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/pending"
	"ai-chatter/internal/storage"
	"ai-chatter/internal/telegram"
)

func main() {
	// Try several common locations for .env
	if err := godotenv.Load(".env" /*, "../.env", "cmd/bot/.env"*/); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	cfg := config.New()

	var allowRepo auth.Repository
	if cfg.AllowlistFilePath != "" {
		repo, err := auth.NewFileRepository(cfg.AllowlistFilePath)
		if err != nil {
			log.Printf("failed to init allowlist repo: %v", err)
		} else {
			allowRepo = repo
		}
	}

	authSvc, err := auth.NewWithRepo(allowRepo, cfg.AllowedUsers)
	if err != nil {
		log.Fatalf("failed to init auth: %v", err)
	}

	// Resolve provider/model with overrides
	prov := string(cfg.LLMProvider)
	if s := readTrim(cfg.ProviderFilePath); s != "" {
		prov = s
	}
	model := cfg.OpenAIModel
	if s := readTrim(cfg.ModelFilePath); s != "" {
		model = s
	}

	llmFactory := llm.NewFactory(cfg)
	llmClient, err := llmFactory.CreateClient(prov, model)
	if err != nil {
		log.Fatalf("failed to create llm client: %v", err)
	}

	systemPrompt := readSystemPrompt(cfg.SystemPromptPath)

	var rec storage.Recorder
	if cfg.LogFilePath != "" {
		fr, err := storage.NewFileRecorder(cfg.LogFilePath)
		if err != nil {
			log.Printf("failed to init file recorder: %v", err)
		} else {
			rec = fr
		}
	}

	var pRepo pending.Repository
	if cfg.PendingFilePath != "" {
		pr, err := pending.NewFileRepository(cfg.PendingFilePath)
		if err != nil {
			log.Printf("failed to init pending repo: %v", err)
		} else {
			pRepo = pr
		}
	}

	bot, err := telegram.New(
		cfg.TelegramBotToken,
		authSvc,
		llmClient,
		llmFactory,
		systemPrompt,
		rec,
		cfg.AdminUserID,
		pRepo,
		cfg.MessageParseMode,
		prov,
		model,
	)
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

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}


