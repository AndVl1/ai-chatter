package telegram

import (
	"context"
	"os"
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

func (f *fakeLLMSeq) Generate(_ context.Context, msgs []llm.Message) (llm.Response, error) {
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

func (f fakeLLM) Generate(_ context.Context, _ []llm.Message) (llm.Response, error) {
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

	if len(fs.sent) != 3 {
		t.Fatalf("expected 3 messages (final TS, prep, instruction), got %d", len(fs.sent))
	}
	if !strings.Contains(fs.sent[0], "ТЗ Готово") {
		t.Fatalf("missing final header: %q", fs.sent[0])
	}
	if !strings.Contains(strings.ToLower(fs.sent[1]), "инструк") {
		t.Fatalf("missing instruction prep notice: %q", fs.sent[1])
	}
	if !strings.Contains(fs.sent[2], "<b>T</b>") || !strings.Contains(fs.sent[2], "A") {
		t.Fatalf("instruction not formatted as expected: %q", fs.sent[2])
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

	if len(fs.sent) != 3 {
		t.Fatalf("expected three messages (final TS, prep, instruction), got %d", len(fs.sent))
	}
	if !strings.Contains(fs.sent[0], "ТЗ Готово") {
		t.Fatalf("missing final header: %q", fs.sent[0])
	}
	if !strings.Contains(strings.ToLower(fs.sent[1]), "инструк") {
		t.Fatalf("missing instruction prep: %q", fs.sent[1])
	}
	if !strings.Contains(fs.sent[2], "<b>T-F</b>") || !strings.Contains(fs.sent[2], "A-F") {
		t.Fatalf("missing instruction content: %q", fs.sent[2])
	}
	if b.isTZMode(userID) {
		t.Fatalf("tz mode should be cleared")
	}
}

func TestAdminSetsModel2_UsedForInstructions(t *testing.T) {
	userID := int64(5555)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	// First: tz bootstrap returns final; Then: second model instruction also returns json
	seq := &fakeLLMSeq{seq: []llm.Response{
		{Content: `{"title":"T","answer":"A","status":"final"}`, Model: "m-primary"},
		{Content: `{"title":"Instr","answer":"Do this","status":"final"}`, Model: "m-secondary"},
	}}
	b := &Bot{
		s:            fs,
		authSvc:      svc,
		llmClient:    seq,
		llmClient2:   seq,
		pending:      make(map[int64]auth.User),
		parseMode:    "HTML",
		systemPrompt: "base",
	}
	b.history = history.NewManager()
	b.provider = "openai"
	b.openaiAPIKey = "test"
	b.openaiBaseURL = "http://example.local"
	b.model = "model-primary"

	// Admin sets model2
	msg := &tgbotapi.Message{From: &tgbotapi.User{ID: 0}, Chat: &tgbotapi.Chat{ID: 42}, Text: "/model2 openai/secondary"}
	ents := []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}}
	msg.Entities = ents
	b.adminUserID = 1
	msg.From.ID = 1
	b.handleAdminConfigCommands(msg)

	// Check file persisted (best-effort)
	_, _ = os.ReadFile("data/model2.txt")

	// Simulate /tz to run flow
	tz := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 99}, Text: "/tz topic"}
	tz.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 3}}
	b.handleCommand(tz)

	// Expect 4 messages: admin confirm, final TS, prep, instruction
	if len(fs.sent) != 4 {
		t.Fatalf("expected 4 messages, got %d: %#v", len(fs.sent), fs.sent)
	}
	if !strings.Contains(fs.sent[1], "ТЗ Готово") {
		t.Fatalf("missing final header: %q", fs.sent[1])
	}
	if !strings.Contains(strings.ToLower(fs.sent[2]), "инструк") {
		t.Fatalf("missing instruction prep notice: %q", fs.sent[2])
	}
	if !strings.Contains(fs.sent[3], "Instr") || !strings.Contains(fs.sent[3], "Do this") {
		t.Fatalf("instruction not sent as expected: %q", fs.sent[3])
	}
}

func TestTZ_CheckerCorrectsPrimaryResponse(t *testing.T) {
	userID := int64(6666)
	svc, _ := auth.NewWithRepo(nil, []int64{userID})
	fs := &fakeSender{}
	// Sequence: primary bad (continue, non-numbered), checker fail, primary corrected (continue, numbered)
	seq := &fakeLLMSeq{seq: []llm.Response{
		{Content: `{"title":"T","answer":"Question one; Question two","status":"continue"}`, Model: "m"},       // primary
		{Content: `{"status":"fail","msg":"Сделай вопросы нумерованными построчно"}`, Model: "m2"},             // checker
		{Content: `{"title":"T","answer":"1. Question one\n2. Question two","status":"continue"}`, Model: "m"}, // corrected
	}}
	b := &Bot{
		s:            fs,
		authSvc:      svc,
		llmClient:    seq,
		llmClient2:   seq,
		pending:      make(map[int64]auth.User),
		parseMode:    "HTML",
		systemPrompt: "base",
	}
	b.history = history.NewManager()
	b.setTZMode(userID, true)
	b.setTZRemaining(userID, 2)

	msg := &tgbotapi.Message{From: &tgbotapi.User{ID: userID}, Chat: &tgbotapi.Chat{ID: 600}, Text: "hi"}
	b.handleIncomingMessage(context.Background(), msg)

	if len(fs.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(fs.sent))
	}
	out := fs.sent[0]
	if !strings.Contains(out, "1. Question one") || !strings.Contains(out, "2. Question two") {
		t.Fatalf("corrected numbering not applied: %q", out)
	}
}
