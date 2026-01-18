package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func TestInvokeTool(t *testing.T) {
	ctx := context.Background()

	t.Run("Weather Query Returns Pending (Async)", func(t *testing.T) {
		e := echo.New()
		handler, store := newTestHandler(t)

		// Setup Data
		store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
		store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})

		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID: "r1",
			Args:  json.RawMessage(`{"city":"Beijing"}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/weather.query/invoke", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tools/:tool_name/invoke")
		c.SetParamNames("tool_name")
		c.SetParamValues("weather.query")

		err := handler.InvokeTool(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.ToolInvokeResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		// Server tools now return pending immediately (async execution)
		assert.Equal(t, "pending", resp.Status)
		assert.Equal(t, "server_tool_executing", resp.Reason)
	})

	t.Run("Block Dangerous Command", func(t *testing.T) {
		e := echo.New()
		handler, store := newTestHandler(t)

		// Setup Data
		store.CreateSession(ctx, &domain.Session{SessionID: "s2", UserID: "u2"})
		store.CreateRun(ctx, &domain.Run{RunID: "r2", SessionID: "s2", RootAgentID: "a1", Status: domain.RunStatusRunning})

		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID: "r2",
			Args:  json.RawMessage(`{}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/dangerous.command/invoke", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tools/:tool_name/invoke")
		c.SetParamNames("tool_name")
		c.SetParamValues("dangerous.command")

		err := handler.InvokeTool(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code) // API returns 200 even for logic failure/block

		var resp domain.ToolInvokeResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, "failed", resp.Status)
		assert.Equal(t, "blocked", resp.Error.Code)
	})

	t.Run("Idempotent Invoke Returns Same Tool Call", func(t *testing.T) {
		e := echo.New()
		handler, store := newTestHandler(t)

		store.CreateSession(ctx, &domain.Session{SessionID: "s4", UserID: "u4"})
		store.CreateRun(ctx, &domain.Run{RunID: "r4", SessionID: "s4", RootAgentID: "a1", Status: domain.RunStatusRunning})

		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID:          "r4",
			Args:           json.RawMessage(`{"url":"https://example.com"}`),
			IdempotencyKey: "idem_tool_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/browser.screenshot/invoke", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tools/:tool_name/invoke")
		c.SetParamNames("tool_name")
		c.SetParamValues("browser.screenshot")

		err := handler.InvokeTool(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var firstResp domain.ToolInvokeResponse
		json.Unmarshal(rec.Body.Bytes(), &firstResp)
		assert.Equal(t, "pending", firstResp.Status)
		assert.Equal(t, "waiting_client", firstResp.Reason)
		assert.NotEmpty(t, firstResp.ToolCallID)

		req2 := httptest.NewRequest(http.MethodPost, "/v1/tools/browser.screenshot/invoke", bytes.NewReader(reqBody))
		req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec2 := httptest.NewRecorder()
		c2 := e.NewContext(req2, rec2)
		c2.SetPath("/v1/tools/:tool_name/invoke")
		c2.SetParamNames("tool_name")
		c2.SetParamValues("browser.screenshot")

		err = handler.InvokeTool(c2)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec2.Code)

		var secondResp domain.ToolInvokeResponse
		json.Unmarshal(rec2.Body.Bytes(), &secondResp)
		assert.Equal(t, firstResp.ToolCallID, secondResp.ToolCallID)
		assert.Equal(t, "pending", secondResp.Status)
		assert.Equal(t, "waiting_client", secondResp.Reason)
	})

	t.Run("Require Approval Payment", func(t *testing.T) {
		e := echo.New()
		handler, store := newTestHandler(t)

		// Setup Data
		store.CreateSession(ctx, &domain.Session{SessionID: "s3", UserID: "u3"})
		store.CreateRun(ctx, &domain.Run{RunID: "r3", SessionID: "s3", RootAgentID: "a1", Status: domain.RunStatusRunning})

		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID: "r3",
			Args:  json.RawMessage(`{"amount": 200}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/payments.transfer/invoke", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tools/:tool_name/invoke")
		c.SetParamNames("tool_name")
		c.SetParamValues("payments.transfer")

		err := handler.InvokeTool(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.ToolInvokeResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, "pending", resp.Status)
		assert.Equal(t, "waiting_approval", resp.Reason)
	})
}
