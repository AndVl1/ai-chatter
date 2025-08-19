package telegram

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/codevalidation"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/storage"
)

// ProgressTracker отслеживает и обновляет прогресс выполнения команд
type ProgressTracker struct {
	bot       *Bot
	chatID    int64
	messageID int
	steps     map[string]*ProgressStep
	mu        sync.RWMutex
	finalURL  string
}

// ProgressStep представляет шаг выполнения с метаданными
type ProgressStep struct {
	Name        string
	Description string
	Status      string // pending, in_progress, completed, error
	StartTime   time.Time
	EndTime     time.Time
}

// NewProgressTracker создает новый трекер прогресса
func NewProgressTracker(bot *Bot, chatID int64, messageID int) *ProgressTracker {
	steps := map[string]*ProgressStep{
		"gmail_data":         {Name: "📧 Сбор данных из Gmail", Description: "Поиск и анализ писем с автоматическими исправлениями", Status: "pending"},
		"validate_data":      {Name: "🔍 Валидация данных", Description: "Проверка релевантности (до 5 попыток)", Status: "pending"},
		"notion_setup":       {Name: "📝 Настройка Notion", Description: "Поиск/создание страницы Gmail summaries", Status: "pending"},
		"generate_summary":   {Name: "🤖 Генерация саммари", Description: "AI анализ с валидацией качества (до 5 попыток)", Status: "pending"},
		"validate_summary":   {Name: "✅ Валидация саммари", Description: "Проверка качества с автоисправлениями", Status: "pending"},
		"create_notion_page": {Name: "📄 Создание страницы", Description: "Сохранение в Notion", Status: "pending"},
	}

	return &ProgressTracker{
		bot:       bot,
		chatID:    chatID,
		messageID: messageID,
		steps:     steps,
	}
}

// UpdateProgress реализует интерфейс ProgressCallback
func (pt *ProgressTracker) UpdateProgress(stepKey string, status string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if step, exists := pt.steps[stepKey]; exists {
		step.Status = status
		if status == "in_progress" {
			step.StartTime = time.Now()
		} else if status == "completed" || status == "error" {
			step.EndTime = time.Now()
		}
	}

	// Обновляем сообщение
	pt.updateMessage()
}

// SetFinalResult устанавливает финальный результат
func (pt *ProgressTracker) SetFinalResult(pageURL string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.finalURL = pageURL
	pt.updateMessage()
}

// updateMessage обновляет сообщение с текущим прогрессом
func (pt *ProgressTracker) updateMessage() {
	message := pt.buildProgressMessage()

	editMsg := tgbotapi.NewEditMessageText(pt.chatID, pt.messageID, message)
	editMsg.ParseMode = pt.bot.parseModeValue()

	if _, err := pt.bot.s.Send(editMsg); err != nil {
		log.Printf("⚠️ Failed to update progress message: %v", err)
	}
}

// buildProgressMessage формирует текст сообщения с прогрессом
func (pt *ProgressTracker) buildProgressMessage() string {
	var message strings.Builder

	if pt.finalURL != "" {
		// Финальное сообщение с результатом
		message.WriteString("✅ **Gmail саммари успешно создан!**\n\n")
		message.WriteString(fmt.Sprintf("🔗 **Ссылка на страницу в Notion:**\n%s\n\n", pt.finalURL))
		message.WriteString("📊 **Выполненные этапы:**\n")
	} else {
		message.WriteString("🔄 **Обработка Gmail запроса...**\n\n")
	}

	// Добавляем информацию о шагах
	stepOrder := []string{"gmail_data", "validate_data", "notion_setup", "generate_summary", "validate_summary", "create_notion_page"}

	for _, stepKey := range stepOrder {
		if step, exists := pt.steps[stepKey]; exists {
			var statusIcon string
			switch step.Status {
			case "pending":
				statusIcon = "⏳"
			case "in_progress":
				statusIcon = "🔄"
			case "completed":
				statusIcon = "✅"
			case "error":
				statusIcon = "❌"
			default:
				statusIcon = "❓"
			}

			message.WriteString(fmt.Sprintf("%s %s\n", statusIcon, step.Name))

			// Если финальное сообщение и шаг завершен, показываем время
			if pt.finalURL != "" && (step.Status == "completed" || step.Status == "error") && !step.EndTime.IsZero() && !step.StartTime.IsZero() {
				duration := step.EndTime.Sub(step.StartTime)
				if duration > 0 && duration < 24*time.Hour { // Проверяем разумные пределы
					if duration < time.Minute {
						message.WriteString(fmt.Sprintf("   ⏱️ %.1fs\n", duration.Seconds()))
					} else {
						message.WriteString(fmt.Sprintf("   ⏱️ %v\n", duration.Round(time.Second)))
					}
				}
			}
		}
	}

	if pt.finalURL == "" {
		message.WriteString("\n💭 *Процесс может занять 30-60 секунд...*")
	}

	return message.String()
}

