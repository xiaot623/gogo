package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// DecideApproval handles approval decisions for tool calls.
func (h *Handler) DecideApproval(c echo.Context) error {
	approvalID := c.Param("approval_id")

	var req domain.ApprovalDecisionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	decision := domain.ApprovalStatus(strings.ToUpper(req.Decision))
	if decision != domain.ApprovalStatusApproved && decision != domain.ApprovalStatusRejected {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "decision must be APPROVED or REJECTED"})
	}

	ctx := c.Request().Context()

	approval, err := h.store.GetApproval(ctx, approvalID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get approval"})
	}
	if approval == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
	}

	toolCall, err := h.store.GetToolCall(ctx, approval.ToolCallID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool call"})
	}
	if toolCall == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
	}

	tool, err := h.store.GetTool(ctx, toolCall.ToolName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool"})
	}
	if tool == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
	}

	// Idempotent handling: if already decided, return current state.
	if approval.Status != domain.ApprovalStatusPending {
		return c.JSON(http.StatusOK, domain.ApprovalDecisionResponse{
			ApprovalID:     approval.ApprovalID,
			Status:         approval.Status,
			ToolCallID:     approval.ToolCallID,
			ToolCallStatus: toolCall.Status,
			Result:         toolCall.Result,
			Error:          toolCall.Error,
		})
	}

	if err := h.store.UpdateApprovalStatus(ctx, approvalID, decision, req.DecidedBy, req.Reason); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}

	if err := h.recordEvent(ctx, approval.RunID, domain.EventTypeApprovalDecision, domain.ApprovalDecisionPayload{
		ApprovalID: approvalID,
		Decision:   decision,
		Reason:     req.Reason,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record event"})
	}

	switch decision {
	case domain.ApprovalStatusRejected:
		errMsg := req.Reason
		if errMsg == "" {
			errMsg = "rejected"
		}
		errData := json.RawMessage(fmt.Sprintf(`{"code":"rejected","message":"%s"}`, errMsg))
		if err := h.store.UpdateToolCallResult(ctx, toolCall.ToolCallID, domain.ToolCallStatusRejected, nil, errData); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update tool call"})
		}
	case domain.ApprovalStatusApproved:
		if err := h.handleApprovedToolCall(ctx, approval.RunID, toolCall, tool); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}

	updatedToolCall, err := h.store.GetToolCall(ctx, approval.ToolCallID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool call"})
	}

	return c.JSON(http.StatusOK, domain.ApprovalDecisionResponse{
		ApprovalID:     approvalID,
		Status:         decision,
		ToolCallID:     approval.ToolCallID,
		ToolCallStatus: updatedToolCall.Status,
		Result:         updatedToolCall.Result,
		Error:          updatedToolCall.Error,
	})
}

// handleApprovedToolCall resumes execution after an approval is granted.
func (h *Handler) handleApprovedToolCall(ctx context.Context, runID string, toolCall *domain.ToolCall, tool *domain.Tool) error {
	// Mark approved before dispatch.
	if err := h.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, domain.ToolCallStatusApproved); err != nil {
		return err
	}

	switch tool.Kind {
	case domain.ToolKindClient:
		if err := h.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, domain.ToolCallStatusDispatched); err != nil {
			return err
		}

		payload := domain.ToolRequestPayload{
			ToolCallID: toolCall.ToolCallID,
			ToolName:   toolCall.ToolName,
			Args:       toolCall.Args,
			DeadlineTs: time.Now().Add(time.Duration(tool.TimeoutMs) * time.Millisecond).UnixMilli(),
		}
		return h.recordEvent(ctx, runID, domain.EventTypeToolRequest, payload)

	case domain.ToolKindServer:
		if err := h.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, domain.ToolCallStatusRunning); err != nil {
			return err
		}

		result := `{"status":"executed"}`
		if toolCall.ToolName == "weather.query" {
			result = `{"weather":"Sunny","temperature":25}`
		}

		if err := h.store.UpdateToolCallResult(ctx, toolCall.ToolCallID, domain.ToolCallStatusSucceeded, []byte(result), nil); err != nil {
			return err
		}

		return h.recordEvent(ctx, runID, domain.EventTypeToolResult, domain.ToolResultPayload{
			ToolCallID: toolCall.ToolCallID,
			Status:     domain.ToolCallStatusSucceeded,
			Result:     json.RawMessage(result),
		})

	default:
		return fmt.Errorf("unsupported tool kind: %s", tool.Kind)
	}
}
