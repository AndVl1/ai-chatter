package telegram

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/storage"
)

const resetCmd = "reset_ctx"
const summaryCmd = "summary_ctx"

type Bot struct {
	api          *tgbotapi.BotAPI
	authSvc      *auth.Service
	llmClient    llm.Client
	systemPrompt string
	history      *history.Manager
	recorder     storage.Recorder
}

func New(botToken string, authSvc *auth.Service, llmClient llm.Client, systemPrompt string, rec storage.Recorder) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		api:          api,
		authSvc:      authSvc,
		llmClient:    llmClient,
		systemPrompt: systemPrompt,
		history:      history.NewManager(),
		recorder:     rec,
	}
	// Preload history from recorder
	if rec != nil {
		if events, err := rec.LoadInteractions(); err == nil {
			for _, ev := range events {
				if ev.UserID == 0 {
					continue
				}
				if ev.UserMessage != "" {
					b.history.AppendUser(ev.UserID, ev.UserMessage)
				}
				if ev.AssistantResponse != "" {
					b.history.AppendAssistant(ev.UserID, ev.AssistantResponse)
				}
			}
		}
	}
	return b, nil
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
			b.handleCallback(ctx, update.CallbackQuery)
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

	// Update history and record
	b.history.AppendUser(msg.From.ID, msg.Text)
	if b.recorder != nil {
		_ = b.recorder.AppendInteraction(storage.Event{
			Timestamp:         time.Now().UTC(),
			UserID:            msg.From.ID,
			UserMessage:       msg.Text,
			AssistantResponse: "",
		})
	}

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

	// Save assistant response into history and record
	b.history.AppendAssistant(msg.From.ID, resp.Content)
	if b.recorder != nil {
		_ = b.recorder.AppendInteraction(storage.Event{
			Timestamp:         time.Now().UTC(),
			UserID:            msg.From.ID,
			UserMessage:       "",
			AssistantResponse: resp.Content,
		})
	}

	log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q",
		resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)

	meta := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	final := meta + "\n\n" + resp.Content

	msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
	msgOut.ReplyMarkup = b.menuKeyboard()
	if _, err := b.api.Send(msgOut); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	switch cb.Data {
	case resetCmd:
		b.history.Reset(cb.From.ID)
		if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, "Контекст сброшен")); err != nil {
			log.Printf("failed to send reset confirmation: %v", err)
		}
	case summaryCmd:
		// Build summary request over user's history
		h := b.history.Get(cb.From.ID)
		if len(h) == 0 {
			if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, "История пуста")); err != nil {
				log.Printf("failed to send empty history notice: %v", err)
			}
			return
		}
		var msgs []llm.Message
		msgs = append(msgs, llm.Message{Role: "system", Content: "Суммируй переписку пользователя с ассистентом. Дай краткое саммари с ключевыми темами, выводами и нерешёнными вопросами. Не выдумывай факты."})
		msgs = append(msgs, h...)

		resp, err := b.llmClient.Generate(ctx, msgs)
		if err != nil {
			log.Printf("failed to generate summary: %v", err)
			if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, "Не удалось собрать саммари")); err != nil {
				log.Printf("failed to send summary error: %v", err)
			}
			return
		}

		// Log and store summary in history and recorder
		log.Printf("Summary [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q",
			resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)
		b.history.AppendUser(cb.From.ID, "[команда] история")
		b.history.AppendAssistant(cb.From.ID, resp.Content)
		if b.recorder != nil {
			_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: cb.From.ID, UserMessage: "[команда] история", AssistantResponse: ""})
			_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: cb.From.ID, UserMessage: "", AssistantResponse: resp.Content})
		}

		meta := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
		final := meta + "\n\n" + resp.Content
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, final)
		msg.ReplyMarkup = b.menuKeyboard()
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("failed to send summary: %v", err)
		}
	}
}

func (b *Bot) menuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Сбросить контекст", resetCmd),
			tgbotapi.NewInlineKeyboardButtonData("История", summaryCmd),
		),
	)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to send message: %v", err)
	}
}