// handleCommand
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	if msg.Command() == "provider" || msg.Command() == "model" || msg.Command() == "model2" {
		b.handleAdminConfigCommands(msg)
		return
	}
	// Notion commands
	if msg.Command() == "notion_save" {
		b.handleNotionSave(msg)
		return
	}
	if msg.Command() == "notion_search" {
		b.handleNotionSearch(msg)
		return
	}
	if msg.Command() == "report" {
		b.handleReportCommand(msg)
		return
	}
	if msg.Command() == "gmail_summary" {
		b.handleGmailSummaryCommand(msg)
		return
	}
	if msg.Command() == "tz" {
		if !b.authSvc.IsAllowed(msg.From.ID) {
			return
		}
		// Reset previous context for this user (do not delete logs, just mark not used)
		b.history.DisableAll(msg.From.ID)
		if b.recorder != nil {
			_ = b.recorder.SetAllCanUse(msg.From.ID, false)
		}

		topic := strings.TrimSpace(msg.CommandArguments())
		addition := "Requirements elicitation mode (Technical Specification). Your job is to iteratively clarify and assemble a complete TS in Russian for the topic: '" + topic + "'. " +
			"Ask up to 5 highly targeted questions per turn until you are confident the TS is complete. Focus on: scope/goals, user roles, environment, constraints (budget/time/tech), functional and non-functional requirements, data and integrations, dependencies, acceptance criteria, risks/mitigations, deliverables and plan. " +
			"When asking questions, prefer concrete options (multiple-choice) and short free-form fields; personalize questions to the user’s previous answers (e.g., preferred and unwanted ingredients, platforms, APIs, performance targets). " +
			"Always respond strictly in JSON {title, answer, compressed_context, status}. Set status='continue' while clarifying. When the TS is fully ready, set status='final'. If your context window is >= 80% full, include 'compressed_context' with a compact string summary of essential facts/decisions to continue without previous messages. You have at most 15 messages to clarify before finalization. " +
			"VERY IMPORTANT: Present your questions as a numbered list (1., 2., 3., ...) with each question on its own new line. Do not merge questions into a single paragraph."
		b.addUserSystemPrompt(msg.From.ID, addition)
		b.setTZMode(msg.From.ID, true)
		b.setTZRemaining(msg.From.ID, tzMaxSteps)
		seed := "Тема ТЗ: " + topic
		b.history.AppendUser(msg.From.ID, seed)
		if b.recorder != nil {
			tru := true
			_ = b.recorder.AppendInteraction(storage.Event{Timestamp: b.nowUTC(), UserID: msg.From.ID, UserMessage: seed, CanUse: &tru})
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
		b.processLLMAndRespond(ctx, msg.Chat.ID, msg.From.ID, resp)
		return
	}
	// admin-only commands
	if msg.From.ID != b.adminUserID {
		b.sendMessage(msg.Chat.ID, "Команда доступна только администратору")
		return
	}
	switch msg.Command() {
	case "allowlist":
		var bld strings.Builder
		bld.WriteString("Allowlist:\n")
		for _, u := range b.authSvc.List() {
			bld.WriteString(fmt.Sprintf("- id=%d, @%s %s %s\n", u.ID, u.Username, u.FirstName, u.LastName))
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
			bld.WriteString(fmt.Sprintf("- id=%d, @%s %s %s\n", u.ID, u.Username, u.FirstName, u.LastName))
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
		b.approveUser(uid)
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
		b.denyUser(uid)
	}
}

// handleIncomingMessage
func (b *Bot) handleIncomingMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		log.Printf("Unauthorized access attempt by user ID: %d, username: @%s", msg.From.ID, msg.From.UserName)
		if _, ok := b.pending[msg.From.ID]; ok {
			b.sendMessage(msg.Chat.ID, "Ваш запрос на доступ уже отправлен администратору. Пожалуйста, ожидайте подтверждения. Как только доступ будет предоставлен, я уведомлю вас.")
			return
		}
		b.pending[msg.From.ID] = auth.User{ID: msg.From.ID, Username: msg.From.UserName, FirstName: msg.From.FirstName, LastName: msg.From.LastName}
		if b.pendingRepo != nil {
			_ = b.pendingRepo.Upsert(b.pending[msg.From.ID])
		}
		b.sendMessage(msg.Chat.ID, "Запрос на доступ отправлен администратору. Как только он подтвердит, вы получите уведомление.")
		b.notifyAdminRequest(msg.From.ID, msg.From.UserName)
		return
	}
	log.Printf("Incoming message from %d (@%s): %q", msg.From.ID, msg.From.UserName, msg.Text)
	b.history.AppendUser(msg.From.ID, msg.Text)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: b.nowUTC(), UserID: msg.From.ID, UserMessage: msg.Text, CanUse: &tru})
	}

	if b.isTZMode(msg.From.ID) && b.getTZRemaining(msg.From.ID) <= 0 {
		if pFinal, respFinal, okFinal := b.produceFinalTS(ctx, msg.From.ID); okFinal {
			b.sendFinalTS(msg.Chat.ID, msg.From.ID, pFinal, respFinal)
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

	// Проверяем наличие файлов или архивов
	if b.codeValidationWorkflow != nil && !b.isTZMode(msg.From.ID) && msg.Document != nil {
		log.Printf("🔍 Document detected: %s", msg.Document.FileName)
		b.handleDocumentValidation(ctx, msg)
		return
	}

	// Проверяем наличие кода в сообщении перед обычной обработкой
	if b.codeValidationWorkflow != nil && !b.isTZMode(msg.From.ID) {
		hasCode, extractedCode, filename, userQuestion, codeErr := codevalidation.DetectCodeInMessage(ctx, b.getLLMClient(), msg.Text)
		if codeErr != nil {
			log.Printf("⚠️ Code detection failed: %v", codeErr)
		} else if hasCode {
			log.Printf("🔍 Code detected in message, triggering validation mode")
			if userQuestion != "" {
				log.Printf("❓ User question detected: %s", userQuestion)
			}
			// Запускаем валидацию кода вместо обычной обработки
			b.handleCodeValidation(ctx, msg, extractedCode, filename, userQuestion)
			return
		}
	}

	// Используем инструменты Notion только если клиент настроен и не в режиме ТЗ
	var resp llm.Response
	var err error
	if b.mcpClient != nil && !b.isTZMode(msg.From.ID) {
		tools := llm.GetNotionTools()
		resp, err = b.getLLMClient().GenerateWithTools(ctx, contextMsgs, tools)
	} else {
		resp, err = b.getLLMClient().Generate(ctx, contextMsgs)
	}

	if err != nil {
		b.sendMessage(msg.Chat.ID, "Sorry, something went wrong.")
		log.Printf("Something went wrong. %v", err)
		return
	}
	b.processLLMAndRespond(ctx, msg.Chat.ID, msg.From.ID, resp)
}

// notifyAdminRequest
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

// handleCallback
func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	switch {
	case cb.Data == resetCmd:
		b.history.DisableAll(cb.From.ID)
		if b.recorder != nil {
			_ = b.recorder.SetAllCanUse(cb.From.ID, false)
		}
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("Контекст очищен"))
		msg.ParseMode = b.parseModeValue()
		msg.ReplyMarkup = b.menuKeyboard()
		if _, err := b.s.Send(msg); err != nil {
			log.Printf("failed to send reset confirmation: %v", err)
		}
	case cb.Data == summaryCmd:
		b.handleSummary(ctx, cb)
	default:
		switch {
		case strings.HasPrefix(cb.Data, approvePrefix):
			idStr := strings.TrimPrefix(cb.Data, approvePrefix)
			id, _ := strconv.ParseInt(idStr, 10, 64)
			b.approveUser(id)
		case strings.HasPrefix(cb.Data, denyPrefix):
			idStr := strings.TrimPrefix(cb.Data, denyPrefix)
			id, _ := strconv.ParseInt(idStr, 10, 64)
			b.denyUser(id)
		}
	}
}

