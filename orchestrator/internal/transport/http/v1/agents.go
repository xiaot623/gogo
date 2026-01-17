package v1

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

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

	agent, err := h.service.RegisterAgent(ctx, req.AgentID, req.Name, req.Endpoint, req.Capabilities)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok":            true,
		"registered_at": agent.CreatedAt.UnixMilli(),
	})
}

// ListAgents lists all registered agents.
// GET /v1/agents
func (h *Handler) ListAgents(c echo.Context) error {
	ctx := c.Request().Context()

	agents, err := h.service.ListAgents(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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

	agent, err := h.service.GetAgent(ctx, agentID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if agent == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}

	return c.JSON(http.StatusOK, agent)
}