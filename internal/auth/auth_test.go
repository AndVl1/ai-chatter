package auth

import "testing"

type memRepo struct{ users []User }

func (m *memRepo) LoadAll() ([]User, error) { return append([]User{}, m.users...), nil }
func (m *memRepo) Upsert(u User) error {
	for i, x := range m.users {
		if x.ID == u.ID {
			m.users[i] = u
			return nil
		}
	}
	m.users = append(m.users, u)
	return nil
}
func (m *memRepo) Remove(id int64) error {
	out := make([]User, 0, len(m.users))
	for _, x := range m.users {
		if x.ID != id {
			out = append(out, x)
		}
	}
	m.users = out
	return nil
}

func TestServiceBasic(t *testing.T) {
	repo := &memRepo{users: []User{{ID: 10, Username: "alice"}}}
	svc, err := NewWithRepo(repo, []int64{20})
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	if !svc.IsAllowed(10) {
		t.Fatalf("repo preload not effective")
	}
	if !svc.IsAllowed(20) {
		t.Fatalf("initial env list not merged")
	}
	if svc.IsAllowed(30) {
		t.Fatalf("unexpected allowed")
	}

	if err := svc.Upsert(User{ID: 30, Username: "bob"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !svc.IsAllowed(30) {
		t.Fatalf("upsert not effective")
	}

	if err := svc.Remove(10); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if svc.IsAllowed(10) {
		t.Fatalf("remove not effective")
	}

	lst := svc.List()
	if len(lst) != 2 {
		t.Fatalf("want 2 users, got %d", len(lst))
	}
}
