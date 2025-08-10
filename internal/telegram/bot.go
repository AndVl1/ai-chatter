package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/pending"
	"ai-chatter/internal/storage"
)

const resetCmd = "reset_ctx"
const summaryCmd = "summary_ctx"
const approvePrefix = "approve:"
const denyPrefix = "deny:"

type Bot struct {
	api          *tgbotapi.BotAPI
	s            sender
	authSvc      *auth.Service
	systemPrompt string
	llmClient    llm.Client
	llmMu        sync.RWMutex
	history      *history.Manager
	recorder     storage.Recorder
	adminUserID  int64
	pending      map[int64]auth.User
	pendingRepo  pending.Repository
	parseMode    string
	provider     string
	model        string
	// creds for rebuilding clients
	openaiAPIKey       string
	openaiBaseURL      string
	openRouterReferrer string
	openRouterTitle    string
	yandexOAuthToken   string
	yandexFolderID     string
}

func New(
	botToken string,
	authSvc *auth.Service,
	llmClient llm.Client,
	systemPrompt string,
	rec storage.Recorder,
	adminUserID int64,
	pendingRepo pending.Repository,
	parseMode string,
	provider string,
	model string,
	openaiAPIKey string,
	openaiBaseURL string,
	openRouterReferrer string,
	openRouterTitle string,
	yandexOAuthToken string,
	yandexFolderID string,
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		api:                api,
		s:                  botAPISender{api: api},
		authSvc:            authSvc,
		systemPrompt:       systemPrompt,
		history:            history.NewManager(),
		recorder:           rec,
		adminUserID:        adminUserID,
		pending:            make(map[int64]auth.User),
		pendingRepo:        pendingRepo,
		parseMode:          parseMode,
		provider:           provider,
		model:              model,
		openaiAPIKey:       openaiAPIKey,
		openaiBaseURL:      openaiBaseURL,
		openRouterReferrer: openRouterReferrer,
		openRouterTitle:    openRouterTitle,
		yandexOAuthToken:   yandexOAuthToken,
		yandexFolderID:     yandexFolderID,
	}
	b.setLLMClient(llmClient)
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
	// Preload pending from repository
	if b.pendingRepo != nil {
		if items, err := b.pendingRepo.LoadAll(); err == nil {
			for _, u := range items {
				b.pending[u.ID] = u
			}
		}
	}
	return b, nil
}

func (b *Bot) getLLMClient() llm.Client {
	b.llmMu.RLock()
	defer b.llmMu.RUnlock()
	return b.llmClient
}

func (b *Bot) setLLMClient(c llm.Client) {
	b.llmMu.Lock()
	defer b.llmMu.Unlock()
	b.llmClient = c
}

func (b *Bot) reloadLLMClient() error {
	var newCli llm.Client
	switch strings.ToLower(b.provider) {
	case "openai":
		newCli = llm.NewOpenAI(b.openaiAPIKey, b.openaiBaseURL, b.model, b.openRouterReferrer, b.openRouterTitle)
	case "yandex":
		cli, err := llm.NewYandex(b.yandexOAuthToken, b.yandexFolderID)
		if err != nil {
			return err
		}
		newCli = cli
	default:
		return fmt.Errorf("unknown provider: %s", b.provider)
	}
	b.setLLMClient(newCli)
	return nil
}

func (b *Bot) escapeIfNeeded(s string) string {
	if strings.EqualFold(b.parseMode, string(tgbotapi.ModeMarkdownV2)) {
		return escapeMarkdownV2(s)
	}
	return s
}

func escapeMarkdownV2(s string) string {
	repl := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return repl.Replace(s)
}

func (b *Bot) parseModeValue() string {
	s := strings.ToLower(b.parseMode)
	switch s {
	case strings.ToLower(tgbotapi.ModeMarkdown), strings.ToLower(tgbotapi.ModeMarkdownV2), strings.ToLower(tgbotapi.ModeHTML):
		return b.parseMode
	default:
		return tgbotapi.ModeHTML
	}
}

