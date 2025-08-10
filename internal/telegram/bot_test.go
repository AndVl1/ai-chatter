package telegram

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/llm"
)

type fakeSender struct{ sent []string }

func (f *fakeSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	sw := c.(tgbotapi.MessageConfig)
	f.sent = append(f.sent, sw.Text)
	return tgbotapi.Message{}, nil
}

type fakeLLM struct {
	resp llm.Response
	err  error
}

func (f fakeLLM) Generate(ctx context.Context, msgs []llm.Message) (llm.Response, error) {
	return f.resp, f.err
}

func TestUnauthorizedFlow_SendsPendingAndAdminNotify(t *testing.T) {
	b := &Bot{
		s:           &fakeSender{},
		authSvc:     &auth.Service{},
		pending:     make(map[int64]auth.User),
		adminUserID: 999,
	}
	b.notifyAdminRequest(123, "user")
	fs := b.s.(*fakeSender)
	if len(fs.sent) != 1 || !strings.Contains(fs.sent[0], "хочет пользоваться ботом") {
		t.Fatalf("admin notify not sent: %+v", fs.sent)
	}
}

func TestSendMessage_UsesParseMode(t *testing.T) {
	b := &Bot{s: &fakeSender{}, parseMode: "Markdown"}
	b.sendMessage(1, "**bold**")
	fs := b.s.(*fakeSender)
	if len(fs.sent) != 1 || fs.sent[0] != "**bold**" {
		t.Fatalf("unexpected sent: %+v", fs.sent)
	}
}
