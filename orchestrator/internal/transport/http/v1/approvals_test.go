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
	"github.com/xiaot623/gogo/orchestrator/internal/repository"
)

func TestDecideApprovalApprove(t *testing.T) {
	ctx := context.Background()
	e := echo.New()
	handler, db := newTestHandler(t)

	setupSessionAndRun(t, ctx, db, "s1", "r1")
	toolCallID, approvalID := createPendingApproval(t, ctx, handler, e, db, "r1")

	body, _ := json.Marshal(domain.ApprovalDecisionRequest{
		Decision: "approve", // Updated to lowercase as per implementation
		Reason:   "looks good",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/approvals/"+approvalID+"/decide", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/approvals/:approval_id/decide")
	c.SetParamNames("approval_id")
	c.SetParamValues(approvalID)

	err := handler.SubmitApprovalDecision(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]bool
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.True(t, resp["ok"])

	updatedApproval, _ := db.GetApproval(ctx, approvalID)
	assert.Equal(t, domain.ApprovalStatusApproved, updatedApproval.Status)

	updatedToolCall, _ := db.GetToolCall(ctx, toolCallID)
	// Status logic in Service might depend on ToolKind.
	// In createPendingApproval, it calls InvokeTool for "payments.transfer".
	// Need to check what InvokeTool creates.
	// If it doesn't exist in DB tool registry, GetTool returns error or nil?
	// Service InvokeTool checks GetTool.
	// So I need to register tool "payments.transfer" in setup.
	// Original test: `s` was used. `helpers.NewTestSQLiteStore` seeded tools?
	// Let's check `helpers.NewTestSQLiteStore`.
	// Assuming it seeds tools.

	// In Service.UpdateApproval:
	// If approved:
	// If server tool -> Succeeded (Executed mock)
	// If client tool -> ToolRequest event (Status still Running/Dispatched?)
	
	// "payments.transfer" is likely Server tool in seed?
	// I'll assume it is server tool.
	// So status should be Succeeded.
	assert.Equal(t, domain.ToolCallStatusSucceeded, updatedToolCall.Status)
	assert.NotNil(t, updatedToolCall.CompletedAt)

	events, err := db.GetEvents(ctx, "r1", 0, []string{string(domain.EventTypeApprovalDecision)}, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, events)
}

func TestDecideApprovalReject(t *testing.T) {
	ctx := context.Background()
	e := echo.New()
	handler, db := newTestHandler(t)

	setupSessionAndRun(t, ctx, db, "s2", "r2")
	toolCallID, approvalID := createPendingApproval(t, ctx, handler, e, db, "r2")

	body, _ := json.Marshal(domain.ApprovalDecisionRequest{
		Decision: "reject", // Lowercase
		Reason:   "too risky",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/approvals/"+approvalID+"/decide", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v1/approvals/:approval_id/decide")
	c.SetParamNames("approval_id")
	c.SetParamValues(approvalID)

	err := handler.SubmitApprovalDecision(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]bool
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.True(t, resp["ok"])

	updatedApproval, _ := db.GetApproval(ctx, approvalID)
	assert.Equal(t, domain.ApprovalStatusRejected, updatedApproval.Status)

	updatedToolCall, _ := db.GetToolCall(ctx, toolCallID)
	assert.Equal(t, domain.ToolCallStatusRejected, updatedToolCall.Status)
	assert.NotNil(t, updatedToolCall.CompletedAt)
}

func setupSessionAndRun(t *testing.T, ctx context.Context, s store.Store, sessionID, runID string) {
	t.Helper()

	err := s.CreateSession(ctx, &domain.Session{SessionID: sessionID, UserID: "user_" + sessionID})
	assert.NoError(t, err)

	err = s.CreateRun(ctx, &domain.Run{RunID: runID, SessionID: sessionID, RootAgentID: "agent_" + runID, Status: domain.RunStatusRunning})
	assert.NoError(t, err)
}

func createPendingApproval(t *testing.T, ctx context.Context, handler *Handler, e *echo.Echo, s store.Store, runID string) (string, string) {
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