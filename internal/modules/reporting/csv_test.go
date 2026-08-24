package reporting_test

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

func readAllSections(t *testing.T, out []byte) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(string(out)))
	r.FieldsPerRecord = -1 // sections have different column counts by design
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	return rows
}

func TestRenderCSV_OneRowPerFinding(t *testing.T) {
	data := &reporting.ReportData{
		ProjectName:   "Demo Project",
		FindingCounts: map[domain.Severity]int{domain.SeverityHigh: 1, domain.SeverityCritical: 1},
		Findings: []reporting.FindingRow{
			{Engine: domain.EngineDepScan, RuleID: "depscan.vuln.known-cve", Severity: domain.SeverityHigh, Confidence: domain.ConfidenceHigh, Title: "Known CVE", Location: "package.json", CVE: []string{"CVE-2024-1234"}, Remediation: "Upgrade to 2.0.1."},
			{Engine: domain.EngineCodeScan, RuleID: "codescan.sonarqube.java:S2076", Severity: domain.SeverityCritical, Confidence: domain.ConfidenceHigh, Title: "SQL injection", Location: "src/Db.java:42", CWE: []string{"CWE-89"}, Remediation: "Use a parameterised query."},
		},
	}

	out, err := reporting.RenderCSV(data)
	if err != nil {
		t.Fatalf("RenderCSV() unexpected error = %v", err)
	}
	text := string(out)

	if !strings.Contains(text, "Demo Project") {
		t.Error("output does not mention the project name")
	}
	if !strings.Contains(text, "Findings") {
		t.Error("output has no Findings section")
	}
	if !strings.Contains(text, "Upgrade to 2.0.1.") {
		t.Error("output is missing a finding's remediation text")
	}

	rows := readAllSections(t, out)
	var findingRows int
	inFindings := false
	for _, row := range rows {
		if len(row) == 1 && row[0] == "Findings" {
			inFindings = true
			continue
		}
		if inFindings && len(row) > 0 && row[0] != "engine" {
			findingRows++
		}
	}
	if findingRows != 2 {
		t.Errorf("got %d finding rows, want 2", findingRows)
	}
}

// TestRenderCSV_IncludesAccountabilityWatermark is this feature's own
// contract for the CSV export: who requested the scan, from what IP, and
// the fixed responsibility disclaimer must all appear in the scan-info
// section, the same accountability stamp the PDF's "Authorisation &
// responsibility" section carries.
func TestRenderCSV_IncludesAccountabilityWatermark(t *testing.T) {
	data := &reporting.ReportData{
		ProjectName:      "Demo Project",
		RequestedByName:  "Ada Lovelace",
		RequestedByEmail: "ada@example.com",
		RequestedFromIP:  "203.0.113.10",
		Disclaimer:       "This scan was initiated by the account identified above.",
	}

	out, err := reporting.RenderCSV(data)
	if err != nil {
		t.Fatalf("RenderCSV() unexpected error = %v", err)
	}
	text := string(out)

	for _, want := range []string{"Ada Lovelace", "ada@example.com", "203.0.113.10", "This scan was initiated by the account identified above."} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing accountability watermark content %q", want)
		}
	}
}

// TestRenderCSV_NearMiss_NoRequesterOmitsWatermarkRows is the near-miss
// half: a report with no attributable requester (empty RequestedByName/IP)
// must not emit a misleading blank "Requested By"/"Source IP" row.
func TestRenderCSV_NearMiss_NoRequesterOmitsWatermarkRows(t *testing.T) {
	data := &reporting.ReportData{ProjectName: "Demo Project"}

	out, err := reporting.RenderCSV(data)
	if err != nil {
		t.Fatalf("RenderCSV() unexpected error = %v", err)
	}
	text := string(out)

	if strings.Contains(text, "Requested By") {
		t.Error("output should not have a Requested By row when no requester is known")
	}
	if strings.Contains(text, "Source IP") {
		t.Error("output should not have a Source IP row when none is recorded")
	}
}

func TestRenderCSV_NearMiss_CleanScanStillHasRealContent(t *testing.T) {
	// The bug this fixes: a 0-finding scan's CSV used to be nothing but a
	// bare header row. It must now still show the scan's own metadata and
	// per-engine coverage — the same "what did it check" information the
	// PDF's coverage page and the live UI already show.
	started := time.Now().Add(-3 * time.Minute)
	finished := time.Now()
	data := &reporting.ReportData{
		ProjectName:   "Empty Project",
		ScanNumber:    5,
		FindingCounts: map[domain.Severity]int{},
		Jobs: []reporting.JobSummary{
			{
				Engine: domain.EnginePentest, Status: domain.JobStatusSucceeded,
				StartedAt: &started, FinishedAt: &finished,
				Coverage: &reporting.PentestCoverage{
					OpenPorts: []int{80, 443}, HTTPServicesFound: 2, TotalScriptRuns: 20,
					PhasesCompleted: []string{"recon", "service_id"},
				},
			},
		},
	}

	out, err := reporting.RenderCSV(data)
	if err != nil {
		t.Fatalf("RenderCSV() unexpected error = %v", err)
	}
	text := string(out)

	if !strings.Contains(text, "Empty Project") {
		t.Error("output does not mention the project name even with zero findings")
	}
	if !strings.Contains(text, "open ports 80,443") {
		t.Error("output does not include the pentest coverage detail")
	}
	if !strings.Contains(text, "Coverage") {
		t.Error("output has no Coverage section")
	}
}
