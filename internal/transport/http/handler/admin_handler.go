package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// AdminHandler implements the platform-operator control plane in
// BUILD_GUIDE.md's Phase 14 — not yet in documentation/07-api-specification.md,
// see dto/admin.go's own doc-debt note. Every route is behind
// RequirePlatformOperator (transport/http/router.go) except CreateFlag,
// which any authenticated user can call.
type AdminHandler struct {
	svc       admin.Service
	validator *validate.Validator
}

func NewAdminHandler(svc admin.Service, validator *validate.Validator) *AdminHandler {
	return &AdminHandler{svc: svc, validator: validator}
}

func (h *AdminHandler) ListOrganizations(c *gin.Context) {
	page := parsePage(c)
	search := c.Query("search")

	orgs, total, err := h.svc.ListOrganizations(c.Request.Context(), search, admin.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.OrganizationSummaryResponse, len(orgs))
	for i, o := range orgs {
		items[i] = dto.FromOrganizationSummary(o)
	}
	c.JSON(http.StatusOK, dto.OrganizationListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

func (h *AdminHandler) GetOrganization(c *gin.Context) {
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	detail, err := h.svc.GetOrganization(c.Request.Context(), orgID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromOrganizationDetail(*detail))
}

func (h *AdminHandler) SuspendOrganization(c *gin.Context) {
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	operator, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.SuspendRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	if err := h.svc.SuspendOrganization(c.Request.Context(), operator, orgID, req.Reason); err != nil {
		c.Error(err)
		return
	}
	h.GetOrganization(c)
}

func (h *AdminHandler) ReinstateOrganization(c *gin.Context) {
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	operator, ok := requireActor(c)
	if !ok {
		return
	}
	if err := h.svc.ReinstateOrganization(c.Request.Context(), operator, orgID); err != nil {
		c.Error(err)
		return
	}
	h.GetOrganization(c)
}

func (h *AdminHandler) SuspendUser(c *gin.Context) {
	userID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	operator, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.SuspendRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	if err := h.svc.SuspendUser(c.Request.Context(), operator, userID, req.Reason); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) ReinstateUser(c *gin.Context) {
	userID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	operator, ok := requireActor(c)
	if !ok {
		return
	}
	if err := h.svc.ReinstateUser(c.Request.Context(), operator, userID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CreateFlag handles `POST /admin/pentest-flags` — deliberately reachable
// by any authenticated user (router.go does not gate this one route behind
// RequirePlatformOperator), since reporting misuse of a target that scanned
// infrastructure you own shouldn't require operator status. Only ListFlags
// and ResolveFlag are operator-only.
func (h *AdminHandler) CreateFlag(c *gin.Context) {
	reporter, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.CreateFlagRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	targetID, err := uuid.Parse(req.TargetID)
	if err != nil {
		c.Error(apperrors.Validation("admin.invalid_input", "target_id is not a valid UUID", nil))
		return
	}

	flag, err := h.svc.CreateFlag(c.Request.Context(), reporter, admin.CreateFlagInput{
		TargetID: targetID, Source: admin.FlagSource(req.Source), Reason: req.Reason,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": flag.ID.String(), "status": string(flag.Status)})
}

func (h *AdminHandler) ListFlags(c *gin.Context) {
	page := parsePage(c)
	var status *admin.FlagStatus
	if v := c.Query("status"); v != "" {
		s := admin.FlagStatus(v)
		if !s.Valid() {
			c.Error(apperrors.Validation("admin.invalid_filter", "status is not a recognised flag status", nil))
			return
		}
		status = &s
	}

	flags, total, err := h.svc.ListFlags(c.Request.Context(), status, admin.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.PentestFlagResponse, len(flags))
	for i, f := range flags {
		items[i] = dto.FromPentestFlagDetail(f)
	}
	c.JSON(http.StatusOK, dto.PentestFlagListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

func (h *AdminHandler) ResolveFlag(c *gin.Context) {
	flagID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	operator, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.ResolveFlagRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	status := admin.FlagStatus(req.Status)
	if !status.Valid() {
		c.Error(apperrors.Validation("admin.invalid_input", "status must be investigating, dismissed, or confirmed_misuse", nil))
		return
	}

	flag, err := h.svc.ResolveFlag(c.Request.Context(), operator, flagID, admin.ResolveFlagInput{Status: status})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": flag.ID.String(), "status": string(flag.Status)})
}

func (h *AdminHandler) ListAuditLog(c *gin.Context) {
	page := parsePage(c)
	filter := audit.ListFilter{}

	if v := c.Query("org_id"); v != "" {
		orgID, err := uuid.Parse(v)
		if err != nil {
			c.Error(apperrors.Validation("admin.invalid_filter", "org_id is not a valid UUID", nil))
			return
		}
		filter.OrgID = &orgID
	}
	if v := c.Query("actor_id"); v != "" {
		actorID, err := uuid.Parse(v)
		if err != nil {
			c.Error(apperrors.Validation("admin.invalid_filter", "actor_id is not a valid UUID", nil))
			return
		}
		filter.ActorID = &actorID
	}
	if v := c.Query("action"); v != "" {
		filter.Action = &v
	}
	if v := c.Query("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.Error(apperrors.Validation("admin.invalid_filter", "from must be an RFC3339 timestamp", nil))
			return
		}
		filter.From = &t
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.Error(apperrors.Validation("admin.invalid_filter", "to must be an RFC3339 timestamp", nil))
			return
		}
		filter.To = &t
	}

	entries, total, err := h.svc.ListAuditLog(c.Request.Context(), filter, admin.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.AuditEntryResponse, len(entries))
	for i, e := range entries {
		items[i] = dto.FromAuditEntry(e)
	}
	c.JSON(http.StatusOK, dto.AuditLogListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

func (h *AdminHandler) SystemHealth(c *gin.Context) {
	health, err := h.svc.SystemHealth(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromSystemHealth(*health))
}

func (h *AdminHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("admin.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("admin.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}
