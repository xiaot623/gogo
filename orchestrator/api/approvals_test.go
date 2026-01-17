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
	"github.com/xiaot623/gogo/orchestrator/store"
	"github.com/xiaot623/gogo/orchestrator/tests/helpers"
)

func TestDecideApprovalApprove(t *testing.T) {
	ctx := context.Background()
	s := helpers.NewTestSQLiteStore(t)
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	assert.NoError(t, err)

	handler := api.NewHandler(s, nil, &config.Config{}, policyEngine)
	e := echo.New()

	setupSessionAndRun(t, ctx, s, "s1", "r1")
	toolCallID, approvalID := createPendingApproval(t, ctx, handler, e, s, "r1")

	body, _ := json.Marshal(domain.ApprovalDecisionRequest{
		Decision: "APPROVED",
		Reason:   "looks good",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/approvals/"+approvalID+"/decide", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/approvals/:approval_id/decide")
	c.SetParamNames("approval_id")
	c.SetParamValues(approvalID)

	err = handler.DecideApproval(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp domain.ApprovalDecisionResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, domain.ApprovalStatusApproved, resp.Status)
	assert.Equal(t, domain.ToolCallStatusSucceeded, resp.ToolCallStatus)
	assert.NotEmpty(t, resp.Result)

	updatedApproval, _ := s.GetApproval(ctx, approvalID)
	assert.Equal(t, domain.ApprovalStatusApproved, updatedApproval.Status)

	updatedToolCall, _ := s.GetToolCall(ctx, toolCallID)
	assert.Equal(t, domain.ToolCallStatusSucceeded, updatedToolCall.Status)
	assert.NotNil(t, updatedToolCall.CompletedAt)

	events, err := s.GetEvents(ctx, "r1", 0, []string{string(domain.EventTypeApprovalDecision)}, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, events)
}

func TestDecideApprovalReject(t *testing.T) {
	ctx := context.Background()
	s := helpers.NewTestSQLiteStore(t)
	policyEngine, err := policy.NewEngine(ctx, policy.DefaultPolicy)
	assert.NoError(t, err)

	handler := api.NewHandler(s, nil, &config.Config{}, policyEngine)
	e := echo.New()

	setupSessionAndRun(t, ctx, s, "s2", "r2")
	toolCallID, approvalID := createPendingApproval(t, ctx, handler, e, s, "r2")

	body, _ := json.Marshal(domain.ApprovalDecisionRequest{
		Decision: "REJECTED",
		Reason:   "too risky",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/approvals/"+approvalID+"/decide", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/approvals/:approval_id/decide")
	c.SetParamNames("approval_id")
	c.SetParamValues(approvalID)

	err = handler.DecideApproval(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp domain.ApprovalDecisionResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, domain.ApprovalStatusRejected, resp.Status)
	assert.Equal(t, domain.ToolCallStatusRejected, resp.ToolCallStatus)
	assert.NotEmpty(t, resp.Error)

	updatedApproval, _ := s.GetApproval(ctx, approvalID)
	assert.Equal(t, domain.ApprovalStatusRejected, updatedApproval.Status)

	updatedToolCall, _ := s.GetToolCall(ctx, toolCallID)
	assert.Equal(t, domain.ToolCallStatusRejected, updatedToolCall.Status)
	assert.NotNil(t, updatedToolCall.CompletedAt)
	assert.NotEmpty(t, updatedToolCall.Error)
}

func setupSessionAndRun(t *testing.T, ctx context.Context, s *store.SQLiteStore, sessionID, runID string) {
	t.Helper()

	err := s.CreateSession(ctx, &domain.Session{SessionID: sessionID, UserID: "user_" + sessionID})
	assert.NoError(t, err)

	err = s.CreateRun(ctx, &domain.Run{RunID: runID, SessionID: sessionID, RootAgentID: "agent_" + runID, Status: domain.RunStatusRunning})
	assert.NoError(t, err)
}

func createPendingApproval(t *testing.T, ctx context.Context, handler *api.Handler, e *echo.Echo, s *store.SQLiteStore, runID string) (string, string) {
	t.Helper()

	reqBody, _ := json.Marshal(domain.ToolInvokeRequest{
		RunID: runID,
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

	tc, err := s.GetToolCall(ctx, resp.ToolCallID)
	assert.NoError(t, err)
	assert.NotNil(t, tc)
	assert.NotEmpty(t, tc.ApprovalID)

	return resp.ToolCallID, tc.ApprovalID
}
