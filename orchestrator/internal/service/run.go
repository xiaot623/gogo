package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/agentclient"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// InvokeAgent handles the agent invocation logic.
func (s *Service) InvokeAgent(ctx context.Context, req domain.InvokeRequest) (*domain.InvokeResponse, error) {
	// Validate required fields
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if req.InputMessage.Content == "" {
		return nil, fmt.Errorf("input_message.content is required")
	}

	// Get or create session
	userID := "default_user" // In M0, we use a default user
	if req.Context != nil {
		if uid, ok := req.Context["user_id"]; ok {
			userID = uid
		}
	}
	session, err := s.store.GetOrCreateSession(ctx, req.SessionID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create session: %w", err)
	}

	// Get agent endpoint
	agent, err := s.store.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent %s not found", req.AgentID)
	}

	// Create run
	runID := "run_" + uuid.New().String()[:8]
	now := time.Now()
	run := &domain.Run{
		RunID:       runID,
		SessionID:   session.SessionID,
		RootAgentID: req.AgentID,
		Status:      domain.RunStatusCreated,
		StartedAt:   now,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// Save user input message
	msgID := "msg_" + uuid.New().String()[:8]
	userMsg := &domain.Message{
		MessageID: msgID,
		SessionID: session.SessionID,
		RunID:     runID,
		Role:      "user",
		Content:   req.InputMessage.Content,
		CreatedAt: now,
	}
	if err := s.store.CreateMessage(ctx, userMsg); err != nil {
		log.Printf("ERROR: failed to save user message: %v", err)
		// Continue anyway - message storage failure shouldn't block the run
	}

	// Record run_started event
	if err := s.recordEvent(ctx, runID, domain.EventTypeRunStarted, domain.RunStartedPayload{
		RequestID: req.RequestID,
		SessionID: session.SessionID,
		AgentID:   req.AgentID,
	}); err != nil {
		log.Printf("ERROR: failed to record run_started event: %v", err)
	}

	// Record user_input event
	if err := s.recordEvent(ctx, runID, domain.EventTypeUserInput, domain.UserInputPayload{
		MessageID: msgID,
		Content:   req.InputMessage.Content,
	}); err != nil {
		log.Printf("ERROR: failed to record user_input event: %v", err)
	}

	// Update run status to RUNNING
	if err := s.store.UpdateRunStatus(ctx, runID, domain.RunStatusRunning); err != nil {
		log.Printf("ERROR: failed to update run status: %v", err)
	}

	// Get conversation history
	messages, err := s.store.GetMessages(ctx, session.SessionID, 50, "")
	if err != nil {
		log.Printf("WARN: failed to get messages: %v", err)
		messages = []domain.Message{}
	}

	// Prepare agent invoke request
	agentReq := &domain.AgentInvokeRequest{
		AgentID:      req.AgentID,
		SessionID:    session.SessionID,
		RunID:        runID,
		InputMessage: req.InputMessage,
		Messages:     messages,
		Context:      req.Context,
	}

	// Record agent_invoke_started event
	if err := s.recordEvent(ctx, runID, domain.EventTypeAgentInvokeStarted, map[string]interface{}{
		"agent_id": req.AgentID,
		"endpoint": agent.Endpoint,
	}); err != nil {
		log.Printf("ERROR: failed to record agent_invoke_started event: %v", err)
	}

	// Trigger async processing
	go s.processAgentStream(runID, session.SessionID, agent.Endpoint, agentReq)

	return &domain.InvokeResponse{
		RunID:     runID,
		SessionID: session.SessionID,
		AgentID:   req.AgentID,
	}, nil
}

