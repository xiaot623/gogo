package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/xiaot623/gogo/orchestrator/api"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func TestInvokeTool(t *testing.T) {
	store := helpers.NewTestSQLiteStore(t)
	ctx := context.Background()
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	assert.NoError(t, err)

	handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
	e := echo.New()

	// Setup Data
	store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
	store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})
	// Tools are seeded by NewSQLiteStore

	t.Run("Allow Weather Query", func(t *testing.T) {
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
		assert.Equal(t, "succeeded", resp.Status)
	})

	t.Run("Block Dangerous Command", func(t *testing.T) {
		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID: "r1",
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

	t.Run("Require Approval Payment", func(t *testing.T) {
		reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
			RunID: "r1",
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

func TestSubmitToolResult(t *testing.T) {
	ctx := context.Background()

	t.Run("Submit Success Result", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		// Setup
		store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
		store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})
		store.CreateToolCall(ctx, &domain.ToolCall{
			ToolCallID: "tc_test1",
			RunID:      "r1",
			ToolName:   "browser.screenshot",
			Kind:       domain.ToolKindClient,
			Status:     domain.ToolCallStatusDispatched,
			Args:       json.RawMessage(`{"url":"https://example.com"}`),
		})

		// Submit result
		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "SUCCEEDED",
			Result: json.RawMessage(`{"screenshot":"base64data"}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_test1/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_test1")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.ToolCallResultResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, "tc_test1", resp.ToolCallID)
		assert.Equal(t, domain.ToolCallStatusSucceeded, resp.Status)
		assert.NotZero(t, resp.CompletedAt)

		// Verify in store
		tc, _ := store.GetToolCall(ctx, "tc_test1")
		assert.Equal(t, domain.ToolCallStatusSucceeded, tc.Status)
	})

	t.Run("Submit Failed Result", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		// Setup
		store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
		store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})
		store.CreateToolCall(ctx, &domain.ToolCall{
			ToolCallID: "tc_test2",
			RunID:      "r1",
			ToolName:   "browser.screenshot",
			Kind:       domain.ToolKindClient,
			Status:     domain.ToolCallStatusRunning,
			Args:       json.RawMessage(`{"url":"https://example.com"}`),
		})

		// Submit failed result
		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "FAILED",
			Error:  json.RawMessage(`{"code":"timeout","message":"browser timeout"}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_test2/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_test2")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.ToolCallResultResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, domain.ToolCallStatusFailed, resp.Status)

		// Verify in store
		tc, _ := store.GetToolCall(ctx, "tc_test2")
		assert.Equal(t, domain.ToolCallStatusFailed, tc.Status)
	})

	t.Run("Tool Call Not Found", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "SUCCEEDED",
			Result: json.RawMessage(`{}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_nonexistent/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_nonexistent")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Invalid Status", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "INVALID",
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_any/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_any")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Idempotency - Already Completed", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		// Setup already completed tool call
		store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
		store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})
		store.CreateToolCall(ctx, &domain.ToolCall{
			ToolCallID: "tc_completed",
			RunID:      "r1",
			ToolName:   "browser.screenshot",
			Kind:       domain.ToolKindClient,
			Status:     domain.ToolCallStatusDispatched,
			CreatedAt:  time.Now(),
		})
		// Update to completed state with result
		store.UpdateToolCallResult(ctx, "tc_completed", domain.ToolCallStatusSucceeded, []byte(`{"existing":"result"}`), nil)

		// Try to submit again
		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "SUCCEEDED",
			Result: json.RawMessage(`{"new":"result"}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_completed/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_completed")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.ToolCallResultResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		// Should return the existing result, not the new one
		assert.Equal(t, domain.ToolCallStatusSucceeded, resp.Status)
		assert.Contains(t, string(resp.Result), "existing")
	})

	t.Run("Invalid State - Waiting Approval", func(t *testing.T) {
		store := helpers.NewTestSQLiteStore(t)
		policyEngine, _ := policy.NewEngine(ctx, policy.DefaultPolicy)
		handler := api.NewHandler(store, nil, &config.Config{}, policyEngine)
		e := echo.New()

		// Setup tool call in WAITING_APPROVAL state
		store.CreateSession(ctx, &domain.Session{SessionID: "s1", UserID: "u1"})
		store.CreateRun(ctx, &domain.Run{RunID: "r1", SessionID: "s1", RootAgentID: "a1", Status: domain.RunStatusRunning})
		store.CreateToolCall(ctx, &domain.ToolCall{
			ToolCallID: "tc_waiting",
			RunID:      "r1",
			ToolName:   "payments.transfer",
			Kind:       domain.ToolKindServer,
			Status:     domain.ToolCallStatusWaitingApproval,
		})

		reqBody, _ := json.Marshal(domain.ToolCallResultRequest{
			Status: "SUCCEEDED",
			Result: json.RawMessage(`{}`),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/tool_calls/tc_waiting/result", bytes.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/tool_calls/:tool_call_id/result")
		c.SetParamNames("tool_call_id")
		c.SetParamValues("tc_waiting")

		err := handler.SubmitToolResult(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}
