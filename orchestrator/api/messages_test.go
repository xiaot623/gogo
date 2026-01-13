package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

func TestGetSessionMessagesDefaults(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	msg := &domain.Message{
		MessageID: "m1",
		SessionID: "s1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}
	if err := h.store.CreateMessage(context.Background(), msg); err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/messages", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("s1")

	if err := h.GetSessionMessages(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Messages []domain.Message `json:"messages"`
		HasMore  bool            `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Messages) != 1 || resp.HasMore {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetRunEventsNotFound(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/r1/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("run_id")
	c.SetParamValues("r1")

	if err := h.GetRunEvents(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetRunEventsSuccess(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	run := &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusCreated, StartedAt: time.Now()}
	if err := h.store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	event := &domain.Event{
		EventID: "e1",
		RunID:   "r1",
		Ts:      time.Now().UnixMilli(),
		Type:    domain.EventTypeRunStarted,
		Payload: json.RawMessage(`{"session_id":"s1"}`),
	}
	if err := h.store.CreateEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/r1/events?limit=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("run_id")
	c.SetParamValues("r1")

	if err := h.GetRunEvents(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Events     []domain.Event `json:"events"`
		HasMore    bool           `json:"has_more"`
		NextCursor string         `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Events) != 1 || resp.HasMore {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetSessionMessagesLimit(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 2; i++ {
		msg := &domain.Message{
			MessageID: "m" + string(rune('1'+i)),
			SessionID: "s1",
			Role:      "user",
			Content:   "hello",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := h.store.CreateMessage(context.Background(), msg); err != nil {
			t.Fatalf("CreateMessage failed: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/messages?limit=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("s1")

	if err := h.GetSessionMessages(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Messages []domain.Message `json:"messages"`
		HasMore  bool            `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Messages) != 1 || !resp.HasMore {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetRunEventsFilters(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	run := &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusCreated, StartedAt: time.Now()}
	if err := h.store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	event := &domain.Event{EventID: "e1", RunID: "r1", Ts: time.Now().UnixMilli(), Type: domain.EventTypeRunStarted}
	if err := h.store.CreateEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/r1/events?types=run_started&limit=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("run_id")
	c.SetParamValues("r1")

	if err := h.GetRunEvents(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Health(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