func (s *Service) processAgentStream(runID, sessionID, endpoint string, req *domain.AgentInvokeRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.AgentTimeout)
	defer cancel()

	var finalMessage string
	var usage *domain.UsageData

	err := s.agentClient.Invoke(ctx, endpoint, req, func(event agentclient.SSEEvent) error {
		nowMs := time.Now().UnixMilli()

		switch event.Event {
		case "delta":
			delta, err := agentclient.ParseDeltaEvent(event.Data)
			if err != nil {
				log.Printf("WARN: failed to parse delta event: %v", err)
				return nil
			}

			// Record event
			if err := s.recordEvent(ctx, runID, domain.EventTypeAgentStreamDelta, domain.AgentStreamDeltaPayload{
				Text: delta.Text,
			}); err != nil {
				log.Printf("ERROR: failed to record delta event: %v", err)
			}

			// Push to ingress
			if s.ingressClient != nil {
				s.ingressClient.PushEvent(sessionID, map[string]interface{}{
					"type":   "delta",
					"ts":     nowMs,
					"run_id": runID,
					"text":   delta.Text,
				})
			}

		case "done":
			done, err := agentclient.ParseDoneEvent(event.Data)
			if err != nil {
				log.Printf("WARN: failed to parse done event: %v", err)
				return nil
			}
			finalMessage = done.FinalMessage
			usage = done.Usage

		case "error":
			errEvt, err := agentclient.ParseErrorEvent(event.Data)
			if err != nil {
				log.Printf("WARN: failed to parse error event: %v", err)
				return nil
			}

			// Record run_failed event
			if err := s.recordEvent(ctx, runID, domain.EventTypeRunFailed, domain.RunFailedPayload{
				Code:    errEvt.Code,
				Message: errEvt.Message,
			}); err != nil {
				log.Printf("ERROR: failed to record run_failed event: %v", err)
			}

			// Update run status
			errData, _ := json.Marshal(errEvt)
			if err := s.store.UpdateRunCompleted(ctx, runID, domain.RunStatusFailed, errData); err != nil {
				log.Printf("ERROR: failed to update run status: %v", err)
			}

			// Push error to ingress
			if s.ingressClient != nil {
				s.ingressClient.PushEvent(sessionID, map[string]interface{}{
					"type":    "error",
					"ts":      nowMs,
					"run_id":  runID,
					"code":    errEvt.Code,
					"message": errEvt.Message,
				})
			}

			return fmt.Errorf("agent error: %s", errEvt.Message)

		case "state":
			// Record state change
			log.Printf("INFO: state event: %s", event.Data)
		}

		return nil
	})

	nowMs := time.Now().UnixMilli()

	if err != nil {
		log.Printf("ERROR: agent invocation failed: %v", err)

		// Record run_failed if not already done
		if err := s.recordEvent(ctx, runID, domain.EventTypeRunFailed, domain.RunFailedPayload{
			Code:    "agent_error",
			Message: err.Error(),
		}); err != nil {
			log.Printf("ERROR: failed to record run_failed event: %v", err)
		}

		errData, _ := json.Marshal(map[string]string{"code": "agent_error", "message": err.Error()})
		if err := s.store.UpdateRunCompleted(ctx, runID, domain.RunStatusFailed, errData); err != nil {
			log.Printf("ERROR: failed to update run status: %v", err)
		}

		if s.ingressClient != nil {
			s.ingressClient.PushEvent(sessionID, map[string]interface{}{
				"type":    "error",
				"ts":      nowMs,
				"run_id":  runID,
				"code":    "agent_error",
				"message": err.Error(),
			})
		}
		return
	}

	// Record agent_invoke_done event
	if err := s.recordEvent(ctx, runID, domain.EventTypeAgentInvokeDone, map[string]interface{}{
		"final_message": finalMessage,
		"usage":         usage,
	}); err != nil {
		log.Printf("ERROR: failed to record agent_invoke_done event: %v", err)
	}

	// Save assistant message
	if finalMessage != "" {
		assistantMsg := &domain.Message{
			MessageID: "msg_" + uuid.New().String()[:8],
			SessionID: sessionID,
			RunID:     runID,
			Role:      "assistant",
			Content:   finalMessage,
			CreatedAt: time.Now(),
		}
		if err := s.store.CreateMessage(ctx, assistantMsg); err != nil {
			log.Printf("ERROR: failed to save assistant message: %v", err)
		}
	}

	// Record run_done event
	if err := s.recordEvent(ctx, runID, domain.EventTypeRunDone, domain.RunDonePayload{
		Usage:        usage,
		FinalMessage: finalMessage,
	}); err != nil {
		log.Printf("ERROR: failed to record run_done event: %v", err)
	}

	// Update run status
	if err := s.store.UpdateRunCompleted(ctx, runID, domain.RunStatusDone, nil); err != nil {
		log.Printf("ERROR: failed to update run status: %v", err)
	}

	// Push done to ingress
	doneEvent := map[string]interface{}{
		"type":   "done",
		"ts":     nowMs,
		"run_id": runID,
	}
	if usage != nil {
		doneEvent["usage"] = usage
	}
	if s.ingressClient != nil {
		s.ingressClient.PushEvent(sessionID, doneEvent)
	}
}

func isTerminalRunStatus(status domain.RunStatus) bool {
	switch status {
	case domain.RunStatusDone, domain.RunStatusFailed, domain.RunStatusCancelled:
		return true
	}
	return false
}

func (s *Service) CancelRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found")
	}

	if isTerminalRunStatus(run.Status) {
		return nil // Already terminal
	}

	if err := s.store.UpdateRunCompleted(ctx, runID, domain.RunStatusCancelled, nil); err != nil {
		return fmt.Errorf("failed to cancel run: %w", err)
	}

	s.recordEvent(ctx, runID, domain.EventTypeRunCancelled, map[string]interface{}{
		"reason": "cancelled by user",
	})

	return nil
}

func (s *Service) GetRun(ctx context.Context, runID string) (*domain.Run, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}
	return run, nil
}
