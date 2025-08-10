package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type FileRepository struct {
	path string
	mu   sync.Mutex
}

func NewFileRepository(path string) (*FileRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("ensure dir: %w", err)
	}
	// Touch file if not exists
	f, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("touch file: %w", err)
	}
	_ = f.Close()
	return &FileRepository{path: path}, nil
}

func (r *FileRepository) LoadAll() ([]User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.Open(r.path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
		}
	}(f)
	var users []User
	dec := json.NewDecoder(f)
	if err := dec.Decode(&users); err != nil {
		if err == io.EOF {
			return []User{}, nil
		}
		// empty or malformed -> start fresh
		return []User{}, nil
	}
	return users, nil
}

func (r *FileRepository) Upsert(user User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	users, _ := r.loadUnlocked()
	updated := false
	for i, u := range users {
		if u.ID == user.ID {
			users[i] = user
			updated = true
			break
		}
	}
	if !updated {
		users = append(users, user)
	}
	return r.saveUnlocked(users)
}

func (r *FileRepository) Remove(userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	users, _ := r.loadUnlocked()
	var out []User
	for _, u := range users {
		if u.ID != userID {
			out = append(out, u)
		}
	}
	return r.saveUnlocked(out)
}

func (r *FileRepository) loadUnlocked() ([]User, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
		}
	}(f)
	var users []User
	dec := json.NewDecoder(f)
	if err := dec.Decode(&users); err != nil {
		return []User{}, nil
	}
	return users, nil
}

func (r *FileRepository) saveUnlocked(users []User) error {
	f, err := os.OpenFile(r.path, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
		}
	}(f)
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(users)
}
