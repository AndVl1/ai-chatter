package telegram

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
	"ai-chatter/internal/llm"
)

const resetCmd = "reset_ctx"

type Bot struct {
	api          *tgbotapi.BotAPI
	authSvc      *auth.Service
	llmClient    llm.Client
	systemPrompt string
	history      *history.Manager
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
		history:      history.NewManager(),
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleIncomingMessage(ctx, update.Message)
			continue
		}
		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
			continue
		}
	}
}

func (b *Bot) handleIncomingMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", msg.From.ID, msg.From.UserName)
		b.sendMessage(msg.Chat.ID, "запрос отправлен на проверку")
		return
	}

	log.Printf("Incoming message from %d (@%s): %q", msg.From.ID, msg.From.UserName, msg.Text)

	// Update history
	b.history.AppendUser(msg.From.ID, msg.Text)

	// Build context: system + history
	var contextMsgs []llm.Message
	if b.systemPrompt != "" {
		contextMsgs = append(contextMsgs, llm.Message{Role: "system", Content: b.systemPrompt})
	}
	contextMsgs = append(contextMsgs, b.history.Get(msg.From.ID)...)

	resp, err := b.llmClient.Generate(ctx, contextMsgs)
	if err != nil {
		log.Printf("failed to generate text: %v", err)
		b.sendMessage(msg.Chat.ID, "Sorry, something went wrong.")
		return
	}

	// Save assistant response into history
	b.history.AppendAssistant(msg.From.ID, resp.Content)

	log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q",
		resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)

	meta := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	final := meta + "\n\n" + resp.Content

	// Reply with inline button to reset context
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Сбросить контекст", resetCmd),
		),
	)

	msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
	msgOut.ReplyMarkup = kb
	if _, err := b.api.Send(msgOut); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Data == resetCmd {
		b.history.Reset(cb.From.ID)
		edit := tgbotapi.NewMessage(cb.Message.Chat.ID, "Контекст сброшен")
		if _, err := b.api.Send(edit); err != nil {
			log.Printf("failed to send reset confirmation: %v", err)
		}
		return
	}
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}
