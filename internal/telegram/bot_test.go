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

type fakeLLM struct {
	resp llm.Response
	err  error
}

type fakeLLMSeq struct {
	seq      []llm.Response
	calls    int
	lastMsgs [][]llm.Message
}

func (f *fakeLLMSeq) Generate(ctx context.Context, msgs []llm.Message) (llm.Response, error) {
	f.lastMsgs = append(f.lastMsgs, append([]llm.Message(nil), msgs...))
	idx := f.calls
	if idx >= len(f.seq) {
		idx = len(f.seq) - 1
	}
	f.calls++
	return f.seq[idx], nil
}

func (f *fakeSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	sw := c.(tgbotapi.MessageConfig)
	f.sent = append(f.sent, sw.Text)
	return tgbotapi.Message{}, nil
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

func TestTZ_BootstrapFinalAddsHeader(t *testing.T) {
	userID := int64(1001)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	seq := &fakeLLMSeq{seq: []llm.Response{{Content: `{"title":"T","answer":"A","status":"final"}`, Model: "m"}}}
	b := &Bot{
		s:         fs,
		authSvc:   svc,
		llmClient: seq,
		pending:   make(map[int64]auth.User),
		parseMode: "HTML",
	}
	b.history = history.NewManager()

	// Simulate /tz command
	msg := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 300}, Text: "/tz topic"}
	ents := []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 3}}
	msg.Entities = ents
	b.handleCommand(msg)

	if len(fs.sent) != 1 {
		t.Fatalf("expected bootstrap answer, got %d", len(fs.sent))
	}
	out := fs.sent[0]
	if !strings.Contains(out, "ТЗ Готово") {
		t.Fatalf("missing final header: %q", out)
	}
	if b.isTZMode(userID) {
		t.Fatalf("tz mode should be off after final")
	}
}

func TestCompressedContext_DisablesHistoryAndAddsToSystem(t *testing.T) {
	userID := int64(2222)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	seq := &fakeLLMSeq{seq: []llm.Response{
		{Content: `{"title":"t1","answer":"a1","compressed_context":"facts"}`, Model: "m"},
		{Content: `{"title":"t2","answer":"a2"}`, Model: "m"},
	}}
	b := &Bot{
		s:            fs,
		authSvc:      svc,
		llmClient:    seq,
		pending:      make(map[int64]auth.User),
		parseMode:    "HTML",
		systemPrompt: "base",
	}
	b.history = history.NewManager()

	// First message produces compressed_context
	m1 := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 400}, Text: "hi"}
	b.handleIncomingMessage(context.Background(), m1)
	// Second message should use system(prompt+facts) and not include previous history
	m2 := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 400}, Text: "next"}
	b.handleIncomingMessage(context.Background(), m2)

	if len(seq.lastMsgs) < 2 {
		t.Fatalf("expected two LLM calls, got %d", len(seq.lastMsgs))
	}
	second := seq.lastMsgs[1]
	if len(second) != 2 {
		t.Fatalf("expected system + current user only, got %d msgs: %+v", len(second), second)
	}
	if second[0].Role != "system" || !strings.Contains(second[0].Content, "base") || !strings.Contains(second[0].Content, "facts") {
		t.Fatalf("system prompt missing base/facts: %q", second[0].Content)
	}
	if second[1].Role != "user" || second[1].Content != "next" {
		t.Fatalf("unexpected second message user content: %+v", second[1])
	}
}

func TestTZ_LimitForcesFinalize(t *testing.T) {
	userID := int64(3333)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	seq := &fakeLLMSeq{seq: []llm.Response{
		{Content: `{"title":"t","answer":"a","status":"continue"}`, Model: "m"},
		{Content: `{"title":"T-F","answer":"A-F","status":"final"}`, Model: "m"}, // for forced finalize
	}}
	b := &Bot{
		s:            fs,
		authSvc:      svc,
		llmClient:    seq,
		pending:      make(map[int64]auth.User),
		parseMode:    "HTML",
		systemPrompt: "base",
	}
	b.history = history.NewManager()
	// Simulate TZ state with 1 step remaining
	b.setTZMode(userID, true)
	b.setTZRemaining(userID, 1)

	m := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 500}, Text: "go"}
	b.handleIncomingMessage(context.Background(), m)

	if len(fs.sent) != 1 {
		t.Fatalf("expected one final message sent, got %d", len(fs.sent))
	}
	out := fs.sent[0]
	if !strings.Contains(out, "ТЗ Готово") || !strings.Contains(out, "A-F") {
		t.Fatalf("forced finalization not reflected: %q", out)
	}
	if b.isTZMode(userID) {
		t.Fatalf("tz mode should be cleared")
	}
}