func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	log.Printf("Bot started")
	if b.adminUserID != 0 {
		info := fmt.Sprintf("Бот запущен и готов к работе. Провайдер: %s, модель: %s.", b.provider, b.model)
		b.sendMessage(b.adminUserID, info)
	}

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.IsCommand() {
				if update.Message.Command() == "start" {
					b.handleStart(update.Message)
					continue
				}
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

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	welcome := "Привет! Я LLM-бот. Отвечаю на вопросы с учётом контекста. Под каждым ответом есть кнопки: ‘История’ (саммари диалога) и ‘Сбросить контекст’."
	if b.authSvc.IsAllowed(msg.From.ID) {
		b.sendMessage(msg.Chat.ID, welcome+"\n\nДоступ уже предоставлен. Можете писать сообщение.")
		return
	}
	// Not allowed: cache and request admin
	b.pending[msg.From.ID] = auth.User{ID: msg.From.ID, Username: msg.From.UserName, FirstName: msg.From.FirstName, LastName: msg.From.LastName}
	b.notifyAdminRequest(msg.From.ID, msg.From.UserName)
	b.sendMessage(msg.Chat.ID, welcome+"\n\nЗапрос на доступ отправлен администратору. Как только он подтвердит, вы получите уведомление.")
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	if msg.Command() == "provider" || msg.Command() == "model" {
		b.handleAdminConfigCommands(msg)
		return
	}
	if msg.From.ID != b.adminUserID {
		b.sendMessage(msg.Chat.ID, "Команда доступна только администратору")
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
	case "pending":
		var bld strings.Builder
		bld.WriteString("Pending заявки:\n")
		for _, u := range b.pending {
			bld.WriteString(fmt.Sprintf("- id=%d, username=@%s, name=%s %s\n", u.ID, u.Username, u.FirstName, u.LastName))
		}
		b.sendMessage(msg.Chat.ID, bld.String())
	case "approve":
		args := strings.Fields(msg.CommandArguments())
		if len(args) != 1 {
			b.sendMessage(msg.Chat.ID, "Usage: /approve <user_id>")
			return
		}
		uid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "Некорректный user_id")
			return
		}
		b.approveUser(uid, msg.Chat.ID)
	case "deny":
		args := strings.Fields(msg.CommandArguments())
		if len(args) != 1 {
			b.sendMessage(msg.Chat.ID, "Usage: /deny <user_id>")
			return
		}
		uid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "Некорректный user_id")
			return
		}
		b.denyUser(uid, msg.Chat.ID)
	}
}

func (b *Bot) handleAdminConfigCommands(msg *tgbotapi.Message) {
	if msg.From.ID != b.adminUserID {
		b.sendMessage(msg.Chat.ID, "Команда доступна только администратору")
		return
	}
	cmd := msg.Command()
	args := strings.Fields(msg.CommandArguments())
	switch cmd {
	case "provider":
		if len(args) != 1 {
			b.sendMessage(msg.Chat.ID, "Usage: /provider <openai|yandex>")
			return
		}
		prov := strings.ToLower(args[0])
		if prov != "openai" && prov != "yandex" {
			b.sendMessage(msg.Chat.ID, "Поддерживаются: openai, yandex")
			return
		}
		if err := os.WriteFile("data/provider.txt", []byte(prov), 0o644); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка сохранения: %v", err))
			return
		}
		b.provider = prov
		if err := b.reloadLLMClient(); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка перезагрузки клиента: %v", err))
			return
		}
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Провайдер установлен и применён: %s", prov))
	case "model":
		if len(args) != 1 {
			b.sendMessage(msg.Chat.ID, "Usage: /model <openai/gpt-5-nano|openai/gpt-oss-20b:free|qwen/qwen3-coder>")
			return
		}
		model := args[0]
		allowed := map[string]bool{"openai/gpt-5-nano": true, "openai/gpt-oss-20b:free": true, "qwen/qwen3-coder": true}
		if !allowed[model] {
			b.sendMessage(msg.Chat.ID, "Неподдерживаемая модель. Доступные: openai/gpt-5-nano, openai/gpt-oss-20b:free, qwen/qwen3-coder")
			return
		}
		if err := os.WriteFile("data/model.txt", []byte(model), 0o644); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка сохранения: %v", err))
			return
		}
		b.model = model
		if err := b.reloadLLMClient(); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка перезагрузки клиента: %v", err))
			return
		}
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Модель установлена и применена: %s", model))
	}
}

