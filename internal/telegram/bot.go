package telegram

import (
	"context"
	"fmt"
	"html"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/analytics"
	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/notion"
	"ai-chatter/internal/pending"
	"ai-chatter/internal/storage"
)

const (
	resetCmd       = "reset_ctx"
	summaryCmd     = "summary_ctx"
	approvePrefix  = "approve:"
	denyPrefix     = "deny:"
	spUpdateMarker = "[system_prompt_update]"
	// TZ conversation limit (assistant clarification turns)
	tzMaxSteps = 15
)

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
	// secondary model for post-TS instruction
	model2           string
	llmClient2       llm.Client
	llmFactory       *llm.Factory
	userSysMu        sync.RWMutex
	userSystemPrompt map[int64]string
	tzMu             sync.RWMutex
	tzMode           map[int64]bool
	// per-user remaining steps in TZ mode
	tzRemaining map[int64]int
	// Notion MCP client
	mcpClient        *notion.MCPClient
	notionParentPage string
}

func New(
	botToken string,
	authSvc *auth.Service,
	llmClient llm.Client,
	llmFactory *llm.Factory,
	systemPrompt string,
	rec storage.Recorder,
	adminUserID int64,
	pendingRepo pending.Repository,
	parseMode string,
	provider string,
	model string,
	mcpClient *notion.MCPClient,
	notionParentPage string,
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		api:              api,
		s:                botAPISender{api: api},
		authSvc:          authSvc,
		systemPrompt:     systemPrompt,
		history:          history.NewManager(),
		recorder:         rec,
		adminUserID:      adminUserID,
		pending:          make(map[int64]auth.User),
		pendingRepo:      pendingRepo,
		parseMode:        parseMode,
		provider:         provider,
		model:            model,
		llmFactory:       llmFactory,
		userSystemPrompt: make(map[int64]string),
		tzMode:           make(map[int64]bool),
		tzRemaining:      make(map[int64]int),
		mcpClient:        mcpClient,
		notionParentPage: notionParentPage,
	}
	// Try to preload model2 from file if present
	if data, err := os.ReadFile("data/model2.txt"); err == nil {
		m2 := strings.TrimSpace(string(data))
		if m2 != "" {
			b.model2 = m2
		}
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

func (b *Bot) getSecondLLMClient() llm.Client {
	b.llmMu.RLock()
	cli := b.llmClient2
	b.llmMu.RUnlock()
	if cli != nil {
		return cli
	}

	desiredModel := b.model
	if strings.TrimSpace(b.model2) != "" {
		desiredModel = b.model2
	}

	newCli, err := b.llmFactory.CreateClient(b.provider, desiredModel)
	if err != nil {
		// Fallback to primary client
		newCli = b.getLLMClient()
	}

	b.llmMu.Lock()
	if b.llmClient2 == nil {
		b.llmClient2 = newCli
		cli = newCli
	} else {
		cli = b.llmClient2
	}
	b.llmMu.Unlock()
	return cli
}

func (b *Bot) reloadLLMClient() error {
	newCli, err := b.llmFactory.CreateClient(b.provider, b.model)
	if err != nil {
		return err
	}

	b.setLLMClient(newCli)
	b.llmMu.Lock()
	b.llmClient2 = nil
	b.llmMu.Unlock()
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

// handleCommand is implemented in handlers.go

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
			allowedModels := strings.Join(llm.GetAllowedModels(), "|")
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Usage: /model <%s>", allowedModels))
			return
		}
		model := args[0]
		if !llm.IsModelAllowed(model) {
			allowedModels := strings.Join(llm.GetAllowedModels(), ", ")
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Неподдерживаемая модель. Доступные: %s", allowedModels))
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
	case "model2":
		if len(args) != 1 {
			allowedModels := strings.Join(llm.GetAllowedModels(), "|")
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Usage: /model2 <%s>", allowedModels))
			return
		}
		model := args[0]
		if !llm.IsModelAllowed(model) {
			allowedModels := strings.Join(llm.GetAllowedModels(), ", ")
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Неподдерживаемая модель. Доступные: %s", allowedModels))
			return
		}
		if err := os.WriteFile("data/model2.txt", []byte(model), 0o644); err != nil {
			b.sendMessage(msg.Chat.ID, fmt.Sprintf("Ошибка сохранения: %v", err))
			return
		}
		b.model2 = model
		b.llmMu.Lock()
		b.llmClient2 = nil
		b.llmMu.Unlock()
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("Вторая модель установлена: %s", model))
	}
}

// JSON parsing moved to process.go

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
	log.Print(bld.String())
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

// moved: handlers in handlers.go

