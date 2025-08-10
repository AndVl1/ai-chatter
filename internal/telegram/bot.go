package telegram

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/llm"
)

type Bot struct {
	api           *tgbotapi.BotAPI
	authSvc       *auth.Service
	llmClient     llm.Client
	systemPrompt  string
}

func New(botToken string, authSvc *auth.Service, llmClient llm.Client, systemPrompt string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:          api,
		authSvc:      authSvc,
		llmClient:    llmClient,
		systemPrompt: systemPrompt,
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if !b.authSvc.IsAllowed(update.Message.From.ID) {
			log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", update.Message.From.ID, update.Message.From.UserName)
			b.sendMessage(update.Message.Chat.ID, "запрос отправлен на проверку")
			continue
		}

		log.Printf("Incoming message from %d (@%s): %q", update.Message.From.ID, update.Message.From.UserName, update.Message.Text)
		go b.handleMessage(ctx, update.Message)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	resp, err := b.llmClient.GenerateText(ctx, b.systemPrompt, msg.Text)
	if err != nil {
		log.Printf("failed to generate text: %v", err)
		b.sendMessage(msg.Chat.ID, "Sorry, something went wrong.")
		return
	}

	log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q",
		resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)

	meta := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	final := meta + "\n\n" + resp.Content
	b.sendMessage(msg.Chat.ID, final)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}
