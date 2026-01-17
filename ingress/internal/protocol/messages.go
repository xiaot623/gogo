// Package protocol defines the WebSocket message protocol between clients and ingress.
package protocol

import "encoding/json"

// Message types from client to ingress
const (
	TypeHello            = "hello"
	TypeAgentInvoke      = "agent_invoke"
	TypeToolResult       = "tool_result"
	TypeApprovalDecision = "approval_decision"
	TypeCancelRun        = "cancel_run"
)

// Message types from ingress to client
const (
	TypeHelloAck         = "hello_ack"
	TypeRunStarted       = "run_started"
	TypeDelta            = "delta"
	TypeState            = "state"
	TypeToolRequest      = "tool_request"
	TypeApprovalRequired = "approval_required"
	TypeDone             = "done"
	TypeError            = "error"
)

// BaseMessage contains common fields for all messages.
type BaseMessage struct {
	Type      string `json:"type"`
	Ts        int64  `json:"ts"`
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}

// HelloMessage is sent by client to establish connection.
type HelloMessage struct {
	BaseMessage
	UserID     string            `json:"user_id,omitempty"`
	APIKey     string            `json:"api_key,omitempty"`
	ClientMeta map[string]string `json:"client_meta,omitempty"`
}

// HelloAckMessage is sent by ingress after successful hello.
type HelloAckMessage struct {
	BaseMessage
}

// AgentInvokeMessage is sent by client to invoke an agent.
type AgentInvokeMessage struct {
	BaseMessage
	AgentID string       `json:"agent_id"`
	Message InputMessage `json:"message"`
}

// InputMessage represents the input message content.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolResultMessage is sent by client to submit tool execution result.
type ToolResultMessage struct {
	BaseMessage
	ToolCallID string          `json:"tool_call_id"`
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      json.RawMessage `json:"error,omitempty"`
}

// ApprovalDecisionMessage is sent by client to submit approval decision.
type ApprovalDecisionMessage struct {
	BaseMessage
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"` // "approve" or "reject"
	Reason     string `json:"reason,omitempty"`
}

// CancelRunMessage is sent by client to cancel a run.
type CancelRunMessage struct {
	BaseMessage
}

// ErrorMessage is sent by ingress when an error occurs.
type ErrorMessage struct {
	BaseMessage
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrorCodeInvalidMessage   = "invalid_message"
	ErrorCodeUnauthorized     = "unauthorized"
	ErrorCodeSessionRequired  = "session_required"
	ErrorCodeInternalError    = "internal_error"
	ErrorCodeOrchestratorFail = "orchestrator_fail"
)

// RawMessage is used for parsing incoming messages before type dispatch.
type RawMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"`
}
