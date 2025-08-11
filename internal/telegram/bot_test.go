package telegram

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"ai-chatter/internal/auth"
	"ai-chatter/internal/history"
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

func TestHandleIncomingMessage_ParsesJSONAndSendsFormatted(t *testing.T) {
	userID := int64(42)
	svc, err := auth.NewWithRepo(nil, []int64{userID})
	if err != nil {
		t.Fatalf("auth init: %v", err)
	}
	fs := &fakeSender{}
	jsonResp := `{"title":"My Title","answer":"My Answer body","meta":"my-internal-meta"}`
	b := &Bot{
		s:         fs,
		authSvc:   svc,
		llmClient: fakeLLM{resp: llm.Response{Content: jsonResp, Model: "test-model"}},
		pending:   make(map[int64]auth.User),
		parseMode: "HTML",
	}
	b.history = history.NewManager()

	msg := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 100}, Text: "hello"}
	b.handleIncomingMessage(context.Background(), msg)

	if len(fs.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(fs.sent))
	}
	out := fs.sent[0]
	if strings.Contains(out, `"title"`) || strings.Contains(out, `{"`) {
		t.Fatalf("raw JSON leaked to user: %q", out)
	}
	if !strings.Contains(out, "<b>My Title</b>") || !strings.Contains(out, "My Answer body") {
		t.Fatalf("formatted title/answer missing: %q", out)
	}
}

func TestHandleIncomingMessage_ParsesJSONWithObjectMeta(t *testing.T) {
	userID := int64(77)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	jsonResp := `{"title":"T","answer":"1. **bold**\n\n<code>python\nprint('hi')\n</code>","meta":{"a":1,"b":"x"}}`
	b := &Bot{
		s:         fs,
		authSvc:   svc,
		llmClient: fakeLLM{resp: llm.Response{Content: jsonResp, Model: "m"}},
		pending:   make(map[int64]auth.User),
		parseMode: "HTML",
	}
	b.history = history.NewManager()

	msg := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 200}, Text: "x"}
	b.handleIncomingMessage(context.Background(), msg)

	if len(fs.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(fs.sent))
	}
	out := fs.sent[0]
	if strings.Contains(out, `{"`) {
		t.Fatalf("raw JSON leaked: %q", out)
	}
	if !strings.Contains(out, "<b>T</b>") {
		t.Fatalf("title not formatted: %q", out)
	}
	if !strings.Contains(out, "<code>python") {
		t.Fatalf("code block lost/escaped: %q", out)
	}
}
