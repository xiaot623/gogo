package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) RunToolCallTimeoutMonitor(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepToolCallTimeouts(ctx)
		}
	}
}

func (s *Service) sweepToolCallTimeouts(ctx context.Context) {
	sweepCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	expired, err := s.store.ListExpiredToolCalls(sweepCtx, 100)
	if err != nil {
		log.Printf("WARN: tool timeout sweep failed: %v", err)
		return
	}

	for _, tc := range expired {
		errData, _ := json.Marshal(map[string]interface{}{
			"code":       "timeout",
			"message":    "tool call timed out",
			"timeout_ms": tc.TimeoutMs,
		})

		updated, err := s.store.UpdateToolCallResult(sweepCtx, tc.ToolCallID, domain.ToolCallStatusTimeout, nil, errData)
		if err != nil {
			log.Printf("WARN: failed to mark tool call timeout %s: %v", tc.ToolCallID, err)
			continue
		}
		if !updated {
			continue
		}

		payload := domain.ToolResultPayload{
			ToolCallID: tc.ToolCallID,
			Status:     domain.ToolCallStatusTimeout,
			Error:      errData,
		}
		if err := s.recordEvent(sweepCtx, tc.RunID, domain.EventTypeToolResult, payload); err != nil {
			log.Printf("WARN: failed to record tool timeout event %s: %v", tc.ToolCallID, err)
		}

		if tc.ApprovalID != "" {
			_, _ = s.store.ExpireApprovalIfPending(sweepCtx, tc.ApprovalID, "tool_call_timeout")
		}
	}
}
