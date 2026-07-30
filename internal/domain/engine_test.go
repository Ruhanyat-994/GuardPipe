package domain_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// fakeEngine is a hand-written fake, not a mock — per the project's testing
// philosophy (no mocking framework). It doubles as living documentation of
// how a real engine implements the interface.
type fakeEngine struct {
	id         domain.EngineID
	applicable bool
	reason     string
	findings   []domain.Finding
	result     domain.EngineResult
	err        error
}

func (f *fakeEngine) ID() domain.EngineID { return f.id }

func (f *fakeEngine) Applicable(_ context.Context, _ domain.ScanInput) (bool, string) {
	return f.applicable, f.reason
}

func (f *fakeEngine) Run(_ context.Context, _ domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	for _, finding := range f.findings {
		emit(finding)
	}
	return f.result, f.err
}

// Compile-time check that fakeEngine actually satisfies the interface — if
// the Engine contract ever changes shape, this line fails to build.
var _ domain.Engine = (*fakeEngine)(nil)

func TestEngine_ApplicableFalseIsNotAnError(t *testing.T) {
	eng := &fakeEngine{id: domain.EngineContainerScan, applicable: false, reason: "no Dockerfile found"}

	ok, reason := eng.Applicable(context.Background(), domain.ScanInput{})
	if ok {
		t.Fatalf("Applicable() = true, want false")
	}
	if reason == "" {
		t.Errorf("Applicable() returned an empty reason for a skip — the UI needs to explain why an engine was skipped")
	}
}

func TestEngine_RunStreamsFindingsViaEmit(t *testing.T) {
	want := []domain.Finding{
		{ID: uuid.New(), RuleID: "codescan.injection.sql-string-concat", Severity: domain.SeverityHigh},
		{ID: uuid.New(), RuleID: "codescan.secrets.hardcoded-aws-key", Severity: domain.SeverityCritical},
	}
	eng := &fakeEngine{
		id:       domain.EngineCodeScan,
		findings: want,
		result:   domain.EngineResult{RulesEvaluated: 42, FilesScanned: 10},
	}

	var got []domain.Finding
	result, err := eng.Run(context.Background(), domain.ScanInput{}, func(f domain.Finding) {
		got = append(got, f)
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("emit() called %d times, want %d", len(got), len(want))
	}
	if result.RulesEvaluated != 42 || result.FilesScanned != 10 {
		t.Errorf("Run() result = %+v, want RulesEvaluated=42 FilesScanned=10", result)
	}
}

func TestEngineID_AllSevenAreDistinct(t *testing.T) {
	ids := []domain.EngineID{
		domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan,
		domain.EngineContainerScan, domain.EngineK8sScan, domain.EngineCICDScan, domain.EnginePentest,
	}
	seen := make(map[domain.EngineID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate EngineID constant: %q", id)
		}
		seen[id] = true
	}
}
