package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileRecorder struct {
	path string
	mu   sync.Mutex
}

func NewFileRecorder(path string) (*FileRecorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to ensure log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to init log file: %w", err)
	}
	_ = f.Close()
	return &FileRecorder{path: path}, nil
}

func (r *FileRecorder) AppendInteraction(event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open append: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
		}
	}(f)
	enc := json.NewEncoder(f)
	if err := enc.Encode(event); err != nil {
		return fmt.Errorf("encode append: %w", err)
	}
	return nil
}

func (r *FileRecorder) LoadInteractions() ([]Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.Open(r.path)
	if err != nil {
		return nil, fmt.Errorf("open read: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
		}
	}(f)
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	s.Buffer(buf, 10*1024*1024)
	var events []Event
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return events, nil
}

func (r *FileRecorder) SetAllCanUse(userID int64, canUse bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// read all
	f, err := os.Open(r.path)
	if err != nil {
		return fmt.Errorf("open read: %w", err)
	}
	var events []Event
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	s.Buffer(buf, 10*1024*1024)
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.UserID == userID {
			ev.CanUse = &canUse
		}
		events = append(events, ev)
	}
	_ = f.Close()
	if err := s.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	// rewrite file
	wf, err := os.OpenFile(r.path, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open write: %w", err)
	}
	defer func(wf *os.File) {
		err := wf.Close()
		if err != nil {
		}
	}(wf)
	enc := json.NewEncoder(wf)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}
	return nil
}
