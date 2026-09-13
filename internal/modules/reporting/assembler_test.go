package reporting_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

// fakeUserReader is a hand-written reporting.UserReader stub — no mocking
// framework, matching every other fake in this file.
type fakeUserReader struct {
	byID map[uuid.UUID]*identity.User
}

func (f *fakeUserReader) GetByID(_ context.Context, id uuid.UUID) (*identity.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, errors.New("user not found")
}

type fakeScanReader struct {
	detail   *orchestrator.ScanDetail
	findings []domain.Finding
}

func (f *fakeScanReader) GetScan(_ context.Context, _ domain.Actor, _ uuid.UUID) (*orchestrator.ScanDetail, error) {
	return f.detail, nil
}

func (f *fakeScanReader) ListFindings(_ context.Context, _ domain.Actor, _ uuid.UUID, page orchestrator.Page) ([]domain.Finding, int, error) {
	start := (page.Page - 1) * page.PageSize
	if start >= len(f.findings) {
		return nil, len(f.findings), nil
	}
	end := min(start+page.PageSize, len(f.findings))
	return f.findings[start:end], len(f.findings), nil
}

type fakeProjectReader struct {
	detail *project.ProjectDetail
}

func (f *fakeProjectReader) Get(_ context.Context, _ domain.Actor, _ uuid.UUID) (*project.ProjectDetail, error) {
	return f.detail, nil
}

// fakeAI is a hand-written ai.Service stub — this package's own tests never
// call a real provider, matching the project-wide "no mocking framework,
// hand-written fakes only" rule (documentation/15-testing-strategy.md).
type fakeAI struct {
	result *ai.RunResult
	err    error
	// gotInput records the last RunInput passed in — lets a test assert on
	// exactly what Assembler sent as prompt vars (e.g. the risk_score/verdict
	// strings attachExecutiveSummary builds from detail.Risk).
	gotInput ai.RunInput
}

func (f *fakeAI) Run(_ context.Context, in ai.RunInput, _ func(domain.Finding)) (*ai.RunResult, error) {
	f.gotInput = in
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func sampleScanDetail() *orchestrator.ScanDetail {
	started := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	finished := started.Add(3 * time.Minute)
	branch := "main"
	sha := "abc1234"
	return &orchestrator.ScanDetail{
		Scan: domain.Scan{
			ID:            uuid.New(),
			ProjectID:     uuid.New(),
			Type:          domain.ScanTypeFullSupplyChain,
			Status:        domain.ScanStatusCompleted,
			Branch:        &branch,
			CommitSHA:     &sha,
			QueuedAt:      started.Add(-time.Minute),
			StartedAt:     &started,
			FinishedAt:    &finished,
			FindingCounts: map[domain.Severity]int{domain.SeverityHigh: 1},
			ScanNumber:    3,
		},
		Jobs: []orchestrator.JobDetail{
			{
				ScanJob: domain.ScanJob{
					Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded,
					StartedAt: &started, FinishedAt: &finished,
				},
				FindingCount: 1,
			},
			{
				ScanJob: domain.ScanJob{
					Engine: domain.EnginePentest, Status: domain.JobStatusSucceeded,
					StartedAt: &started, FinishedAt: &finished,
					Stats: map[string]any{
						"target_host": "example.com",
						"coverage": map[string]any{
							"open_ports":            []any{80.0, 443.0},
							"http_services_found":   2.0,
							"tls_ports_checked":     1.0,
							"nuclei_categories_run": []any{"exposures", "misconfiguration"},
							"total_script_runs":     12.0,
							"phases_completed":      []any{"recon", "service_id", "tls", "headers"},
						},
						"assets": []any{
							map[string]any{"Value": "example.com", "Type": "host", "Validated": true},
							map[string]any{"Value": "dev.example.com", "Type": "subdomain", "Validated": true},
							map[string]any{"Value": "unresolvable.example.com", "Type": "subdomain", "Validated": false},
						},
						"validated_directories": []any{
							map[string]any{"URL": "http://example.com/admin", "Host": "example.com", "Port": 80.0, "Source": "ffuf_disclosure"},
						},
					},
				},
				FindingCount: 0,
			},
		},
	}
}

func sampleProjectDetail() *project.ProjectDetail {
	return &project.ProjectDetail{
		Project: project.Project{Name: "Demo Project"},
		Repository: &project.Repository{
			Owner: "guardpipe", Name: "demo", DefaultBranch: "main",
		},
	}
}

func TestAssembler_Build_AssemblesScanProjectAndCoverage(t *testing.T) {
	findings := []domain.Finding{
		{Engine: domain.EngineDepScan, RuleID: "depscan.vuln.known-cve", Title: "Known CVE", Severity: domain.SeverityHigh, Remediation: "Upgrade the package."},
	}
	scans := &fakeScanReader{detail: sampleScanDetail(), findings: findings}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}

	a := reporting.NewAssembler(scans, projects, nil, nil, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}

	if data.ProjectName != "Demo Project" {
		t.Errorf("ProjectName = %q, want %q", data.ProjectName, "Demo Project")
	}
	if data.RepositoryOwner != "guardpipe" || data.RepositoryName != "demo" {
		t.Errorf("Repository = %s/%s, want guardpipe/demo", data.RepositoryOwner, data.RepositoryName)
	}
	if data.TotalFindings != 1 {
		t.Errorf("TotalFindings = %d, want 1", data.TotalFindings)
	}
	if data.PentestTargetHost != "example.com" {
		t.Errorf("PentestTargetHost = %q, want example.com", data.PentestTargetHost)
	}

	var pentestJob *reporting.JobSummary
	for i := range data.Jobs {
		if data.Jobs[i].Engine == domain.EnginePentest {
			pentestJob = &data.Jobs[i]
		}
	}
	if pentestJob == nil {
		t.Fatal("no pentest job in assembled report")
	}
	if pentestJob.Coverage == nil {
		t.Fatal("pentest job's Coverage is nil — a clean pentest run must still report what it checked")
	}
	if pentestJob.Coverage.TotalScriptRuns != 12 {
		t.Errorf("Coverage.TotalScriptRuns = %d, want 12", pentestJob.Coverage.TotalScriptRuns)
	}
	if len(pentestJob.Coverage.OpenPorts) != 2 {
		t.Errorf("Coverage.OpenPorts = %v, want 2 entries", pentestJob.Coverage.OpenPorts)
	}
	if want := []string{"dev.example.com"}; !slices.Equal(pentestJob.ValidatedSubdomains, want) {
		t.Errorf("ValidatedSubdomains = %v, want %v (only the validated subdomain, not the root host asset or the unresolvable dropped one)", pentestJob.ValidatedSubdomains, want)
	}
	if want := []string{"http://example.com/admin"}; !slices.Equal(pentestJob.ValidatedDirectories, want) {
		t.Errorf("ValidatedDirectories = %v, want %v", pentestJob.ValidatedDirectories, want)
	}

	// AI is nil in this test — the report must still be fully usable without it.
	if data.ExecutiveSummary != "" {
		t.Errorf("ExecutiveSummary = %q, want empty when ai.Service is nil", data.ExecutiveSummary)
	}
}

