package telegram

import (
	"context"
	"encoding/json"
	"fmt"
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

// reformatToSchema is defined in bot.go (single owner)

// buildContextWithOverflow is defined in bot.go

func (b *Bot) processLLMAndRespond(ctx context.Context, chatID int64, userID int64, resp llm.Response, tzBootstrap bool) {
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
	if status == "final" && b.isTZMode(userID) {
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
		b.clearTZState(userID)
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
	b.clearTZState(userID)
}

func (b *Bot) logResponse(resp llm.Response) {
	// already logged request; here we ensure response details are printed
	// printed elsewhere too when needed
}

func (b *Bot) nowUTC() time.Time { return time.Now().UTC() }
