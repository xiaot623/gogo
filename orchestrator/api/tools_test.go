package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