// TestAssembler_Build_AttachesRequestedByWatermark is this feature's own
// contract: a report must carry who ran the scan (name/email, resolved from
// domain.Scan.TriggeredBy) and from where (RequestedFromIP, already on the
// scan row), plus the fixed responsibility disclaimer — GuardPipe's
// accountability watermark, the same "who ran this and from where" stamp
// Nessus/Qualys-class tools already carry on their own exported reports.
func TestAssembler_Build_AttachesRequestedByWatermark(t *testing.T) {
	userID := uuid.New()
	detail := sampleScanDetail()
	detail.TriggeredBy = &userID
	detail.RequestedFromIP = "203.0.113.10"

	scans := &fakeScanReader{detail: detail, findings: nil}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	users := &fakeUserReader{byID: map[uuid.UUID]*identity.User{
		userID: {ID: userID, DisplayName: "Ada Lovelace", Email: "ada@example.com"},
	}}

	a := reporting.NewAssembler(scans, projects, users, nil, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}

	if data.RequestedByName != "Ada Lovelace" {
		t.Errorf("RequestedByName = %q, want %q", data.RequestedByName, "Ada Lovelace")
	}
	if data.RequestedByEmail != "ada@example.com" {
		t.Errorf("RequestedByEmail = %q, want %q", data.RequestedByEmail, "ada@example.com")
	}
	if data.RequestedFromIP != "203.0.113.10" {
		t.Errorf("RequestedFromIP = %q, want %q", data.RequestedFromIP, "203.0.113.10")
	}
	if data.Disclaimer == "" {
		t.Error("Disclaimer must never be empty — every report carries the fixed responsibility statement")
	}
}

// TestAssembler_Build_NoTriggeringUser_OmitsWatermarkWithoutError is the
// near-miss half: a scan with no recorded triggering user (nil TriggeredBy,
// or a UserReader lookup failure) must still produce a complete report —
// RequestedByName/Email just stay empty, never a Build failure.
func TestAssembler_Build_NoTriggeringUser_OmitsWatermarkWithoutError(t *testing.T) {
	detail := sampleScanDetail()
	detail.TriggeredBy = nil

	scans := &fakeScanReader{detail: detail}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	users := &fakeUserReader{byID: map[uuid.UUID]*identity.User{}}

	a := reporting.NewAssembler(scans, projects, users, nil, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}
	if data.RequestedByName != "" || data.RequestedByEmail != "" {
		t.Errorf("RequestedByName/Email = %q/%q, want empty when no triggering user is recorded", data.RequestedByName, data.RequestedByEmail)
	}
	if data.Disclaimer == "" {
		t.Error("Disclaimer must still be present even with no attributable requester")
	}
}

