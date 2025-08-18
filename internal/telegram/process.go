package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/llm"
	"ai-chatter/internal/storage"
)

// moved types live in bot.go currently; keep helpers here only if not duplicated
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

// Checker response from model2
type checkerJSON struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

func parseCheckerJSON(s string) (checkerJSON, bool) {
	var c checkerJSON
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return checkerJSON{}, false
	}
	if c.Status == "ok" || c.Status == "fail" {
		return c, true
	}
	return checkerJSON{}, false
}

func buildCheckerPrompt() string {
	return "Ты — модель-проверяющий статуса другой модели в режиме составления ТЗ. " +
		"Тебе передают только два поля из ответа: 'answer' и 'status'. " +
		"'status' может быть 'continue' или 'final'. Статус 'continue' должен содержать в себе" +
		"уточняющие вопросы, статус 'final' – итоговое ТЗ. " +
		"Проверь, соответствует ли выбранный статус " +
		"здравому смыслу, исходя из информативности/конкретности сообщения (например, 'continue' " +
		"всегда должен содержать вопросы, 'final' – итоговое ТЗ). " +
		"Верни строго JSON {\"status\": \"ok|fail\", \"msg\": \"если fail — кратко что " +
		"исправить (например: 'уточнить требования'), иначе пусто\"}. Не использую какого-либо форматирования, только JSON" +
		" чистым текстом"
}

func buildCheckerInput(answer, status string) string {
	return fmt.Sprintf("answer: %s\nstatus: %s", strings.TrimSpace(answer), strings.TrimSpace(status))
}

func (b *Bot) runTZChecker(ctx context.Context, userID int64, lastPrimary string) (checkerJSON, llm.Response, error) {
	msgs := []llm.Message{
		{Role: "system", Content: buildCheckerPrompt()},
		{Role: "user", Content: lastPrimary},
	}
	b.logLLMRequest(userID, "tz_check", msgs)
	resp, err := b.getSecondLLMClient().Generate(ctx, msgs)
	if err != nil {
		return checkerJSON{}, llm.Response{}, err
	}
	b.logResponse(resp)
	cj, ok := parseCheckerJSON(resp.Content)
	// Persist checker response for audit (not used in context)
	if b.recorder != nil {
		f := false
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: userID, UserMessage: "[tz_check]", AssistantResponse: resp.Content, CanUse: &f})
	}
	if !ok {
		return checkerJSON{}, resp, fmt.Errorf("checker returned invalid schema")
	}
	return cj, resp, nil
}

func (b *Bot) correctPrimaryWithMsg(ctx context.Context, userID int64, original string, msg string) (llmJSON, llm.Response, error) {
	instr := "Исправь предыдущий ответ согласно замечаниям: " + msg + ". Сохрани строгую JSON-схему {title, answer, compressed_context, status}."
	// Persist correction request intent
	if b.recorder != nil {
		f := false
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: userID, UserMessage: "[tz_correct_req]", AssistantResponse: msg, CanUse: &f})
	}
	msgs := []llm.Message{{Role: "system", Content: instr}, {Role: "user", Content: original}}
	b.logLLMRequest(userID, "tz_correct", msgs)
	resp, err := b.getLLMClient().Generate(ctx, msgs)
	if err != nil {
		return llmJSON{}, llm.Response{}, err
	}
	p, ok := parseLLMJSON(resp.Content)
	if !ok {
		return llmJSON{}, resp, fmt.Errorf("primary returned invalid JSON on correction")
	}
	return p, resp, nil
}

