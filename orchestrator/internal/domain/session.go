package domain

import (
	"encoding/json"
	"time"
)

// Session represents a conversation session.
type Session struct {
	SessionID string          `json:"session_id"`
	UserID    string          `json:"user_id"`
	CreatedAt time.Time       `json:"created_at"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// Message represents a single message in a session.
type Message struct {
	MessageID string          `json:"message_id"`
	SessionID string          `json:"session_id"`
	RunID     string          `json:"run_id,omitempty"`
	Role      string          `json:"role"` // user, assistant, system
	Content   string          `json:"content"`
	CreatedAt time.Time       `json:"created_at"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}
