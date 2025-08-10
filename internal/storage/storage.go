package storage

import "time"

// Event represents a single interaction of a user and assistant.
// It is intentionally simple to allow future DB implementations.
// A record combines the user's message and the assistant's response.
// Events are expected to be appended in chronological order.
type Event struct {
	Timestamp         time.Time `json:"timestamp"`
	UserID            int64     `json:"user_id"`
	UserMessage       string    `json:"user_message"`
	AssistantResponse string    `json:"assistant_response"`
}

// Recorder abstracts persistence of interaction events.
// Implementations can be file-based, database, etc.
// LoadInteractions should return events in chronological order.
// AppendInteraction should atomically append a new event.
// Implementations must be safe for concurrent use.
type Recorder interface {
	AppendInteraction(event Event) error
	LoadInteractions() ([]Event, error)
}