func compactJSON(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var anyJson interface{}
	if err := json.Unmarshal(raw, &anyJson); err != nil {
		return "", false
	}
	b, err := json.Marshal(anyJson)
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

// Auto-numbering for questions in TZ mode when status=continue
func isNumberedLine(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	// scan leading digits
	i := 0
	for i < len(ss) && ss[i] >= '0' && ss[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	if i < len(ss) && ss[i] == '.' {
		return true
	}
	return false
}

func enforceNumberedListIfNeeded(answer string) string {
	lines := strings.Split(answer, "\n")
	var content []string
	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		if l != "" {
			content = append(content, l)
		}
	}
	if len(content) < 2 {
		return answer
	}
	// if already has 2+ numbered lines, keep as is
	num := 0
	for _, l := range content {
		if isNumberedLine(l) {
			num++
		}
	}
	if num >= 2 {
		return answer
	}
	// produce numbered
	var out []string
	for i, l := range content {
		out = append(out, fmt.Sprintf("%d. %s", i+1, l))
	}
	return strings.Join(out, "\n")
}

// reformatToSchema is defined in bot.go (single owner)

// buildContextWithOverflow is defined in bot.go

func (b *Bot) processLLMAndRespond(ctx context.Context, chatID int64, userID int64, resp llm.Response) {
	b.processLLMAndRespondWithMCP(ctx, chatID, userID, resp, nil)
}

func (b *Bot) processLLMAndRespondWithMCP(ctx context.Context, chatID int64, userID int64, resp llm.Response, mcpFunctionCalls []string) {
	// log inbound
	b.logResponse(resp)

	// Обрабатываем function calls если они есть
	if len(resp.ToolCalls) > 0 {
		b.handleFunctionCalls(ctx, chatID, userID, resp.ToolCalls)
		return
	}

	parsed, ok := parseLLMJSON(resp.Content)
	if !ok {
		if p2, ok2 := b.reformatToSchema(ctx, userID, resp.Content); ok2 {
			parsed = p2
			ok = true
		}
	}

	compressed := false
	if ok && strings.TrimSpace(parsed.CompressedContext) != "" {
		b.addUserSystemPrompt(userID, parsed.CompressedContext)
		b.history.DisableAll(userID)
		compressed = true
	}
	answerToSend := resp.Content
	if ok && parsed.Answer != "" {
		answerToSend = parsed.Answer
	}
	status := ""
	if ok {
		status = strings.ToLower(strings.TrimSpace(parsed.Status))
	}

	// Checker and possible correction: provide only title+status
	if b.isTZMode(userID) {
		checkerInput := buildCheckerInput(parsed.Answer, parsed.Status)
		if cj, _, err := b.runTZChecker(ctx, userID, checkerInput); err == nil {
			if strings.ToLower(cj.Status) == "fail" && strings.TrimSpace(cj.Msg) != "" {
				if pFix, _, errFix := b.correctPrimaryWithMsg(ctx, userID, answerToSend, cj.Msg); errFix == nil {
					parsed = pFix
					answerToSend = pFix.Answer
					status = strings.ToLower(strings.TrimSpace(pFix.Status))
					if strings.TrimSpace(pFix.CompressedContext) != "" {
						b.addUserSystemPrompt(userID, pFix.CompressedContext)
						b.history.DisableAll(userID)
						compressed = true
					}
				}
			}
		}
	}

	// Enforce numbered list for questions while clarifying TZ
	if b.isTZMode(userID) && status != "final" {
		answerToSend = enforceNumberedListIfNeeded(answerToSend)
	}

	// TZ steps control in both paths
	if b.isTZMode(userID) && status != "final" {
		left := b.decTZRemaining(userID)
		if left <= 0 {
			if pFinal, respFinal, okFinal := b.produceFinalTS(ctx, userID); okFinal {
				b.sendFinalTS(chatID, userID, pFinal, respFinal)
				return
			}
		}
	}

	// Unified final handling: send via sendFinalTS and stop
	if b.isTZMode(userID) && status == "final" {
		b.sendFinalTSWithMCP(chatID, userID, parsed, resp, mcpFunctionCalls)
		return
	}

	used := !compressed
	b.history.AppendAssistantWithUsed(userID, answerToSend, used)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{
			Timestamp:         time.Now().UTC(),
			UserID:            userID,
			AssistantResponse: answerToSend,
			CanUse:            &tru,
			MCPFunctionCalls:  mcpFunctionCalls,
		})
	}

	metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	metaEsc := b.escapeIfNeeded(metaLine)
	body := answerToSend
	if ok && parsed.Title != "" {
		body = b.formatTitleAnswer(parsed.Title, answerToSend)
	}
	final := metaEsc + "\n\n" + body
	msgOut := tgbotapi.NewMessage(chatID, final)
	msgOut.ReplyMarkup = b.menuKeyboard()
	msgOut.ParseMode = b.parseModeValue()
	_, _ = b.s.Send(msgOut)
}

