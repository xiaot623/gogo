package llmproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func newTestHandler(t *testing.T, liteLLMURL string) *Handler {
	cfg := &config.Config{
		LiteLLMURL: liteLLMURL,
		LLMTimeout: time.Second,
	}
	store := helpers.NewTestSQLiteStore(t)
	return &Handler{
		client: NewClient(cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout),
		store:  store,
		config: cfg,
	}
}

func TestChatCompletionsValidation(t *testing.T) {
	e := echo.New()
	h := newTestHandler(t, "http://example.com")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ChatCompletions(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatCompletionsNonStreaming(t *testing.T) {
	liteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"c1","object":"chat.completion","created":1,"model":"gpt","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer liteServer.Close()

	h := newTestHandler(t, liteServer.URL)
	e := echo.New()

	ctx := context.Background()
	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	run := &domain.Run{RunID: "run_1", SessionID: "s1", RootAgentID: "agent", Status: domain.RunStatusCreated, StartedAt: time.Now()}
	if err := h.store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	body := `{"model":"gpt","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-run-id", "run_1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ChatCompletions(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	events, err := h.store.GetEvents(ctx, "run_1", 0, []string{string(domain.EventTypeLLMCallStarted), string(domain.EventTypeLLMCallDone)}, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestChatCompletionsStreaming(t *testing.T) {
	liteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer liteServer.Close()

	h := newTestHandler(t, liteServer.URL)
	e := echo.New()

	ctx := context.Background()
	session := &domain.Session{SessionID: "s1", UserID: "u1", CreatedAt: time.Now()}
	if err := h.store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	run := &domain.Run{RunID: "run_stream", SessionID: "s1", RootAgentID: "agent", Status: domain.RunStatusCreated, StartedAt: time.Now()}
	if err := h.store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	body := `{"model":"gpt","messages":[{"role":"user","content":"hello"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-run-id", "run_stream")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ChatCompletions(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if !bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("expected DONE marker")
	}

	events, err := h.store.GetEvents(ctx, "run_stream", 0, []string{string(domain.EventTypeLLMCallStarted), string(domain.EventTypeLLMCallDone)}, 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestListModels(t *testing.T) {
	liteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := ModelsResponse{
			Object: "list",
			Data: []Model{{ID: "gpt", Object: "model", Created: 1, OwnedBy: "openai"}},
		}
		b, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}))
	defer liteServer.Close()

	h := newTestHandler(t, liteServer.URL)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListModels(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
