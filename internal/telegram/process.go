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
	// log inbound
	b.logResponse(resp)
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
		b.sendFinalTS(chatID, userID, parsed, resp)
		return
	}

	used := !compressed
	b.history.AppendAssistantWithUsed(userID, answerToSend, used)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: userID, AssistantResponse: answerToSend, CanUse: &tru})
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
	answerToSend := p.Answer
	if p.Title != "" {
		answerToSend = b.formatTitleAnswer(p.Title, p.Answer)
	}
	b.history.AppendAssistantWithUsed(userID, answerToSend, true)
	if b.recorder != nil {
		tru := true
		_ = b.recorder.AppendInteraction(storage.Event{Timestamp: time.Now().UTC(), UserID: userID, AssistantResponse: answerToSend, CanUse: &tru})
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
