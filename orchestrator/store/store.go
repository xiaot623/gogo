// Package store defines the storage interface and implementations.
package store

import (
	"context"

	"github.com/xiaot623/gogo/orchestrator/domain"
)

// Store defines the interface for data persistence.
type Store interface {
	// Session operations
	CreateSession(ctx context.Context, session *domain.Session) error
	GetSession(ctx context.Context, sessionID string) (*domain.Session, error)
	GetOrCreateSession(ctx context.Context, sessionID, userID string) (*domain.Session, error)

	// Message operations
	CreateMessage(ctx context.Context, message *domain.Message) error
	GetMessages(ctx context.Context, sessionID string, limit int, before string) ([]domain.Message, error)

	// Run operations
	CreateRun(ctx context.Context, run *domain.Run) error
	GetRun(ctx context.Context, runID string) (*domain.Run, error)
	UpdateRunStatus(ctx context.Context, runID string, status domain.RunStatus) error
	UpdateRunCompleted(ctx context.Context, runID string, status domain.RunStatus, errData []byte) error

	// Event operations
	CreateEvent(ctx context.Context, event *domain.Event) error
	GetEvents(ctx context.Context, runID string, afterTs int64, types []string, limit int) ([]domain.Event, error)

	// Agent operations
	RegisterAgent(ctx context.Context, agent *domain.Agent) error
	GetAgent(ctx context.Context, agentID string) (*domain.Agent, error)
	ListAgents(ctx context.Context) ([]domain.Agent, error)

	// Tool operations
	CreateTool(ctx context.Context, tool *domain.Tool) error
	GetTool(ctx context.Context, toolName string) (*domain.Tool, error)
	ListTools(ctx context.Context) ([]domain.Tool, error)

	// ToolCall operations
	CreateToolCall(ctx context.Context, toolCall *domain.ToolCall) error
	GetToolCall(ctx context.Context, toolCallID string) (*domain.ToolCall, error)
	UpdateToolCallStatus(ctx context.Context, toolCallID string, status domain.ToolCallStatus) error
	UpdateToolCallResult(ctx context.Context, toolCallID string, status domain.ToolCallStatus, result []byte, errData []byte) error
	UpdateToolCallApproval(ctx context.Context, toolCallID string, approvalID string, status domain.ToolCallStatus) error

	// Approval operations
	CreateApproval(ctx context.Context, approval *domain.Approval) error
	GetApproval(ctx context.Context, approvalID string) (*domain.Approval, error)
	UpdateApprovalStatus(ctx context.Context, approvalID string, status domain.ApprovalStatus, decidedBy string, reason string) error

	// Lifecycle
	Close() error
}

// EventFilter provides filtering options for events.
type EventFilter struct {
	RunID   string
	AfterTs int64
	Types   []string
	Limit   int
}
