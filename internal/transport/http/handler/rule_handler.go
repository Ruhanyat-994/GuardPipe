package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// RuleHandler implements the rules catalogue endpoints in
// documentation/07-api-specification.md §8.
type RuleHandler struct {
	svc       advisory.Service
	validator *validate.Validator
}

func NewRuleHandler(svc advisory.Service, validator *validate.Validator) *RuleHandler {
	return &RuleHandler{svc: svc, validator: validator}
}

// List handles `GET /rules`, filterable by engine/tier/severity query
// params.
func (h *RuleHandler) List(c *gin.Context) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	page := parsePage(c)

	rules, total, err := h.svc.ListRules(c.Request.Context(), filter, advisory.RulePage{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]dto.RuleResponse, len(rules))
	for i, r := range rules {
		items[i] = dto.FromRule(r)
	}
	c.JSON(http.StatusOK, dto.RuleListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

// Get handles `GET /rules/{id}`.
func (h *RuleHandler) Get(c *gin.Context) {
	rule, err := h.svc.GetRule(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRule(*rule))
}

// SetEnabled handles `PATCH /rules/{id}` — admin-only, enforced by the
// router's RBAC middleware, not re-checked here since the rules catalogue
// is global, not org-scoped (nothing to re-verify ownership against, unlike
// project.Service's actor-scoped methods).
func (h *RuleHandler) SetEnabled(c *gin.Context) {
	var req dto.SetRuleEnabledRequest
	if !h.bindAndValidate(c, &req) {
		return
	}

	rule, err := h.svc.SetRuleEnabled(c.Request.Context(), c.Param("id"), *req.Enabled)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRule(*rule))
}

func (h *RuleHandler) parseFilter(c *gin.Context) (advisory.RuleFilter, bool) {
	var filter advisory.RuleFilter

	if v := c.Query("engine"); v != "" {
		engine := domain.EngineID(v)
		if !engine.Valid() {
			c.Error(apperrors.Validation("rule.invalid_filter", "engine is not a recognised engine ID", nil))
			return advisory.RuleFilter{}, false
		}
		filter.Engine = &engine
	}
	if v := c.Query("tier"); v != "" {
		tier := domain.Tier(v)
		if !tier.Valid() {
			c.Error(apperrors.Validation("rule.invalid_filter", "tier must be \"core\" or \"stretch\"", nil))
			return advisory.RuleFilter{}, false
		}
		filter.Tier = &tier
	}
	if v := c.Query("severity"); v != "" {
		severity := domain.Severity(v)
		if !severity.Valid() {
			c.Error(apperrors.Validation("rule.invalid_filter", "severity is not a recognised severity", nil))
			return advisory.RuleFilter{}, false
		}
		filter.Severity = &severity
	}

	return filter, true
}

func (h *RuleHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("rule.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("rule.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}
