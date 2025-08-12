package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
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

const (
	resetCmd        = "reset_ctx"
	summaryCmd      = "summary_ctx"
	approvePrefix   = "approve:"
	denyPrefix      = "deny:"
	maxContextChars = 16000
	spUpdateMarker  = "[system_prompt_update]"
	// TZ conversation limit (assistant clarification turns)
	tzMaxSteps = 15
)

type Bot struct {
	api                *tgbotapi.BotAPI
	s                  sender
	authSvc            *auth.Service
	systemPrompt       string
	llmClient          llm.Client
	llmMu              sync.RWMutex
	history            *history.Manager
	recorder           storage.Recorder
	adminUserID        int64
	pending            map[int64]auth.User
	pendingRepo        pending.Repository
	parseMode          string
	provider           string
	model              string
	openaiAPIKey       string
	openaiBaseURL      string
	openRouterReferrer string
	openRouterTitle    string
	yandexOAuthToken   string
	yandexFolderID     string
	userSysMu          sync.RWMutex
	userSystemPrompt   map[int64]string
	tzMu               sync.RWMutex
	tzMode             map[int64]bool
	// per-user remaining steps in TZ mode
	tzRemaining map[int64]int
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
		userSystemPrompt:   make(map[int64]string),
		tzMode:             make(map[int64]bool),
		tzRemaining:        make(map[int64]int),
	}
	b.setLLMClient(llmClient)
	if rec != nil {
		if events, err := rec.LoadInteractions(); err == nil {
			for _, ev := range events {
				if ev.UserID == 0 {
					continue
				}
				if ev.UserMessage == spUpdateMarker && ev.AssistantResponse != "" {
					b.addUserSystemPromptInternal(ev.UserID, ev.AssistantResponse, false)
					continue
				}
				used := true
				if ev.CanUse != nil {
					used = *ev.CanUse
				}
				if ev.UserMessage != "" {
					b.history.AppendUserWithUsed(ev.UserID, ev.UserMessage, used)
				}
				if ev.AssistantResponse != "" {
					b.history.AppendAssistantWithUsed(ev.UserID, ev.AssistantResponse, used)
				}
			}
		}
	}
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
	pm := strings.ToLower(b.parseModeValue())
	switch pm {
	case strings.ToLower(tgbotapi.ModeMarkdownV2):
		return escapeMarkdownV2(s)
	case strings.ToLower(tgbotapi.ModeHTML):
		return html.EscapeString(s)
	default:
		return s
	}
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
		return tgbotapi.ModeMarkdown
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
	if msg.Command() == "tz" {
		if !b.authSvc.IsAllowed(msg.From.ID) {
			return
		}
		topic := strings.TrimSpace(msg.CommandArguments())
		addition := "Requirements elicitation mode (Technical Specification). Your job is to iteratively clarify and assemble a complete TS in Russian for the topic: '" + topic + "'. " +
			"Ask up to 5 highly targeted questions per turn until you are confident the TS is complete. Focus on: scope/goals, user roles, environment, constraints (budget/time/tech), functional and non-functional requirements, data and integrations, dependencies, acceptance criteria, risks/mitigations, deliverables and plan. " +
			"When asking questions, prefer concrete options (multiple-choice) and short free-form fields; personalize questions to the user’s previous answers (e.g., preferred and unwanted ingredients, platforms, APIs, performance targets). " +
			"Always respond strictly in JSON {title, answer, compressed_context, status}. Set status='continue' while clarifying. When the TS is fully ready, set status='final'. If your context window is >= 80% full, include 'compressed_context' with a compact string summary of essential facts/decisions to continue without previous messages. You have at most 15 messages to clarify before finalization."
		b.addUserSystemPrompt(msg.From.ID, addition)
		b.setTZMode(msg.From.ID, true)
		b.setTZRemaining(msg.From.ID, tzMaxSteps)
		seed := "Тема ТЗ: " + topic
		b.history.AppendUser(msg.From.ID, seed)
		if b.recorder != nil {
			tru := true
			_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: msg.From.ID, UserMessage: seed, CanUse: &tru})
		}
		ctx := context.Background()
		contextMsgs := b.buildContextWithOverflow(ctx, msg.From.ID)
		if b.isTZMode(msg.From.ID) {
			left := b.getTZRemaining(msg.From.ID)
			if left > 0 && left <= 2 {
				accel := "Осталось очень мало сообщений для уточнений (<=2). Сократи количество вопросов и постарайся завершить формирование ТЗ как можно скорее. Если возможно — финализируй уже в этом ответе (status='final')."
				contextMsgs = append([]llm.Message{{Role: "system", Content: accel}}, contextMsgs...)
			}
		}
		b.logLLMRequest(msg.From.ID, "tz_bootstrap", contextMsgs)
		resp, err := b.getLLMClient().Generate(ctx, contextMsgs)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "Не удалось стартовать режим ТЗ, попробуйте ещё раз.")
			log.Println(err)
			return
		}
		log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)
		var parsed llmJSON
		ok := false
		if p1, ok1 := parseLLMJSON(resp.Content); ok1 {
			parsed = p1
			ok = true
		} else {
			if p2, ok2 := b.reformatToSchema(ctx, msg.From.ID, resp.Content); ok2 {
				parsed = p2
				ok = true
			}
		}
		titleToSend := ""
		answerToSend := resp.Content
		status := ""
		if ok {
			titleToSend = parsed.Title
			if parsed.Answer != "" {
				answerToSend = parsed.Answer
			}
			if strings.TrimSpace(parsed.CompressedContext) != "" {
				b.addUserSystemPrompt(msg.From.ID, parsed.CompressedContext)
				b.history.DisableAll(msg.From.ID)
			}
			status = strings.ToLower(strings.TrimSpace(parsed.Status))
		}
		if b.isTZMode(msg.From.ID) && status != "final" {
			left := b.decTZRemaining(msg.From.ID)
			if left <= 0 {
				if pFinal, respFinal, okFinal := b.produceFinalTS(ctx, msg.From.ID); okFinal {
					answerToSend = pFinal.Answer
					if pFinal.Title != "" {
						answerToSend = b.formatTitleAnswer(pFinal.Title, pFinal.Answer)
					}
					status = "final"
					// override meta line tokens with final response
					metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", respFinal.Model, respFinal.PromptTokens, respFinal.CompletionTokens, respFinal.TotalTokens)
					metaEsc := b.escapeIfNeeded(metaLine)
					body := answerToSend
					if titleToSend != "" {
						body = b.formatTitleAnswer(titleToSend, answerToSend)
					}
					pm := strings.ToLower(b.parseModeValue())
					var header string
					switch pm {
					case strings.ToLower(tgbotapi.ModeHTML):
						header = "<b>ТЗ Готово</b>"
					case strings.ToLower(tgbotapi.ModeMarkdownV2):
						header = escapeMarkdownV2("ТЗ Готово")
					default:
						header = "**ТЗ Готово**"
					}
					body = header + "\n\n" + body
					final := metaEsc + "\n\n" + body
					msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
					msgOut.ParseMode = b.parseModeValue()
					msgOut.ReplyMarkup = b.menuKeyboard()
					_, _ = b.s.Send(msgOut)
					b.clearTZState(msg.From.ID)
					return
				}
			}
		}
		// normal sending path continues below (unchanged)
		metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
		metaEsc := b.escapeIfNeeded(metaLine)
		body := answerToSend
		if titleToSend != "" {
			body = b.formatTitleAnswer(titleToSend, answerToSend)
		}
		if status == "final" && b.isTZMode(msg.From.ID) {
			pm := strings.ToLower(b.parseModeValue())
			var header string
			switch pm {
			case strings.ToLower(tgbotapi.ModeHTML):
				header = "<b>ТЗ Готово</b>"
			case strings.ToLower(tgbotapi.ModeMarkdownV2):
				header = escapeMarkdownV2("ТЗ Готово")
			default:
				header = "**ТЗ Готово**"
			}
			body = header + "\n\n" + body
			b.clearTZState(msg.From.ID)
		}
		final := metaEsc + "\n\n" + body
		msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
		msgOut.ParseMode = b.parseModeValue()
		msgOut.ReplyMarkup = b.menuKeyboard()
		if _, err := b.s.Send(msgOut); err != nil {
			log.Printf("failed to send tz bootstrap message: %v", err)
		}
		return
	}
	// admin-only commands...
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