func TestAssembler_Build_SortsFindingsWorstFirst(t *testing.T) {
	findings := []domain.Finding{
		{Engine: domain.EngineDepScan, RuleID: "a", Title: "Low", Severity: domain.SeverityLow},
		{Engine: domain.EngineDepScan, RuleID: "b", Title: "Critical", Severity: domain.SeverityCritical},
		{Engine: domain.EngineDepScan, RuleID: "c", Title: "Medium", Severity: domain.SeverityMedium},
	}
	scans := &fakeScanReader{detail: sampleScanDetail(), findings: findings}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}

	a := reporting.NewAssembler(scans, projects, nil, nil, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}
	if len(data.Findings) != 3 || data.Findings[0].Severity != domain.SeverityCritical {
		t.Fatalf("Findings not sorted worst-first: %+v", data.Findings)
	}
	if data.Findings[2].Severity != domain.SeverityLow {
		t.Fatalf("last finding = %v, want low", data.Findings[2].Severity)
	}
}

func TestAssembler_Build_AttachesAIExecutiveSummaryWhenAvailable(t *testing.T) {
	scans := &fakeScanReader{detail: sampleScanDetail()}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	stub := &fakeAI{result: &ai.RunResult{Value: ai.ScanSummaryResponse{
		Summary:       "The scan found one high-severity issue.",
		TopPriorities: []string{"Fix the CVE", "Rotate credentials", "Enable HSTS"},
	}}}

	a := reporting.NewAssembler(scans, projects, nil, stub, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}
	if data.ExecutiveSummary == "" {
		t.Error("ExecutiveSummary is empty, want the AI-authored summary")
	}
	if len(data.TopPriorities) != 3 {
		t.Errorf("TopPriorities = %v, want 3 items", data.TopPriorities)
	}
}

// TestAssembler_Build_ExecutiveSummaryPrompt_UsesRealScoreWhenPresent
// guards against the AI prompt silently going back to a hardcoded
// "not yet computed" placeholder now that orchestrator.ScanDetail.Risk is
// real (BUILD_GUIDE.md Phase 13's scoring wiring).
func TestAssembler_Build_ExecutiveSummaryPrompt_UsesRealScoreWhenPresent(t *testing.T) {
	detail := sampleScanDetail()
	detail.Risk = &orchestrator.RiskAssessmentRecord{Score: 68, Verdict: domain.VerdictBlock, FormulaVersion: "1.0"}
	scans := &fakeScanReader{detail: detail}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	stub := &fakeAI{result: &ai.RunResult{Value: ai.ScanSummaryResponse{Summary: "s"}}}

	a := reporting.NewAssembler(scans, projects, nil, stub, nil)
	_, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}
	if got := stub.gotInput.Vars["risk_score"]; got != "68" {
		t.Errorf(`Vars["risk_score"] = %q, want "68"`, got)
	}
	if got := stub.gotInput.Vars["verdict"]; got != "block" {
		t.Errorf(`Vars["verdict"] = %q, want "block"`, got)
	}
}

// TestAssembler_Build_ExecutiveSummaryPrompt_NoScoreYet is the near-miss:
// a scan whose last job hasn't finalized yet (detail.Risk is nil) must tell
// the AI prompt exactly that, not send a fabricated or stale number.
func TestAssembler_Build_ExecutiveSummaryPrompt_NoScoreYet(t *testing.T) {
	detail := sampleScanDetail()
	detail.Risk = nil
	scans := &fakeScanReader{detail: detail}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	stub := &fakeAI{result: &ai.RunResult{Value: ai.ScanSummaryResponse{Summary: "s"}}}

	a := reporting.NewAssembler(scans, projects, nil, stub, nil)
	_, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v", err)
	}
	if got := stub.gotInput.Vars["risk_score"]; got != "not yet computed for this scan" {
		t.Errorf(`Vars["risk_score"] = %q, want the no-score-yet message`, got)
	}
	if got := stub.gotInput.Vars["verdict"]; got != "not yet computed" {
		t.Errorf(`Vars["verdict"] = %q, want "not yet computed"`, got)
	}
}

func TestAssembler_Build_NearMiss_AIFailureNeverFailsBuild(t *testing.T) {
	// The near-miss half of the AI-attachment behaviour: Gemini being down
	// (or returning an error/discard) must degrade the report, not break it —
	// same fallback contract every AI-consuming engine already follows.
	scans := &fakeScanReader{detail: sampleScanDetail()}
	projects := &fakeProjectReader{detail: sampleProjectDetail()}
	stub := &fakeAI{err: errors.New("gemini: 429 resource exhausted")}

	a := reporting.NewAssembler(scans, projects, nil, stub, nil)
	data, err := a.Build(context.Background(), domain.Actor{}, uuid.New())
	if err != nil {
		t.Fatalf("Build() unexpected error = %v — an AI failure must not fail the whole report", err)
	}
	if data.ExecutiveSummary != "" {
		t.Errorf("ExecutiveSummary = %q, want empty when the AI call failed", data.ExecutiveSummary)
	}
	if data.TotalFindings != 0 {
		t.Errorf("TotalFindings = %d, want 0 (no findings were seeded in this test)", data.TotalFindings)
	}
}