// handleSummary
func (b *Bot) handleSummary(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	h := b.history.Get(cb.From.ID)
	if len(h) == 0 {
		m := tgbotapi.NewMessage(cb.Message.Chat.ID, b.escapeIfNeeded("История пуста"))
		m.ParseMode = b.parseModeValue()
		_, _ = b.s.Send(m)
		return
	}
	msgs := b.buildContextWithOverflow(ctx, cb.From.ID)
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
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: b.nowUTC(), UserID: cb.From.ID, AssistantResponse: answerToSend, CanUse: &tru})
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

// handleNotionSave сохраняет диалог в Notion
func (b *Bot) handleNotionSave(msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		return
	}

	if b.mcpClient == nil {
		b.sendMessage(msg.Chat.ID, "Notion интеграция не настроена. Установите NOTION_TOKEN в конфигурации.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendMessage(msg.Chat.ID, "Использование: /notion_save <название страницы>")
		return
	}

	// Собираем контекст диалога
	history := b.history.Get(msg.From.ID)
	if len(history) == 0 {
		b.sendMessage(msg.Chat.ID, "История диалога пуста, нечего сохранять.")
		return
	}

	// Формируем содержимое страницы
	var content strings.Builder
	for _, msg := range history {
		if msg.Role == "user" {
			content.WriteString(fmt.Sprintf("**Пользователь:** %s\n\n", msg.Content))
		} else if msg.Role == "assistant" {
			content.WriteString(fmt.Sprintf("**Ассистент:** %s\n\n", msg.Content))
		}
	}

	ctx := context.Background()

	// Проверяем настройку parent page
	if b.notionParentPage == "" {
		b.sendMessage(msg.Chat.ID, "❌ Не настроен NOTION_PARENT_PAGE_ID. Настройте переменную окружения с ID страницы из Notion.")
		return
	}

	result := b.mcpClient.CreateDialogSummary(
		ctx,
		args, // title
		content.String(),
		fmt.Sprintf("%d", msg.From.ID),
		msg.From.UserName,
		"dialog_summary",
		b.notionParentPage,
	)

	if result.Success {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Диалог успешно сохранен в Notion!\n\n%s", result.Message))
	} else {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка сохранения в Notion: %s", result.Message))
	}
}

