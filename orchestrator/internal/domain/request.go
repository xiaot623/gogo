package domain

import "encoding/json"

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

// ToolInvokeRequest represents the request to invoke a tool.
type ToolInvokeRequest struct {
	RunID          string          `json:"run_id"`
	Args           json.RawMessage `json:"args"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	TimeoutMs      int             `json:"timeout_ms,omitempty"`
}

// ToolInvokeResponse represents the response from invoking a tool.
type ToolInvokeResponse struct {
	Status     string          `json:"status"` // succeeded, pending, failed
	ToolCallID string          `json:"tool_call_id"`
	Result     json.RawMessage `json:"result,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Error      *ToolError      `json:"error,omitempty"`
}

// ToolError represents a tool error.
type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ToolCallResponse represents the response for querying a tool call.
type ToolCallResponse struct {
	ToolCallID string          `json:"tool_call_id"`
	Status     ToolCallStatus  `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      json.RawMessage `json:"error,omitempty"`
	Timestamps Timestamps      `json:"timestamps"`
}

// Timestamps represents timestamps for a tool call.
type Timestamps struct {
	CreatedAt   int64 `json:"created_at"`
	StartedAt   int64 `json:"started_at,omitempty"`
	CompletedAt int64 `json:"completed_at,omitempty"`
}

// ApprovalDecisionRequest represents a decision on an approval.
type ApprovalDecisionRequest struct {
	Decision  string `json:"decision"` // APPROVED or REJECTED
	Reason    string `json:"reason,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
}

// ApprovalDecisionResponse represents the response after submitting an approval decision.
type ApprovalDecisionResponse struct {
	ApprovalID     string          `json:"approval_id"`
	Status         ApprovalStatus  `json:"status"`
	ToolCallID     string          `json:"tool_call_id"`
	ToolCallStatus ToolCallStatus  `json:"tool_call_status"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          json.RawMessage `json:"error,omitempty"`
}

// ToolCallResultRequest represents a request to submit a tool call result.
type ToolCallResultRequest struct {
	Status string          `json:"status"` // SUCCEEDED or FAILED
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

// ToolCallResultResponse represents the response after submitting a tool call result.
type ToolCallResultResponse struct {
	ToolCallID  string          `json:"tool_call_id"`
	Status      ToolCallStatus  `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	CompletedAt int64           `json:"completed_at"`
}

// ToolListItem represents a tool in the list response.
type ToolListItem struct {
	Name      string          `json:"name"`
	Source    string          `json:"source"` // "server" or "client"
	Schema    json.RawMessage `json:"schema,omitempty"`
	TimeoutMs int             `json:"timeout_ms"`
}

// ListToolsResponse represents the response for listing tools.
type ListToolsResponse struct {
	Tools []ToolListItem `json:"tools"`
}

// ToolRegistrationItem represents a single tool to register.
type ToolRegistrationItem struct {
	Name      string          `json:"name"`
	Schema    json.RawMessage `json:"schema"`
	TimeoutMs int             `json:"timeout_ms,omitempty"`
}

// ToolRegistrationRequest represents a request to register tools from a client.
type ToolRegistrationRequest struct {
	ClientID string                 `json:"client_id"`
	Tools    []ToolRegistrationItem `json:"tools"`
}

// ToolRegistrationResponse represents the response after registering tools.
type ToolRegistrationResponse struct {
	OK              bool `json:"ok"`
	RegisteredCount int  `json:"registered_count"`
}
