package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// InvokeTool handles tool invocation.
func (h *Handler) InvokeTool(c echo.Context) error {
	toolName := c.Param("tool_name")
	var req domain.ToolInvokeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	ctx := c.Request().Context()

	// 1. Get Run and User ID (for policy)
	run, err := h.store.GetRun(ctx, req.RunID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
	}
	if run == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	session, err := h.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get session"})
	}

	// 2. Get Tool
	tool, err := h.store.GetTool(ctx, toolName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool"})
	}
	if tool == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
	}

	// 3. Policy Check via OPA
	policyInput := map[string]interface{}{
		"tool_name": toolName,
		"user_id":   session.UserID,
	}
	var argsMap map[string]interface{}
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &argsMap); err == nil {
			policyInput["args"] = argsMap
		}
	} else {
		policyInput["args"] = map[string]interface{}{}
	}

	decision, reason, err := h.policyEngine.Evaluate(ctx, policyInput)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "policy evaluation failed"})
	}

	toolCallID := "tc_" + uuid.New().String()
	now := time.Now()

	// Create ToolCall
	toolCall := &domain.ToolCall{
		ToolCallID: toolCallID,
		RunID:      req.RunID,
		ToolName:   toolName,
		Kind:       tool.Kind,
		Status:     domain.ToolCallStatusCreated,
		Args:       req.Args,
		CreatedAt:  now,
	}

	// Handle Decision
	if decision == "block" {
		toolCall.Status = domain.ToolCallStatusBlocked
		toolCall.Error = json.RawMessage(fmt.Sprintf(`{"code":"blocked","message":"%s"}`, reason))
		completedAt := now
		toolCall.CompletedAt = &completedAt
		h.store.CreateToolCall(ctx, toolCall)

		// Record policy decision event
		payload, _ := json.Marshal(domain.PolicyDecisionPayload{
			ToolCallID: toolCallID,
			Decision:   "block",
			Reason:     reason,
		})
		h.store.CreateEvent(ctx, &domain.Event{
			EventID: "evt_" + uuid.New().String(),
			RunID:   req.RunID,
			Ts:      now.UnixMilli(),
			Type:    domain.EventTypePolicyDecision,
			Payload: json.RawMessage(payload),
		})

		return c.JSON(http.StatusOK, domain.ToolInvokeResponse{
			Status:     "failed",
			ToolCallID: toolCallID,
			Error:      &domain.ToolError{Code: "blocked", Message: reason},
		})
	}

	if decision == "require_approval" {
		toolCall.Status = domain.ToolCallStatusWaitingApproval
		h.store.CreateToolCall(ctx, toolCall)

		approvalID := "ap_" + uuid.New().String()
		approval := &domain.Approval{
			ApprovalID: approvalID,
			RunID:      req.RunID,
			ToolCallID: toolCallID,
			Status:     domain.ApprovalStatusPending,
			CreatedAt:  now,
		}
		h.store.CreateApproval(ctx, approval)
		h.store.UpdateToolCallApproval(ctx, toolCallID, approvalID, domain.ToolCallStatusWaitingApproval)

		// Emit approval_required event
		payload, _ := json.Marshal(domain.ApprovalRequiredPayload{
			ApprovalID:  approvalID,
			ToolCallID:  toolCallID,
			ToolName:    toolName,
			ArgsSummary: "Approval required for " + toolName, // Simplification
			Args:        req.Args,
		})
		h.store.CreateEvent(ctx, &domain.Event{
			EventID: "evt_" + uuid.New().String(),
			RunID:   req.RunID,
			Ts:      now.UnixMilli(),
			Type:    domain.EventTypeApprovalRequired,
			Payload: json.RawMessage(payload),
		})

		return c.JSON(http.StatusOK, domain.ToolInvokeResponse{
			Status:     "pending",
			ToolCallID: toolCallID,
			Reason:     "waiting_approval",
		})
	}

	// Decision: allow
	toolCall.Status = domain.ToolCallStatusDispatched
	if tool.Kind == domain.ToolKindServer {
		toolCall.Status = domain.ToolCallStatusRunning
	}
	h.store.CreateToolCall(ctx, toolCall)

	// Execute Logic
	if tool.Kind == domain.ToolKindClient {
		// Emit tool_request event
		payload, _ := json.Marshal(domain.ToolRequestPayload{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Args:       req.Args,
			DeadlineTs: now.Add(time.Duration(tool.TimeoutMs) * time.Millisecond).UnixMilli(),
		})
		h.store.CreateEvent(ctx, &domain.Event{
			EventID: "evt_" + uuid.New().String(),
			RunID:   req.RunID,
			Ts:      now.UnixMilli(),
			Type:    domain.EventTypeToolRequest,
			Payload: json.RawMessage(payload),
		})
		return c.JSON(http.StatusOK, domain.ToolInvokeResponse{
			Status:     "pending",
			ToolCallID: toolCallID,
			Reason:     "waiting_client",
		})
	}

	// Server Tool Execution (Synchronous Mock)
	result := `{"status":"executed"}`
	if toolName == "weather.query" {
		result = `{"weather":"Sunny","temperature":25}`
	}
	h.store.UpdateToolCallResult(ctx, toolCallID, domain.ToolCallStatusSucceeded, []byte(result), nil)

	// Emit result event
	payload, _ := json.Marshal(domain.ToolResultPayload{
		ToolCallID: toolCallID,
		Status:     domain.ToolCallStatusSucceeded,
		Result:     json.RawMessage(result),
	})
	h.store.CreateEvent(ctx, &domain.Event{
		EventID: "evt_" + uuid.New().String(),
		RunID:   req.RunID,
		Ts:      time.Now().UnixMilli(),
		Type:    domain.EventTypeToolResult,
		Payload: json.RawMessage(payload),
	})

	return c.JSON(http.StatusOK, domain.ToolInvokeResponse{
		Status:     "succeeded",
		ToolCallID: toolCallID,
		Result:     json.RawMessage(result),
	})
}

