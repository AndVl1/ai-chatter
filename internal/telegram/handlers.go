package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/storage"
)

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
	result := b.mcpClient.CreateDialogSummary(
		ctx,
		args, // title
		content.String(),
		fmt.Sprintf("%d", msg.From.ID),
		msg.From.UserName,
		"dialog_summary",
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
