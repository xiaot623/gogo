package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// the handler about agent, including manage, invoke

// AgentRegisterRequest is the request to register an agent.
type AgentRegisterRequest struct {
	AgentID      string   `json:"agent_id"`
	Name         string   `json:"name"`
	Endpoint     string   `json:"endpoint"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// RegisterAgent registers a new agent.
// POST /v1/agents/register
func (h *Handler) RegisterAgent(c echo.Context) error {
	ctx := c.Request().Context()

	var req AgentRegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.AgentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}
	if req.Endpoint == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
	}

	caps, _ := json.Marshal(req.Capabilities)
	now := time.Now()
	agent := &domain.Agent{
		AgentID:      req.AgentID,
		Name:         req.Name,
		Endpoint:     req.Endpoint,
		Capabilities: caps,
		Status:       "healthy",
		CreatedAt:    now,
	}

	if err := h.store.RegisterAgent(ctx, agent); err != nil {
		log.Printf("ERROR: failed to register agent: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to register agent"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok":            true,
		"registered_at": now.UnixMilli(),
	})
}

// ListAgents lists all registered agents.
// GET /v1/agents
func (h *Handler) ListAgents(c echo.Context) error {
	ctx := c.Request().Context()

	agents, err := h.store.ListAgents(ctx)
	if err != nil {
		log.Printf("ERROR: failed to list agents: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
	}

	// Convert to response format
	agentList := make([]map[string]interface{}, len(agents))
	for i, a := range agents {
		agentList[i] = map[string]interface{}{
			"agent_id":          a.AgentID,
			"name":              a.Name,
			"status":            a.Status,
			"last_heartbeat_at": nil,
		}
		if a.LastHeartbeat != nil {
			agentList[i]["last_heartbeat_at"] = a.LastHeartbeat.UnixMilli()
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"agents": agentList,
	})
}

// GetAgent gets a specific agent by ID.
// GET /v1/agents/:agent_id
func (h *Handler) GetAgent(c echo.Context) error {
	ctx := c.Request().Context()
	agentID := c.Param("agent_id")

	agent, err := h.store.GetAgent(ctx, agentID)
	if err != nil {
		log.Printf("ERROR: failed to get agent: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
	}
	if agent == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}

	return c.JSON(http.StatusOK, agent)
}

// InternalInvoke handles agent invocation from ingress.
// POST /internal/invoke
func (h *Handler) InternalInvoke(c echo.Context) error {
	ctx := c.Request().Context()

	var req domain.InvokeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate required fields
	if req.SessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "session_id is required"})
	}
	if req.AgentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
	}
	if req.InputMessage.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "input_message.content is required"})
	}

	// Get or create session
	userID := "default_user" // In M0, we use a default user
	if req.Context != nil {
		if uid, ok := req.Context["user_id"]; ok {
			userID = uid
		}
	}
	session, err := h.store.GetOrCreateSession(ctx, req.SessionID, userID)
	if err != nil {
		log.Printf("ERROR: failed to get/create session: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}

	// Get agent endpoint (for M0, we use a static config or look up from DB)
	agent, err := h.store.GetAgent(ctx, req.AgentID)
	if err != nil {
		log.Printf("ERROR: failed to get agent: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
	}
	if agent == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": fmt.Sprintf("agent %s not found", req.AgentID)})
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
	if err := h.store.CreateRun(ctx, run); err != nil {
		log.Printf("ERROR: failed to create run: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create run"})
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
	if err := h.store.CreateMessage(ctx, userMsg); err != nil {
		log.Printf("ERROR: failed to save user message: %v", err)
		// Continue anyway - message storage failure shouldn't block the run
	}

	// Record run_started event
	if err := h.recordEvent(ctx, runID, domain.EventTypeRunStarted, domain.RunStartedPayload{
		RequestID: req.RequestID,
		SessionID: session.SessionID,
		AgentID:   req.AgentID,
	}); err != nil {
		log.Printf("ERROR: failed to record run_started event: %v", err)
	}

	// Record user_input event
	if err := h.recordEvent(ctx, runID, domain.EventTypeUserInput, domain.UserInputPayload{
		MessageID: msgID,
		Content:   req.InputMessage.Content,
	}); err != nil {
		log.Printf("ERROR: failed to record user_input event: %v", err)
	}

	// Update run status to RUNNING
	if err := h.store.UpdateRunStatus(ctx, runID, domain.RunStatusRunning); err != nil {
		log.Printf("ERROR: failed to update run status: %v", err)
	}

	// Get conversation history
	messages, err := h.store.GetMessages(ctx, session.SessionID, 50, "")
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
	if err := h.recordEvent(ctx, runID, domain.EventTypeAgentInvokeStarted, map[string]interface{}{
		"agent_id": req.AgentID,
		"endpoint": agent.Endpoint,
	}); err != nil {
		log.Printf("ERROR: failed to record agent_invoke_started event: %v", err)
	}

	// Send initial response to indicate run started
	// Then invoke agent and stream events
	go h.invokeAgentAndStream(runID, session.SessionID, agent.Endpoint, agentReq)

	// Return immediately with run info
	return c.JSON(http.StatusOK, domain.InvokeResponse{
		RunID:     runID,
		SessionID: session.SessionID,
		AgentID:   req.AgentID,
	})
}

// invokeAgentAndStream invokes the agent and streams events to ingress.
func (h *Handler) invokeAgentAndStream(runID, sessionID, endpoint string, req *domain.AgentInvokeRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.AgentTimeout)
	defer cancel()

	var finalMessage string
	var usage *domain.UsageData

	err := h.agentClient.Invoke(ctx, endpoint, req, func(event agentclient.SSEEvent) error {
		nowMs := time.Now().UnixMilli()

		switch event.Event {
		case "delta":
			delta, err := agentclient.ParseDeltaEvent(event.Data)
			if err != nil {
				log.Printf("WARN: failed to parse delta event: %v", err)
				return nil
			}

			// Record event
			if err := h.recordEvent(ctx, runID, domain.EventTypeAgentStreamDelta, domain.AgentStreamDeltaPayload{
				Text: delta.Text,
			}); err != nil {
				log.Printf("ERROR: failed to record delta event: %v", err)
			}

			// Push to ingress
			h.pushEventToIngress(sessionID, map[string]interface{}{
				"type":   "delta",
				"ts":     nowMs,
				"run_id": runID,
				"text":   delta.Text,
			})

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
			if err := h.recordEvent(ctx, runID, domain.EventTypeRunFailed, domain.RunFailedPayload{
				Code:    errEvt.Code,
				Message: errEvt.Message,
			}); err != nil {
				log.Printf("ERROR: failed to record run_failed event: %v", err)
			}

			// Update run status
			errData, _ := json.Marshal(errEvt)
			if err := h.store.UpdateRunCompleted(ctx, runID, domain.RunStatusFailed, errData); err != nil {
				log.Printf("ERROR: failed to update run status: %v", err)
			}

			// Push error to ingress
			h.pushEventToIngress(sessionID, map[string]interface{}{
				"type":    "error",
				"ts":      nowMs,
				"run_id":  runID,
				"code":    errEvt.Code,
				"message": errEvt.Message,
			})

			return fmt.Errorf("agent error: %s", errEvt.Message)

		case "state":
			// Record state change (for future use)
			log.Printf("INFO: state event: %s", event.Data)
		}

		return nil
	})

	nowMs := time.Now().UnixMilli()

	if err != nil {
		log.Printf("ERROR: agent invocation failed: %v", err)

		// Record run_failed if not already done
		if err := h.recordEvent(ctx, runID, domain.EventTypeRunFailed, domain.RunFailedPayload{
			Code:    "agent_error",
			Message: err.Error(),
		}); err != nil {
			log.Printf("ERROR: failed to record run_failed event: %v", err)
		}

		errData, _ := json.Marshal(map[string]string{"code": "agent_error", "message": err.Error()})
		if err := h.store.UpdateRunCompleted(ctx, runID, domain.RunStatusFailed, errData); err != nil {
			log.Printf("ERROR: failed to update run status: %v", err)
		}

		h.pushEventToIngress(sessionID, map[string]interface{}{
			"type":    "error",
			"ts":      nowMs,
			"run_id":  runID,
			"code":    "agent_error",
			"message": err.Error(),
		})
		return
	}

	// Record agent_invoke_done event
	if err := h.recordEvent(ctx, runID, domain.EventTypeAgentInvokeDone, map[string]interface{}{
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
		if err := h.store.CreateMessage(ctx, assistantMsg); err != nil {
			log.Printf("ERROR: failed to save assistant message: %v", err)
		}
	}

	// Record run_done event
	if err := h.recordEvent(ctx, runID, domain.EventTypeRunDone, domain.RunDonePayload{
		Usage:        usage,
		FinalMessage: finalMessage,
	}); err != nil {
		log.Printf("ERROR: failed to record run_done event: %v", err)
	}

	// Update run status
	if err := h.store.UpdateRunCompleted(ctx, runID, domain.RunStatusDone, nil); err != nil {
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
	h.pushEventToIngress(sessionID, doneEvent)
}
