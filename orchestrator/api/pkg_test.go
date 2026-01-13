package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func TestRecordEvent(t *testing.T) {
	store := helpers.NewTestSQLiteStore(t)
	h := &Handler{store: store}

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	run := &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "agent", Status: domain.RunStatusCreated, StartedAt: time.Now()}
	if err := h.store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	payload := domain.RunStartedPayload{SessionID: "s1", AgentID: "a1"}
	if err := h.recordEvent(context.Background(), "r1", domain.EventTypeRunStarted, payload); err != nil {
		t.Fatalf("recordEvent failed: %v", err)
	}

	events, err := h.store.GetEvents(context.Background(), "r1", 0, nil, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestPushEventToIngressNoURL(t *testing.T) {
	h := &Handler{config: &config.Config{IngressURL: ""}}
	h.pushEventToIngress("s1", map[string]interface{}{"type": "done"})
}

func TestPushEventToIngress(t *testing.T) {
	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/send" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	h := &Handler{config: &config.Config{IngressURL: server.URL}}
	h.pushEventToIngress("s1", map[string]interface{}{"type": "done"})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if gotPayload != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if gotPayload == nil {
		t.Fatalf("expected payload")
	}
	if gotPayload["session_id"] != "s1" {
		t.Fatalf("unexpected session_id: %+v", gotPayload)
	}
}