func (b *Bot) approveUser(id int64) {
	u := b.pending[id]
	if u.ID == 0 {
		return
	}
	delete(b.pending, id)
	if b.pendingRepo != nil {
		_ = b.pendingRepo.Remove(id)
	}
	_ = b.authSvc.Upsert(u)
	msg := tgbotapi.NewMessage(b.adminUserID, b.escapeIfNeeded(fmt.Sprintf("Пользователь @%s (%d) добавлен в allowlist", u.Username, u.ID)))
	msg.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msg); err != nil {
		log.Printf("failed to notify approval: %v", err)
	}
	msg2 := tgbotapi.NewMessage(u.ID, b.escapeIfNeeded("Ваш доступ к боту подтвержден. Добро пожаловать!"))
	msg2.ParseMode = b.parseModeValue()
	if _, err := b.s.Send(msg2); err != nil {
		log.Printf("failed to notify user approval: %v", err)
	}
}

func (b *Bot) denyUser(id int64) {
	u := b.pending[id]
	if u.ID == 0 {
		return
	}
	delete(b.pending, id)
	if b.pendingRepo != nil {
		_ = b.pendingRepo.Remove(id)
	}
	msg := tgbotapi.NewMessage(b.adminUserID, b.escapeIfNeeded(fmt.Sprintf("Пользователю @%s (%d) отказано в доступе", u.Username, u.ID)))
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
	if b.userSystemPrompt == nil {
		b.userSystemPrompt = make(map[int64]string)
	}
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
	if b.tzMode == nil {
		b.tzMode = make(map[int64]bool)
	}
	b.tzMode[userID] = on
	b.tzMu.Unlock()
}
func (b *Bot) isTZMode(userID int64) bool {
	b.tzMu.RLock()
	v := false
	if b.tzMode != nil {
		v = b.tzMode[userID]
	}
	b.tzMu.RUnlock()
	return v
}

