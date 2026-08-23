package reporting_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

func TestRenderPDF_CleanPentestScan(t *testing.T) {
	// The exact scenario this feature started from: a clean pentest run with
	// zero findings must still produce a real, readable report — coverage,
	// not just an empty findings list.
	started := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	finished := started.Add(4 * time.Minute)
	data := &reporting.ReportData{
		GeneratedAt:       time.Now(),
		ProjectName:       "Demo Project",
		PentestTargetHost: "example.com",
		ScanID:            uuid.New(),
		ScanNumber:        1,
		ScanType:          domain.ScanTypePentestOnly,
		ScanStatus:        domain.ScanStatusCompleted,
		QueuedAt:          started.Add(-time.Second),
		StartedAt:         &started,
		FinishedAt:        &finished,
		FindingCounts:     map[domain.Severity]int{},
		Jobs: []reporting.JobSummary{
			{
				Engine: domain.EnginePentest, Status: domain.JobStatusSucceeded,
				StartedAt: &started, FinishedAt: &finished,
				Coverage: &reporting.PentestCoverage{
					OpenPorts: []int{80, 443}, HTTPServicesFound: 2, TLSPortsChecked: 1,
					NucleiCategoriesRun: []string{"exposures", "misconfiguration"},
					TotalScriptRuns:     14,
					PhasesCompleted:     []string{"recon", "service_id", "tls", "headers", "crawl", "wordlist", "disclosure", "misconfig"},
				},
			},
		},
	}

	out, err := reporting.RenderPDF(data)
	if err != nil {
		t.Fatalf("RenderPDF() unexpected error = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output does not start with the PDF magic bytes: %q", out[:min(20, len(out))])
	}
	if len(out) < 500 {
		t.Errorf("PDF is only %d bytes — suspiciously small even for a clean report", len(out))
	}
}

func TestRenderPDF_ScanWithFindingsAndExecutiveSummary(t *testing.T) {
	data := &reporting.ReportData{
		GeneratedAt:      time.Now(),
		ProjectName:      "Demo Project",
		RepositoryOwner:  "guardpipe",
		RepositoryName:   "demo",
		RepositoryBranch: "main",
		ScanID:           uuid.New(),
		ScanNumber:       5,
		ScanType:         domain.ScanTypeFullSupplyChain,
		ScanStatus:       domain.ScanStatusCompleted,
		FindingCounts:    map[domain.Severity]int{domain.SeverityCritical: 1, domain.SeverityMedium: 1},
		ExecutiveSummary: "This scan found one critical secret exposure and one medium-severity misconfiguration.",
		TopPriorities:    []string{"Rotate the exposed credential immediately", "Add a NetworkPolicy", "Pin the GitHub Action to a SHA"},
		Jobs: []reporting.JobSummary{
			{Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded, FindingCount: 1},
			{Engine: domain.EngineK8sScan, Status: domain.JobStatusSucceeded, FindingCount: 1},
		},
		Findings: []reporting.FindingRow{
			{
				Engine: domain.EngineDepScan, RuleID: "depscan.secrets.committed-credential",
				Title: "Committed AWS credential", Description: "A live AWS access key was found committed to the repository.",
				Severity: domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
				Location: ".env:3", Remediation: "Revoke the key immediately and remove it from git history.",
			},
			{
				Engine: domain.EngineK8sScan, RuleID: "k8sscan.network.no-networkpolicy",
				Title: "No NetworkPolicy restricting traffic", Description: "The namespace has no NetworkPolicy, so all pod-to-pod traffic is allowed by default.",
				Severity: domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
				Location: "default/Deployment/api in deployment.yaml", Remediation: "Add a default-deny NetworkPolicy and allow only required traffic.",
			},
		},
	}

	out, err := reporting.RenderPDF(data)
	if err != nil {
		t.Fatalf("RenderPDF() unexpected error = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output does not start with the PDF magic bytes")
	}
}

func TestRenderPDF_NearMiss_EmptyReportStillRenders(t *testing.T) {
	// A near-empty ReportData (no jobs, no findings, no executive summary)
	// must still render a valid, short PDF rather than erroring or panicking.
	out, err := reporting.RenderPDF(&reporting.ReportData{
		GeneratedAt:   time.Now(),
		ProjectName:   "Empty Project",
		ScanID:        uuid.New(),
		ScanType:      domain.ScanTypeFullSupplyChain,
		ScanStatus:    domain.ScanStatusCompleted,
		FindingCounts: map[domain.Severity]int{},
	})
	if err != nil {
		t.Fatalf("RenderPDF() unexpected error on an empty report = %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output does not start with the PDF magic bytes")
	}
}
