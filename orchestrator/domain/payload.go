package domain

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

// LLMCallStartedPayload is the payload for llm_call_started event.
type LLMCallStartedPayload struct {
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
}

// LLMCallDonePayload is the payload for llm_call_done event.
type LLMCallDonePayload struct {
	RequestID        string `json:"request_id"`
	Model            string `json:"model"`
	LatencyMs        int64  `json:"latency_ms"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	Error            string `json:"error,omitempty"`
}
