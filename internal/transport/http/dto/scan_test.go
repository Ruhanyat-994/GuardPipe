package dto

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
)

func TestFromRiskAssessment_Nil_ReturnsNil(t *testing.T) {
	if got := fromRiskAssessment(nil); got != nil {
		t.Errorf("fromRiskAssessment(nil) = %+v, want nil", got)
	}
}

func TestFromRiskAssessment_MapsEveryField(t *testing.T) {
	previous := 74
	delta := -6
	rec := &orchestrator.RiskAssessmentRecord{
		ScanID:  uuid.New(),
		Score:   68,
		Verdict: domain.VerdictBlock,
		EngineScores: map[domain.EngineID]int{
			domain.EngineCodeScan: 72,
			domain.EngineDepScan:  61,
		},
		Breakdown: []scoring.Contribution{
			{Reason: scoring.ReasonCriticalFloor, Detail: "1 critical finding set a minimum score of 70", Impact: 5},
			{Reason: scoring.ReasonEngineContribution, Engine: domain.EngineCodeScan, Detail: "codescan contributed", Impact: 24.9},
		},
		PreviousScore:  &previous,
		Delta:          &delta,
		IsPartial:      false,
		FormulaVersion: "1.0",
	}

	got := fromRiskAssessment(rec)
	if got == nil {
		t.Fatal("fromRiskAssessment returned nil for a non-nil record")
	}
	if got.Score != 68 {
		t.Errorf("Score = %d, want 68", got.Score)
	}
	if got.Verdict != "block" {
		t.Errorf("Verdict = %q, want block", got.Verdict)
	}
	if got.PreviousScore == nil || *got.PreviousScore != 74 {
		t.Errorf("PreviousScore = %v, want 74", got.PreviousScore)
	}
	if got.Delta == nil || *got.Delta != -6 {
		t.Errorf("Delta = %v, want -6", got.Delta)
	}
	if got.IsPartial {
		t.Errorf("IsPartial = true, want false")
	}
	if got.FormulaVersion != "1.0" {
		t.Errorf("FormulaVersion = %q, want 1.0", got.FormulaVersion)
	}
	if got.EngineScores["codescan"] != 72 || got.EngineScores["depscan"] != 61 {
		t.Errorf("EngineScores = %v, want codescan:72 depscan:61", got.EngineScores)
	}

	if len(got.Breakdown) != 2 {
		t.Fatalf("Breakdown has %d entries, want 2", len(got.Breakdown))
	}
	if got.Breakdown[0].Reason != "critical_floor_applied" || got.Breakdown[0].Engine != "" {
		t.Errorf("Breakdown[0] = %+v, want the floor entry with no engine", got.Breakdown[0])
	}
	if got.Breakdown[1].Reason != "engine_contribution" || got.Breakdown[1].Engine != "codescan" {
		t.Errorf("Breakdown[1] = %+v, want the codescan engine_contribution entry", got.Breakdown[1])
	}
}

func TestFromRiskAssessment_NoPreviousScore_DeltaAlsoNil(t *testing.T) {
	rec := &orchestrator.RiskAssessmentRecord{Score: 10, Verdict: domain.VerdictPass, FormulaVersion: "1.0"}
	got := fromRiskAssessment(rec)
	if got.PreviousScore != nil {
		t.Errorf("PreviousScore = %v, want nil (no prior scan)", got.PreviousScore)
	}
	if got.Delta != nil {
		t.Errorf("Delta = %v, want nil when there's no previous score to compare against", got.Delta)
	}
}

// TestFromScanDetail_RiskIsNilUntilScored is the "nulls are present, not
// omitted" convention (documentation/07-api-specification.md §1) applied to
// a scan with no RiskAssessmentRecord yet (still running, or queued).
func TestFromScanDetail_RiskIsNilUntilScored(t *testing.T) {
	detail := &orchestrator.ScanDetail{Scan: domain.Scan{ID: uuid.New(), Status: domain.ScanStatusRunning}}
	got := FromScanDetail(detail)
	if got.Risk != nil {
		t.Errorf("Risk = %+v, want nil for a scan with no assessment yet", got.Risk)
	}
}

func TestFromScanDetail_RiskPresentOnceScored(t *testing.T) {
	detail := &orchestrator.ScanDetail{
		Scan: domain.Scan{ID: uuid.New(), Status: domain.ScanStatusCompleted},
		Risk: &orchestrator.RiskAssessmentRecord{Score: 42, Verdict: domain.VerdictWarn, FormulaVersion: "1.0"},
	}
	got := FromScanDetail(detail)
	if got.Risk == nil {
		t.Fatal("Risk is nil, want the assessment to flow through")
	}
	if got.Risk.Score != 42 || got.Risk.Verdict != "warn" {
		t.Errorf("Risk = %+v, want score 42 verdict warn", got.Risk)
	}
}
