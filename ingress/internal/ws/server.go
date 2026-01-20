// Package ws provides WebSocket server functionality for client connections.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/xiaot623/gogo/ingress/internal/config"
	"github.com/xiaot623/gogo/ingress/internal/hub"
	"github.com/xiaot623/gogo/ingress/internal/orchestrator"
	"github.com/xiaot623/gogo/ingress/internal/protocol"
)

// Server handles WebSocket connections.
type Server struct {
	cfg          *config.Config
	hub          *hub.Hub
	orchestrator *orchestrator.Client
	upgrader     websocket.Upgrader
}

// NewServer creates a new WebSocket server.
func NewServer(cfg *config.Config, h *hub.Hub, orch *orchestrator.Client) *Server {
	return &Server{
		cfg:          cfg,
		hub:          h,
		orchestrator: orch,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for MVP
				return true
			},
		},
	}
}

// HandleWebSocket handles WebSocket upgrade and connection lifecycle.
func (s *Server) HandleWebSocket(c echo.Context) error {
	ws, err := s.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return err
	}

	// Create and register connection
	conn := s.hub.NewConnection(ws)
	s.hub.Register(conn)

	// Set up connection parameters
	ws.SetReadLimit(s.cfg.MaxMessageSize)

	// Start reader and writer goroutines
	go s.writePump(conn)
	go s.readPump(conn)

	return nil
}

// readPump reads messages from the WebSocket connection.
func (s *Server) readPump(conn *hub.Connection) {
	defer func() {
		s.hub.Unregister(conn)
		conn.Close()
	}()

	conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
	conn.Conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
		return nil
	})

	for {
		_, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		s.handleMessage(conn, message)
	}
}

// writePump writes messages to the WebSocket connection.
func (s *Server) writePump(conn *hub.Connection) {
	ticker := time.NewTicker(s.cfg.PingInterval)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-conn.Send:
			conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
			if !ok {
				// Hub closed the channel
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Failed to write message: %v", err)
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage dispatches incoming messages to appropriate handlers.
func (s *Server) handleMessage(conn *hub.Connection, data []byte) {
	// Parse message type
	var baseMsg protocol.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid JSON message")
		return
	}

	switch baseMsg.Type {
	case protocol.TypeHello:
		s.handleHello(conn, data)
	case protocol.TypeAgentInvoke:
		s.handleAgentInvoke(conn, data)
	case protocol.TypeToolResult:
		s.handleToolResult(conn, data)
	case protocol.TypeApprovalDecision:
		s.handleApprovalDecision(conn, data)
	case protocol.TypeCancelRun:
		s.handleCancelRun(conn, data)
	default:
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "unknown message type: "+baseMsg.Type)
	}
}

// handleHello handles the hello handshake message.
func (s *Server) handleHello(conn *hub.Connection, data []byte) {
	var msg protocol.HelloMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid hello message")
		return
	}

	// Validate API key if configured
	if s.cfg.APIKey != "" && msg.APIKey != s.cfg.APIKey {
		s.sendError(conn, "", protocol.ErrorCodeUnauthorized, "invalid api_key")
		return
	}

	// Generate or use provided session ID
	sessionID := msg.SessionID
	if sessionID == "" {
		sessionID = "sess_" + uuid.New().String()[:8]
	}

	// Bind connection to session
	s.hub.BindSession(conn, sessionID)

	// Send hello_ack
	ack := protocol.HelloAckMessage{
		BaseMessage: protocol.BaseMessage{
			Type:      protocol.TypeHelloAck,
			Ts:        time.Now().UnixMilli(),
			SessionID: sessionID,
		},
	}
	s.hub.SendJSONToConnection(conn, ack)

	log.Printf("Hello handshake completed for session: %s", sessionID)
}

// handleAgentInvoke handles agent invocation requests.
func (s *Server) handleAgentInvoke(conn *hub.Connection, data []byte) {
	var msg protocol.AgentInvokeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid agent_invoke message")
		return
	}

	// Require session binding
	if conn.SessionID == "" {
		s.sendError(conn, "", protocol.ErrorCodeSessionRequired, "must send hello first")
		return
	}

	// Use session from connection or message
	sessionID := conn.SessionID
	if msg.SessionID != "" {
		sessionID = msg.SessionID
	}

	// Prepare orchestrator request
	req := &orchestrator.InvokeRequest{
		SessionID: sessionID,
		AgentID:   msg.AgentID,
		InputMessage: orchestrator.InputMessage{
			Role:    msg.Message.Role,
			Content: msg.Message.Content,
		},
		RequestID: msg.RequestID,
	}

	// Call orchestrator (async - don't block the WebSocket)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := s.orchestrator.Invoke(ctx, req)
		if err != nil {
			log.Printf("Orchestrator invoke failed: %v", err)
			s.sendErrorToSession(sessionID, msg.RequestID, protocol.ErrorCodeOrchestratorFail, err.Error())
			return
		}

		log.Printf("Agent invoked successfully: run_id=%s", resp.RunID)
		// Note: run_started and subsequent events will come via ingress RPC fanout.
	}()
}

