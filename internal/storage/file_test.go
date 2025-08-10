package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileRecorder_AppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.jsonl")
	rec, err := NewFileRecorder(p)
	if err != nil {
		t.Fatalf("init recorder: %v", err)
	}

	ev1 := Event{Timestamp: time.Unix(1, 0).UTC(), UserID: 1, UserMessage: "hi", AssistantResponse: "hello"}
	ev2 := Event{Timestamp: time.Unix(2, 0).UTC(), UserID: 2, UserMessage: "foo", AssistantResponse: "bar"}
	if err := rec.AppendInteraction(ev1); err != nil {
		t.Fatalf("append1: %v", err)
	}
	if err := rec.AppendInteraction(ev2); err != nil {
		t.Fatalf("append2: %v", err)
	}

	events, err := rec.LoadInteractions()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("want 2, got %d", len(events))
	}
	if events[0].UserID != 1 || events[1].UserID != 2 {
		t.Fatalf("order mismatch: %+v", events)
	}

	// ensure file exists and non-empty
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		t.Fatalf("file not written")
	}
}
