package history

import (
	"testing"

	"ai-chatter/internal/llm"
)

func TestHistoryAppendGetReset(t *testing.T) {
	h := NewManager()
	userA := int64(1)
	userB := int64(2)

	h.AppendUser(userA, "hello")
	h.AppendAssistant(userA, "hi")
	h.AppendUser(userB, "foo")
	h.AppendAssistant(userB, "bar")

	msgsA := h.Get(userA)
	msgsB := h.Get(userB)

	if len(msgsA) != 2 || len(msgsB) != 2 {
		t.Fatalf("unexpected lengths: A=%d B=%d", len(msgsA), len(msgsB))
	}
	if msgsA[0].Role != "user" || msgsA[0].Content != "hello" {
		t.Fatalf("unexpected A[0]: %+v", msgsA[0])
	}
	if msgsA[1].Role != "assistant" || msgsA[1].Content != "hi" {
		t.Fatalf("unexpected A[1]: %+v", msgsA[1])
	}
	if msgsB[0].Role != "user" || msgsB[0].Content != "foo" {
		t.Fatalf("unexpected B[0]: %+v", msgsB[0])
	}
	if msgsB[1].Role != "assistant" || msgsB[1].Content != "bar" {
		t.Fatalf("unexpected B[1]: %+v", msgsB[1])
	}

	// Ensure copy semantics (modifying returned slice does not affect internal state)
	msgsA[0] = llm.Message{Role: "user", Content: "mutated"}
	msgsA2 := h.Get(userA)
	if msgsA2[0].Content != "hello" {
		t.Fatalf("internal state mutated via returned slice")
	}

	h.Reset(userA)
	if len(h.Get(userA)) != 0 {
		t.Fatalf("reset did not clear user A")
	}
	if len(h.Get(userB)) != 2 {
		t.Fatalf("reset should not affect other users")
	}
}