// handleNotionSearch ищет в Notion
func (b *Bot) handleNotionSearch(msg *tgbotapi.Message) {
	if !b.authSvc.IsAllowed(msg.From.ID) {
		return
	}

	if b.mcpClient == nil {
		b.sendMessage(msg.Chat.ID, "Notion интеграция не настроена. Установите NOTION_TOKEN в конфигурации.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.sendMessage(msg.Chat.ID, "Использование: /notion_search <поисковый запрос>")
		return
	}

	ctx := context.Background()
	result := b.mcpClient.SearchDialogSummaries(
		ctx,
		args,
		fmt.Sprintf("%d", msg.From.ID),
		"dialog_summary",
	)

	if result.Success {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("🔍 Результаты поиска в Notion:\n\n%s", result.Message))
	} else {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка поиска в Notion: %s", result.Message))
	}
}

// handleReportCommand обрабатывает команду /report (только для админа)
func (b *Bot) handleReportCommand(msg *tgbotapi.Message) {
	// Проверяем, что это админ
	if msg.From.ID != b.adminUserID {
		b.sendMessage(msg.Chat.ID, "❌ Команда доступна только администратору.")
		return
	}

	ctx := context.Background()
	if err := b.generateDailyReport(ctx, msg.Chat.ID); err != nil {
		log.Printf("❌ Report generation failed: %v", err)
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка генерации отчёта: %v", err))
	}
}

