package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// FindingHandler implements the per-finding endpoints
// documentation/07-api-specification.md §6 describes: detail, triage
// (status transition), and the status-change history audit trail. Query/
// filter/sort/pagination on a scan's whole finding list stays
// ScanHandler.ListFindings's job — this handler is only ever addressed by
// one finding's own ID.
type FindingHandler struct {
	svc       reporting.Service
	validator *validate.Validator
}

func NewFindingHandler(svc reporting.Service, validator *validate.Validator) *FindingHandler {
	return &FindingHandler{svc: svc, validator: validator}
}

// Get handles `GET /findings/{id}`.
func (h *FindingHandler) Get(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	findingID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	finding, suggestion, err := h.svc.GetFinding(c.Request.Context(), actor, findingID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromFindingDetail(*finding, suggestion))
}

// UpdateStatus handles `PATCH /findings/{id}/status` — the triage action
// (documentation/05-module-specifications.md §15's state machine,
// enforced by reporting.ValidateTransition inside the service, not here).
func (h *FindingHandler) UpdateStatus(c *gin.Context) {
	var req dto.UpdateFindingStatusRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	findingID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	finding, changedByName, err := h.svc.UpdateFindingStatus(c.Request.Context(), actor, findingID, domain.Status(req.Status), req.Reason)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromUpdatedFinding(finding, changedByName))
}

// History handles `GET /findings/{id}/history` — the raw
// finding_status_history audit trail, newest first.
func (h *FindingHandler) History(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	findingID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	entries, err := h.svc.GetFindingHistory(c.Request.Context(), actor, findingID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromStatusHistory(entries))
}

func (h *FindingHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("finding.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("finding.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}