// handleToolResult handles tool result submissions.
func (s *Server) handleToolResult(conn *hub.Connection, data []byte) {
	var msg protocol.ToolResultMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid tool_result message")
		return
	}

	if conn.SessionID == "" {
		s.sendError(conn, msg.RunID, protocol.ErrorCodeSessionRequired, "must send hello first")
		return
	}

	// Map ok field to status
	status := "SUCCEEDED"
	if !msg.OK {
		status = "FAILED"
	}

	req := &orchestrator.ToolCallResultRequest{
		Status: status,
		Result: msg.Result,
		Error:  msg.Error,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := s.orchestrator.SubmitToolResult(ctx, msg.ToolCallID, req)
		if err != nil {
			log.Printf("Submit tool result failed: %v", err)
			s.sendErrorToSession(conn.SessionID, msg.RunID, protocol.ErrorCodeOrchestratorFail, err.Error())
			return
		}

		log.Printf("Tool result submitted: tool_call_id=%s", msg.ToolCallID)
	}()
}

// handleApprovalDecision handles approval decision submissions.
func (s *Server) handleApprovalDecision(conn *hub.Connection, data []byte) {
	var msg protocol.ApprovalDecisionMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid approval_decision message")
		return
	}

	if conn.SessionID == "" {
		s.sendError(conn, msg.RunID, protocol.ErrorCodeSessionRequired, "must send hello first")
		return
	}

	// Map decision to orchestrator format
	decision := strings.ToUpper(msg.Decision)
	if decision == "APPROVE" {
		decision = "APPROVED"
	} else if decision == "REJECT" {
		decision = "REJECTED"
	}

	req := &orchestrator.ApprovalDecisionRequest{
		Decision: decision,
		Reason:   msg.Reason,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := s.orchestrator.SubmitApprovalDecision(ctx, msg.ApprovalID, req)
		if err != nil {
			log.Printf("Submit approval decision failed: %v", err)
			s.sendErrorToSession(conn.SessionID, msg.RunID, protocol.ErrorCodeOrchestratorFail, err.Error())
			return
		}

		log.Printf("Approval decision submitted: approval_id=%s, decision=%s", msg.ApprovalID, decision)
	}()
}

// handleCancelRun handles run cancellation requests.
func (s *Server) handleCancelRun(conn *hub.Connection, data []byte) {
	var msg protocol.CancelRunMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "invalid cancel_run message")
		return
	}

	if conn.SessionID == "" {
		s.sendError(conn, msg.RunID, protocol.ErrorCodeSessionRequired, "must send hello first")
		return
	}

	if msg.RunID == "" {
		s.sendError(conn, "", protocol.ErrorCodeInvalidMessage, "run_id is required")
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := s.orchestrator.CancelRun(ctx, msg.RunID)
		if err != nil {
			log.Printf("Cancel run failed: %v", err)
			s.sendErrorToSession(conn.SessionID, msg.RunID, protocol.ErrorCodeOrchestratorFail, err.Error())
			return
		}

		log.Printf("Run cancelled: run_id=%s", msg.RunID)
	}()
}

// sendError sends an error message to a connection.
func (s *Server) sendError(conn *hub.Connection, runID, code, message string) {
	errMsg := protocol.ErrorMessage{
		BaseMessage: protocol.BaseMessage{
			Type:      protocol.TypeError,
			Ts:        time.Now().UnixMilli(),
			RunID:     runID,
			SessionID: conn.SessionID,
		},
		Code:    code,
		Message: message,
	}
	s.hub.SendJSONToConnection(conn, errMsg)
}

// sendErrorToSession sends an error message to all connections of a session.
func (s *Server) sendErrorToSession(sessionID, runID, code, message string) {
	errMsg := protocol.ErrorMessage{
		BaseMessage: protocol.BaseMessage{
			Type:      protocol.TypeError,
			Ts:        time.Now().UnixMilli(),
			RunID:     runID,
			SessionID: sessionID,
		},
		Code:    code,
		Message: message,
	}
	s.hub.BroadcastJSON(sessionID, errMsg)
}
