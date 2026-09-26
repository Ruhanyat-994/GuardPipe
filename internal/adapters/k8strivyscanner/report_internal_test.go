package k8strivyscanner

import (
	"strings"
	"testing"
)

func TestExtractReport(t *testing.T) {
	logs := "2026-09-26T00:00:00Z INFO Need to update DB\n" + reportMarker + "\n" +
		`{"Results":[{"Target":"Dockerfile","Misconfigurations":[{"ID":"DS002","Severity":"HIGH"}]}]}` + "\n"
	report, err := extractReport([]byte(logs))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Target != "Dockerfile" || len(report.Results[0].Misconfigurations) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

// Near miss: Trivy's log lines alone (no marker) are an error, not an empty
// "clean" report — a scan that printed nothing must never read as no findings.
func TestExtractReport_NoMarkerIsAnError(t *testing.T) {
	_, err := extractReport([]byte("FATAL failed to download vulnerability DB\n"))
	if err == nil || !strings.Contains(err.Error(), "printed no report") {
		t.Fatalf("want a no-report error, got %v", err)
	}
}
