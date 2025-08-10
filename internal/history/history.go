package history

import (
	"sync"

	"ai-chatter/internal/llm"
)

type Manager struct {
	mu       sync.RWMutex
	sessions map[int64][]llm.Message
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[int64][]llm.Message)}
}

func (m *Manager) Reset(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, userID)
}

func (m *Manager) AppendUser(userID int64, content string) {
	m.append(userID, llm.Message{Role: "user", Content: content})
}

func (m *Manager) AppendAssistant(userID int64, content string) {
	m.append(userID, llm.Message{Role: "assistant", Content: content})
}

func (m *Manager) append(userID int64, msg llm.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[userID] = append(m.sessions[userID], msg)
}

func (m *Manager) Get(userID int64) []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.sessions[userID]
	// return a copy to avoid external mutation
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}
