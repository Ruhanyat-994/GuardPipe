package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeAdvisoryService is a hand-written fake — these tests are about the
// HTTP layer, business logic is covered by internal/modules/advisory's own
// tests.
type fakeAdvisoryService struct {
	rule  *advisory.Rule
	rules []advisory.Rule
	total int
	err   error

	gotFilter advisory.RuleFilter
}

func (f *fakeAdvisoryService) Lookup(context.Context, []advisory.Dependency) ([]advisory.Result, error) {
	return nil, nil
}
func (f *fakeAdvisoryService) SyncRules(context.Context) error { return nil }
func (f *fakeAdvisoryService) ListRules(_ context.Context, filter advisory.RuleFilter, _ advisory.RulePage) ([]advisory.Rule, int, error) {
	f.gotFilter = filter
	return f.rules, f.total, f.err
}
func (f *fakeAdvisoryService) GetRule(context.Context, string) (*advisory.Rule, error) {
	return f.rule, f.err
}
func (f *fakeAdvisoryService) SetRuleEnabled(context.Context, string, bool) (*advisory.Rule, error) {
	return f.rule, f.err
}

func newRuleRouter(svc advisory.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())

	h := handler.NewRuleHandler(svc, validate.New())
	r.GET("/rules", h.List)
	r.GET("/rules/:id", h.Get)
	r.PATCH("/rules/:id", h.SetEnabled)
	return r
}

func sampleRule() *advisory.Rule {
	return &advisory.Rule{
		ID: "codescan.injection.sql-string-concat", Engine: domain.EngineCodeScan, Category: "injection",
		Title: "SQL built via string concatenation", DefaultSeverity: domain.SeverityHigh,
		Tier: domain.TierCore, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestRuleList_Returns200(t *testing.T) {
	svc := &fakeAdvisoryService{rules: []advisory.Rule{*sampleRule()}, total: 1}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/rules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleList_FilterByEngine_PassesThroughToService(t *testing.T) {
	svc := &fakeAdvisoryService{}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/rules?engine=codescan", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if svc.gotFilter.Engine == nil || *svc.gotFilter.Engine != domain.EngineCodeScan {
		t.Fatalf("filter.Engine = %v, want codescan", svc.gotFilter.Engine)
	}
}

func TestRuleList_InvalidEngineFilter_Returns400(t *testing.T) {
	svc := &fakeAdvisoryService{}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/rules?engine=not-a-real-engine", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleGet_NotFound_Returns404(t *testing.T) {
	svc := &fakeAdvisoryService{err: apperrors.NotFound("rule.not_found", "rule not found")}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/rules/codescan.injection.does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleGet_Found_Returns200(t *testing.T) {
	svc := &fakeAdvisoryService{rule: sampleRule()}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/rules/codescan.injection.sql-string-concat", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleSetEnabled_ValidBody_Returns200(t *testing.T) {
	svc := &fakeAdvisoryService{rule: sampleRule()}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/rules/codescan.injection.sql-string-concat", map[string]bool{"enabled": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleSetEnabled_MissingEnabledField_Returns400(t *testing.T) {
	svc := &fakeAdvisoryService{}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/rules/codescan.injection.sql-string-concat", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuleSetEnabled_NotFound_Returns404(t *testing.T) {
	svc := &fakeAdvisoryService{err: apperrors.NotFound("rule.not_found", "rule not found")}
	r := newRuleRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/rules/codescan.injection.does-not-exist", map[string]bool{"enabled": true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}