// handleGmailSummaryCommand обрабатывает команду /gmail_summary (только для админа)
func (b *Bot) handleGmailSummaryCommand(msg *tgbotapi.Message) {
	// Проверяем, что это админ
	if msg.From.ID != b.adminUserID {
		b.sendMessage(msg.Chat.ID, "❌ Команда доступна только администратору.")
		return
	}

	// Проверяем наличие Gmail workflow
	if b.gmailWorkflow == nil {
		b.sendMessage(msg.Chat.ID, "❌ Gmail интеграция не настроена. Проверьте конфигурацию GMAIL_CREDENTIALS_JSON или GMAIL_CREDENTIALS_JSON_PATH.")
		return
	}

	// Получаем текст запроса
	userQuery := strings.TrimSpace(msg.CommandArguments())
	if userQuery == "" {
		b.sendMessage(msg.Chat.ID, "❌ Использование: /gmail_summary <запрос для анализа>\n\nПример: /gmail_summary что важного я пропустил за последний день")
		return
	}

	// Отправляем начальное сообщение с прогрессом
	initialMsg := tgbotapi.NewMessage(msg.Chat.ID, "🔄 **Обработка Gmail запроса...**\n\n⏳ Инициализация...")
	initialMsg.ParseMode = b.parseModeValue()

	sentMsg, err := b.s.Send(initialMsg)
	if err != nil {
		log.Printf("⚠️ Failed to send initial progress message: %v", err)
		b.sendMessage(msg.Chat.ID, "❌ Ошибка отправки сообщения")
		return
	}

	// Создаем progress tracker
	progressTracker := NewProgressTracker(b, msg.Chat.ID, sentMsg.MessageID)

	ctx := context.Background()

	// Запускаем обработку в горутине для неблокирующего выполнения
	go func() {
		// Обрабатываем запрос через Gmail workflow с прогрессом
		pageURL, err := b.gmailWorkflow.ProcessGmailSummaryRequestWithProgress(ctx, userQuery, progressTracker)
		if err != nil {
			log.Printf("❌ Gmail summary workflow failed: %v", err)
			// Обновляем сообщение с ошибкой
			errorMsg := fmt.Sprintf("❌ **Ошибка обработки Gmail запроса**\n\n%s\n\n📧 **Запрос:** %s", html.EscapeString(err.Error()), html.EscapeString(userQuery))
			editMsg := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, errorMsg)
			editMsg.ParseMode = b.parseModeValue()
			if _, editErr := b.s.Send(editMsg); editErr != nil {
				log.Printf("⚠️ Failed to update error message: %v", editErr)
			}
			return
		}

		// Устанавливаем финальный результат
		progressTracker.SetFinalResult(pageURL)

		log.Printf("✅ Gmail summary completed successfully: %s", pageURL)
	}()
}

// handleDocumentValidation обрабатывает валидацию загруженных файлов и архивов
func (b *Bot) handleDocumentValidation(ctx context.Context, msg *tgbotapi.Message) {
	log.Printf("🔍 Starting document validation for user %d, file: %s", msg.From.ID, msg.Document.FileName)

	// Проверяем наличие code validation workflow
	if b.codeValidationWorkflow == nil {
		b.sendMessage(msg.Chat.ID, "❌ Валидация кода недоступна. Проверьте конфигурацию Docker.")
		return
	}

	// Получаем файл от Telegram
	file, err := b.s.GetFile(tgbotapi.FileConfig{FileID: msg.Document.FileID})
	if err != nil {
		b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Ошибка получения файла: %v", err))
		return
	}

	// Отправляем начальное сообщение с прогрессом
	initialMsg := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("🔄 **Запуск валидации файла...**\n\n📄 **Файл:** %s\n⏳ Инициализация...", msg.Document.FileName))
	initialMsg.ParseMode = b.parseModeValue()

	sentMsg, err := b.s.Send(initialMsg)
	if err != nil {
		log.Printf("⚠️ Failed to send initial document validation message: %v", err)
		b.sendMessage(msg.Chat.ID, "❌ Ошибка отправки сообщения")
		return
	}

	// Создаем progress tracker
	progressTracker := codevalidation.NewCodeValidationProgressTracker(b, msg.Chat.ID, sentMsg.MessageID, msg.Document.FileName, "")

	// Запускаем валидацию в горутине для неблокирующего выполнения
	go func() {
		// Загружаем и обрабатываем файл
		files, err := b.downloadAndProcessFile(file, msg.Document.FileName)
		if err != nil {
			log.Printf("❌ File processing failed: %v", err)
			errorMsg := fmt.Sprintf("❌ **Ошибка обработки файла**\n\n%s\n\n📄 **Файл:** %s", html.EscapeString(err.Error()), html.EscapeString(msg.Document.FileName))
			editMsg := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, errorMsg)
			editMsg.ParseMode = b.parseModeValue()
			if _, editErr := b.s.Send(editMsg); editErr != nil {
				log.Printf("⚠️ Failed to update error message: %v", editErr)
			}
			return
		}

		// Извлекаем пользовательский вопрос из описания к файлу
		var userQuestion string
		if msg.Caption != "" {
			log.Printf("📝 Document caption found: %s", msg.Caption)
			// Используем функцию DetectCodeInMessage для извлечения вопроса из описания
			hasCode, _, _, extractedQuestion, err := codevalidation.DetectCodeInMessage(ctx, b.llmClient, msg.Caption)
			if err != nil {
				log.Printf("⚠️ Failed to extract question from caption: %v", err)
			} else if extractedQuestion != "" {
				userQuestion = extractedQuestion
				log.Printf("❓ Extracted user question from document caption: %s", userQuestion)
			} else if !hasCode {
				// Если нет кода в описании, то вся caption может быть вопросом
				userQuestion = msg.Caption
				log.Printf("❓ Using entire caption as user question: %s", userQuestion)
			}
		}

		// Если вопроса нет, генерируем краткое описание проекта
		if userQuestion == "" {
			userQuestion = "Опиши этот проект: его структуру, основные технологии, назначение и архитектуру"
			log.Printf("📋 No user question found, using default project summary request")
		}

		// Обрабатываем запрос через Code Validation workflow с прогрессом и вопросом
		result, err := b.codeValidationWorkflow.ProcessProjectValidationWithQuestion(ctx, files, userQuestion, progressTracker)
		if err != nil {
			log.Printf("❌ Document validation workflow failed: %v", err)
			// Обновляем сообщение с ошибкой
			errorMsg := fmt.Sprintf("❌ **Ошибка валидации файла**\n\n%s\n\n📄 **Файл:** %s", html.EscapeString(err.Error()), html.EscapeString(msg.Document.FileName))
			editMsg := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, errorMsg)
			editMsg.ParseMode = b.parseModeValue()
			if _, editErr := b.s.Send(editMsg); editErr != nil {
				log.Printf("⚠️ Failed to update error message: %v", editErr)
			}
			return
		}

		// Устанавливаем финальный результат
		progressTracker.SetFinalResult(result)

		log.Printf("✅ Document validation completed successfully for: %s", msg.Document.FileName)
	}()
}

