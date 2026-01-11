// Package store defines the storage interface and implementations.
package store

import (
	"context"

	"github.com/gogo/orchestrator/domain"
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
