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
	// Touch file if not exists
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
	// Increase buffer in case of long lines
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
			// skip malformed lines, but report later if needed
			continue
		}
		events = append(events, ev)
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return events, nil
}
