// Package domain defines the core domain models for the orchestrator.
package domain

// RunStatus represents the status of a run.
type RunStatus string

const (
	RunStatusCreated               RunStatus = "CREATED"
	RunStatusRunning               RunStatus = "RUNNING"
	RunStatusPausedWaitingTool     RunStatus = "PAUSED_WAITING_TOOL"
	RunStatusPausedWaitingApproval RunStatus = "PAUSED_WAITING_APPROVAL"
	RunStatusDone                  RunStatus = "DONE"
	RunStatusFailed                RunStatus = "FAILED"
	RunStatusCancelled             RunStatus = "CANCELLED"
)

// EventType represents the type of an event.
type EventType string

const (
	EventTypeRunStarted         EventType = "run_started"
	EventTypeUserInput          EventType = "user_input"
	EventTypeAgentInvokeStarted EventType = "agent_invoke_started"
	EventTypeAgentStreamDelta   EventType = "agent_stream_delta"
	EventTypeAgentInvokeDone    EventType = "agent_invoke_done"
	EventTypeRunDone            EventType = "run_done"
	EventTypeRunFailed          EventType = "run_failed"
	EventTypeRunCancelled       EventType = "run_cancelled"
	// LLM call events
	EventTypeLLMCallStarted EventType = "llm_call_started"
	EventTypeLLMCallDone    EventType = "llm_call_done"

	// Tool events
	EventTypeToolCallCreated  EventType = "tool_call_created"
	EventTypePolicyDecision   EventType = "policy_decision"
	EventTypeToolDispatched   EventType = "tool_dispatched"
	EventTypeToolResult       EventType = "tool_result"
	EventTypeToolRequest      EventType = "tool_request" // For client tools
	EventTypeApprovalRequired EventType = "approval_required"
	EventTypeApprovalDecision EventType = "approval_decision"
)

// ToolKind represents the kind of a tool.
type ToolKind string

const (
	ToolKindServer ToolKind = "server"
	ToolKindClient ToolKind = "client"
)

// ToolCallStatus represents the status of a tool call.
type ToolCallStatus string

const (
	ToolCallStatusCreated         ToolCallStatus = "CREATED"
	ToolCallStatusPolicyChecked   ToolCallStatus = "POLICY_CHECKED"
	ToolCallStatusBlocked         ToolCallStatus = "BLOCKED"
	ToolCallStatusWaitingApproval ToolCallStatus = "WAITING_APPROVAL"
	ToolCallStatusApproved        ToolCallStatus = "APPROVED"
	ToolCallStatusRejected        ToolCallStatus = "REJECTED"
	ToolCallStatusDispatched      ToolCallStatus = "DISPATCHED"
	ToolCallStatusRunning         ToolCallStatus = "RUNNING"
	ToolCallStatusSucceeded       ToolCallStatus = "SUCCEEDED"
	ToolCallStatusFailed          ToolCallStatus = "FAILED"
	ToolCallStatusTimeout         ToolCallStatus = "TIMEOUT"
)

// ApprovalStatus represents the status of an approval.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "PENDING"
	ApprovalStatusApproved ApprovalStatus = "APPROVED"
	ApprovalStatusRejected ApprovalStatus = "REJECTED"
	ApprovalStatusExpired  ApprovalStatus = "EXPIRED"
)
