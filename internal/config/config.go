package config

import (
	"log"

	"github.com/caarlos0/env/v6"
)

type LLMProvider string

const (
	ProviderOpenAI LLMProvider = "openai"
	ProviderYandex LLMProvider = "yandex"
)

type Config struct {
	TelegramBotToken string  `env:"TELEGRAM_BOT_TOKEN,required"`
	AllowedUsers     []int64 `env:"ALLOWED_USERS" envSeparator:":"`
	AdminUserID      int64   `env:"ADMIN_USER"`

	// LLM settings
	LLMProvider      LLMProvider `env:"LLM_PROVIDER" envDefault:"openai"`
	OpenAIAPIKey     string      `env:"OPENAI_API_KEY"`
	OpenAIBaseURL    string      `env:"OPENAI_BASE_URL"`
	OpenAIModel      string      `env:"OPENAI_MODEL" envDefault:"gpt-3.5-turbo"`
	YandexOAuthToken string      `env:"YANDEX_OAUTH_TOKEN"`
	YandexFolderID   string      `env:"YANDEX_FOLDER_ID"`

	// Prompts
	SystemPromptPath string `env:"SYSTEM_PROMPT_PATH" envDefault:"prompts/system_prompt.txt"`

	// Storage
	LogFilePath       string `env:"LOG_FILE_PATH" envDefault:"logs/log.jsonl"`
	AllowlistFilePath string `env:"ALLOWLIST_FILE_PATH" envDefault:"data/allowlist.json"`
}

func New() *Config {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}
	return cfg
}