// handleCodeValidation обрабатывает валидацию кода с пользовательским вопросом
func (b *Bot) handleCodeValidation(ctx context.Context, msg *tgbotapi.Message, code, filename, userQuestion string) {
	log.Printf("🔍 Starting code validation for user %d", msg.From.ID)

	// Проверяем наличие code validation workflow
	if b.codeValidationWorkflow == nil {
		b.sendMessage(msg.Chat.ID, "❌ Валидация кода недоступна. Проверьте конфигурацию Docker.")
		return
	}

	// Отправляем начальное сообщение с прогрессом
	initialMsg := tgbotapi.NewMessage(msg.Chat.ID, "🔄 **Запуск валидации кода...**\n\n⏳ Инициализация...")
	initialMsg.ParseMode = b.parseModeValue()

	sentMsg, err := b.s.Send(initialMsg)
	if err != nil {
		log.Printf("⚠️ Failed to send initial code validation message: %v", err)
		b.sendMessage(msg.Chat.ID, "❌ Ошибка отправки сообщения")
		return
	}

	// Создаем progress tracker
	progressTracker := codevalidation.NewCodeValidationProgressTracker(b, msg.Chat.ID, sentMsg.MessageID, filename, "")

	// Запускаем валидацию в горутине для неблокирующего выполнения
	go func() {
		// Обрабатываем запрос через Code Validation workflow с прогрессом и пользовательским вопросом
		files := map[string]string{filename: code}
		result, err := b.codeValidationWorkflow.ProcessProjectValidationWithQuestion(ctx, files, userQuestion, progressTracker)
		if err != nil {
			log.Printf("❌ Code validation workflow failed: %v", err)
			// Обновляем сообщение с ошибкой
			errorMsg := fmt.Sprintf("❌ **Ошибка валидации кода**\n\n%s\n\n📄 **Файл:** %s", html.EscapeString(err.Error()), html.EscapeString(filename))
			editMsg := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, errorMsg)
			editMsg.ParseMode = b.parseModeValue()
			if _, editErr := b.s.Send(editMsg); editErr != nil {
				log.Printf("⚠️ Failed to update error message: %v", editErr)
			}
			return
		}

		// Устанавливаем финальный результат
		progressTracker.SetFinalResult(result)

		log.Printf("✅ Code validation completed successfully for: %s", filename)
	}()
}

