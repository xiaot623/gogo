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
)
