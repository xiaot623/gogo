package internalapi

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/domain"
)

// CancelRun cancels a running execution.
// POST /internal/runs/:run_id/cancel
func (h *Handler) CancelRun(c echo.Context) error {
	runID := c.Param("run_id")
	ctx := c.Request().Context()

	// Get run
	run, err := h.store.GetRun(ctx, runID)
	if err != nil {
		log.Printf("ERROR: failed to get run: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
	}
	if run == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}

	// Check if already in terminal state (idempotency)
	if h.isTerminalState(run.Status) {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"run_id": runID,
			"status": run.Status,
			"message": "run already in terminal state",
		})
	}

	// Update run status to CANCELLED
	if err := h.store.UpdateRunCompleted(ctx, runID, domain.RunStatusCancelled, nil); err != nil {
		log.Printf("ERROR: failed to cancel run: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to cancel run"})
	}

	// Record run_cancelled event
	if err := h.recordEvent(ctx, runID, domain.EventTypeRunCancelled, map[string]interface{}{
		"reason": "cancelled by user",
	}); err != nil {
		log.Printf("ERROR: failed to record run_cancelled event: %v", err)
	}

	log.Printf("INFO: run %s cancelled", runID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id": runID,
		"status": domain.RunStatusCancelled,
		"message": "run cancelled successfully",
	})
}