// downloadAndProcessFile скачивает файл от Telegram и обрабатывает его (включая архивы)
func (b *Bot) downloadAndProcessFile(file tgbotapi.File, filename string) (map[string]string, error) {
	log.Printf("📥 Downloading file: %s", filename)

	// Скачиваем файл
	fileURL := file.Link(b.api.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// Читаем весь контент файла в память
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	log.Printf("📁 Processing file: %s, size: %d bytes", filename, len(content))

	// Определяем тип файла и обрабатываем соответственно
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".zip":
		return b.processZipArchive(content, filename)
	case ".tar":
		return b.processTarArchive(content, filename)
	case ".gz":
		if strings.HasSuffix(strings.ToLower(filename), ".tar.gz") {
			return b.processTarGzArchive(content, filename)
		}
		fallthrough
	default:
		// Обычный файл - просто возвращаем его содержимое
		return map[string]string{filename: string(content)}, nil
	}
}

// processZipArchive обрабатывает ZIP архивы
func (b *Bot) processZipArchive(data []byte, filename string) (map[string]string, error) {
	log.Printf("📦 Processing ZIP archive: %s", filename)

	// Создаем reader для zip данных
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to read ZIP archive: %w", err)
	}

	files := make(map[string]string)
	maxFiles := 50 // Ограничиваем количество файлов для безопасности
	fileCount := 0

	for _, f := range reader.File {
		if fileCount >= maxFiles {
			log.Printf("⚠️ ZIP archive contains too many files, limiting to %d", maxFiles)
			break
		}

		// Пропускаем директории и скрытые файлы
		if f.FileInfo().IsDir() || strings.HasPrefix(filepath.Base(f.Name), ".") {
			continue
		}

		// Пропускаем слишком большие файлы
		if f.UncompressedSize64 > 1024*1024 { // 1MB limit
			log.Printf("⚠️ Skipping large file: %s (%d bytes)", f.Name, f.UncompressedSize64)
			continue
		}

		// Читаем файл
		rc, err := f.Open()
		if err != nil {
			log.Printf("⚠️ Failed to open file %s in ZIP: %v", f.Name, err)
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Printf("⚠️ Failed to read file %s in ZIP: %v", f.Name, err)
			continue
		}
		log.Printf("ℹ️ Extracted file %s", f.Name)

		files[f.Name] = string(content)
		fileCount++
	}

	log.Printf("✅ Extracted %d files from ZIP archive", fileCount)
	return files, nil
}

// processTarArchive обрабатывает TAR архивы
func (b *Bot) processTarArchive(data []byte, filename string) (map[string]string, error) {
	log.Printf("📦 Processing TAR archive: %s", filename)

	reader := tar.NewReader(bytes.NewReader(data))
	files := make(map[string]string)
	maxFiles := 50 // Ограничиваем количество файлов для безопасности
	fileCount := 0

	for {
		if fileCount >= maxFiles {
			log.Printf("⚠️ TAR archive contains too many files, limiting to %d", maxFiles)
			break
		}

		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read TAR entry: %w", err)
		}

		// Пропускаем директории и скрытые файлы
		if header.Typeflag == tar.TypeDir || strings.HasPrefix(filepath.Base(header.Name), ".") {
			continue
		}

		// Пропускаем слишком большие файлы
		if header.Size > 1024*1024 { // 1MB limit
			log.Printf("⚠️ Skipping large file: %s (%d bytes)", header.Name, header.Size)
			continue
		}

		content, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("⚠️ Failed to read file %s in TAR: %v", header.Name, err)
			continue
		}

		files[header.Name] = string(content)
		fileCount++
	}

	log.Printf("✅ Extracted %d files from TAR archive", fileCount)
	return files, nil
}

// processTarGzArchive обрабатывает TAR.GZ архивы
func (b *Bot) processTarGzArchive(data []byte, filename string) (map[string]string, error) {
	log.Printf("📦 Processing TAR.GZ archive: %s", filename)

	// Сначала распаковываем gzip
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Читаем распакованные данные
	uncompressedData, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip data: %w", err)
	}

	// Теперь обрабатываем как обычный TAR
	return b.processTarArchive(uncompressedData, filename)
}
