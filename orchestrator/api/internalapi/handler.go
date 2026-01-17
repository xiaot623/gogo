// Package internal provides HTTP handlers for internal orchestrator APIs.
// These APIs are only accessible to the ingress service.
package internalapi

import (
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/store"
)

// Handler handles internal HTTP requests from ingress.
type Handler struct {
	store        store.Store
	agentClient  *agentclient.Client
	config       *config.Config
	policyEngine *policy.Engine
}

// NewHandler creates a new internal API handler.
func NewHandler(store store.Store, agentClient *agentclient.Client, config *config.Config, policyEngine *policy.Engine) *Handler {
	return &Handler{
		store:        store,
		agentClient:  agentClient,
		config:       config,
		policyEngine: policyEngine,
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
