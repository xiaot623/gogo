package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) InvokeTool(ctx context.Context, toolName string, req domain.ToolInvokeRequest) (*domain.ToolInvokeResponse, error) {
	// 1. Get Run and User ID (for policy)
	run, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("run not found")
	}

	session, err := s.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// 2. Get Tool
	tool, err := s.store.GetTool(ctx, toolName)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool: %w", err)
	}
	if tool == nil {
		return nil, fmt.Errorf("tool not found")
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

	decision, reason, err := s.policyEngine.Evaluate(ctx, policyInput)
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
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
		s.store.CreateToolCall(ctx, toolCall)

		// Record policy decision event
		payload, _ := json.Marshal(domain.PolicyDecisionPayload{
			ToolCallID: toolCallID,
			Decision:   "block",
			Reason:     reason,
		})
		s.recordEvent(ctx, req.RunID, domain.EventTypePolicyDecision, payload)

		return &domain.ToolInvokeResponse{
			Status:     "failed",
			ToolCallID: toolCallID,
			Error:      &domain.ToolError{Code: "blocked", Message: reason},
		}, nil
	}

	if decision == "require_approval" {
		toolCall.Status = domain.ToolCallStatusWaitingApproval
		s.store.CreateToolCall(ctx, toolCall)

		approvalID := "ap_" + uuid.New().String()
		approval := &domain.Approval{
			ApprovalID: approvalID,
			RunID:      req.RunID,
			ToolCallID: toolCallID,
			Status:     domain.ApprovalStatusPending,
			CreatedAt:  now,
		}
		s.store.CreateApproval(ctx, approval)
		s.store.UpdateToolCallApproval(ctx, toolCallID, approvalID, domain.ToolCallStatusWaitingApproval)

		// Emit approval_required event
		payload, _ := json.Marshal(domain.ApprovalRequiredPayload{
			ApprovalID:  approvalID,
			ToolCallID:  toolCallID,
			ToolName:    toolName,
			ArgsSummary: "Approval required for " + toolName, // Simplification
			Args:        req.Args,
		})
		s.recordEvent(ctx, req.RunID, domain.EventTypeApprovalRequired, payload)
		
		// Push to ingress
		// We need to push the approval request to the client
		if s.ingressClient != nil {
			var argsObj interface{}
			json.Unmarshal(req.Args, &argsObj)
			s.ingressClient.PushEvent(session.SessionID, map[string]interface{}{
				"type": "approval_required",
				"ts": now.UnixMilli(),
				"run_id": req.RunID,
				"approval_id": approvalID,
				"tool_call_id": toolCallID,
				"tool_name": toolName,
				"args_summary": "Approval required for " + toolName,
			})
		}

		return &domain.ToolInvokeResponse{
			Status:     "pending",
			ToolCallID: toolCallID,
			Reason:     "waiting_approval",
		}, nil
	}

	// Decision: allow
	toolCall.Status = domain.ToolCallStatusDispatched
	if tool.Kind == domain.ToolKindServer {
		toolCall.Status = domain.ToolCallStatusRunning
	}
	s.store.CreateToolCall(ctx, toolCall)

	// Execute Logic
	if tool.Kind == domain.ToolKindClient {
		// Emit tool_request event
		payload, _ := json.Marshal(domain.ToolRequestPayload{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Args:       req.Args,
			DeadlineTs: now.Add(time.Duration(tool.TimeoutMs) * time.Millisecond).UnixMilli(),
		})
		s.recordEvent(ctx, req.RunID, domain.EventTypeToolRequest, payload)
		
		// Push to ingress
		if s.ingressClient != nil {
			var argsObj interface{}
			json.Unmarshal(req.Args, &argsObj)
			s.ingressClient.PushEvent(session.SessionID, map[string]interface{}{
				"type": "tool_request",
				"ts": now.UnixMilli(),
				"run_id": req.RunID,
				"tool_call_id": toolCallID,
				"tool_name": toolName,
				"args": argsObj,
				"deadline_ts": now.Add(time.Duration(tool.TimeoutMs) * time.Millisecond).UnixMilli(),
			})
		}

		return &domain.ToolInvokeResponse{
			Status:     "pending",
			ToolCallID: toolCallID,
			Reason:     "waiting_client",
		}, nil
	}

	// Server Tool Execution (Synchronous Mock)
	result := `{"status":"executed"}`
	if toolName == "weather.query" {
		result = `{"weather":"Sunny","temperature":25}`
	}
	s.store.UpdateToolCallResult(ctx, toolCallID, domain.ToolCallStatusSucceeded, []byte(result), nil)

	// Emit result event
	payload, _ := json.Marshal(domain.ToolResultPayload{
		ToolCallID: toolCallID,
		Status:     domain.ToolCallStatusSucceeded,
		Result:     json.RawMessage(result),
	})
	s.recordEvent(ctx, req.RunID, domain.EventTypeToolResult, payload)

	return &domain.ToolInvokeResponse{
		Status:     "succeeded",
		ToolCallID: toolCallID,
		Result:     json.RawMessage(result),
	}, nil
}

