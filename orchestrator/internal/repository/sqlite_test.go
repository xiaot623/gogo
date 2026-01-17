package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return store
}

func TestSQLiteStoreSessionAndMessages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	defer store.Close()

	session := &domain.Session{
		SessionID: "s1",
		UserID:    "u1",
		CreatedAt: time.Now(),
		Metadata:  json.RawMessage(`{"tier":"pro"}`),
	}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	gotSession, err := store.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if gotSession == nil || gotSession.UserID != "u1" {
		t.Fatalf("unexpected session: %+v", gotSession)
	}

	msg := &domain.Message{
		MessageID: "m1",
		SessionID: "s1",
		RunID:     "r1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}
	if err := store.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	messages, err := store.GetMessages(ctx, "s1", 10, "")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func TestSQLiteStoreRunAndEvents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	defer store.Close()

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	run := &domain.Run{
		RunID:       "r1",
		SessionID:   "s1",
		RootAgentID: "agent",
		Status:      domain.RunStatusCreated,
		StartedAt:   time.Now(),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	if err := store.UpdateRunStatus(ctx, "r1", domain.RunStatusRunning); err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}

	errPayload := json.RawMessage(`{"error":"boom"}`)
	if err := store.UpdateRunCompleted(ctx, "r1", domain.RunStatusFailed, errPayload); err != nil {
		t.Fatalf("UpdateRunCompleted failed: %v", err)
	}

	gotRun, err := store.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if gotRun == nil || gotRun.Status != domain.RunStatusFailed {
		t.Fatalf("unexpected run: %+v", gotRun)
	}

	event := &domain.Event{
		EventID: "e1",
		RunID:   "r1",
		Ts:      time.Now().UnixMilli(),
		Type:    domain.EventTypeRunStarted,
		Payload: json.RawMessage(`{"session_id":"s1"}`),
	}
	if err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	events, err := store.GetEvents(ctx, "r1", 0, []string{}, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestSQLiteStoreAgents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	defer store.Close()

	agent := &domain.Agent{
		AgentID:      "a1",
		Name:         "Demo",
		Endpoint:     "http://agent",
		Status:       "healthy",
		CreatedAt:    time.Now(),
		Capabilities: json.RawMessage(`{"tools":["calc"]}`),
	}
	if err := store.RegisterAgent(ctx, agent); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	gotAgent, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if gotAgent == nil || gotAgent.AgentID != "a1" {
		t.Fatalf("unexpected agent: %+v", gotAgent)
	}

	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}
