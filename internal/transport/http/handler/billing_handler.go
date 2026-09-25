package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// OrgLookup confirms an organisation exists (admin.Service), so the
// operator billing endpoints answer 404 for an unknown org.
type OrgLookup interface {
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*admin.OrganizationDetail, error)
}

// BillingHandler serves /billing/* and the operator's /admin/billing/*
// (TOKENIZATION-ARCHITECTURE.md §10). Thin: bind, call billing.Service, map.
type BillingHandler struct {
	svc       *billing.Service
	previewer orchestrator.ScanPreviewer
	orgs      OrgLookup
	validator *validate.Validator
}

func NewBillingHandler(svc *billing.Service, previewer orchestrator.ScanPreviewer, orgs OrgLookup, validator *validate.Validator) *BillingHandler {
	return &BillingHandler{svc: svc, previewer: previewer, orgs: orgs, validator: validator}
}

func (h *BillingHandler) bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("billing.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("billing.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}

// Catalog handles `GET /billing/catalog` — public, no auth: the pricing
// page shows it to logged-out visitors.
func (h *BillingHandler) Catalog(c *gin.Context) {
	c.JSON(http.StatusOK, dto.FromBillingCatalog(h.svc.Catalog()))
}

// Summary handles `GET /billing/summary`.
func (h *BillingHandler) Summary(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	sum, err := h.svc.Summary(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromBillingSummary(sum))
}

// Ledger handles `GET /billing/ledger?before=<RFC3339>&limit=<n>`.
func (h *BillingHandler) Ledger(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	var before *time.Time
	if v := c.Query("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			c.Error(apperrors.Validation("billing.invalid_cursor", "before must be an RFC 3339 timestamp", nil))
			return
		}
		before = &t
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	entries, err := h.svc.Ledger(c.Request.Context(), actor, before, limit)
	if err != nil {
		c.Error(err)
		return
	}
	resp := dto.BillingLedgerResponse{Data: dto.FromLedgerEntries(entries)}
	if len(entries) == limit {
		last := entries[len(entries)-1].CreatedAt
		resp.NextBefore = &last
	}
	c.JSON(http.StatusOK, resp)
}

// ScanTokens handles `GET /billing/scans/{id}` — what one scan cost.
func (h *BillingHandler) ScanTokens(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	scanID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	st, err := h.svc.ScanTokens(c.Request.Context(), actor, scanID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.BillingScanTokensResponse{Charged: st.Charged, Refunded: st.Refunded, Entries: dto.FromLedgerEntries(st.Entries)})
}

// Estimate handles `POST /billing/estimate`.
func (h *BillingHandler) Estimate(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	if actor.ProjectID != nil {
		c.Error(apperrors.Forbidden("billing.not_available", "billing isn't available in a shared-project session"))
		return
	}
	var req dto.BillingEstimateRequest
	if !h.bind(c, &req) {
		return
	}
	trigger := domain.TriggerSource(req.Trigger)
	if trigger == "" {
		trigger = domain.TriggerManual
	}

	orgID := actor.OrgID
	var engines []domain.EngineID
	preset := domain.PentestPresetStealth
	if req.ProjectID != "" && req.Type != "" {
		scanReq := dto.CreateScanRequest{Type: req.Type, Engines: req.Engines, PentestConfig: req.PentestConfig}
		preview, err := h.previewer.PreviewScan(c.Request.Context(), actor, uuid.MustParse(req.ProjectID), scanReq.ToInput(""))
		if err != nil {
			c.Error(err)
			return
		}
		orgID, engines, preset = preview.OrgID, preview.Engines, preview.Preset
	} else {
		if len(req.Engines) == 0 {
			c.Error(apperrors.Validation("billing.invalid_input", "give project_id and type, or a list of engines", nil))
			return
		}
		for _, e := range req.Engines {
			engines = append(engines, domain.EngineID(e))
		}
		if req.PentestConfig != nil && req.PentestConfig.Preset != "" {
			preset = domain.PentestPreset(req.PentestConfig.Preset)
		}
	}

	est, err := h.svc.Estimate(c.Request.Context(), orgID, engines, preset, trigger)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromBillingEstimate(engines, est))
}

// StartCheckout handles `POST /billing/checkout`.
func (h *BillingHandler) StartCheckout(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.StartCheckoutRequest
	if !h.bind(c, &req) {
		return
	}
	sess, redirect, err := h.svc.StartCheckout(c.Request.Context(), actor, req.ItemCode)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromCheckout(sess, redirect))
}

// GetCheckout handles `GET /billing/checkout/{id}`.
func (h *BillingHandler) GetCheckout(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	id, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	sess, err := h.svc.GetCheckout(c.Request.Context(), actor, id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromCheckout(sess, ""))
}

// ConfirmCheckout handles `POST /billing/checkout/{id}/confirm` — demo
// mode only (404 otherwise).
func (h *BillingHandler) ConfirmCheckout(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	id, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	var req dto.ConfirmCheckoutRequest
	if !h.bind(c, &req) {
		return
	}
	sess, err := h.svc.ConfirmDemo(c.Request.Context(), actor, id, req.Outcome)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromCheckout(sess, ""))
}

// CancelSubscription handles `POST /billing/subscription/cancel`.
func (h *BillingHandler) CancelSubscription(c *gin.Context) { h.setCancel(c, true) }

// ResumeSubscription handles `POST /billing/subscription/resume`.
func (h *BillingHandler) ResumeSubscription(c *gin.Context) { h.setCancel(c, false) }

func (h *BillingHandler) setCancel(c *gin.Context, cancel bool) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	sum, err := h.svc.SetCancelAtPeriodEnd(c.Request.Context(), actor, cancel)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromBillingSummary(sum))
}

// adminOrg resolves the path org and 404s an unknown one.
func (h *BillingHandler) adminOrg(c *gin.Context) (uuid.UUID, bool) {
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return uuid.Nil, false
	}
	if _, err := h.orgs.GetOrganization(c.Request.Context(), orgID); err != nil {
		c.Error(err)
		return uuid.Nil, false
	}
	return orgID, true
}

// AdminGet handles `GET /admin/billing/orgs/{id}` (platform operator).
func (h *BillingHandler) AdminGet(c *gin.Context) {
	orgID, ok := h.adminOrg(c)
	if !ok {
		return
	}
	sum, ledger, err := h.svc.AdminSummary(c.Request.Context(), orgID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.AdminBillingResponse{Summary: dto.FromBillingSummary(sum), Ledger: dto.FromLedgerEntries(ledger)})
}

// AdminAdjust handles `POST /admin/billing/orgs/{id}/adjust` (operator).
func (h *BillingHandler) AdminAdjust(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := h.adminOrg(c)
	if !ok {
		return
	}
	var req dto.AdminAdjustTokensRequest
	if !h.bind(c, &req) {
		return
	}
	if err := h.svc.AdminAdjust(c.Request.Context(), actor.UserID, orgID, req.Delta, req.Reason); err != nil {
		c.Error(err)
		return
	}
	h.AdminGet(c)
}

// AdminAdvanceCycle handles `POST /admin/billing/orgs/{id}/advance-cycle`
// (operator, demo mode only).
func (h *BillingHandler) AdminAdvanceCycle(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := h.adminOrg(c)
	if !ok {
		return
	}
	if err := h.svc.AdminAdvanceCycle(c.Request.Context(), actor.UserID, orgID); err != nil {
		c.Error(err)
		return
	}
	h.AdminGet(c)
}
