package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeReportingService is a hand-written reporting.Service fake — these
// tests are about the HTTP layer (status codes, request binding, response
// shape); the triage state machine itself is covered by
// internal/modules/reporting's own tests.
type fakeReportingService struct {
	finding       *domain.Finding
	suggestion    *ai.Suggestion
	changedByName string
	history       []reporting.StatusHistoryEntry
	err           error
}

func (f *fakeReportingService) GetFinding(context.Context, domain.Actor, uuid.UUID) (*domain.Finding, *ai.Suggestion, error) {
	return f.finding, f.suggestion, f.err
}
func (f *fakeReportingService) UpdateFindingStatus(context.Context, domain.Actor, uuid.UUID, domain.Status, string) (*domain.Finding, string, error) {
	return f.finding, f.changedByName, f.err
}
func (f *fakeReportingService) GetFindingHistory(context.Context, domain.Actor, uuid.UUID) ([]reporting.StatusHistoryEntry, error) {
	return f.history, f.err
}

func newFindingRouter(svc reporting.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.Use(func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember})
		c.Next()
	})

	h := handler.NewFindingHandler(svc, validate.New())
	r.GET("/findings/:id", h.Get)
	r.PATCH("/findings/:id/status", h.UpdateStatus)
	r.GET("/findings/:id/history", h.History)
	return r
}

func sampleFinding() *domain.Finding {
	return &domain.Finding{
		ID: id.New(), ScanID: id.New(), Engine: domain.EngineCodeScan, RuleID: "codescan.injection.example",
		Title: "t", Description: "d", Severity: domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "fix it", Status: domain.StatusOpen,
	}
}

func TestFindingGet_Returns200WithDetail(t *testing.T) {
	svc := &fakeReportingService{finding: sampleFinding()}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/findings/"+id.New().String(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingGet_NotFound_Returns404(t *testing.T) {
	svc := &fakeReportingService{err: apperrors.NotFound("finding.not_found", "finding not found")}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/findings/"+id.New().String(), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingUpdateStatus_ValidRequest_Returns200(t *testing.T) {
	f := sampleFinding()
	f.Status = domain.StatusAcknowledged
	svc := &fakeReportingService{finding: f, changedByName: "Ada Lovelace"}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/findings/"+id.New().String()+"/status", map[string]string{"status": "acknowledged"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingUpdateStatus_UnrecognisedStatus_Returns400(t *testing.T) {
	svc := &fakeReportingService{finding: sampleFinding()}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/findings/"+id.New().String()+"/status", map[string]string{"status": "not-a-real-status"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (fails the oneof= validator tag before ever reaching the service), body: %s", rec.Code, rec.Body.String())
	}
}

// TestFindingUpdateStatus_ReasonTooShort_Returns400 is the transport-layer
// half of FR-RPT-005 — the service returns finding.reason_required, and
// this proves it maps to 400, not 422 or 500.
func TestFindingUpdateStatus_ReasonTooShort_Returns400(t *testing.T) {
	svc := &fakeReportingService{err: apperrors.Validation("finding.reason_required", "suppression requires at least 20 characters of justification", nil)}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/findings/"+id.New().String()+"/status", map[string]string{"status": "suppressed", "reason": "too short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingUpdateStatus_InvalidTransition_Returns422(t *testing.T) {
	svc := &fakeReportingService{err: apperrors.Unprocessable("finding.invalid_transition", "cannot move a finding from open to fixed")}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/findings/"+id.New().String()+"/status", map[string]string{"status": "fixed"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingUpdateStatus_ConcurrentConflict_Returns409(t *testing.T) {
	svc := &fakeReportingService{err: apperrors.Conflict("finding.status_conflict", "this finding's status changed since it was last read")}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodPatch, "/findings/"+id.New().String()+"/status", map[string]string{"status": "acknowledged"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body: %s", rec.Code, rec.Body.String())
	}
}

func TestFindingHistory_Returns200(t *testing.T) {
	svc := &fakeReportingService{history: []reporting.StatusHistoryEntry{
		{FromStatus: domain.StatusOpen, ToStatus: domain.StatusAcknowledged},
	}}
	r := newFindingRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/findings/"+id.New().String()+"/history", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}