// JSON parsing

type llmJSON struct {
	Title             string `json:"title"`
	Answer            string `json:"answer"`
	CompressedContext string `json:"compressed_context"`
	Status            string `json:"status"`
}

type llmJSONFlexible struct {
	Title             string          `json:"title"`
	Answer            string          `json:"answer"`
	CompressedContext json.RawMessage `json:"compressed_context"`
	Status            string          `json:"status"`
}

func compactJSON(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var any interface{}
	if err := json.Unmarshal(raw, &any); err != nil {
		return "", false
	}
	b, err := json.Marshal(any)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func parseLLMJSON(s string) (llmJSON, bool) {
	var v llmJSON
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		if v.Title != "" || v.Answer != "" || v.CompressedContext != "" || v.Status != "" {
			return v, true
		}
	}
	var f llmJSONFlexible
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		return llmJSON{}, false
	}
	cc, _ := compactJSON(f.CompressedContext)
	return llmJSON{Title: f.Title, Answer: f.Answer, CompressedContext: cc, Status: f.Status}, true
}

func (b *Bot) formatTitleAnswer(title, answer string) string {
	pm := strings.ToLower(b.parseModeValue())
	switch pm {
	case strings.ToLower(tgbotapi.ModeHTML):
		// Preserve answer formatting as-is; escape only title
		return fmt.Sprintf("<b>%s</b>\n\n%s", html.EscapeString(title), answer)
	case strings.ToLower(tgbotapi.ModeMarkdownV2):
		// Preserve answer; escape title
		return fmt.Sprintf("%s\n\n%s", escapeMarkdownV2(title), answer)
	default: // Markdown
		return fmt.Sprintf("%s\n\n%s", title, answer)
	}
}

