// Package internalapi provides HTTP handlers for internal orchestrator APIs.
// These APIs are only accessible to the ingress service.
package internalapi

import (
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
)

// Handler handles internal HTTP requests from ingress.
type Handler struct {
	service *service.Service
}

// NewHandler creates a new internal API handler.
func NewHandler(service *service.Service) *Handler {
	return &Handler{
		service: service,
	}
}

// RegisterRoutes registers internal routes with the echo server.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Agent invocation
	e.POST("/internal/invoke", h.Invoke)

	// Event streaming
	e.GET("/internal/runs/:run_id/events/stream", h.StreamRunEvents)

	// Tool calls
	e.POST("/internal/tool_calls/:tool_call_id/submit", h.SubmitToolResult)

	// Approvals
	e.POST("/internal/approvals/:approval_id/submit", h.SubmitApprovalDecision)

	// Run management
	e.POST("/internal/runs/:run_id/cancel", h.CancelRun)
}