func (b *Bot) sendFinalTS(chatID, userID int64, p llmJSON, resp llm.Response) {
	b.sendFinalTSWithMCP(chatID, userID, p, resp, nil)
}

func (b *Bot) sendFinalTSWithMCP(chatID, userID int64, p llmJSON, resp llm.Response, mcpFunctionCalls []string) {
	answerToSend := p.Answer
	if p.Title != "" {
		answerToSend = b.formatTitleAnswer(p.Title, p.Answer)
	}
	b.history.AppendAssistantWithUsed(userID, answerToSend, true)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{
			Timestamp:         time.Now().UTC(),
			UserID:            userID,
			AssistantResponse: answerToSend,
			CanUse:            &tru,
			MCPFunctionCalls:  mcpFunctionCalls,
		})
	}
	metaLine := fmt.Sprintf("[model=%s, tokens: prompt=%d, completion=%d, total=%d]", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
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
	msgOut := tgbotapi.NewMessage(chatID, final)
	msgOut.ReplyMarkup = b.menuKeyboard()
	msgOut.ParseMode = b.parseModeValue()
	_, _ = b.s.Send(msgOut)

	log.Println("Готовим инструкцию")
	// Announce instruction preparation
	prep := tgbotapi.NewMessage(chatID, b.escapeIfNeeded("Готовлю инструкцию по итоговому ТЗ…"))
	prep.ParseMode = b.parseModeValue()
	_, _ = b.s.Send(prep)

	// Call secondary model to generate actionable instructions
	ctx := context.Background()
	instructionPrompt := buildInstructionPrompt(p)
	msgs := []llm.Message{{Role: "system", Content: instructionPrompt}}
	b.logLLMRequest(userID, "tz_instructions", msgs)
	resp2, err := b.getSecondLLMClient().Generate(ctx, msgs)
	if err != nil {
		log.Printf("second model error: %v", err)
		errMsg := tgbotapi.NewMessage(chatID, b.escapeIfNeeded("Не удалось подготовить инструкцию. Попробуйте ещё раз."))
		errMsg.ParseMode = b.parseModeValue()
		_, _ = b.s.Send(errMsg)
		b.clearTZState(userID)
		return
	}
	b.logResponse(resp2)
	// Try to parse as our JSON; if not, send as is
	if p2, ok := parseLLMJSON(resp2.Content); ok && strings.TrimSpace(p2.Answer) != "" {
		inst := p2.Answer
		if p2.Title != "" {
			inst = b.formatTitleAnswer(p2.Title, p2.Answer)
		}
		msg2 := tgbotapi.NewMessage(chatID, inst)
		msg2.ParseMode = b.parseModeValue()
		msg2.ReplyMarkup = b.menuKeyboard()
		_, _ = b.s.Send(msg2)
	} else {
		msg2 := tgbotapi.NewMessage(chatID, resp2.Content)
		msg2.ParseMode = b.parseModeValue()
		msg2.ReplyMarkup = b.menuKeyboard()
		_, _ = b.s.Send(msg2)
	}
	b.clearTZState(userID)
}

func buildInstructionPrompt(ts llmJSON) string {
	// Keep it simple and provider-agnostic; instruction in Russian
	return "Ты получаешь итоговое техническое задание (ТЗ). На его основе составь детальную пошаговую инструкцию действий для пользователя в русском языке." +
		" Наример, если это кулинарный рецепт — выдай полный рецепт с этапами и ингредиентами;" +
		" если это разработка — выдай рекомендуемый стек, этапы работ, приоритеты и зависимости; и так далее" +
		" Будь конкретным: нумеруй шаги, пиши каждый шаг с новой строки. Не добавляй лишний контент, не обсуждай сам процесс составления ТЗ." +
		" Ответ верни в понятном человеку виде без JSON формата " +
		"\n\nИтоговое ТЗ:\n" + ts.Answer
}

