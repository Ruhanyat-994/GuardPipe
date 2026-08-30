package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeAdminService is a hand-written fake — these tests are about the HTTP
// layer (binding, status codes, error mapping); business logic is covered
// by internal/modules/admin's own tests.
type fakeAdminService struct {
	isOperator bool

	orgs      []admin.OrganizationSummary
	orgDetail *admin.OrganizationDetail
	flags     []admin.PentestFlagDetail
	flag      *admin.PentestFlag
	auditLog  []audit.Entry
	health    *admin.SystemHealth
	total     int
	err       error

	gotSuspendReason string
	gotResolveStatus admin.FlagStatus
}

func (f *fakeAdminService) IsOperator(context.Context, uuid.UUID) (bool, error) {
	return f.isOperator, nil
}
func (f *fakeAdminService) ListOrganizations(context.Context, string, admin.Page) ([]admin.OrganizationSummary, int, error) {
	return f.orgs, f.total, f.err
}
func (f *fakeAdminService) GetOrganization(context.Context, uuid.UUID) (*admin.OrganizationDetail, error) {
	return f.orgDetail, f.err
}
func (f *fakeAdminService) SuspendOrganization(_ context.Context, _ domain.Actor, _ uuid.UUID, reason string) error {
	f.gotSuspendReason = reason
	return f.err
}
func (f *fakeAdminService) ReinstateOrganization(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeAdminService) SuspendUser(_ context.Context, _ domain.Actor, _ uuid.UUID, reason string) error {
	f.gotSuspendReason = reason
	return f.err
}
func (f *fakeAdminService) ReinstateUser(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeAdminService) CreateFlag(context.Context, domain.Actor, admin.CreateFlagInput) (*admin.PentestFlag, error) {
	return f.flag, f.err
}
func (f *fakeAdminService) ListFlags(context.Context, *admin.FlagStatus, admin.Page) ([]admin.PentestFlagDetail, int, error) {
	return f.flags, f.total, f.err
}
func (f *fakeAdminService) ResolveFlag(_ context.Context, _ domain.Actor, _ uuid.UUID, in admin.ResolveFlagInput) (*admin.PentestFlag, error) {
	f.gotResolveStatus = in.Status
	return f.flag, f.err
}
func (f *fakeAdminService) ListAuditLog(context.Context, audit.ListFilter, admin.Page) ([]audit.Entry, int, error) {
	return f.auditLog, f.total, f.err
}
func (f *fakeAdminService) SystemHealth(context.Context) (*admin.SystemHealth, error) {
	return f.health, f.err
}

func newAdminRouter(svc admin.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.Use(func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember})
		c.Next()
	})

	h := handler.NewAdminHandler(svc, validate.New())
	r.GET("/admin/organizations", h.ListOrganizations)
	r.GET("/admin/organizations/:id", h.GetOrganization)
	r.POST("/admin/organizations/:id/suspend", h.SuspendOrganization)
	r.POST("/admin/organizations/:id/reinstate", h.ReinstateOrganization)
	r.POST("/admin/users/:id/suspend", h.SuspendUser)
	r.POST("/admin/users/:id/reinstate", h.ReinstateUser)
	r.POST("/admin/pentest-flags", h.CreateFlag)
	r.GET("/admin/pentest-flags", h.ListFlags)
	r.PATCH("/admin/pentest-flags/:id", h.ResolveFlag)
	r.GET("/admin/audit-log", h.ListAuditLog)
	r.GET("/admin/system-health", h.SystemHealth)
	return r
}

func TestAdminListOrganizations_Returns200(t *testing.T) {
	svc := &fakeAdminService{orgs: []admin.OrganizationSummary{{ID: id.New(), Name: "Acme"}}, total: 1}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/admin/organizations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminGetOrganization_InvalidUUID_Returns400(t *testing.T) {
	svc := &fakeAdminService{}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/admin/organizations/not-a-uuid", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminSuspendOrganization_PassesReasonThrough(t *testing.T) {
	svc := &fakeAdminService{orgDetail: &admin.OrganizationDetail{OrganizationSummary: admin.OrganizationSummary{ID: id.New()}}}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/admin/organizations/"+id.New().String()+"/suspend", map[string]string{"reason": "ToS violation"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if svc.gotSuspendReason != "ToS violation" {
		t.Errorf("gotSuspendReason = %q, want %q", svc.gotSuspendReason, "ToS violation")
	}
}

// TestAdminSuspendOrganization_MissingReason_Returns400 is the near-miss:
// the request-body validator alone (not just the service layer) rejects an
// empty reason, matching SuspendRequest's `validate:"required"` tag.
func TestAdminSuspendOrganization_MissingReason_Returns400(t *testing.T) {
	svc := &fakeAdminService{}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/admin/organizations/"+id.New().String()+"/suspend", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminCreateFlag_InvalidTargetUUID_Returns400(t *testing.T) {
	svc := &fakeAdminService{}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/admin/pentest-flags", map[string]string{
		"target_id": "not-a-uuid", "source": "self_reported", "reason": "scanned my server",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminCreateFlag_Valid_Returns201(t *testing.T) {
	svc := &fakeAdminService{flag: &admin.PentestFlag{ID: id.New(), Status: admin.FlagOpen}}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/admin/pentest-flags", map[string]string{
		"target_id": id.New().String(), "source": "self_reported", "reason": "scanned my server",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminResolveFlag_InvalidStatus_Returns400(t *testing.T) {
	svc := &fakeAdminService{}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/admin/pentest-flags/"+id.New().String(), map[string]string{"status": "not_a_real_status"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminResolveFlag_ValidStatus_PassesThrough(t *testing.T) {
	svc := &fakeAdminService{flag: &admin.PentestFlag{ID: id.New(), Status: admin.FlagConfirmedMisuse}}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/admin/pentest-flags/"+id.New().String(), map[string]string{"status": "confirmed_misuse"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if svc.gotResolveStatus != admin.FlagConfirmedMisuse {
		t.Errorf("gotResolveStatus = %q, want %q", svc.gotResolveStatus, admin.FlagConfirmedMisuse)
	}
}

func TestAdminListAuditLog_InvalidFromTimestamp_Returns400(t *testing.T) {
	svc := &fakeAdminService{}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/admin/audit-log?from=not-a-timestamp", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminListAuditLog_Returns200(t *testing.T) {
	orgID := id.New()
	svc := &fakeAdminService{auditLog: []audit.Entry{{ID: 1, OrgID: &orgID, Action: "org.suspended", CreatedAt: time.Now()}}, total: 1}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/admin/audit-log", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminSystemHealth_Returns200(t *testing.T) {
	svc := &fakeAdminService{health: &admin.SystemHealth{CheckedAt: time.Now()}}
	r := newAdminRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/admin/system-health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}
