package internalapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// SubmitApprovalDecision handles approval decision submission from ingress.
// POST /internal/approvals/:approval_id/submit
func (h *Handler) SubmitApprovalDecision(c echo.Context) error {
	approvalID := c.Param("approval_id")
	var req domain.ApprovalDecisionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Decision != "approve" && req.Decision != "reject" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "decision must be approve or reject"})
	}

	ctx := c.Request().Context()
	
	if err := h.service.UpdateApproval(ctx, approvalID, req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}