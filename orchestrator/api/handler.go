// Package api provides HTTP handlers for the orchestrator.
package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/policy"
	"github.com/xiaot623/gogo/orchestrator/store"
)

// Handler handles HTTP requests.
type Handler struct {
	store        store.Store
	agentClient  *agentclient.Client
	config       *config.Config
	policyEngine *policy.Engine
}

// NewHandler creates a new handler.
func NewHandler(store store.Store, agentClient *agentclient.Client, config *config.Config, policyEngine *policy.Engine) *Handler {
	return &Handler{
		store:        store,
		agentClient:  agentClient,
		config:       config,
		policyEngine: policyEngine,
	}
}

// RegisterRoutes registers external routes with the echo server.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Public API (for retrieving data)
	e.GET("/v1/runs/:run_id/events", h.GetRunEvents)
	e.GET("/v1/sessions/:session_id/messages", h.GetSessionMessages)

	// Agent registry API
	e.POST("/v1/agents/register", h.RegisterAgent)
	e.GET("/v1/agents", h.ListAgents)
	e.GET("/v1/agents/:agent_id", h.GetAgent)

	// Tool API
	e.POST("/v1/tools/:tool_name/invoke", h.InvokeTool)
	e.GET("/v1/tool_calls/:tool_call_id", h.GetToolCall)
	e.POST("/v1/tool_calls/:tool_call_id/wait", h.WaitToolCall)

	e.GET("/health", h.Health)
}

// Health returns health status.
func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "0.1.0",
	})
}