// Context management

func sizeOfMessages(msgs []llm.Message) int {
	t := 0
	for _, m := range msgs {
		t += len(m.Content)
	}
	return t
}

func truncateForLog(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

func (b *Bot) logLLMRequest(userID int64, purpose string, msgs []llm.Message) {
	var bld strings.Builder
	bld.WriteString(fmt.Sprintf("LLM request | user=%d | purpose=%s | provider=%s | model=%s | messages=%d\n", userID, purpose, b.provider, b.model, len(msgs)))
	for i, m := range msgs {
		content := truncateForLog(m.Content, 1500)
		bld.WriteString(fmt.Sprintf("  [%d] role=%s len=%d\n      %s\n", i, m.Role, len(m.Content), content))
	}
	log.Printf(bld.String())
}

// Retry to conform to schema
func (b *Bot) reformatToSchema(ctx context.Context, userID int64, raw string) (llmJSON, bool) {
	instr := "You are a formatter. Reformat the previous output strictly into a JSON object with exactly these fields: {title, answer, compressed_context, status}. Values: status must be one of ['continue','final']. Do not add other top-level keys. Do not change content, only structure."
	msgs := []llm.Message{{Role: "system", Content: instr}, {Role: "user", Content: raw}}
	b.logLLMRequest(userID, "reformat_to_schema", msgs)
	resp, err := b.getLLMClient().Generate(ctx, msgs)
	if err != nil {
		return llmJSON{}, false
	}
	p, ok := parseLLMJSON(resp.Content)
	return p, ok
}

// Context build no longer proactively compresses
func (b *Bot) buildContextWithOverflow(ctx context.Context, userID int64) []llm.Message {
	var msgs []llm.Message
	sys := b.getUserSystemPrompt(userID)
	if sys != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: sys})
	}
	msgs = append(msgs, b.history.Get(userID)...)
	_ = ctx
	return msgs
}