func (s *Service) GetToolCall(ctx context.Context, toolCallID string) (*domain.ToolCall, error) {
	tc, err := s.store.GetToolCall(ctx, toolCallID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool call: %w", err)
	}
	return tc, nil
}

func (s *Service) WaitToolCall(ctx context.Context, toolCallID string, timeoutMs int) (*domain.ToolCall, error) {
	// Polling loop
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(time.Duration(timeoutMs) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			// Timeout
			return s.GetToolCall(ctx, toolCallID)
		case <-ticker.C:
			tc, err := s.store.GetToolCall(ctx, toolCallID)
			if err != nil {
				return nil, fmt.Errorf("failed to get tool call: %w", err)
			}
			if tc == nil {
				return nil, fmt.Errorf("tool call not found")
			}

			if isTerminalStatus(tc.Status) {
				return tc, nil
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

func (s *Service) SubmitToolResult(ctx context.Context, toolCallID string, req domain.ToolCallResultRequest) (*domain.ToolCallResultResponse, error) {
	// Get tool call
	tc, err := s.store.GetToolCall(ctx, toolCallID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool call: %w", err)
	}
	if tc == nil {
		return nil, fmt.Errorf("tool call not found")
	}

	// Check if already in terminal state (idempotency)
	if isTerminalStatus(tc.Status) {
		var completedAt int64
		if tc.CompletedAt != nil {
			completedAt = tc.CompletedAt.UnixMilli()
		}
		return &domain.ToolCallResultResponse{
			ToolCallID:  tc.ToolCallID,
			Status:      tc.Status,
			Result:      tc.Result,
			Error:       tc.Error,
			CompletedAt: completedAt,
		}, nil
	}

	// Validate tool call is in a valid state for result submission
	if tc.Status != domain.ToolCallStatusDispatched && tc.Status != domain.ToolCallStatusRunning {
		return nil, fmt.Errorf("tool call is in state %s, cannot submit result", tc.Status)
	}

	// Determine new status
	var newStatus domain.ToolCallStatus
	if req.Status == "SUCCEEDED" {
		newStatus = domain.ToolCallStatusSucceeded
	} else {
		newStatus = domain.ToolCallStatusFailed
	}

	// Update tool call result
	if err := s.store.UpdateToolCallResult(ctx, toolCallID, newStatus, req.Result, req.Error); err != nil {
		return nil, fmt.Errorf("failed to update tool call: %w", err)
	}

	// Record event
	now := time.Now()
	payload, _ := json.Marshal(domain.ToolResultPayload{
		ToolCallID: toolCallID,
		Status:     newStatus,
		Result:     req.Result,
		Error:      req.Error,
	})
	s.recordEvent(ctx, tc.RunID, domain.EventTypeToolResult, payload)

	return &domain.ToolCallResultResponse{
		ToolCallID:  toolCallID,
		Status:      newStatus,
		Result:      req.Result,
		Error:       req.Error,
		CompletedAt: now.UnixMilli(),
	}, nil
}
