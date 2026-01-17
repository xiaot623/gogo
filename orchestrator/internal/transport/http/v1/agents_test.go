package v1

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/ingress"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/config"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func newTestHandler(t *testing.T) (*Handler, store.Store) {
	cfg := &config.Config{IngressURL: "", AgentTimeout: time.Second}
	db := helpers.NewTestSQLiteStore(t)
	client := agentclient.NewClient()
	ingressClient := ingress.NewClient("")
	llmClient := llm.NewClient("", "", time.Second)
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	svc := service.New(db, client, ingressClient, llmClient, cfg, policyEngine)
	return NewHandler(svc), db
}

func TestRegisterAgentValidation(t *testing.T) {
	e := echo.New()
	h, _ := newTestHandler(t)

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
	h, db := newTestHandler(t)

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

	got, err := db.GetAgent(context.Background(), "demo")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if got == nil || got.Endpoint != "http://agent" {
		t.Fatalf("unexpected agent: %+v", got)
	}
}

func TestListAgents(t *testing.T) {
	e := echo.New()
	h, db := newTestHandler(t)

	agent := &domain.Agent{
		AgentID:   "a1",
		Name:      "Demo",
		Endpoint:  "http://agent",
		Status:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := db.RegisterAgent(context.Background(), agent); err != nil {
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
	h, _ := newTestHandler(t)

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
	h, db := newTestHandler(t)

	agent := &domain.Agent{
		AgentID:   "a1",
		Name:      "Demo",
		Endpoint:  "http://agent",
		Status:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := db.RegisterAgent(context.Background(), agent); err != nil {
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