// Package domain defines the core domain models for the orchestrator.
package domain

import (
	"encoding/json"
	"time"
)

// RunStatus represents the status of a run.
type RunStatus string

const (
	RunStatusCreated              RunStatus = "CREATED"
	RunStatusRunning              RunStatus = "RUNNING"
	RunStatusPausedWaitingTool    RunStatus = "PAUSED_WAITING_TOOL"
	RunStatusPausedWaitingApproval RunStatus = "PAUSED_WAITING_APPROVAL"
	RunStatusDone                 RunStatus = "DONE"
	RunStatusFailed               RunStatus = "FAILED"
	RunStatusCancelled            RunStatus = "CANCELLED"
)

// EventType represents the type of an event.
type EventType string

const (
	EventTypeRunStarted        EventType = "run_started"
	EventTypeUserInput         EventType = "user_input"
	EventTypeAgentInvokeStarted EventType = "agent_invoke_started"
	EventTypeAgentStreamDelta  EventType = "agent_stream_delta"
	EventTypeAgentInvokeDone   EventType = "agent_invoke_done"
	EventTypeRunDone           EventType = "run_done"
	EventTypeRunFailed         EventType = "run_failed"
	EventTypeRunCancelled      EventType = "run_cancelled"
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

// Agent represents a registered agent.
type Agent struct {
	AgentID       string          `json:"agent_id"`
	Name          string          `json:"name"`
	Endpoint      string          `json:"endpoint"`
	Capabilities  json.RawMessage `json:"capabilities,omitempty"`
	Status        string          `json:"status"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// InputMessage represents the input message from the client.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InvokeRequest represents the request to invoke an agent.
type InvokeRequest struct {
	SessionID    string            `json:"session_id"`
	AgentID      string            `json:"agent_id"`
	InputMessage InputMessage      `json:"input_message"`
	RequestID    string            `json:"request_id,omitempty"`
	Context      map[string]string `json:"context,omitempty"`
}

// InvokeResponse represents the response from invoking an agent.
type InvokeResponse struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

// AgentInvokeRequest is the request sent to an external agent.
type AgentInvokeRequest struct {
	AgentID      string            `json:"agent_id"`
	SessionID    string            `json:"session_id"`
	RunID        string            `json:"run_id"`
	InputMessage InputMessage      `json:"input_message"`
	Messages     []Message         `json:"messages,omitempty"`
	Context      map[string]string `json:"context,omitempty"`
}

// SSE Event types from agent
type AgentSSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type DeltaEventData struct {
	Text  string `json:"text"`
	RunID string `json:"run_id"`
}

type DoneEventData struct {
	Usage        *UsageData `json:"usage,omitempty"`
	FinalMessage string     `json:"final_message,omitempty"`
}

type UsageData struct {
	Tokens           int `json:"tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	DurationMs       int `json:"duration_ms,omitempty"`
}

type ErrorEventData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RunStartedPayload is the payload for run_started event.
type RunStartedPayload struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

// UserInputPayload is the payload for user_input event.
type UserInputPayload struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

// AgentStreamDeltaPayload is the payload for agent_stream_delta event.
type AgentStreamDeltaPayload struct {
	Text string `json:"text"`
}

// RunDonePayload is the payload for run_done event.
type RunDonePayload struct {
	Usage        *UsageData `json:"usage,omitempty"`
	FinalMessage string     `json:"final_message,omitempty"`
}

// RunFailedPayload is the payload for run_failed event.
type RunFailedPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
