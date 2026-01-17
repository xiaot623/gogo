package domain

import (
	"encoding/json"
	"time"
)

// Tool represents a registered tool.
type Tool struct {
	Name      string          `json:"name"`
	Kind      ToolKind        `json:"kind"`      // server or client
	Policy    json.RawMessage `json:"policy"`    // policy config (optional, logic moved to OPA but kept for metadata)
	TimeoutMs int             `json:"timeout_ms"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// ToolCall represents a tool execution record.
type ToolCall struct {
	ToolCallID  string          `json:"tool_call_id"`
	RunID       string          `json:"run_id"`
	ToolName    string          `json:"tool_name"`
	Kind        ToolKind        `json:"kind"`
	Status      ToolCallStatus  `json:"status"`
	Args        json.RawMessage `json:"args"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	ApprovalID  string          `json:"approval_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// Approval represents an approval request.
type Approval struct {
	ApprovalID string         `json:"approval_id"`
	RunID      string         `json:"run_id"`
	ToolCallID string         `json:"tool_call_id"`
	Status     ApprovalStatus `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	DecidedAt  *time.Time     `json:"decided_at,omitempty"`
	DecidedBy  string         `json:"decided_by,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}