func (b *Bot) handleIncomingMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", msg.From.ID, msg.From.UserName)
		// If already pending, inform user and don't spam admin
		if _, ok := b.pending[msg.From.ID]; ok {
			b.sendMessage(msg.Chat.ID, "Ваш запрос на доступ уже отправлен администратору. Пожалуйста, ожидайте подтверждения. Как только доступ будет предоставлен, я уведомлю вас.")
			return
		}
		// cache pending user data and notify admin once
		u := auth.User{ID: msg.From.ID, Username: msg.From.UserName, FirstName: msg.From.FirstName, LastName: msg.From.LastName}
		b.pending[msg.From.ID] = u
		if b.pendingRepo != nil {
			_ = b.pendingRepo.Upsert(u)
		}
		b.sendMessage(msg.Chat.ID, "Запрос на доступ отправлен администратору. Как только он подтвердит, вы получите уведомление.")
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

	resp, err := b.getLLMClient().Generate(ctx, contextMsgs)
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
	metaEsc := b.escapeIfNeeded(meta)
	final := metaEsc + "\n\n" + resp.Content

	msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
	msgOut.ReplyMarkup = b.menuKeyboard()
	msgOut.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msgOut); err != nil {
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
	msg := tgbotapi.NewMessage(b.adminUserID, b.escapeIfNeeded(text))
	msg.ParseMode = b.parseModeValue()
	msg.ReplyMarkup = kb
	_, _ = b.s.Send(msg)
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	switch {
	case cb.Data == resetCmd:
		b.history.Reset(cb.From.ID)
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, "Контекст сброшен")
		msg.ParseMode = b.parseModeValue()
		if _, err := b.s.Send(msg); err != nil {
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
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("История пуста"))
		msg.ParseMode = b.parseModeValue()
		if _, err := b.s.Send(msg); err != nil {
			log.Printf("failed to send empty history notice: %v", err)
		}
		return
	}
	var msgs []llm.Message
	msgs = append(msgs, llm.Message{Role: "system", Content: "Суммируй переписку пользователя с ассистентом. Дай краткое саммари с ключевыми темами, выводами и нерешёнными вопросами. Не выдумывай факты."})
	msgs = append(msgs, h...)

	resp, err := b.getLLMClient().Generate(ctx, msgs)
	if err != nil {
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("Не удалось собрать саммари"))
		msg.ParseMode = b.parseModeValue()
		if _, err := b.s.Send(msg); err != nil {
			log.Printf("failed to send summary error: %v", err)
		}
		return
	}

	log.Printf("Summary [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)
	b.history.AppendUser(cb.From.ID, "[команда] история")
	b.history.AppendAssistant(cb.From.ID, resp.Content)
	if b.recorder != nil {
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: cb.From.ID, UserMessage: "[команда] история", AssistantResponse: ""})
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: cb.From.ID, UserMessage: "", AssistantResponse: resp.Content})
	}

	meta := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	metaEsc := b.escapeIfNeeded(meta)
	final := metaEsc + "\n\n" + resp.Content
	msg := tgbotapi.NewMessage(cb.Message.Chat.ID, final)
	msg.ParseMode = b.parseModeValue()
	msg.ReplyMarkup = b.menuKeyboard()
	if _, err := b.s.Send(msg); err != nil {
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
		b.approveUser(userID, cb.Message.Chat.ID)
	} else {
		b.denyUser(userID, cb.Message.Chat.ID)
	}
}

func (b *Bot) approveUser(userID int64, notifyChatID int64) {
	u := b.pending[userID]
	if u.ID == 0 { // fallback if no pending cache
		u = auth.User{ID: userID}
	}
	_ = b.authSvc.Upsert(u)
	delete(b.pending, userID)
	if b.pendingRepo != nil {
		_ = b.pendingRepo.Remove(userID)
	}
	msg := tgbotapi.NewMessage(notifyChatID, fmt.Sprintf("Пользователь %d разрешен", userID))
	msg.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msg); err != nil {
		log.Printf("failed to notify approval: %v", err)
	}
	msg2 := tgbotapi.NewMessage(userID, "Доступ к боту разрешён. Можете пользоваться.")
	msg2.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msg2); err != nil {
		log.Printf("failed to notify user approval: %v", err)
	}
}

func (b *Bot) denyUser(userID int64, notifyChatID int64) {
	_ = b.authSvc.Remove(userID)
	delete(b.pending, userID)
	if b.pendingRepo != nil {
		_ = b.pendingRepo.Remove(userID)
	}
	msg := tgbotapi.NewMessage(notifyChatID, fmt.Sprintf("Пользователю %d отказано", userID))
	msg.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msg); err != nil {
		log.Printf("failed to notify denial: %v", err)
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
	msg := tgbotapi.NewMessage(chatID, b.escapeIfNeeded(text))
	msg.ParseMode = b.parseModeValue()
	_, _ = b.s.Send(msg)
}