func (b *Bot) logResponse(resp llm.Response) {
	log.Printf("LLM response [model=%s, tokens: prompt=%d, completion=%d, total=%d]: %q", resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens, resp.Content)
}

func (b *Bot) nowUTC() time.Time { return time.Now().UTC() }

// handleFunctionCalls обрабатывает вызовы функций от LLM
func (b *Bot) handleFunctionCalls(ctx context.Context, chatID, userID int64, toolCalls []llm.ToolCall) {
	if b.mcpClient == nil {
		b.sendMessage(chatID, "Notion интеграция не настроена.")
		return
	}

	// Собираем результаты всех tool calls
	toolResults := make([]llm.ToolCallResult, 0, len(toolCalls))

	// Собираем названия вызванных функций для логирования
	mcpFunctionCalls := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		mcpFunctionCalls = append(mcpFunctionCalls, tc.Function.Name)
	}

	for _, tc := range toolCalls {
		switch tc.Function.Name {
		case "save_dialog_to_notion":
			// Отправляем уведомление о начале операции
			b.sendMessage(chatID, "💾 Сохраняю диалог в Notion...")

			title, ok := tc.Function.Arguments["title"].(string)
			if !ok || title == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не указано название страницы",
				})
				continue
			}

			// Собираем контекст диалога
			history := b.history.Get(userID)
			if len(history) == 0 {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: история диалога пуста",
				})
				continue
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

			// Проверяем настройку parent page
			if b.notionParentPage == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не настроен NOTION_PARENT_PAGE_ID",
				})
				continue
			}

			result := b.mcpClient.CreateDialogSummary(
				ctx, title, content.String(),
				fmt.Sprintf("%d", userID),
				getUsernameFromID(userID),
				"dialog_summary",
				b.notionParentPage,
			)

			if result.Success {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Диалог успешно сохранён в Notion под названием '%s'. Page ID: %s", title, result.PageID),
				})
			} else {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Ошибка сохранения: %s", result.Message),
				})
			}

		case "search_notion":
			// Отправляем уведомление о начале поиска
			b.sendMessage(chatID, "🔍 Ищу в Notion...")

			query, ok := tc.Function.Arguments["query"].(string)
			if !ok || query == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не указан поисковый запрос",
				})
				continue
			}

			result := b.mcpClient.SearchDialogSummaries(
				ctx, query,
				fmt.Sprintf("%d", userID),
				"dialog_summary",
			)

			if result.Success {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Результаты поиска по запросу '%s': %s", query, result.Message),
				})
			} else {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Ошибка поиска: %s", result.Message),
				})
			}

		case "create_notion_page":
			// Отправляем уведомление о начале создания
			b.sendMessage(chatID, "📝 Создаю страницу в Notion...")

			title, ok := tc.Function.Arguments["title"].(string)
			if !ok || title == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не указано название страницы",
				})
				continue
			}

			content, ok := tc.Function.Arguments["content"].(string)
			if !ok || content == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не указано содержимое страницы",
				})
				continue
			}

			// Поддерживаем и старый parent_page и новый parent_page_id
			parentPage, _ := tc.Function.Arguments["parent_page"].(string)
			parentPageID, _ := tc.Function.Arguments["parent_page_id"].(string)

			// Приоритет у parent_page_id
			if parentPageID != "" {
				parentPage = parentPageID
			} else if parentPage == "" {
				// Если не указан ни parent_page, ни parent_page_id, используем default
				if b.notionParentPage == "" {
					toolResults = append(toolResults, llm.ToolCallResult{
						ToolCallID: tc.ID,
						Content:    "Ошибка: не настроен NOTION_PARENT_PAGE_ID",
					})
					continue
				}
				parentPage = b.notionParentPage
			}

			result := b.mcpClient.CreateFreeFormPage(ctx, title, content, parentPage, nil)

			if result.Success {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Страница '%s' успешно создана в Notion. Page ID: %s", title, result.PageID),
				})
			} else {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Ошибка создания страницы: %s", result.Message),
				})
			}

		case "search_pages_with_id":
			// Отправляем уведомление о начале поиска страниц
			b.sendMessage(chatID, "🔍 Ищу страницы в Notion...")

			query, ok := tc.Function.Arguments["query"].(string)
			if !ok || query == "" {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не указан поисковый запрос",
				})
				continue
			}

			// Извлекаем параметры
			var limit int
			if limitVal, ok := tc.Function.Arguments["limit"].(float64); ok {
				limit = int(limitVal)
			}

			exactMatch := false
			if exactVal, ok := tc.Function.Arguments["exact_match"].(bool); ok {
				exactMatch = exactVal
			}

			result := b.mcpClient.SearchPagesWithID(ctx, query, limit, exactMatch)

			if result.Success {
				if len(result.Pages) == 0 {
					toolResults = append(toolResults, llm.ToolCallResult{
						ToolCallID: tc.ID,
						Content:    fmt.Sprintf("Страницы по запросу '%s' не найдены", query),
					})
				} else {
					responseText := fmt.Sprintf("Найдено %d страниц по запросу '%s':", len(result.Pages), query)
					for i, page := range result.Pages {
						responseText += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, page.Title, page.ID)
					}
					toolResults = append(toolResults, llm.ToolCallResult{
						ToolCallID: tc.ID,
						Content:    responseText,
					})
				}
			} else {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Ошибка поиска страниц: %s", result.Message),
				})
			}

		case "list_available_pages":
			// Отправляем уведомление о получении списка страниц
			b.sendMessage(chatID, "📋 Получаю список доступных страниц...")

			// Извлекаем параметры
			var limit int
			if limitVal, ok := tc.Function.Arguments["limit"].(float64); ok {
				limit = int(limitVal)
			}

			pageType := ""
			if typeVal, ok := tc.Function.Arguments["page_type"].(string); ok {
				pageType = typeVal
			}

			parentOnly := false
			if parentVal, ok := tc.Function.Arguments["parent_only"].(bool); ok {
				parentOnly = parentVal
			}

			result := b.mcpClient.ListAvailablePages(ctx, limit, pageType, parentOnly)

			if result.Success {
				if len(result.Pages) == 0 {
					toolResults = append(toolResults, llm.ToolCallResult{
						ToolCallID: tc.ID,
						Content:    "📋 Доступные страницы не найдены",
					})
				} else {
					responseText := fmt.Sprintf("📋 Найдено %d доступных страниц:", len(result.Pages))
					for i, page := range result.Pages {
						responseText += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, page.Title, page.ID)
						if page.CanBeParent {
							responseText += " ✅"
						}
					}
					toolResults = append(toolResults, llm.ToolCallResult{
						ToolCallID: tc.ID,
						Content:    responseText,
					})
				}
			} else {
				toolResults = append(toolResults, llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Ошибка получения списка страниц: %s", result.Message),
				})
			}

		default:
			toolResults = append(toolResults, llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Неизвестная функция: %s", tc.Function.Name),
			})
			log.Printf("Unknown function call: %s", tc.Function.Name)
		}
	}

	// Теперь отправляем результаты обратно в LLM для формирования ответа
	if len(toolResults) > 0 {
		b.continueConversationWithToolResults(ctx, chatID, userID, toolResults, mcpFunctionCalls)
	}
}

