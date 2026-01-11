package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// the handler for client to pull output

// GetSessionMessages returns messages for a session.
// GET /v1/sessions/:session_id/messages
func (h *Handler) GetSessionMessages(c echo.Context) error {
	ctx := c.Request().Context()
	sessionID := c.Param("session_id")

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}
	before := c.QueryParam("before")

	// Get messages
	messages, err := h.store.GetMessages(ctx, sessionID, limit+1, before)
	if err != nil {
		log.Printf("ERROR: failed to get messages: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get messages"})
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"messages": messages,
		"has_more": hasMore,
	})
}

// GetRunEvents returns events for a run.
// GET /v1/runs/:run_id/events
func (h *Handler) GetRunEvents(c echo.Context) error {
	ctx := c.Request().Context()
	runID := c.Param("run_id")

	// Parse query params
	afterTs, _ := strconv.ParseInt(c.QueryParam("after_ts"), 10, 64)
	typesStr := c.QueryParam("types")
	var types []string
	if typesStr != "" {
		types = strings.Split(typesStr, ",")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}

	// Check run exists
	run, err := h.store.GetRun(ctx, runID)
	if err != nil {
		log.Printf("ERROR: failed to get run: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
	}
	if run == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	// Get events
	events, err := h.store.GetEvents(ctx, runID, afterTs, types, limit+1)
	if err != nil {
		log.Printf("ERROR: failed to get events: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get events"})
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		nextCursor = events[len(events)-1].EventID
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"events":      events,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
	})
}
