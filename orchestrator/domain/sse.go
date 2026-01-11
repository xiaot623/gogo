package domain

import "encoding/json"

// AgentSSEEvent represents an SSE event from an agent.
type AgentSSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// DeltaEventData is the data for a delta SSE event.
type DeltaEventData struct {
	Text  string `json:"text"`
	RunID string `json:"run_id"`
}

// DoneEventData is the data for a done SSE event.
type DoneEventData struct {
	Usage        *UsageData `json:"usage,omitempty"`
	FinalMessage string     `json:"final_message,omitempty"`
}

// UsageData represents token usage information.
type UsageData struct {
	Tokens           int `json:"tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	DurationMs       int `json:"duration_ms,omitempty"`
}

// ErrorEventData is the data for an error SSE event.
type ErrorEventData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
