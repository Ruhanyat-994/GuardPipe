package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeReportBuilder is a hand-written handler.ReportBuilder fake — Export's
// own tests are about the HTTP layer (content-type, Content-Disposition,
// format validation), not report assembly, which internal/modules/reporting's
// own tests already cover.
type fakeReportBuilder struct {
	data *reporting.ReportData
	err  error
}

func (f *fakeReportBuilder) Build(context.Context, domain.Actor, uuid.UUID) (*reporting.ReportData, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.data != nil {
		return f.data, nil
	}
	return &reporting.ReportData{ScanNumber: 1, FindingCounts: map[domain.Severity]int{}}, nil
}

// fakeOrchestratorService is a hand-written fake — these tests are about
// the HTTP layer, business logic is covered by
// internal/modules/orchestrator's own tests.
type fakeOrchestratorService struct {
	detail   *orchestrator.ScanDetail
	progress *orchestrator.Progress
	findings []domain.Finding
	scans    []domain.Scan
	orgScans []orchestrator.OrgScanSummary
	total    int
	err      error
}

func (f *fakeOrchestratorService) CreateScan(context.Context, domain.Actor, uuid.UUID, orchestrator.CreateScanInput) (*orchestrator.ScanDetail, error) {
	return f.detail, f.err
}
func (f *fakeOrchestratorService) GetScan(context.Context, domain.Actor, uuid.UUID) (*orchestrator.ScanDetail, error) {
	return f.detail, f.err
}
func (f *fakeOrchestratorService) ListScans(context.Context, domain.Actor, uuid.UUID, orchestrator.Page) ([]domain.Scan, int, error) {
	return f.scans, f.total, f.err
}
func (f *fakeOrchestratorService) ListOrgScans(context.Context, domain.Actor, orchestrator.Page) ([]orchestrator.OrgScanSummary, int, error) {
	return f.orgScans, f.total, f.err
}
func (f *fakeOrchestratorService) ListActiveScans(context.Context, domain.Actor) ([]orchestrator.OrgScanSummary, error) {
	return f.orgScans, f.err
}
func (f *fakeOrchestratorService) GetProgress(context.Context, domain.Actor, uuid.UUID) (*orchestrator.Progress, error) {
	return f.progress, f.err
}
func (f *fakeOrchestratorService) CancelScan(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeOrchestratorService) ListFindings(context.Context, domain.Actor, uuid.UUID, orchestrator.Page) ([]domain.Finding, int, error) {
	return f.findings, f.total, f.err
}
func (f *fakeOrchestratorService) CreateSchedule(context.Context, domain.Actor, uuid.UUID, orchestrator.CreateScheduleInput) (*orchestrator.ScanSchedule, error) {
	return nil, f.err
}
func (f *fakeOrchestratorService) GetSchedule(context.Context, domain.Actor, uuid.UUID) (*orchestrator.ScanSchedule, error) {
	return nil, f.err
}
func (f *fakeOrchestratorService) ListSchedules(context.Context, domain.Actor, uuid.UUID) ([]orchestrator.ScanSchedule, error) {
	return nil, f.err
}
func (f *fakeOrchestratorService) UpdateSchedule(context.Context, domain.Actor, uuid.UUID, orchestrator.UpdateScheduleInput) (*orchestrator.ScanSchedule, error) {
	return nil, f.err
}
func (f *fakeOrchestratorService) DeleteSchedule(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeOrchestratorService) TriggerSchedule(context.Context, uuid.UUID) (*orchestrator.ScanDetail, error) {
	return nil, f.err
}

func newScanRouter(svc orchestrator.Service) *gin.Engine {
	return newScanRouterWithReports(svc, &fakeReportBuilder{})
}

func newScanRouterWithReports(svc orchestrator.Service, reports handler.ReportBuilder) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.Use(func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember})
		c.Next()
	})

	h := handler.NewScanHandler(svc, reports, validate.New())
	r.POST("/projects/:id/scans", h.Create)
	r.GET("/projects/:id/scans", h.List)
	r.GET("/scans", h.ListForOrg)
	r.GET("/scans/:id", h.Get)
	r.GET("/scans/:id/progress", h.Progress)
	r.POST("/scans/:id/cancel", h.Cancel)
	r.GET("/scans/:id/findings", h.ListFindings)
	r.GET("/scans/:id/export", h.Export)
	return r
}

func sampleScanDetail() *orchestrator.ScanDetail {
	return &orchestrator.ScanDetail{
		Scan: domain.Scan{
			ID: id.New(), ProjectID: id.New(), Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued,
			RequestedEngines: []domain.EngineID{domain.EngineDepScan}, FindingCounts: map[domain.Severity]int{},
		},
		Jobs: []orchestrator.JobDetail{{ScanJob: domain.ScanJob{ID: id.New(), Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}}},
	}
}

