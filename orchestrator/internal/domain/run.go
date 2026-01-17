package domain

import (
	"encoding/json"
	"time"
)

// Run represents a single execution of an agent.
type Run struct {
	RunID       string          `json:"run_id"`
	SessionID   string          `json:"session_id"`
	RootAgentID string          `json:"root_agent_id"`
	ParentRunID string          `json:"parent_run_id,omitempty"`
	Status      RunStatus       `json:"status"`
	StartedAt   time.Time       `json:"started_at"`
	EndedAt     *time.Time      `json:"ended_at,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
}

// Event represents a trace event for replay.
type Event struct {
	EventID string          `json:"event_id"`
	RunID   string          `json:"run_id"`
	Ts      int64           `json:"ts"` // Unix milliseconds
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
