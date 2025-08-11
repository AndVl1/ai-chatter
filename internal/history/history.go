package history

import (
	"sync"

	"ai-chatter/internal/llm"
)

type entry struct {
	msg  llm.Message
	used bool
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[int64][]entry
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[int64][]entry)}
}

func (m *Manager) Reset(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, userID)
}

func (m *Manager) DisableAll(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.sessions[userID]
	for i := range entries {
		entries[i].used = false
	}
	m.sessions[userID] = entries
}

func (m *Manager) AppendUser(userID int64, content string) {
	m.AppendUserWithUsed(userID, content, true)
}
func (m *Manager) AppendAssistant(userID int64, content string) {
	m.AppendAssistantWithUsed(userID, content, true)
}

func (m *Manager) AppendUserWithUsed(userID int64, content string, used bool) {
	m.append(userID, llm.Message{Role: "user", Content: content}, used)
}

func (m *Manager) AppendAssistantWithUsed(userID int64, content string, used bool) {
	m.append(userID, llm.Message{Role: "assistant", Content: content}, used)
}

func (m *Manager) append(userID int64, msg llm.Message, used bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[userID] = append(m.sessions[userID], entry{msg: msg, used: used})
}

// Get returns only messages that are marked as used in context (backward-compatible behavior)
func (m *Manager) Get(userID int64) []llm.Message { return m.GetUsed(userID) }

func (m *Manager) GetUsed(userID int64) []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	es := m.sessions[userID]
	var out []llm.Message
	for _, e := range es {
		if e.used {
			out = append(out, e.msg)
		}
	}
	return out
}

func (m *Manager) GetAll(userID int64) []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	es := m.sessions[userID]
	out := make([]llm.Message, 0, len(es))
	for _, e := range es {
		out = append(out, e.msg)
	}
	return out
}
