// Package api provides HTTP handlers for the orchestrator.
package api

import (
	"net/http"

	"github.com/xiaot623/gogo/orchestrator/agentclient"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/store"
	"github.com/labstack/echo/v4"
)

// Handler handles HTTP requests.
type Handler struct {
	store       store.Store
	agentClient *agentclient.Client
	config      *config.Config
}

// NewHandler creates a new handler.
func NewHandler(store store.Store, agentClient *agentclient.Client, config *config.Config) *Handler {
	return &Handler{
		store:       store,
		agentClient: agentClient,
		config:      config,
	}
}

// RegisterRoutes registers routes with the echo server.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Internal API (for ingress)
	e.POST("/internal/invoke", h.InternalInvoke)

	// Public API
	e.GET("/v1/runs/:run_id/events", h.GetRunEvents)
	e.GET("/v1/sessions/:session_id/messages", h.GetSessionMessages)

	// Agent registry API
	e.POST("/v1/agents/register", h.RegisterAgent)
	e.GET("/v1/agents", h.ListAgents)
	e.GET("/v1/agents/:agent_id", h.GetAgent)

	e.GET("/health", h.Health)
}

// Health returns health status.
func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "0.1.0",
	})
}
