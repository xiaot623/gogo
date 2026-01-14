package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func newTestHandler(t *testing.T) *Handler {
	cfg := &config.Config{IngressURL: "", AgentTimeout: time.Second}
	store := helpers.NewTestSQLiteStore(t)
	client := agentclient.NewClient()
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	return NewHandler(store, client, cfg, policyEngine)
}

func TestRegisterAgentValidation(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/register", bytes.NewBufferString(`{"name":"demo"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.RegisterAgent(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRegisterAgentSuccess(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	body := `{"agent_id":"demo","name":"Demo","endpoint":"http://agent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.RegisterAgent(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	got, err := h.store.GetAgent(context.Background(), "demo")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil || got.Endpoint != "http://agent" {
		t.Fatalf("unexpected agent: %+v", got)
	}
}

func TestListAgents(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	agent := &domain.Agent{
		AgentID:   "a1",
		Name:      "Demo",
		Endpoint:  "http://agent",
		Status:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := h.store.RegisterAgent(context.Background(), agent); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListAgents(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/agents/a1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("agent_id")
	c.SetParamValues("a1")

	if err := h.GetAgent(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetAgentSuccess(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	agent := &domain.Agent{
		AgentID:   "a1",
		Name:      "Demo",
		Endpoint:  "http://agent",
		Status:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := h.store.RegisterAgent(context.Background(), agent); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/agents/a1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("agent_id")
	c.SetParamValues("a1")

	if err := h.GetAgent(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestInternalInvokeValidation(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/internal/invoke", bytes.NewBufferString(`{"agent_id":"demo"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.InternalInvoke(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInternalInvokeSuccess(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t)

	agent := &domain.Agent{
		AgentID:   "demo",
		Name:      "Demo",
		Endpoint:  "http://agent",
		Status:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := h.store.RegisterAgent(context.Background(), agent); err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	body := `{"session_id":"s1","agent_id":"demo","input_message":{"role":"user","content":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/internal/invoke", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.InternalInvoke(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
