package reporting_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

type fakeAISuggestionReader struct {
	byFindingID map[uuid.UUID]*ai.Suggestion
	err         error
}

func (f *fakeAISuggestionReader) GetByFindingID(_ context.Context, findingID uuid.UUID) (*ai.Suggestion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byFindingID[findingID], nil
}

type fakeReportingFindingRepo struct {
	byID map[uuid.UUID]*domain.Finding
}

func (f *fakeReportingFindingRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Finding, error) {
	finding, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	cp := *finding
	return &cp, nil
}

type fakeReportingScanRepo struct {
	byID map[uuid.UUID]*domain.Scan
}

func (f *fakeReportingScanRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Scan, error) {
	s, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("scan.not_found", "scan not found")
	}
	cp := *s
	return &cp, nil
}

type statusUpdateCall struct {
	findingID uuid.UUID
	from, to  domain.Status
	reason    string
	changedBy uuid.UUID
}

type fakeFindingStatusRepo struct {
	updateErr  error
	lastUpdate *statusUpdateCall
	history    []reporting.StatusHistoryEntry
	historyErr error
}

func (f *fakeFindingStatusRepo) UpdateStatus(_ context.Context, findingID uuid.UUID, from, to domain.Status, reason string, changedBy uuid.UUID) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.lastUpdate = &statusUpdateCall{findingID: findingID, from: from, to: to, reason: reason, changedBy: changedBy}
	return nil
}

func (f *fakeFindingStatusRepo) ListHistory(_ context.Context, _ uuid.UUID) ([]reporting.StatusHistoryEntry, error) {
	return f.history, f.historyErr
}

func newTestFinding(status domain.Status) *domain.Finding {
	return &domain.Finding{
		ID: uuid.New(), ScanID: uuid.New(), Engine: domain.EngineCodeScan, RuleID: "codescan.injection.example",
		Title: "t", Description: "d", Severity: domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "fix it", Status: status,
	}
}

// setupTriageTest wires a reporting.Service whose finding's scan belongs to
// a project fakeProjectReader will authorize — the common case every test
// below starts from, then tweaks one dependency for its own scenario.
func setupTriageTest(t *testing.T, finding *domain.Finding) (reporting.Service, *fakeFindingStatusRepo, *fakeUserReader) {
	t.Helper()
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	status := &fakeFindingStatusRepo{}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	users := &fakeUserReader{byID: map[uuid.UUID]*identity.User{}}

	svc := reporting.NewService(findings, status, scans, projects, users, nil)
	return svc, status, users
}

func TestService_GetFinding_ReturnsOwnedFinding(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, _, _ := setupTriageTest(t, finding)

	got, _, err := svc.GetFinding(context.Background(), domain.Actor{}, finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if got.ID != finding.ID {
		t.Errorf("ID = %v, want %v", got.ID, finding.ID)
	}
}

func TestService_GetFinding_ReturnsSuggestionWhenOneExists(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	suggestions := &fakeAISuggestionReader{byFindingID: map[uuid.UUID]*ai.Suggestion{
		finding.ID: {FindingID: finding.ID, Explanation: "explained"},
	}}

	svc := reporting.NewService(findings, &fakeFindingStatusRepo{}, scans, projects, nil, suggestions)

	_, suggestion, err := svc.GetFinding(context.Background(), domain.Actor{}, finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if suggestion == nil || suggestion.Explanation != "explained" {
		t.Errorf("suggestion = %+v, want the seeded suggestion", suggestion)
	}
}

func TestService_GetFinding_NoSuggestionYet_ReturnsNilNotError(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, _, _ := setupTriageTest(t, finding) // setupTriageTest wires no AISuggestionReader

	_, suggestion, err := svc.GetFinding(context.Background(), domain.Actor{}, finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if suggestion != nil {
		t.Errorf("suggestion = %+v, want nil — a nil AISuggestionReader means never-enriched", suggestion)
	}
}

// TestService_GetFinding_SuggestionLookupFails_DegradesToNilNotError proves
// documentation/10-ai-integration.md §9's "degrade, don't fail" shape: a
// broken AI-suggestion lookup must never hide the finding itself.
func TestService_GetFinding_SuggestionLookupFails_DegradesToNilNotError(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	suggestions := &fakeAISuggestionReader{err: errors.New("db unreachable")}

	svc := reporting.NewService(findings, &fakeFindingStatusRepo{}, scans, projects, nil, suggestions)

	got, suggestion, err := svc.GetFinding(context.Background(), domain.Actor{}, finding.ID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v, want nil — a suggestion-lookup failure must not fail the whole call", err)
	}
	if got == nil {
		t.Fatal("finding is nil, want it returned despite the suggestion lookup failing")
	}
	if suggestion != nil {
		t.Errorf("suggestion = %+v, want nil", suggestion)
	}
}

func TestService_GetFinding_UnknownFinding_ReturnsNotFound(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, _, _ := setupTriageTest(t, finding)

	_, _, err := svc.GetFinding(context.Background(), domain.Actor{}, uuid.New())
	requireNotFound(t, err)
}

// TestService_GetFinding_CrossOrgProject_ReturnsNotFound is the 404-not-403
// rule every other service in this codebase enforces the same way
// (orchestrator.service.getOwnedScan is the precedent this package's own
// getOwnedFinding mirrors).
func TestService_GetFinding_CrossOrgProject_ReturnsNotFound(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{err: apperrors.NotFound("project.not_found", "project not found")}

	svc := reporting.NewService(findings, &fakeFindingStatusRepo{}, scans, projects, nil, nil)

	_, _, err := svc.GetFinding(context.Background(), domain.Actor{}, finding.ID)
	requireNotFound(t, err)
}

func TestService_UpdateFindingStatus_ValidTransition_Succeeds(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	actorID := uuid.New()
	svc, status, users := setupTriageTest(t, finding)
	users.byID[actorID] = &identity.User{ID: actorID, DisplayName: "Ada Lovelace"}

	got, changedByName, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{UserID: actorID}, finding.ID, domain.StatusAcknowledged, "")
	if err != nil {
		t.Fatalf("UpdateFindingStatus() error = %v", err)
	}
	if got.Status != domain.StatusAcknowledged {
		t.Errorf("Status = %s, want acknowledged", got.Status)
	}
	if changedByName != "Ada Lovelace" {
		t.Errorf("changedByName = %q, want %q", changedByName, "Ada Lovelace")
	}
	if status.lastUpdate == nil {
		t.Fatal("FindingStatusRepository.UpdateStatus was never called")
	}
	if status.lastUpdate.from != domain.StatusOpen || status.lastUpdate.to != domain.StatusAcknowledged {
		t.Errorf("recorded transition = %s -> %s, want open -> acknowledged", status.lastUpdate.from, status.lastUpdate.to)
	}
}

func TestService_UpdateFindingStatus_NoUserReader_EmptyNameNoError(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}

	svc := reporting.NewService(findings, &fakeFindingStatusRepo{}, scans, projects, nil, nil)

	_, changedByName, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{UserID: uuid.New()}, finding.ID, domain.StatusAcknowledged, "")
	if err != nil {
		t.Fatalf("UpdateFindingStatus() error = %v", err)
	}
	if changedByName != "" {
		t.Errorf("changedByName = %q, want empty (no UserReader wired)", changedByName)
	}
}

