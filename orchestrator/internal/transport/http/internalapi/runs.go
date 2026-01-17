package internalapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// CancelRun cancels a running execution.
// POST /internal/runs/:run_id/cancel
func (h *Handler) CancelRun(c echo.Context) error {
	runID := c.Param("run_id")
	ctx := c.Request().Context()

	if err := h.service.CancelRun(ctx, runID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id": runID,
		"status": domain.RunStatusCancelled,
		"message": "run cancelled successfully",
	})
}