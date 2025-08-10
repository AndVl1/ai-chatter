package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/storage"
)

const resetCmd = "reset_ctx"
const summaryCmd = "summary_ctx"
const approvePrefix = "approve:"
const denyPrefix = "deny:"

type Bot struct {
	api          *tgbotapi.BotAPI
	authSvc      *auth.Service
	systemPrompt string
	llmClient    llm.Client
	history      *history.Manager
	recorder     storage.Recorder
	adminUserID  int64
	pending      map[int64]auth.User
}

func New(botToken string, authSvc *auth.Service, llmClient llm.Client, systemPrompt string, rec storage.Recorder, adminUserID int64) (*Bot, error) {
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
		adminUserID:  adminUserID,
		pending:      make(map[int64]auth.User),
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
			if update.Message.IsCommand() {
				b.handleCommand(update.Message)
				continue
			}
			b.handleIncomingMessage(ctx, update.Message)
			continue
		}
		if update.CallbackQuery != nil {
			b.handleCallback(ctx, update.CallbackQuery)
			continue
		}
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	if msg.From.ID != b.adminUserID {
		return
	}
	switch msg.Command() {
	case "allowlist":
		var bld strings.Builder
		bld.WriteString("Allowlist:\n")
		for _, u := range b.authSvc.List() {
			bld.WriteString(fmt.Sprintf("- id=%d, username=@%s, name=%s %s\n", u.ID, u.Username, u.FirstName, u.LastName))
		}
		b.sendMessage(msg.Chat.ID, bld.String())
	case "remove":
		args := strings.Fields(msg.CommandArguments())
		if len(args) != 1 {
			b.sendMessage(msg.Chat.ID, "Usage: /remove <user_id>")
			return
		}
		uid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "Некорректный user_id")
			return
		}
		if err := b.authSvc.Remove(uid); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка удаления: %v", err))
			return
		}
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Пользователь %d удален из allowlist", uid))
	}
}

func (b *Bot) handleIncomingMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", msg.From.ID, msg.From.UserName)
		// cache pending user data
		b.pending[msg.From.ID] = auth.User{ID: msg.From.ID, Username: msg.From.UserName, FirstName: msg.From.FirstName, LastName: msg.From.LastName}
		b.sendMessage(msg.Chat.ID, "запрос отправлен на проверку")
		b.notifyAdminRequest(msg.From.ID, msg.From.UserName)
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

func (b *Bot) notifyAdminRequest(userID int64, username string) {
	if b.adminUserID == 0 {
		return
	}
	text := fmt.Sprintf("Пользователь @%s с id %d хочет пользоваться ботом", username, userID)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("разрешить", approvePrefix+strconv.FormatInt(userID, 10)),
			tgbotapi.NewInlineKeyboardButtonData("запретить", denyPrefix+strconv.FormatInt(userID, 10)),
		),
	)
	msg := tgbotapi.NewMessage(b.adminUserID, text)
	msg.ReplyMarkup = kb
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("failed to notify admin: %v", err)
	}
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	switch {
	case cb.Data == resetCmd:
		b.history.Reset(cb.From.ID)
		if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, "Контекст сброшен")); err != nil {
			log.Printf("failed to send reset confirmation: %v", err)
		}
	case cb.Data == summaryCmd:
		b.handleSummary(ctx, cb)
	case len(cb.Data) > len(approvePrefix) && cb.Data[:len(approvePrefix)] == approvePrefix:
		b.handleApproval(cb, true)
	case len(cb.Data) > len(denyPrefix) && cb.Data[:len(denyPrefix)] == denyPrefix:
		b.handleApproval(cb, false)
	}
}

func (b *Bot) handleSummary(ctx context.Context, cb *tgbotapi.CallbackQuery) {
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

func (b *Bot) handleApproval(cb *tgbotapi.CallbackQuery, approve bool) {
	idStr := cb.Data
	pref := denyPrefix
	if approve {
		pref = approvePrefix
	}
	userID, err := strconv.ParseInt(idStr[len(pref):], 10, 64)
	if err != nil {
		return
	}
	if approve {
		u := b.pending[userID]
		if u.ID == 0 { // fallback if no pending cache
			u = auth.User{ID: userID}
		}
		_ = b.authSvc.Upsert(u)
		delete(b.pending, userID)
		if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, fmt.Sprintf("Пользователь %d разрешен", userID))); err != nil {
			log.Printf("failed to notify approval: %v", err)
		}
		// Notify the user about approval
		if _, err := b.api.Send(tgbotapi.NewMessage(userID, "Доступ к боту разрешён. Можете пользоваться.")); err != nil {
			log.Printf("failed to notify user approval: %v", err)
		}
	} else {
		_ = b.authSvc.Remove(userID)
		delete(b.pending, userID)
		if _, err := b.api.Send(tgbotapi.NewMessage(cb.Message.Chat.ID, fmt.Sprintf("Пользователю %d отказано", userID))); err != nil {
			log.Printf("failed to notify denial: %v", err)
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
