package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) UpdateApproval(ctx context.Context, approvalID string, req domain.ApprovalDecisionRequest) error {
	approval, err := s.store.GetApproval(ctx, approvalID)
	if err != nil {
		return fmt.Errorf("failed to get approval: %w", err)
	}
	if approval == nil {
		return fmt.Errorf("approval not found")
	}

	if approval.Status != domain.ApprovalStatusPending {
		return fmt.Errorf("approval is not pending")
	}

	tc, err := s.store.GetToolCall(ctx, approval.ToolCallID)
	if err != nil {
		return fmt.Errorf("failed to get tool call: %w", err)
	}
	if tc == nil {
		return fmt.Errorf("tool call not found")
	}
	if isTerminalStatus(tc.Status) {
		return fmt.Errorf("tool call already completed")
	}

	// Update approval status
	newStatus := domain.ApprovalStatusApproved
	if req.Decision == "reject" {
		newStatus = domain.ApprovalStatusRejected
	}

	if err := s.store.UpdateApprovalStatus(ctx, approvalID, newStatus, req.DecidedBy, req.Reason); err != nil {
		return fmt.Errorf("failed to update approval status: %w", err)
	}

	// Record event
	decisionPayload := domain.ApprovalDecisionPayload{
		ApprovalID: approvalID,
		Decision:   newStatus,
		Reason:     req.Reason,
	}
	s.recordEvent(ctx, approval.RunID, domain.EventTypeApprovalDecision, decisionPayload)

	// Rejected: finalize tool call.
	if newStatus == domain.ApprovalStatusRejected {
		errData := json.RawMessage(`{"code":"rejected","message":"approval rejected"}`)
		updated, err := s.store.UpdateToolCallResult(ctx, approval.ToolCallID, domain.ToolCallStatusRejected, nil, errData)
		if err != nil {
			return fmt.Errorf("failed to update tool call: %w", err)
		}
		if updated {
			payload := domain.ToolResultPayload{
				ToolCallID: approval.ToolCallID,
				Status:     domain.ToolCallStatusRejected,
				Error:      errData,
			}
			s.recordEvent(ctx, approval.RunID, domain.EventTypeToolResult, payload)
		}
		return nil
	}

	// Approved: dispatch/execute tool call.
	if tc.Kind == domain.ToolKindServer {
		_, _ = s.store.UpdateToolCallStatus(ctx, tc.ToolCallID, domain.ToolCallStatusRunning)

		tool, err := s.store.GetTool(ctx, tc.ToolName)
		if err != nil {
			return fmt.Errorf("failed to get tool: %w", err)
		}
		if tool == nil {
			return fmt.Errorf("tool not found")
		}
		go s.executeServerToolAsync(tc, tool)
		return nil
	}

	_, _ = s.store.UpdateToolCallStatus(ctx, tc.ToolCallID, domain.ToolCallStatusDispatched)

	nowMs := time.Now().UnixMilli()
	deadlineTs := time.Now().Add(time.Duration(tc.TimeoutMs) * time.Millisecond).UnixMilli()
	requestPayload := domain.ToolRequestPayload{
		ToolCallID: tc.ToolCallID,
		ToolName:   tc.ToolName,
		Args:       tc.Args,
		DeadlineTs: deadlineTs,
	}
	s.recordEvent(ctx, tc.RunID, domain.EventTypeToolRequest, requestPayload)

	if s.ingressClient != nil {
		var argsObj interface{}
		_ = json.Unmarshal(tc.Args, &argsObj)
		run, _ := s.store.GetRun(ctx, tc.RunID)
		if run != nil {
			s.ingressClient.PushEvent(run.SessionID, map[string]interface{}{
				"type":         "tool_request",
				"ts":           nowMs,
				"run_id":       tc.RunID,
				"tool_call_id": tc.ToolCallID,
				"tool_name":    tc.ToolName,
				"args":         argsObj,
				"deadline_ts":  deadlineTs,
			})
		}
	}

	return nil
}