func TestService_UpdateFindingStatus_InvalidTransition_ReturnsUnprocessable(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, _, _ := setupTriageTest(t, finding)

	_, _, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{}, finding.ID, domain.StatusFixed, "")
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperrors.KindUnprocessable {
		t.Fatalf("err = %v, want KindUnprocessable", err)
	}
	if appErr.Code != "finding.invalid_transition" {
		t.Errorf("Code = %q, want finding.invalid_transition", appErr.Code)
	}
}

func TestService_UpdateFindingStatus_SuppressionReasonTooShort_ReturnsValidation(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, _, _ := setupTriageTest(t, finding)

	_, _, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{}, finding.ID, domain.StatusSuppressed, "too short")
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperrors.KindValidation {
		t.Fatalf("err = %v, want KindValidation", err)
	}
	if appErr.Code != "finding.reason_required" {
		t.Errorf("Code = %q, want finding.reason_required", appErr.Code)
	}
}

func TestService_UpdateFindingStatus_SuppressionWithReason_Succeeds(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	svc, status, _ := setupTriageTest(t, finding)

	reason := strings.Repeat("a", 25)
	_, _, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{}, finding.ID, domain.StatusSuppressed, reason)
	if err != nil {
		t.Fatalf("UpdateFindingStatus() error = %v", err)
	}
	if status.lastUpdate.reason != reason {
		t.Errorf("recorded reason = %q, want %q", status.lastUpdate.reason, reason)
	}
}

// TestService_UpdateFindingStatus_ConcurrentConflict_ReturnsConflict proves
// FindingStatusRepository.ErrStatusConflict (an optimistic-concurrency
// guard failing at the repo layer) maps to a 409, not a 500.
func TestService_UpdateFindingStatus_ConcurrentConflict_ReturnsConflict(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	status := &fakeFindingStatusRepo{updateErr: reporting.ErrStatusConflict}

	svc := reporting.NewService(findings, status, scans, projects, nil, nil)

	_, _, err := svc.UpdateFindingStatus(context.Background(), domain.Actor{}, finding.ID, domain.StatusAcknowledged, "")
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperrors.KindConflict {
		t.Fatalf("err = %v, want KindConflict", err)
	}
}

func TestService_GetFindingHistory_ReturnsEntries(t *testing.T) {
	finding := newTestFinding(domain.StatusAcknowledged)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	status := &fakeFindingStatusRepo{history: []reporting.StatusHistoryEntry{
		{FromStatus: domain.StatusOpen, ToStatus: domain.StatusAcknowledged},
	}}

	svc := reporting.NewService(findings, status, scans, projects, nil, nil)

	entries, err := svc.GetFindingHistory(context.Background(), domain.Actor{}, finding.ID)
	if err != nil {
		t.Fatalf("GetFindingHistory() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}

func TestService_GetFindingHistory_UnownedFinding_ReturnsNotFound(t *testing.T) {
	finding := newTestFinding(domain.StatusOpen)
	scanID := finding.ScanID
	projectID := uuid.New()
	scans := &fakeReportingScanRepo{byID: map[uuid.UUID]*domain.Scan{scanID: {ID: scanID, ProjectID: projectID}}}
	findings := &fakeReportingFindingRepo{byID: map[uuid.UUID]*domain.Finding{finding.ID: finding}}
	projects := &fakeProjectReader{err: apperrors.NotFound("project.not_found", "project not found")}

	svc := reporting.NewService(findings, &fakeFindingStatusRepo{}, scans, projects, nil, nil)

	_, err := svc.GetFindingHistory(context.Background(), domain.Actor{}, finding.ID)
	requireNotFound(t, err)
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperrors.KindNotFound {
		t.Fatalf("err = %v, want KindNotFound", err)
	}
}
