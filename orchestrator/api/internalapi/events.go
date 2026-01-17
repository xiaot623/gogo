package internalapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// StreamRunEvents streams events for a specific run via SSE.
// GET /internal/runs/:run_id/events/stream
//
// This endpoint continuously streams events for a run until it reaches a terminal state.
// Ingress uses this to get real-time events to push to connected clients.
func (h *Handler) StreamRunEvents(c echo.Context) error {
	ctx := c.Request().Context()
	runID := c.Param("run_id")

	// Validate run exists
	run, err := h.store.GetRun(ctx, runID)
	if err != nil {
		log.Printf("ERROR: failed to get run: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
	}
	if run == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	// Flush headers
	if flusher, ok := c.Response().Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	// Start streaming events
	lastTs := int64(0)
	pollInterval := 100 * time.Millisecond
	maxDuration := 5 * time.Minute // Maximum streaming duration

	deadline := time.Now().Add(maxDuration)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return nil

		case <-ticker.C:
			// Check if exceeded max duration
			if time.Now().After(deadline) {
				log.Printf("INFO: event stream for run %s exceeded max duration", runID)
				return nil
			}

			// Poll for new events
			events, err := h.store.GetEvents(ctx, runID, lastTs, nil, 100)
			if err != nil {
				log.Printf("ERROR: failed to get events: %v", err)
				continue
			}

			// Send new events
			for _, event := range events {
				if event.Ts > lastTs {
					if err := h.sendSSEEvent(c, event); err != nil {
						log.Printf("ERROR: failed to send SSE event: %v", err)
						return err
					}
					lastTs = event.Ts
				}
			}

			// Check if run is in terminal state
			currentRun, err := h.store.GetRun(ctx, runID)
			if err != nil {
				log.Printf("ERROR: failed to get run status: %v", err)
				continue
			}

			if h.isTerminalState(currentRun.Status) {
				// Send a final marker event and close the stream
				log.Printf("INFO: run %s reached terminal state: %s", runID, currentRun.Status)
				return nil
			}
		}
	}
}

// sendSSEEvent sends a single event in SSE format.
func (h *Handler) sendSSEEvent(c echo.Context, event domain.Event) error {
	// Format: event: <event_type>\ndata: <json>\n\n
	data, err := json.Marshal(map[string]interface{}{
		"event_id": event.EventID,
		"run_id":   event.RunID,
		"ts":       event.Ts,
		"type":     event.Type,
		"payload":  json.RawMessage(event.Payload),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write SSE format
	if _, err := fmt.Fprintf(c.Response().Writer, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Response().Writer, "data: %s\n\n", data); err != nil {
		return err
	}

	// Flush immediately
	if flusher, ok := c.Response().Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// isTerminalState checks if a run status is terminal.
func (h *Handler) isTerminalState(status domain.RunStatus) bool {
	return status == domain.RunStatusDone ||
		status == domain.RunStatusFailed ||
		status == domain.RunStatusCancelled
}
