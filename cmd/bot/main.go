package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/config"
	"ai-chatter/internal/github"
	"ai-chatter/internal/gmail"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/notion"
	"ai-chatter/internal/pending"
	"ai-chatter/internal/rustore"
	"ai-chatter/internal/scheduler"
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

	// Initialize Notion MCP client
	var mcpClient *notion.MCPClient
	if cfg.NotionToken != "" {
		mcpClient = notion.NewMCPClient(cfg.NotionToken)

		// Подключаемся к MCP серверу
		ctx := context.Background()
		if err := mcpClient.Connect(ctx, cfg.NotionToken); err != nil {
			log.Printf("⚠️ Failed to connect to Notion MCP server: %v", err)
			log.Printf("Notion functionality will be disabled")
			mcpClient = nil
		} else {
			log.Printf("✅ Notion MCP client connected successfully")
		}
	} else {
		log.Printf("NOTION_TOKEN not set, Notion functionality disabled")
	}

	// Initialize Gmail MCP client
	var gmailClient *gmail.GmailMCPClient
	gmailCredentials := os.Getenv("GMAIL_CREDENTIALS_JSON")

	// Если не задано прямо, пытаемся прочитать из файла
	if gmailCredentials == "" {
		if credentialsPath := os.Getenv("GMAIL_CREDENTIALS_JSON_PATH"); credentialsPath != "" {
			if credentialsData, err := os.ReadFile(credentialsPath); err == nil {
				gmailCredentials = string(credentialsData)
			}
		}
	}

	if gmailCredentials != "" {
		gmailClient = gmail.NewGmailMCPClient()

		// Подключаемся к Gmail MCP серверу
		ctx := context.Background()
		if err := gmailClient.Connect(ctx, gmailCredentials); err != nil {
			log.Printf("⚠️ Failed to connect to Gmail MCP server: %v", err)
			log.Printf("Gmail functionality will be disabled")
			gmailClient = nil
		} else {
			log.Printf("✅ Gmail MCP client connected successfully")
		}
	} else {
		log.Printf("GMAIL_CREDENTIALS_JSON or GMAIL_CREDENTIALS_JSON_PATH not set, Gmail functionality disabled")
	}

	// Initialize GitHub MCP client
	var githubClient *github.GitHubMCPClient
	githubToken := os.Getenv("GITHUB_TOKEN")

	log.Printf("🔍 Bot: Checking GitHub token...")
	log.Printf("📦 Bot: GITHUB_TOKEN available: %v", githubToken != "")

	if githubToken != "" {
		// Показываем маскированный токен для отладки
		if len(githubToken) > 8 {
			maskedToken := githubToken[:4] + "..." + githubToken[len(githubToken)-4:]
			log.Printf("🔑 Bot: GitHub token: %s (length: %d)", maskedToken, len(githubToken))
		}

		githubClient = github.NewGitHubMCPClient()

		// Подключаемся к GitHub MCP серверу
		ctx := context.Background()
		if err := githubClient.Connect(ctx, githubToken); err != nil {
			log.Printf("⚠️ Failed to connect to GitHub MCP server: %v", err)
			log.Printf("GitHub functionality will be disabled")
			githubClient = nil
		} else {
			log.Printf("✅ GitHub MCP client connected successfully")
		}
	} else {
		log.Printf("❌ Bot: GITHUB_TOKEN not set, GitHub functionality disabled")
		log.Printf("💡 Bot: Please set GITHUB_TOKEN environment variable")
	}

	// Initialize RuStore MCP client
	var rustoreClient *rustore.RuStoreMCPClient
	// RuStore клиент инициализируется без токена - авторизация будет происходить при использовании
	rustoreClient = rustore.NewRuStoreMCPClient()

	// Подключаемся к RuStore MCP серверу
	ctx := context.Background()
	if err := rustoreClient.Connect(ctx); err != nil {
		log.Printf("⚠️ Failed to connect to RuStore MCP server: %v", err)
		log.Printf("RuStore functionality will be disabled")
		rustoreClient = nil
	} else {
		log.Printf("✅ RuStore MCP client connected successfully")
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
		mcpClient,
		cfg.NotionParentPage,
		gmailClient,
		githubClient,
		rustoreClient,
	)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	// Инициализируем и запускаем планировщик
	sched := scheduler.New()
	sched.SetReportFunction(func(ctx context.Context) error {
		return bot.GenerateDailyReportForAdmin(ctx)
	})

	if err := sched.Start(); err != nil {
		log.Printf("⚠️ Failed to start scheduler: %v", err)
	}

	// Настраиваем graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("🛑 Получен сигнал остановки, завершаем работу...")
		sched.Stop()
		cancel()
	}()

	bot.Start(ctx)
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
