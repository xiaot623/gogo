package internalapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// SubmitToolResult handles tool result submission from ingress (on behalf of client).
// POST /internal/tool_calls/:tool_call_id/submit
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

	log.Printf("INFO: tool call %s result submitted: %s", toolCallID, newStatus)

	return c.JSON(http.StatusOK, domain.ToolCallResultResponse{
		ToolCallID:  toolCallID,
		Status:      newStatus,
		Result:      req.Result,
		Error:       req.Error,
		CompletedAt: now.UnixMilli(),
	})
}

// isTerminalStatus checks if a tool call status is terminal.
func isTerminalStatus(status domain.ToolCallStatus) bool {
	return status == domain.ToolCallStatusSucceeded ||
		status == domain.ToolCallStatusFailed ||
		status == domain.ToolCallStatusTimeout ||
		status == domain.ToolCallStatusBlocked ||
		status == domain.ToolCallStatusRejected
}
