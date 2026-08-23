package reporting_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

func TestRenderJSON_RoundTrips(t *testing.T) {
	data := &reporting.ReportData{
		ScanID:      uuid.New(),
		ProjectName: "Demo Project",
		ScanType:    domain.ScanTypeFullSupplyChain,
		Findings: []reporting.FindingRow{
			{Engine: domain.EngineK8sScan, RuleID: "k8sscan.rbac.wildcard-verbs", Severity: domain.SeverityHigh, Title: "Wildcard verbs"},
		},
	}

	out, err := reporting.RenderJSON(data)
	if err != nil {
		t.Fatalf("RenderJSON() unexpected error = %v", err)
	}

	var decoded reporting.ReportData
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output is not valid JSON matching ReportData: %v", err)
	}
	if decoded.ProjectName != "Demo Project" {
		t.Errorf("decoded ProjectName = %q, want %q", decoded.ProjectName, "Demo Project")
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0].RuleID != "k8sscan.rbac.wildcard-verbs" {
		t.Errorf("decoded Findings = %+v, want the one seeded finding", decoded.Findings)
	}
}