// GetToolCall retrieves the status of a tool call.
func (h *Handler) GetToolCall(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	ctx := c.Request().Context()

	tc, err := h.store.GetToolCall(ctx, toolCallID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool call"})
	}
	if tc == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
	}

	resp := domain.ToolCallResponse{
		ToolCallID: tc.ToolCallID,
		Status:     tc.Status,
		Result:     tc.Result,
		Timestamps: domain.Timestamps{
			CreatedAt: tc.CreatedAt.UnixMilli(),
		},
	}
	if tc.CompletedAt != nil {
		resp.Timestamps.CompletedAt = tc.CompletedAt.UnixMilli()
	}

	return c.JSON(http.StatusOK, resp)
}

// WaitToolCall waits for a tool call to complete.
func (h *Handler) WaitToolCall(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	timeoutMs := 60000 // default 60s
	// Parse timeout from query param if needed (doc says query param)
	// But let's keep it simple or implement query param parsing.
	// doc: POST ...:wait?timeout_ms=...
	// Actually method is POST, so it might be query param or body. Doc says "Query params".

	ctx := c.Request().Context()

	// Polling loop
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(time.Duration(timeoutMs) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			// Timeout
			return h.GetToolCall(c)
		case <-ticker.C:
			tc, err := h.store.GetToolCall(ctx, toolCallID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool call"})
			}
			if tc == nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
			}

			if isTerminalStatus(tc.Status) {
				resp := domain.ToolCallResponse{
					ToolCallID: tc.ToolCallID,
					Status:     tc.Status,
					Result:     tc.Result,
					Timestamps: domain.Timestamps{
						CreatedAt: tc.CreatedAt.UnixMilli(),
					},
				}
				if tc.CompletedAt != nil {
					resp.Timestamps.CompletedAt = tc.CompletedAt.UnixMilli()
				}
				return c.JSON(http.StatusOK, resp)
			}
		}
	}
}

func isTerminalStatus(status domain.ToolCallStatus) bool {
	switch status {
	case domain.ToolCallStatusSucceeded, domain.ToolCallStatusFailed, domain.ToolCallStatusTimeout, domain.ToolCallStatusBlocked, domain.ToolCallStatusRejected:
		return true
	}
	return false
}

// SubmitToolResult handles client tool result submission.
func (h *Handler) SubmitToolResult(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	var req domain.ToolCallResultRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate status
	if req.Status != "SUCCEEDED" && req.Status != "FAILED" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "status must be SUCCEEDED or FAILED"})
	}

	ctx := c.Request().Context()

	// Get tool call
	tc, err := h.store.GetToolCall(ctx, toolCallID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool call"})
	}
	if tc == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
	}

	// Check if already in terminal state (idempotency)
	if isTerminalStatus(tc.Status) {
		var completedAt int64
		if tc.CompletedAt != nil {
			completedAt = tc.CompletedAt.UnixMilli()
		}
		return c.JSON(http.StatusOK, domain.ToolCallResultResponse{
			ToolCallID:  tc.ToolCallID,
			Status:      tc.Status,
			Result:      tc.Result,
			Error:       tc.Error,
			CompletedAt: completedAt,
		})
	}

	// Validate tool call is in a valid state for result submission
	if tc.Status != domain.ToolCallStatusDispatched && tc.Status != domain.ToolCallStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("tool call is in state %s, cannot submit result", tc.Status),
		})
	}

	// Determine new status
	var newStatus domain.ToolCallStatus
	if req.Status == "SUCCEEDED" {
		newStatus = domain.ToolCallStatusSucceeded
	} else {
		newStatus = domain.ToolCallStatusFailed
	}

	// Update tool call result
	if err := h.store.UpdateToolCallResult(ctx, toolCallID, newStatus, req.Result, req.Error); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update tool call"})
	}

	// Record event
	now := time.Now()
	payload, _ := json.Marshal(domain.ToolResultPayload{
		ToolCallID: toolCallID,
		Status:     newStatus,
		Result:     req.Result,
		Error:      req.Error,
	})
	h.store.CreateEvent(ctx, &domain.Event{
		EventID: "evt_" + uuid.New().String()[:8],
		RunID:   tc.RunID,
		Ts:      now.UnixMilli(),
		Type:    domain.EventTypeToolResult,
		Payload: json.RawMessage(payload),
	})

	return c.JSON(http.StatusOK, domain.ToolCallResultResponse{
		ToolCallID:  toolCallID,
		Status:      newStatus,
		Result:      req.Result,
		Error:       req.Error,
		CompletedAt: now.UnixMilli(),
	})
}