// continueConversationWithToolResults продолжает диалог с результатами tool calls
func (b *Bot) continueConversationWithToolResults(ctx context.Context, chatID, userID int64, toolResults []llm.ToolCallResult, mcpFunctionCalls []string) {
	b.continueConversationWithToolResultsRecursive(ctx, chatID, userID, toolResults, mcpFunctionCalls, 0)
}

// continueConversationWithToolResultsRecursive обрабатывает цепочки function calls рекурсивно
func (b *Bot) continueConversationWithToolResultsRecursive(ctx context.Context, chatID, userID int64, toolResults []llm.ToolCallResult, mcpFunctionCalls []string, depth int) {
	// Ограничиваем глубину рекурсии для предотвращения бесконечных циклов
	const maxDepth = 5
	if depth >= maxDepth {
		log.Printf("⚠️ Достигнута максимальная глубина function calls (%d), прекращаем цепочку", maxDepth)
		b.sendMessage(chatID, "✅ Операции выполнены успешно.")
		return
	}

	// Получаем текущий контекст
	contextMsgs := b.buildContextWithOverflow(ctx, userID)

	// Добавляем результаты tool calls
	for _, result := range toolResults {
		contextMsgs = append(contextMsgs, llm.Message{
			Role:       "tool",
			Content:    result.Content,
			ToolCallID: result.ToolCallID,
		})
	}

	// Настраиваем инструкцию в зависимости от глубины
	var systemInstruction string
	if depth == 0 {
		systemInstruction = "Проанализируй результаты выполненных действий. АВТОМАТИЧЕСКИ продолжай выполнение задачи:\n\n- Если найдена страница 'Reports', создай в ней подстраницу с отчётом\n- Если нужно создать страницу Reports, создай её и затем отчёт\n- Если создаёшь отчёт, используй полную статистику и анализ\n- НЕ спрашивай разрешения, выполняй действия автоматически\n\nЕсли все действия завершены, сообщи о результате с ссылкой на созданную страницу."
	} else {
		systemInstruction = "Продолжай автоматическое выполнение задачи на основе предыдущих результатов. НЕ останавливайся, пока задача не будет полностью завершена. Если все действия завершены, сформулируй финальный ответ пользователю."
	}

	contextMsgs = append(contextMsgs, llm.Message{
		Role:    "system",
		Content: systemInstruction,
	})

	b.logLLMRequest(userID, fmt.Sprintf("tool_response_depth_%d", depth), contextMsgs)

	// Получаем ответ от LLM с tools
	tools := llm.GetNotionTools()
	resp, err := b.getLLMClient().GenerateWithTools(ctx, contextMsgs, tools)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("Действия выполнены, но произошла ошибка формирования ответа: %v", err))
		return
	}

	// Если есть новые function calls, обрабатываем их рекурсивно
	if len(resp.ToolCalls) > 0 {
		log.Printf("🔄 Обработка дополнительных function calls на глубине %d", depth+1)

		// Собираем результаты новых tool calls
		newToolResults := make([]llm.ToolCallResult, 0, len(resp.ToolCalls))
		newMCPFunctionCalls := make([]string, 0, len(resp.ToolCalls))

		for _, tc := range resp.ToolCalls {
			newMCPFunctionCalls = append(newMCPFunctionCalls, tc.Function.Name)
		}

		// Выполняем новые function calls
		if b.mcpClient != nil {
			for _, tc := range resp.ToolCalls {
				result := b.executeSingleFunctionCall(ctx, chatID, userID, tc)
				newToolResults = append(newToolResults, result)
			}
		}

		// Объединяем с предыдущими вызовами для логирования
		allMCPCalls := append(mcpFunctionCalls, newMCPFunctionCalls...)

		// Рекурсивно продолжаем с новыми результатами
		b.continueConversationWithToolResultsRecursive(ctx, chatID, userID, newToolResults, allMCPCalls, depth+1)
		return
	}

	// Нет новых function calls - завершаем цепочку
	b.processLLMAndRespondWithMCP(ctx, chatID, userID, resp, mcpFunctionCalls)
}

