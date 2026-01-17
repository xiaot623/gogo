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

	// Update approval status
	newStatus := domain.ApprovalStatusApproved
	if req.Decision == "reject" {
		newStatus = domain.ApprovalStatusRejected
	}

	if err := s.store.UpdateApprovalStatus(ctx, approvalID, newStatus, req.DecidedBy, req.Reason); err != nil {
		return fmt.Errorf("failed to update approval status: %w", err)
	}

	// Update ToolCall status
	toolCallStatus := domain.ToolCallStatusRunning
	if newStatus == domain.ApprovalStatusRejected {
		toolCallStatus = domain.ToolCallStatusRejected
	}

	if err := s.store.UpdateToolCallApproval(ctx, approval.ToolCallID, approvalID, toolCallStatus); err != nil {
		return fmt.Errorf("failed to update tool call status: %w", err)
	}

	// Record event
	payload, _ := json.Marshal(domain.ApprovalDecisionPayload{
		ApprovalID: approvalID,
		Decision:   newStatus,
		Reason:     req.Reason,
	})
	s.recordEvent(ctx, approval.RunID, domain.EventTypeApprovalDecision, payload)

	// If approved, we need to continue execution
	if newStatus == domain.ApprovalStatusApproved {
		// In MVP, we just mark it as Running (if server tool) or Dispatched (if needed).
		// But if it's a server tool, we should execute it now?
		// Architecture doc says: "审批通过后平台自动继续执行该 tool 节点"
		// If it's a server tool, we need to execute it.
		// We need to fetch the tool call to know if it's server or client.
		
		tc, err := s.store.GetToolCall(ctx, approval.ToolCallID)
		if err != nil {
			return fmt.Errorf("failed to get tool call: %w", err)
		}

		if tc.Kind == domain.ToolKindServer {
			// Execute Logic (Synchronous Mock)
			// This is duplicated logic from InvokeTool. Ideally refactor into `executeServerTool`.
			result := `{"status":"executed"}`
			if tc.ToolName == "weather.query" {
				result = `{"weather":"Sunny","temperature":25}`
			}
			s.store.UpdateToolCallResult(ctx, tc.ToolCallID, domain.ToolCallStatusSucceeded, []byte(result), nil)

			// Emit result event
			payload, _ := json.Marshal(domain.ToolResultPayload{
				ToolCallID: tc.ToolCallID,
				Status:     domain.ToolCallStatusSucceeded,
				Result:     json.RawMessage(result),
			})
			s.recordEvent(ctx, tc.RunID, domain.EventTypeToolResult, payload)
		} else {
			// Client tool
			// It should be dispatched again? Or just marked as running?
			// If approved, we should send tool_request to client?
			// Yes.
			
			// Emit tool_request event
			// We need tool definition for timeout.
			tool, _ := s.store.GetTool(ctx, tc.ToolName)
			timeout := 60000
			if tool != nil {
				timeout = tool.TimeoutMs
			}

			payload, _ := json.Marshal(domain.ToolRequestPayload{
				ToolCallID: tc.ToolCallID,
				ToolName:   tc.ToolName,
				Args:       tc.Args,
				DeadlineTs: time.Now().Add(time.Duration(timeout) * time.Millisecond).UnixMilli(),
			})
			s.recordEvent(ctx, tc.RunID, domain.EventTypeToolRequest, payload)

			// Push to ingress
			if s.ingressClient != nil {
				var argsObj interface{}
				json.Unmarshal(tc.Args, &argsObj)
				
				// We need session ID. Run has session ID.
				run, _ := s.store.GetRun(ctx, tc.RunID)
				if run != nil {
					s.ingressClient.PushEvent(run.SessionID, map[string]interface{}{
						"type": "tool_request",
						"ts": time.Now().UnixMilli(),
						"run_id": tc.RunID,
						"tool_call_id": tc.ToolCallID,
						"tool_name": tc.ToolName,
						"args": argsObj,
						"deadline_ts": time.Now().Add(time.Duration(timeout) * time.Millisecond).UnixMilli(),
					})
				}
			}
		}
	} else {
		// Rejected
		// Emit tool result event (failed)
		payload, _ := json.Marshal(domain.ToolResultPayload{
			ToolCallID: approval.ToolCallID,
			Status:     domain.ToolCallStatusRejected,
			Error:      json.RawMessage(`{"code":"rejected","message":"approval rejected"}`),
		})
		s.recordEvent(ctx, approval.RunID, domain.EventTypeToolResult, payload)
	}

	return nil
}