func TestScanCreate_ValidRequest_Returns202(t *testing.T) {
	svc := &fakeOrchestratorService{detail: sampleScanDetail()}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/scans", map[string]string{"type": "full_supply_chain"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanCreate_InvalidType_Returns400(t *testing.T) {
	svc := &fakeOrchestratorService{}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/scans", map[string]string{"type": "not-a-real-type"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanCreate_NoEnginesAvailable_Returns422(t *testing.T) {
	svc := &fakeOrchestratorService{err: apperrors.Unprocessable("scan.no_engines_available", "no engine is registered to run this scan yet")}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/scans", map[string]string{"type": "full_supply_chain"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanList_Returns200WithData(t *testing.T) {
	svc := &fakeOrchestratorService{
		scans: []domain.Scan{{ID: id.New(), Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusCompleted, FindingCounts: map[domain.Severity]int{}}},
		total: 1,
	}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/projects/"+id.New().String()+"/scans", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanListForOrg_Returns200WithProjectName(t *testing.T) {
	svc := &fakeOrchestratorService{
		orgScans: []orchestrator.OrgScanSummary{{
			Scan:        domain.Scan{ID: id.New(), Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusCompleted, FindingCounts: map[domain.Severity]int{}},
			ProjectName: "PaperPulse",
		}},
		total: 1,
	}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PaperPulse") {
		t.Fatalf("expected response to carry the project name, body: %s", rec.Body.String())
	}
}

func TestScanList_UnknownProject_Returns404(t *testing.T) {
	svc := &fakeOrchestratorService{err: apperrors.NotFound("project.not_found", "project not found")}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/projects/"+id.New().String()+"/scans", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanGet_NotFound_Returns404(t *testing.T) {
	svc := &fakeOrchestratorService{err: apperrors.NotFound("scan.not_found", "scan not found")}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String(), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanGet_Found_Returns200(t *testing.T) {
	svc := &fakeOrchestratorService{detail: sampleScanDetail()}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanProgress_Returns200(t *testing.T) {
	svc := &fakeOrchestratorService{progress: &orchestrator.Progress{
		ScanID: id.New(), Status: domain.ScanStatusRunning, ProgressPct: 50,
		Engines: []orchestrator.EngineProgress{{Engine: domain.EngineDepScan, Status: domain.JobStatusRunning, ProgressPct: 50}},
	}}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/progress", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanCancel_Returns204(t *testing.T) {
	svc := &fakeOrchestratorService{}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/scans/"+id.New().String()+"/cancel", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanListFindings_Returns200(t *testing.T) {
	svc := &fakeOrchestratorService{
		findings: []domain.Finding{{ID: id.New(), RuleID: "depscan.hygiene.no-lockfile", Severity: domain.SeverityMedium, Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "package.json"}}},
		total:    1,
	}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/findings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanGet_InvalidUUID_Returns400(t *testing.T) {
	svc := &fakeOrchestratorService{}
	r := newScanRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/scans/not-a-uuid", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestScanExport_DefaultFormatIsJSON(t *testing.T) {
	svc := &fakeOrchestratorService{}
	reports := &fakeReportBuilder{data: &reporting.ReportData{ScanNumber: 4, ProjectName: "Demo", FindingCounts: map[domain.Severity]int{}}}
	r := newScanRouterWithReports(svc, reports)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if disp := rec.Header().Get("Content-Disposition"); !strings.Contains(disp, "guardpipe-scan-4.json") {
		t.Errorf("Content-Disposition = %q, want it to name guardpipe-scan-4.json", disp)
	}
}

func TestScanExport_CSVFormat(t *testing.T) {
	svc := &fakeOrchestratorService{}
	reports := &fakeReportBuilder{data: &reporting.ReportData{ScanNumber: 1, FindingCounts: map[domain.Severity]int{}}}
	r := newScanRouterWithReports(svc, reports)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/export?format=csv", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
}

func TestScanExport_PDFFormat(t *testing.T) {
	svc := &fakeOrchestratorService{}
	reports := &fakeReportBuilder{data: &reporting.ReportData{ScanNumber: 1, FindingCounts: map[domain.Severity]int{}}}
	r := newScanRouterWithReports(svc, reports)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/export?format=pdf", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/pdf") {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Error("body does not start with the PDF magic bytes")
	}
}

func TestScanExport_NearMiss_UnsupportedFormatReturns400(t *testing.T) {
	// sarif and any other unrecognised value must be a clean validation
	// error, not a 500 or a silently-wrong format.
	svc := &fakeOrchestratorService{}
	reports := &fakeReportBuilder{data: &reporting.ReportData{FindingCounts: map[domain.Severity]int{}}}
	r := newScanRouterWithReports(svc, reports)

	rec := doJSON(t, r, http.MethodGet, "/scans/"+id.New().String()+"/export?format=sarif", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}