// Command handling additions

func (b *Bot) handleIncomingMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", msg.From.ID, msg.From.UserName)
		if _, ok := b.pending[msg.From.ID]; ok {
			b.sendMessage(msg.Chat.ID, "Ваш запрос на доступ уже отправлен администратору. Пожалуйста, ожидайте подтверждения. Как только доступ будет предоставлен, я уведомлю вас.")
			return
		}
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
	b.history.AppendUser(msg.From.ID, msg.Text)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: msg.From.ID, UserMessage: msg.Text, CanUse: &tru})
	}

	// In TZ mode: if no steps left, finalize immediately
	if b.isTZMode(msg.From.ID) && b.getTZRemaining(msg.From.ID) <= 0 {
		if pFinal, respFinal, okFinal := b.produceFinalTS(ctx, msg.From.ID); okFinal {
			answerToSend := pFinal.Answer
			if pFinal.Title != "" {
				answerToSend = b.formatTitleAnswer(pFinal.Title, pFinal.Answer)
			}
			b.history.AppendAssistantWithUsed(msg.From.ID, answerToSend, true)
			if b.recorder != nil {
				tru := true
				_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: msg.From.ID, AssistantResponse: answerToSend, CanUse: &tru})
			}
			metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", respFinal.Model, respFinal.PromptTokens, respFinal.CompletionTokens, respFinal.TotalTokens)
			metaEsc := b.escapeIfNeeded(metaLine)
			pm := strings.ToLower(b.parseModeValue())
			var header string
			switch pm {
			case strings.ToLower(tgbotapi.ModeHTML):
				header = "<b>ТЗ Готово</b>"
			case strings.ToLower(tgbotapi.ModeMarkdownV2):
				header = escapeMarkdownV2("ТЗ Готово")
			default:
				header = "**ТЗ Готово**"
			}
			final := metaEsc + "\n\n" + header + "\n\n" + answerToSend
			msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
			msgOut.ReplyMarkup = b.menuKeyboard()
			msgOut.ParseMode = b.parseModeValue()
			_, _ = b.s.Send(msgOut)
			b.clearTZState(msg.From.ID)
			return
		}
	}

	contextMsgs := b.buildContextWithOverflow(ctx, msg.From.ID)
	if b.isTZMode(msg.From.ID) {
		left := b.getTZRemaining(msg.From.ID)
		if left > 0 && left <= 2 {
			accel := "Осталось очень мало сообщений для уточнений (<=2). Сократи количество вопросов и постарайся завершить формирование ТЗ как можно скорее. Если возможно — финализируй уже в этом ответе (status='final')."
			contextMsgs = append([]llm.Message{{Role: "system", Content: accel}}, contextMsgs...)
		}
	}
	b.logLLMRequest(msg.From.ID, "chat", contextMsgs)
	resp, err := b.getLLMClient().Generate(ctx, contextMsgs)
	if err != nil {
		b.sendMessage(msg.Chat.ID, "Sorry, something went wrong.")
		return
	}
	log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)
	parsed, ok := parseLLMJSON(resp.Content)
	if !ok {
		if p2, ok2 := b.reformatToSchema(ctx, msg.From.ID, resp.Content); ok2 {
			parsed = p2
			ok = true
		}
	}

	if ok && strings.TrimSpace(parsed.CompressedContext) != "" {
		b.addUserSystemPrompt(msg.From.ID, parsed.CompressedContext)
		b.history.DisableAll(msg.From.ID)
	}

	answerToSend := resp.Content
	if ok && parsed.Answer != "" {
		answerToSend = parsed.Answer
	}
	status := ""
	if ok {
		status = strings.ToLower(strings.TrimSpace(parsed.Status))
	}

	if b.isTZMode(msg.From.ID) && status != "final" {
		left := b.decTZRemaining(msg.From.ID)
		if left <= 0 {
			if pFinal, respFinal, okFinal := b.produceFinalTS(ctx, msg.From.ID); okFinal {
				answerToSend = pFinal.Answer
				if pFinal.Title != "" {
					answerToSend = b.formatTitleAnswer(pFinal.Title, pFinal.Answer)
				}
				status = "final"
				metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", respFinal.Model, respFinal.PromptTokens, respFinal.CompletionTokens, respFinal.TotalTokens)
				metaEsc := b.escapeIfNeeded(metaLine)
				pm := strings.ToLower(b.parseModeValue())
				var header string
				switch pm {
				case strings.ToLower(tgbotapi.ModeHTML):
					header = "<b>ТЗ Готово</b>"
				case strings.ToLower(tgbotapi.ModeMarkdownV2):
					header = escapeMarkdownV2("ТЗ Готово")
				default:
					header = "**ТЗ Готово**"
				}
				final := metaEsc + "\n\n" + header + "\n\n" + answerToSend
				msgOut := tgbotapi.NewMessage(msg.Chat.ID, final)
				msgOut.ReplyMarkup = b.menuKeyboard()
				msgOut.ParseMode = b.parseModeValue()
				_, _ = b.s.Send(msgOut)
				b.clearTZState(msg.From.ID)
				return
			}
		}
	}

	b.history.AppendAssistantWithUsed(msg.From.ID, answerToSend, true)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: msg.From.ID, AssistantResponse: answerToSend, CanUse: &tru})
	}
	metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	metaEsc := b.escapeIfNeeded(metaLine)
	body := answerToSend
	if ok && parsed.Title != "" {
		body = b.formatTitleAnswer(parsed.Title, answerToSend)
	}
	if status == "final" && b.isTZMode(msg.From.ID) {
		pm := strings.ToLower(b.parseModeValue())
		var header string
		switch pm {
		case strings.ToLower(tgbotapi.ModeHTML):
			header = "<b>ТЗ Готово</b>"
		case strings.ToLower(tgbotapi.ModeMarkdownV2):
			header = escapeMarkdownV2("ТЗ Готово")
		default:
			header = "**ТЗ Готово**"
		}
		body = header + "\n\n" + body
		b.clearTZState(msg.From.ID)
	}
	final := metaEsc + "\n\n" + body
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
		b.history.DisableAll(cb.From.ID)
		if b.recorder != nil {
			if err := b.recorder.SetAllCanUse(cb.From.ID, false); err != nil {
				log.Printf("failed to persist can_use=false: %v", err)
			}
		}
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("Контекст сброшен"))
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
		m := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("История пуста"))
		m.ParseMode = b.parseModeValue()
		_, _ = b.s.Send(m)
		return
	}
	msgs := b.buildContextWithOverflow(ctx, cb.From.ID)
	// strengthen instruction to return our schema
	msgs = append([]llm.Message{{Role: "system", Content: "Суммируй переписку. Ответ строго в JSON со схемой {title, answer, compressed_context}."}}, msgs...)
	b.logLLMRequest(cb.From.ID, "summary", msgs)
	resp, err := b.getLLMClient().Generate(ctx, msgs)
	if err != nil {
		m := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("Не удалось собрать саммари"))
		m.ParseMode = b.parseModeValue()
		_, _ = b.s.Send(m)
		return
	}
	parsed, ok := parseLLMJSON(resp.Content)
	if !ok {
		if p2, ok2 := b.reformatToSchema(ctx, cb.From.ID, resp.Content); ok2 {
			parsed = p2
			ok = true
		}
	}
	if ok && strings.TrimSpace(parsed.CompressedContext) != "" {
		b.addUserSystemPrompt(cb.From.ID, parsed.CompressedContext)
		b.history.DisableAll(cb.From.ID)
	}
	answerToSend := resp.Content
	if ok && parsed.Answer != "" {
		answerToSend = parsed.Answer
	}
	b.history.AppendAssistantWithUsed(cb.From.ID, answerToSend, true)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: cb.From.ID, AssistantResponse: answerToSend, CanUse: &tru})
	}
	metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	metaEsc := b.escapeIfNeeded(metaLine)
	body := answerToSend
	if ok && parsed.Title != "" {
		body = b.formatTitleAnswer(parsed.Title, answerToSend)
	}
	final := metaEsc + "\n\n" + body
	m := tgbotapi.NewMessage(cb.Message.Chat.ID, final)
	m.ParseMode = b.parseModeValue()
	m.ReplyMarkup = b.menuKeyboard()
	_, _ = b.s.Send(m)
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
	if _, err := b.s.Send(msg); err != nil {
		log.Println(err)
	}
}

