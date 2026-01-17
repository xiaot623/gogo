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
	timeoutMs := tool.TimeoutMs
	if req.TimeoutMs > 0 {
		timeoutMs = req.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = int(s.config.ToolTimeout.Milliseconds())
	}
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	// Create ToolCall
	toolCall := &domain.ToolCall{
		ToolCallID: toolCallID,
		RunID:      req.RunID,
		ToolName:   toolName,
		Kind:       tool.Kind,
		Status:     domain.ToolCallStatusCreated,
		Args:       req.Args,
		TimeoutMs:  timeoutMs,
		CreatedAt:  now,
	}

	// Handle Decision
	if decision == "block" {
		toolCall.Status = domain.ToolCallStatusBlocked
		errData, _ := json.Marshal(map[string]string{"code": "blocked", "message": reason})
		toolCall.Error = errData
		completedAt := now
		toolCall.CompletedAt = &completedAt
		_ = s.store.CreateToolCall(ctx, toolCall)

		// Record policy decision event
		payload := domain.PolicyDecisionPayload{
			ToolCallID: toolCallID,
			Decision:   "block",
			Reason:     reason,
		}
		s.recordEvent(ctx, req.RunID, domain.EventTypePolicyDecision, payload)

		return &domain.ToolInvokeResponse{
			Status:     "failed",
			ToolCallID: toolCallID,
			Error:      &domain.ToolError{Code: "blocked", Message: reason},
		}, nil
	}

	if decision == "require_approval" {
		toolCall.Status = domain.ToolCallStatusWaitingApproval
		_ = s.store.CreateToolCall(ctx, toolCall)

		approvalID := "ap_" + uuid.New().String()
		approval := &domain.Approval{
			ApprovalID: approvalID,
			RunID:      req.RunID,
			ToolCallID: toolCallID,
			Status:     domain.ApprovalStatusPending,
			CreatedAt:  now,
		}
		s.store.CreateApproval(ctx, approval)
		_, _ = s.store.UpdateToolCallApproval(ctx, toolCallID, approvalID, domain.ToolCallStatusWaitingApproval)

		// Emit approval_required event
		payload := domain.ApprovalRequiredPayload{
			ApprovalID:  approvalID,
			ToolCallID:  toolCallID,
			ToolName:    toolName,
			ArgsSummary: "Approval required for " + toolName, // Simplification
			Args:        req.Args,
		}
		s.recordEvent(ctx, req.RunID, domain.EventTypeApprovalRequired, payload)

		// Push to ingress
		// We need to push the approval request to the client
		if s.ingressClient != nil {
			var argsObj interface{}
			json.Unmarshal(req.Args, &argsObj)
			s.ingressClient.PushEvent(session.SessionID, map[string]interface{}{
				"type":         "approval_required",
				"ts":           now.UnixMilli(),
				"run_id":       req.RunID,
				"approval_id":  approvalID,
				"tool_call_id": toolCallID,
				"tool_name":    toolName,
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
	_ = s.store.CreateToolCall(ctx, toolCall)

	// Execute Logic
	if tool.Kind == domain.ToolKindClient {
		// Emit tool_request event
		payload := domain.ToolRequestPayload{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Args:       req.Args,
			DeadlineTs: now.Add(time.Duration(timeoutMs) * time.Millisecond).UnixMilli(),
		}
		s.recordEvent(ctx, req.RunID, domain.EventTypeToolRequest, payload)

		// Push to ingress
		if s.ingressClient != nil {
			var argsObj interface{}
			json.Unmarshal(req.Args, &argsObj)
			s.ingressClient.PushEvent(session.SessionID, map[string]interface{}{
				"type":         "tool_request",
				"ts":           now.UnixMilli(),
				"run_id":       req.RunID,
				"tool_call_id": toolCallID,
				"tool_name":    toolName,
				"args":         argsObj,
				"deadline_ts":  now.Add(time.Duration(timeoutMs) * time.Millisecond).UnixMilli(),
			})
		}

		return &domain.ToolInvokeResponse{
			Status:     "pending",
			ToolCallID: toolCallID,
			Reason:     "waiting_client",
		}, nil
	}

	// Server Tool Execution (Async)
	go s.executeServerToolAsync(toolCall, tool)

	return &domain.ToolInvokeResponse{
		Status:     "pending",
		ToolCallID: toolCallID,
		Reason:     "server_tool_executing",
	}, nil
}

// executeServerToolAsync executes a server tool asynchronously.
func (s *Service) executeServerToolAsync(toolCall *domain.ToolCall, tool *domain.Tool) {
	timeoutMs := toolCall.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = tool.TimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = int(s.config.ToolTimeout.Milliseconds())
	}
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Update status to RUNNING
	_, _ = s.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, domain.ToolCallStatusRunning)

	// Execute tool logic (mock implementation)
	type execResult struct {
		result json.RawMessage
		err    error
	}
	resultCh := make(chan execResult, 1)
	go func() {
		res, err := s.executeServerTool(ctx, tool.Name, toolCall.Args)
		resultCh <- execResult{result: res, err: err}
	}()

	select {
	case <-ctx.Done():
		errData, _ := json.Marshal(map[string]interface{}{
			"code":       "timeout",
			"message":    "tool execution timeout",
			"timeout_ms": timeoutMs,
		})
		updated, err := s.store.UpdateToolCallResult(context.Background(), toolCall.ToolCallID, domain.ToolCallStatusTimeout, nil, errData)
		if err == nil && updated {
			payload := domain.ToolResultPayload{
				ToolCallID: toolCall.ToolCallID,
				Status:     domain.ToolCallStatusTimeout,
				Error:      errData,
			}
			s.recordEvent(context.Background(), toolCall.RunID, domain.EventTypeToolResult, payload)
		}
		return
	case out := <-resultCh:
		result, err := out.result, out.err
		// Update result
		if err != nil {
			errData, _ := json.Marshal(map[string]string{
				"code":    "execution_error",
				"message": err.Error(),
			})
			updated, updErr := s.store.UpdateToolCallResult(context.Background(), toolCall.ToolCallID, domain.ToolCallStatusFailed, nil, errData)
			if updErr != nil || !updated {
				return
			}

			// Emit result event
			payload := domain.ToolResultPayload{
				ToolCallID: toolCall.ToolCallID,
				Status:     domain.ToolCallStatusFailed,
				Error:      errData,
			}
			s.recordEvent(context.Background(), toolCall.RunID, domain.EventTypeToolResult, payload)
		} else {
			updated, updErr := s.store.UpdateToolCallResult(context.Background(), toolCall.ToolCallID, domain.ToolCallStatusSucceeded, result, nil)
			if updErr != nil || !updated {
				return
			}

			// Emit result event
			payload := domain.ToolResultPayload{
				ToolCallID: toolCall.ToolCallID,
				Status:     domain.ToolCallStatusSucceeded,
				Result:     result,
			}
			s.recordEvent(context.Background(), toolCall.RunID, domain.EventTypeToolResult, payload)
		}
	}
}

// executeServerTool executes a server-side tool and returns the result.
func (s *Service) executeServerTool(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
	// Mock implementation for different server tools
	switch toolName {
	case "weather.query":
		return json.RawMessage(`{"weather":"Sunny","temperature":25}`), nil
	case "payments.transfer":
		return json.RawMessage(`{"status":"completed","transaction_id":"tx_123"}`), nil
	default:
		return json.RawMessage(`{"status":"executed"}`), nil
	}
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
	updated, err := s.store.UpdateToolCallResult(ctx, toolCallID, newStatus, req.Result, req.Error)
	if err != nil {
		return nil, fmt.Errorf("failed to update tool call: %w", err)
	}
	if !updated {
		tc, err := s.store.GetToolCall(ctx, toolCallID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool call: %w", err)
		}
		if tc == nil {
			return nil, fmt.Errorf("tool call not found")
		}
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

	// Record event
	now := time.Now()
	payload := domain.ToolResultPayload{
		ToolCallID: toolCallID,
		Status:     newStatus,
		Result:     req.Result,
		Error:      req.Error,
	}
	s.recordEvent(ctx, tc.RunID, domain.EventTypeToolResult, payload)

	return &domain.ToolCallResultResponse{
		ToolCallID:  toolCallID,
		Status:      newStatus,
		Result:      req.Result,
		Error:       req.Error,
		CompletedAt: now.UnixMilli(),
	}, nil
}

// ListTools returns all registered tools.
func (s *Service) ListTools(ctx context.Context) ([]domain.Tool, error) {
	return s.store.ListTools(ctx)
}

// RegisterTools registers tools from a client.
func (s *Service) RegisterTools(ctx context.Context, req domain.ToolRegistrationRequest) (*domain.ToolRegistrationResponse, error) {
	registeredCount := 0

	for _, t := range req.Tools {
		tool := &domain.Tool{
			Name:      t.Name,
			Kind:      domain.ToolKindClient,
			Schema:    t.Schema,
			ClientID:  req.ClientID,
			TimeoutMs: t.TimeoutMs,
		}
		// Default timeout if not specified
		if tool.TimeoutMs == 0 {
			tool.TimeoutMs = 60000
		}

		if err := s.store.UpsertTool(ctx, tool); err != nil {
			return nil, fmt.Errorf("failed to register tool %s: %w", t.Name, err)
		}
		registeredCount++
	}

	return &domain.ToolRegistrationResponse{
		OK:              true,
		RegisteredCount: registeredCount,
	}, nil
}
