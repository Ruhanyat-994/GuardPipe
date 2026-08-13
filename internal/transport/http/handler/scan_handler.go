package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// ScanHandler implements the scan and finding-list endpoints in
// documentation/07-api-specification.md §5-6 that BUILD_GUIDE.md Phase 6
// scopes — risk/AI/triage endpoints are Phase 13's.
type ScanHandler struct {
	svc       orchestrator.Service
	validator *validate.Validator
}

func NewScanHandler(svc orchestrator.Service, validator *validate.Validator) *ScanHandler {
	return &ScanHandler{svc: svc, validator: validator}
}

// Create handles `POST /projects/{id}/scans` — 202 Accepted, asynchronous
// by design (documentation/07-api-specification.md §5: "the API returns
// 202 immediately; the client polls").
func (h *ScanHandler) Create(c *gin.Context) {
	var req dto.CreateScanRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	detail, err := h.svc.CreateScan(c.Request.Context(), actor, projectID, req.ToInput())
	if err != nil {
		c.Error(err)
		return
	}
	c.Header("Location", "/api/v1/scans/"+detail.ID.String())
	c.JSON(http.StatusAccepted, dto.FromScanDetail(detail))
}

// List handles `GET /projects/{id}/scans` — the scan-history page, newest
// first.
func (h *ScanHandler) List(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	page := parsePage(c)

	scans, total, err := h.svc.ListScans(c.Request.Context(), actor, projectID, orchestrator.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]dto.ScanSummaryResponse, len(scans))
	for i, s := range scans {
		items[i] = dto.FromScan(s)
	}
	c.JSON(http.StatusOK, dto.ScanListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

// ListForOrg handles `GET /scans` — the global, cross-project scan history
// (documentation/07-api-specification.md §5's org-wide equivalent of
// List): every scan across every project the actor's org owns, newest
// first, each row carrying its project's name.
func (h *ScanHandler) ListForOrg(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	page := parsePage(c)

	scans, total, err := h.svc.ListOrgScans(c.Request.Context(), actor, orchestrator.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]dto.OrgScanSummaryResponse, len(scans))
	for i, s := range scans {
		items[i] = dto.FromOrgScanSummary(s)
	}
	c.JSON(http.StatusOK, dto.OrgScanListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

// Get handles `GET /scans/{id}`.
func (h *ScanHandler) Get(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	scanID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	detail, err := h.svc.GetScan(c.Request.Context(), actor, scanID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromScanDetail(detail))
}

// Progress handles `GET /scans/{id}/progress` — the polling endpoint.
func (h *ScanHandler) Progress(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	scanID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	progress, err := h.svc.GetProgress(c.Request.Context(), actor, scanID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromProgress(progress))
}

// Cancel handles `POST /scans/{id}/cancel`.
func (h *ScanHandler) Cancel(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	scanID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.CancelScan(c.Request.Context(), actor, scanID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListFindings handles `GET /scans/{id}/findings` — the minimal raw list
// BUILD_GUIDE.md Phase 6 scopes (no filter/sort beyond pagination; that's
// Phase 13's FindingsTable).
func (h *ScanHandler) ListFindings(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	scanID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	page := parsePage(c)

	findings, total, err := h.svc.ListFindings(c.Request.Context(), actor, scanID, orchestrator.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]dto.FindingListItemResponse, len(findings))
	for i, f := range findings {
		items[i] = dto.FromFinding(f)
	}
	c.JSON(http.StatusOK, dto.FindingListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

func (h *ScanHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("scan.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("scan.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}