func (b *Bot) setTZRemaining(userID int64, steps int) {
	b.tzMu.Lock()
	if b.tzRemaining == nil {
		b.tzRemaining = make(map[int64]int)
	}
	b.tzRemaining[userID] = steps
	b.tzMu.Unlock()
}
func (b *Bot) getTZRemaining(userID int64) int {
	b.tzMu.RLock()
	v := 0
	if b.tzRemaining != nil {
		v = b.tzRemaining[userID]
	}
	b.tzMu.RUnlock()
	return v
}
func (b *Bot) decTZRemaining(userID int64) int {
	b.tzMu.Lock()
	if b.tzRemaining == nil {
		b.tzRemaining = make(map[int64]int)
	}
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
	if b.tzMode != nil {
		delete(b.tzMode, userID)
	}
	if b.tzRemaining != nil {
		delete(b.tzRemaining, userID)
	}
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

// generateDailyReport генерирует отчёт за последние сутки
func (b *Bot) generateDailyReport(ctx context.Context, chatID int64) error {
	// Отправляем уведомление о начале генерации отчёта
	b.sendMessage(chatID, "📊 Начинаю формирование отчёта об использовании бота за последние сутки...")

	if b.recorder == nil {
		return fmt.Errorf("recorder не настроен")
	}

	// Загружаем все события
	events, err := b.recorder.LoadInteractions()
	if err != nil {
		return fmt.Errorf("не удалось загрузить логи: %w", err)
	}

	// Анализируем данные за вчерашний день
	yesterday := time.Now().AddDate(0, 0, -1)
	stats := analytics.AnalyzeDailyLogs(events, yesterday)

	// Генерируем резюме для LLM
	reportSummary := stats.GenerateReportSummary()

	// Выполняем генерацию отчёта в изолированном контексте
	currentDate := yesterday.Format("2006-01-02")
	reportTitle := fmt.Sprintf("Отчёт за %s", currentDate)

	err = b.executeReportGenerationPipeline(ctx, chatID, reportTitle, reportSummary, currentDate)
	if err != nil {
		return fmt.Errorf("ошибка выполнения генерации отчёта: %w", err)
	}

	return nil
}

// executeReportGenerationPipeline выполняет пошаговую генерацию отчёта в изолированном контексте
func (b *Bot) executeReportGenerationPipeline(ctx context.Context, chatID int64, reportTitle, reportSummary, currentDate string) error {
	// Шаг 1: Поиск страницы Reports
	b.sendMessage(chatID, "🔍 Ищу страницу Reports в Notion...")

	reportsPageID, err := b.findOrCreateReportsPage(ctx, chatID)
	if err != nil {
		return fmt.Errorf("не удалось найти/создать страницу Reports: %w", err)
	}

	// Шаг 2: Генерация содержимого отчёта
	b.sendMessage(chatID, "📝 Генерирую содержимое отчёта...")

	reportContent, err := b.generateReportContent(ctx, reportSummary, currentDate)
	if err != nil {
		return fmt.Errorf("не удалось сгенерировать содержимое отчёта: %w", err)
	}

	// Шаг 3: Создание отчёта как подстраницы
	b.sendMessage(chatID, fmt.Sprintf("📊 Создаю отчёт '%s' в Notion...", reportTitle))

	pageID, err := b.createReportPage(ctx, reportTitle, reportContent, reportsPageID)
	if err != nil {
		return fmt.Errorf("не удалось создать страницу отчёта: %w", err)
	}

	// Шаг 4: Уведомление о завершении
	pageURL := fmt.Sprintf("https://notion.so/%s", pageID)
	successMessage := fmt.Sprintf("✅ Отчёт '%s' успешно создан!\n\n🔗 Ссылка: %s", reportTitle, pageURL)
	b.sendMessage(chatID, successMessage)

	return nil
}

// findOrCreateReportsPage находит или создаёт страницу Reports
func (b *Bot) findOrCreateReportsPage(ctx context.Context, chatID int64) (string, error) {
	if b.mcpClient == nil {
		return "", fmt.Errorf("MCP клиент не настроен")
	}

	// Ищем страницу Reports
	result := b.mcpClient.SearchPagesWithID(ctx, "Reports", 5, true)
	if result.Success && len(result.Pages) > 0 {
		b.sendMessage(chatID, fmt.Sprintf("✅ Найдена страница Reports (ID: %s)", result.Pages[0].ID))
		return result.Pages[0].ID, nil
	}

	// Если не найдена, создаём
	b.sendMessage(chatID, "📄 Страница Reports не найдена, создаю новую...")

	reportsContent := `# Reports

Эта страница содержит автоматически генерируемые отчёты об использовании AI Chatter бота.

## Автоматические отчёты
Отчёты создаются ежедневно в 21:00 UTC и содержат:
- Статистику сообщений и пользователей
- Анализ использования функций MCP
- Рекомендации по улучшению

---
*Создано автоматически*`

	createResult := b.mcpClient.CreateFreeFormPage(ctx, "Reports", reportsContent, b.notionParentPage, nil)
	if !createResult.Success {
		return "", fmt.Errorf("не удалось создать страницу Reports: %s", createResult.Message)
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ Создана страница Reports (ID: %s)", createResult.PageID))
	return createResult.PageID, nil
}

// generateReportContent генерирует содержимое отчёта через LLM
func (b *Bot) generateReportContent(ctx context.Context, reportSummary, currentDate string) (string, error) {
	reportPrompt := fmt.Sprintf(`Создай подробный отчёт об использовании AI Chatter бота за %s в формате markdown.

Требования к отчёту:
1. Используй профессиональный, но дружелюбный тон
2. Добавь анализ и выводы, а не только сухие цифры  
3. Включи рекомендации по улучшению, если есть проблемы
4. Структурируй отчёт с заголовками и эмодзи
5. Отчёт должен быть информативным для администратора
6. Отчёт должен быть не более 1700 символо в длину

Статистика:
%s

Создай полный отчёт с анализом активности пользователей и использования функций бота.
Ответ должен содержать ТОЛЬКО markdown текст отчёта, без дополнительных комментариев.`, currentDate, reportSummary)

	// Изолированный контекст без истории пользователя
	messages := []llm.Message{
		{Role: "system", Content: "Ты — аналитик, который создаёт отчёты об использовании AI чат-бота. Отвечай только markdown текстом отчёта."},
		{Role: "user", Content: reportPrompt},
	}

	b.logLLMRequest(b.adminUserID, "report_content_generation", messages)

	resp, err := b.getLLMClient().Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ошибка генерации содержимого: %w", err)
	}

	b.logResponse(resp)
	return resp.Content, nil
}

// createReportPage создаёт страницу отчёта в Notion
func (b *Bot) createReportPage(ctx context.Context, title, content, parentPageID string) (string, error) {
	if b.mcpClient == nil {
		return "", fmt.Errorf("MCP клиент не настроен")
	}

	result := b.mcpClient.CreateFreeFormPage(ctx, title, content, parentPageID, nil)
	if !result.Success {
		return "", fmt.Errorf("не удалось создать страницу отчёта: %s", result.Message)
	}

	return result.PageID, nil
}

// GenerateDailyReportForAdmin генерирует отчёт и отправляет админу (для планировщика)
func (b *Bot) GenerateDailyReportForAdmin(ctx context.Context) error {
	return b.generateDailyReport(ctx, b.adminUserID)
}
