package pending

import (
	"path/filepath"
	"testing"

	"ai-chatter/internal/auth"
)

func TestPendingFileRepo_CRUD(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pending.json")
	repo, err := NewFileRepository(p)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	u1 := auth.User{ID: 1, Username: "alice", FirstName: "A", LastName: "L"}
	u2 := auth.User{ID: 2, Username: "bob", FirstName: "B", LastName: "K"}
	if err := repo.Upsert(u1); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	if err := repo.Upsert(u2); err != nil {
		t.Fatalf("upsert2: %v", err)
	}

	items, err := repo.LoadAll()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}

	if err := repo.Remove(1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	items, _ = repo.LoadAll()
	if len(items) != 1 || items[0].ID != 2 {
		t.Fatalf("unexpected items: %+v", items)
	}
}