func (b *Bot) getUserSystemPrompt(userID int64) string {
	b.userSysMu.RLock()
	sp, ok := b.userSystemPrompt[userID]
	b.userSysMu.RUnlock()
	if !ok || sp == "" {
		return b.systemPrompt
	}
	return sp
}

func (b *Bot) addUserSystemPrompt(userID int64, addition string) {
	b.addUserSystemPromptInternal(userID, addition, true)
}

func (b *Bot) addUserSystemPromptInternal(userID int64, addition string, persist bool) {
	if strings.TrimSpace(addition) == "" {
		return
	}
	b.userSysMu.Lock()
	current := b.userSystemPrompt[userID]
	if current == "" {
		current = b.systemPrompt
	}
	if !strings.Contains(current, addition) {
		if current != "" {
			current = current + "\n\n" + addition
		} else {
			current = addition
		}
		b.userSystemPrompt[userID] = current
	}
	b.userSysMu.Unlock()
	if persist && b.recorder != nil {
		f := false
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: userID, UserMessage: spUpdateMarker, AssistantResponse: addition, CanUse: &f})
	}
}

// --- TZ helpers ---

func (b *Bot) setTZMode(userID int64, on bool) {
	b.tzMu.Lock()
	b.tzMode[userID] = on
	b.tzMu.Unlock()
}
func (b *Bot) isTZMode(userID int64) bool {
	b.tzMu.RLock()
	v := b.tzMode[userID]
	b.tzMu.RUnlock()
	return v
}

