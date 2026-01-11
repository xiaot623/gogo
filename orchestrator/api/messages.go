package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

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
