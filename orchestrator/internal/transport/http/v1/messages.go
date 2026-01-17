package v1

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// GetSessionMessages retrieves messages for a session.
// GET /v1/sessions/:session_id/messages
func (h *Handler) GetSessionMessages(c echo.Context) error {
	sessionID := c.Param("session_id")
	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}
	before := c.QueryParam("before")

	ctx := c.Request().Context()
	
	messages, err := h.service.GetMessages(ctx, sessionID, limit, before)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"messages": messages,
		"has_more": len(messages) == limit, // Approximate
	})
}

// GetRunEvents retrieves events for a run.
// GET /v1/runs/:run_id/events
func (h *Handler) GetRunEvents(c echo.Context) error {
	runID := c.Param("run_id")
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}
	afterTs := int64(0)
	if t := c.QueryParam("after_ts"); t != "" {
		if val, err := strconv.ParseInt(t, 10, 64); err == nil {
			afterTs = val
		}
	}
	// types currently ignored in service for simplicity or assume implementation supports it
	var types []string // Parse from query param if needed

	ctx := c.Request().Context()
	
	events, err := h.service.GetRunEvents(ctx, runID, afterTs, types, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	
	// Need to check if more events available, but store API doesn't return that info directly usually unless limit+1 fetched.
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"events": events,
	})
}