func (b *Bot) setTZRemaining(userID int64, steps int) {
	b.tzMu.Lock()
	b.tzRemaining[userID] = steps
	b.tzMu.Unlock()
}
func (b *Bot) getTZRemaining(userID int64) int {
	b.tzMu.RLock()
	v := b.tzRemaining[userID]
	b.tzMu.RUnlock()
	return v
}
func (b *Bot) decTZRemaining(userID int64) int {
	b.tzMu.Lock()
	left := b.tzRemaining[userID]
	if left > 0 {
		left--
		b.tzRemaining[userID] = left
	}
	b.tzMu.Unlock()
	return left
}
func (b *Bot) clearTZState(userID int64) {
	b.tzMu.Lock()
	delete(b.tzMode, userID)
	delete(b.tzRemaining, userID)
	b.tzMu.Unlock()
}

// Building context with overflow protection

func (b *Bot) produceFinalTS(ctx context.Context, userID int64) (llmJSON, llm.Response, bool) {
	msgs := b.buildContextWithOverflow(ctx, userID)
	finalInstr := "Сформируй итоговое техническое задание (ТЗ) по собранным данным. Ответ строго в JSON со схемой {title, answer, compressed_context, status}. В 'answer' помести полноценное, структурированное ТЗ. Установи status='final'."
	msgs = append([]llm.Message{{Role: "system", Content: finalInstr}}, msgs...)
	b.logLLMRequest(userID, "tz_finalize", msgs)
	resp, err := b.getLLMClient().Generate(ctx, msgs)
	if err != nil {
		return llmJSON{}, llm.Response{}, false
	}
	p, ok := parseLLMJSON(resp.Content)
	if !ok {
		if p2, ok2 := b.reformatToSchema(ctx, userID, resp.Content); ok2 {
			return p2, resp, true
		}
		return llmJSON{}, llm.Response{}, false
	}
	return p, resp, true
}
