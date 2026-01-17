package v1

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// InvokeTool handles tool invocation.
func (h *Handler) InvokeTool(c echo.Context) error {
	toolName := c.Param("tool_name")
	var req domain.ToolInvokeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	ctx := c.Request().Context()
	
	resp, err := h.service.InvokeTool(ctx, toolName, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	
	return c.JSON(http.StatusOK, resp)
}

// GetToolCall retrieves the status of a tool call.
func (h *Handler) GetToolCall(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	ctx := c.Request().Context()

	tc, err := h.service.GetToolCall(ctx, toolCallID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if tc == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
	}

	resp := domain.ToolCallResponse{
		ToolCallID: tc.ToolCallID,
		Status:     tc.Status,
		Result:     tc.Result,
		Timestamps: domain.Timestamps{
			CreatedAt: tc.CreatedAt.UnixMilli(),
		},
	}
	if tc.CompletedAt != nil {
		resp.Timestamps.CompletedAt = tc.CompletedAt.UnixMilli()
	}

	return c.JSON(http.StatusOK, resp)
}

// WaitToolCall waits for a tool call to complete.
func (h *Handler) WaitToolCall(c echo.Context) error {
	toolCallID := c.Param("tool_call_id")
	timeoutMs := 60000 // default 60s
	
	// Parse timeout (if query param) - simpler for MVP
	if t := c.QueryParam("timeout_ms"); t != "" {
		if val, err := strconv.Atoi(t); err == nil {
			timeoutMs = val
		}
	}

	ctx := c.Request().Context()
	
	tc, err := h.service.WaitToolCall(ctx, toolCallID, timeoutMs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if tc == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
	}
	
	resp := domain.ToolCallResponse{
		ToolCallID: tc.ToolCallID,
		Status:     tc.Status,
		Result:     tc.Result,
		Timestamps: domain.Timestamps{
			CreatedAt: tc.CreatedAt.UnixMilli(),
		},
	}
	if tc.CompletedAt != nil {
		resp.Timestamps.CompletedAt = tc.CompletedAt.UnixMilli()
	}

	return c.JSON(http.StatusOK, resp)
}

// SubmitToolResult handles client tool result submission.
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
		if err.Error() == "tool call not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool call not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	
	return c.JSON(http.StatusOK, resp)
}