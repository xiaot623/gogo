package internalapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// SubmitToolResult handles client tool result submission from ingress (e.g. via WebSocket).
// POST /internal/tool_calls/:tool_call_id/submit
func (h *Handler) SubmitToolResult(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	var req domain.ToolCallResultRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate status
	if req.Status != "SUCCEEDED" && req.Status != "FAILED" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "status must be SUCCEEDED or FAILED"})
	}

	ctx := c.Request().Context()
	
	resp, err := h.service.SubmitToolResult(ctx, toolCallID, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}