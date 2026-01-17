// Package http provides the internal HTTP server for ingress.
package http

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/xiaot623/gogo/ingress/internal/hub"
)

// Server is the internal HTTP server for ingress.
type Server struct {
	echo *echo.Echo
	hub  *hub.Hub
}

// NewServer creates a new internal HTTP server.
func NewServer(h *hub.Hub) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	s := &Server{
		echo: e,
		hub:  h,
	}

	// Register routes
	e.GET("/health", s.handleHealth)
	e.POST("/internal/send", s.handleInternalSend)

	return s
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":      "healthy",
		"connections": s.hub.GetConnectionCount(),
		"sessions":    s.hub.GetSessionCount(),
	})
}

// SendRequest represents the request body for POST /internal/send.
type SendRequest struct {
	SessionID string                 `json:"session_id"`
	Event     map[string]interface{} `json:"event"`
}

// SendResponse represents the response for POST /internal/send.
type SendResponse struct {
	OK        bool `json:"ok"`
	Delivered bool `json:"delivered"`
}

// handleInternalSend handles event forwarding from orchestrator to WebSocket clients.
func (s *Server) handleInternalSend(c echo.Context) error {
	var req SendRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.SessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "session_id is required"})
	}

	if req.Event == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "event is required"})
	}

	// Add timestamp if not present
	if _, ok := req.Event["ts"]; !ok {
		req.Event["ts"] = time.Now().UnixMilli()
	}

	// Check if there are active connections
	hasConnections := s.hub.HasActiveConnections(req.SessionID)

	// Broadcast event to session
	if err := s.hub.BroadcastJSON(req.SessionID, req.Event); err != nil {
		log.Printf("Failed to broadcast event: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to broadcast event"})
	}

	log.Printf("Event sent to session %s: type=%v, delivered=%v", req.SessionID, req.Event["type"], hasConnections)

	return c.JSON(http.StatusOK, SendResponse{
		OK:        true,
		Delivered: hasConnections,
	})
}