// executeSingleFunctionCall выполняет один вызов функции и возвращает результат
func (b *Bot) executeSingleFunctionCall(ctx context.Context, chatID, userID int64, tc llm.ToolCall) llm.ToolCallResult {
	switch tc.Function.Name {
	case "save_dialog_to_notion":
		title, ok := tc.Function.Arguments["title"].(string)
		if !ok || title == "" {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: не указано название страницы",
			}
		}

		// Собираем контекст диалога
		history := b.history.Get(userID)
		if len(history) == 0 {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: история диалога пуста",
			}
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

		// Проверяем настройку parent page
		if b.notionParentPage == "" {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: не настроен NOTION_PARENT_PAGE_ID",
			}
		}

		result := b.mcpClient.CreateDialogSummary(
			ctx, title, content.String(),
			fmt.Sprintf("%d", userID),
			getUsernameFromID(userID),
			"dialog_summary",
			b.notionParentPage,
		)

		if result.Success {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Диалог успешно сохранён в Notion под названием '%s'. Page ID: %s", title, result.PageID),
			}
		} else {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Ошибка сохранения: %s", result.Message),
			}
		}

	case "create_notion_page":
		title, ok := tc.Function.Arguments["title"].(string)
		if !ok || title == "" {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: не указано название страницы",
			}
		}

		content, ok := tc.Function.Arguments["content"].(string)
		if !ok || content == "" {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: не указано содержимое страницы",
			}
		}

		// Поддерживаем и старый parent_page и новый parent_page_id
		parentPage, _ := tc.Function.Arguments["parent_page"].(string)
		parentPageID, _ := tc.Function.Arguments["parent_page_id"].(string)

		// Приоритет у parent_page_id
		if parentPageID != "" {
			parentPage = parentPageID
		} else if parentPage == "" {
			// Если не указан ни parent_page, ни parent_page_id, используем default
			if b.notionParentPage == "" {
				return llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "Ошибка: не настроен NOTION_PARENT_PAGE_ID",
				}
			}
			parentPage = b.notionParentPage
		}

		result := b.mcpClient.CreateFreeFormPage(ctx, title, content, parentPage, nil)

		if result.Success {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Страница '%s' успешно создана в Notion. Page ID: %s", title, result.PageID),
			}
		} else {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Ошибка создания страницы: %s", result.Message),
			}
		}

	case "search_pages_with_id":
		query, ok := tc.Function.Arguments["query"].(string)
		if !ok || query == "" {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    "Ошибка: не указан поисковый запрос",
			}
		}

		// Извлекаем параметры
		var limit int
		if limitVal, ok := tc.Function.Arguments["limit"].(float64); ok {
			limit = int(limitVal)
		}

		exactMatch := false
		if exactVal, ok := tc.Function.Arguments["exact_match"].(bool); ok {
			exactMatch = exactVal
		}

		result := b.mcpClient.SearchPagesWithID(ctx, query, limit, exactMatch)

		if result.Success {
			if len(result.Pages) == 0 {
				return llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("Страницы по запросу '%s' не найдены", query),
				}
			} else {
				responseText := fmt.Sprintf("Найдено %d страниц по запросу '%s':", len(result.Pages), query)
				for i, page := range result.Pages {
					responseText += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, page.Title, page.ID)
				}
				return llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    responseText,
				}
			}
		} else {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Ошибка поиска страниц: %s", result.Message),
			}
		}

	case "list_available_pages":
		// Извлекаем параметры
		var limit int
		if limitVal, ok := tc.Function.Arguments["limit"].(float64); ok {
			limit = int(limitVal)
		}

		pageType := ""
		if typeVal, ok := tc.Function.Arguments["page_type"].(string); ok {
			pageType = typeVal
		}

		parentOnly := false
		if parentVal, ok := tc.Function.Arguments["parent_only"].(bool); ok {
			parentOnly = parentVal
		}

		result := b.mcpClient.ListAvailablePages(ctx, limit, pageType, parentOnly)

		if result.Success {
			if len(result.Pages) == 0 {
				return llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    "📋 Доступные страницы не найдены",
				}
			} else {
				responseText := fmt.Sprintf("📋 Найдено %d доступных страниц:", len(result.Pages))
				for i, page := range result.Pages {
					responseText += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, page.Title, page.ID)
					if page.CanBeParent {
						responseText += " ✅"
					}
				}
				return llm.ToolCallResult{
					ToolCallID: tc.ID,
					Content:    responseText,
				}
			}
		} else {
			return llm.ToolCallResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("Ошибка получения списка страниц: %s", result.Message),
			}
		}

	default:
		return llm.ToolCallResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Неизвестная функция: %s", tc.Function.Name),
		}
	}
}

// getUsernameFromID возвращает имя пользователя по ID (упрощённая версия)
func getUsernameFromID(userID int64) string {
	return fmt.Sprintf("user_%d", userID)
}
