package domain_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// The remaining closed-set types follow the same shape as Severity: a small
// fixed vocabulary plus a near-miss that must not validate. One table per
// type keeps each one's true members explicit and catches a copy-paste typo
// in the const block immediately.

func TestConfidence_Valid(t *testing.T) {
	valid := []domain.Confidence{domain.ConfidenceHigh, domain.ConfidenceMedium, domain.ConfidenceLow}
	for _, c := range valid {
		if !c.Valid() {
			t.Errorf("Confidence(%q).Valid() = false, want true", c)
		}
	}
	if domain.Confidence("certain").Valid() {
		t.Errorf("near-miss Confidence value must not validate")
	}
}

func TestStatus_Valid(t *testing.T) {
	valid := []domain.Status{
		domain.StatusOpen, domain.StatusAcknowledged, domain.StatusSuppressed,
		domain.StatusFixed, domain.StatusFalsePositive,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("Status(%q).Valid() = false, want true", s)
		}
	}
	if domain.Status("closed").Valid() {
		t.Errorf("near-miss Status value must not validate")
	}
}

func TestEngineID_Valid(t *testing.T) {
	valid := []domain.EngineID{
		domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan,
		domain.EngineContainerScan, domain.EngineK8sScan, domain.EngineCICDScan, domain.EnginePentest,
	}
	if len(valid) != 7 {
		t.Fatalf("expected all 7 engines listed in this test, got %d", len(valid))
	}
	for _, id := range valid {
		if !id.Valid() {
			t.Errorf("EngineID(%q).Valid() = false, want true", id)
		}
	}
	if domain.EngineID("sast").Valid() {
		t.Errorf("near-miss EngineID value must not validate")
	}
}

func TestScanType_Valid(t *testing.T) {
	valid := []domain.ScanType{domain.ScanTypeFullSupplyChain, domain.ScanTypePartial, domain.ScanTypePentestOnly}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("ScanType(%q).Valid() = false, want true", s)
		}
	}
	if domain.ScanType("full").Valid() {
		t.Errorf("near-miss ScanType value must not validate")
	}
}

func TestScanStatus_Valid(t *testing.T) {
	valid := []domain.ScanStatus{
		domain.ScanStatusQueued, domain.ScanStatusRunning, domain.ScanStatusCompleted,
		domain.ScanStatusFailed, domain.ScanStatusCancelled,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("ScanStatus(%q).Valid() = false, want true", s)
		}
	}
	if domain.ScanStatus("done").Valid() {
		t.Errorf("near-miss ScanStatus value must not validate")
	}
}

func TestJobStatus_Valid(t *testing.T) {
	valid := []domain.JobStatus{
		domain.JobStatusQueued, domain.JobStatusRunning, domain.JobStatusSucceeded,
		domain.JobStatusFailed, domain.JobStatusSkipped, domain.JobStatusCancelled,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("JobStatus(%q).Valid() = false, want true", s)
		}
	}
	if domain.JobStatus("done").Valid() {
		t.Errorf("near-miss JobStatus value must not validate")
	}
}

func TestTier_Valid(t *testing.T) {
	if !domain.TierCore.Valid() || !domain.TierStretch.Valid() {
		t.Errorf("both Tier values must validate")
	}
	if domain.Tier("optional").Valid() {
		t.Errorf("near-miss Tier value must not validate")
	}
}

func TestLocationType_Valid(t *testing.T) {
	valid := []domain.LocationType{
		domain.LocationTypeFile, domain.LocationTypeImage, domain.LocationTypeK8s,
		domain.LocationTypeNetwork, domain.LocationTypeDependency,
	}
	for _, lt := range valid {
		if !lt.Valid() {
			t.Errorf("LocationType(%q).Valid() = false, want true", lt)
		}
	}
	if domain.LocationType("registry").Valid() {
		t.Errorf("near-miss LocationType value must not validate")
	}
}

func TestEvidenceKind_Valid(t *testing.T) {
	valid := []domain.EvidenceKind{
		domain.EvidenceKindCodeSnippet, domain.EvidenceKindCommandOutput, domain.EvidenceKindHTTPResponse,
		domain.EvidenceKindManifestExcerpt, domain.EvidenceKindLayerReference,
	}
	for _, k := range valid {
		if !k.Valid() {
			t.Errorf("EvidenceKind(%q).Valid() = false, want true", k)
		}
	}
	if domain.EvidenceKind("screenshot").Valid() {
		t.Errorf("near-miss EvidenceKind value must not validate")
	}
}
