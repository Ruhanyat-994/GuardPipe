package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

func TestLevelForSeverity_MatchesDocumentedPriorityOrder(t *testing.T) {
	tests := []struct {
		sev  domain.Severity
		want ai.EnrichmentLevel
	}{
		{domain.SeverityCritical, ai.EnrichExplanationAndPatch},
		{domain.SeverityHigh, ai.EnrichExplanationAndPatch},
		{domain.SeverityMedium, ai.EnrichExplanationOnly},
		{domain.SeverityLow, ai.EnrichNone},
		{domain.SeverityInformational, ai.EnrichNone},
	}
	for _, tt := range tests {
		t.Run(string(tt.sev), func(t *testing.T) {
			if got := ai.LevelForSeverity(tt.sev); got != tt.want {
				t.Errorf("LevelForSeverity(%s) = %v, want %v", tt.sev, got, tt.want)
			}
		})
	}
}

// fakeAIService is a hand-written ai.Service fake — no mocking framework,
// matching modules/reporting's own fakeAI precedent.
type fakeAIService struct {
	explainResult *ai.RunResult
	explainErr    error
	patchResult   *ai.RunResult
	patchErr      error
	calls         []ai.RunInput
}

func (f *fakeAIService) Run(_ context.Context, in ai.RunInput, _ func(domain.Finding)) (*ai.RunResult, error) {
	f.calls = append(f.calls, in)
	switch in.PromptID {
	case ai.PromptExplainFinding:
		if f.explainErr != nil {
			return nil, f.explainErr
		}
		return f.explainResult, nil
	case ai.PromptGeneratePatch:
		if f.patchErr != nil {
			return nil, f.patchErr
		}
		return f.patchResult, nil
	default:
		return nil, errors.New("fakeAIService: unexpected prompt " + string(in.PromptID))
	}
}

type fakeBudgetTracker struct {
	exhausted bool
	reserved  int
	released  int
}

func (f *fakeBudgetTracker) Reserve(_ context.Context, _ uuid.UUID, amount int) error {
	if f.exhausted {
		return ai.ErrBudgetExhausted
	}
	f.reserved += amount
	return nil
}

func (f *fakeBudgetTracker) Release(_ context.Context, _ uuid.UUID, amount int) {
	f.released += amount
}

type fakeSuggestionRepo struct {
	upserted []ai.Suggestion
	err      error
}

func (f *fakeSuggestionRepo) Upsert(_ context.Context, s ai.Suggestion) error {
	if f.err != nil {
		return f.err
	}
	f.upserted = append(f.upserted, s)
	return nil
}

func explainOK() *ai.RunResult {
	return &ai.RunResult{
		Value: ai.ExplainFindingResponse{What: "w", WhyItMatters: "y", HowExploited: "h", Confidence: "high"},
		Model: "gemini-2.5-flash", TokensIn: 100, TokensOut: 50,
	}
}

func patchOK() *ai.RunResult {
	return &ai.RunResult{
		Value: ai.GeneratePatchResponse{Patch: "--- a\n+++ b\n", Explanation: "e", Confidence: "high"},
		Model: "gemini-2.5-pro", TokensIn: 200, TokensOut: 80,
	}
}

func testFinding(sev domain.Severity) domain.Finding {
	return domain.Finding{
		ID: uuid.New(), Engine: domain.EngineCodeScan, RuleID: "codescan.injection.example",
		Title: "t", Severity: sev, Remediation: "fix it",
		Location: domain.Location{Type: domain.LocationTypeFile, Path: "app.py", LineStart: 10},
	}
}

func TestEnricher_EnrichScan_Critical_GetsExplanationAndPatch(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK(), patchResult: patchOK()}
	budget := &fakeBudgetTracker{}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, budget, store, nil)

	f := testFinding(domain.SeverityCritical)
	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{f})

	if result.Enriched != 1 || result.SkippedForBudget != 0 {
		t.Fatalf("result = %+v, want Enriched:1", result)
	}
	if len(svc.calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2 (explain + patch)", len(svc.calls))
	}
	if svc.calls[0].PromptID != ai.PromptExplainFinding || svc.calls[1].PromptID != ai.PromptGeneratePatch {
		t.Errorf("call order = %v, %v — want explain then patch", svc.calls[0].PromptID, svc.calls[1].PromptID)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("len(upserted) = %d, want 1", len(store.upserted))
	}
	s := store.upserted[0]
	if s.FindingID != f.ID {
		t.Errorf("FindingID = %v, want %v", s.FindingID, f.ID)
	}
	if s.Explanation == "" {
		t.Error("Explanation is empty")
	}
	if s.PatchDiff == "" {
		t.Error("PatchDiff is empty, want the generated patch")
	}
	if s.PatchStatus != "unverified" {
		t.Errorf("PatchStatus = %q, want unverified", s.PatchStatus)
	}
	if s.TokensIn != 300 || s.TokensOut != 130 {
		t.Errorf("tokens = %d/%d, want 300/130 (explain + patch combined)", s.TokensIn, s.TokensOut)
	}
}

func TestEnricher_EnrichScan_Medium_GetsExplanationOnly(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK()}
	e := ai.NewEnricher(svc, &fakeBudgetTracker{}, &fakeSuggestionRepo{}, nil)

	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityMedium)})

	if result.Enriched != 1 {
		t.Fatalf("result = %+v, want Enriched:1", result)
	}
	if len(svc.calls) != 1 || svc.calls[0].PromptID != ai.PromptExplainFinding {
		t.Fatalf("calls = %v, want exactly one explain_finding call", svc.calls)
	}
}

func TestEnricher_EnrichScan_LowAndInformational_NeverCalled(t *testing.T) {
	svc := &fakeAIService{}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, &fakeBudgetTracker{}, store, nil)

	findings := []domain.Finding{testFinding(domain.SeverityLow), testFinding(domain.SeverityInformational)}
	result := e.EnrichScan(context.Background(), uuid.New(), findings)

	if result.Enriched != 0 || result.SkippedForBudget != 0 {
		t.Errorf("result = %+v, want both zero", result)
	}
	if len(svc.calls) != 0 {
		t.Errorf("calls = %v, want none — low/informational are never auto-enriched", svc.calls)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserted = %v, want none", store.upserted)
	}
}

func TestEnricher_EnrichScan_BudgetExhausted_SkipsWithoutCalling(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK()}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, &fakeBudgetTracker{exhausted: true}, store, nil)

	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityCritical)})

	if result.SkippedForBudget != 1 || result.Enriched != 0 {
		t.Fatalf("result = %+v, want SkippedForBudget:1", result)
	}
	if len(svc.calls) != 0 {
		t.Errorf("calls = %v, want none — budget exhausted before the call was ever made", svc.calls)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserted = %v, want none", store.upserted)
	}
}

// TestEnricher_EnrichScan_PriorityOrder_WorstSeverityFirst proves §8's
// "spend it where it matters": given findings out of order, and a budget
// that only covers the first call, the critical finding — not whichever
// came first in the input slice — must be the one that actually got
// enriched.
func TestEnricher_EnrichScan_PriorityOrder_WorstSeverityFirst(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK()}
	budget := &limitedBudget{callsAllowed: 1}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, budget, store, nil)

	mediumFirst := testFinding(domain.SeverityMedium)
	criticalSecond := testFinding(domain.SeverityCritical)
	e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{mediumFirst, criticalSecond})

	if len(store.upserted) != 1 {
		t.Fatalf("len(upserted) = %d, want 1", len(store.upserted))
	}
	if store.upserted[0].FindingID != criticalSecond.ID {
		t.Errorf("the enriched finding was %v, want the critical one (%v) even though it was second in the input", store.upserted[0].FindingID, criticalSecond.ID)
	}
}

// limitedBudget allows exactly callsAllowed Reserve calls to succeed, then
// exhausts — a stand-in for "the budget ran out partway through."
type limitedBudget struct {
	callsAllowed int
	calls        int
}

func (b *limitedBudget) Reserve(_ context.Context, _ uuid.UUID, _ int) error {
	b.calls++
	if b.calls > b.callsAllowed {
		return ai.ErrBudgetExhausted
	}
	return nil
}
func (b *limitedBudget) Release(context.Context, uuid.UUID, int) {}

func TestEnricher_EnrichScan_CacheHit_ReleasesReservation(t *testing.T) {
	cached := explainOK()
	cached.FromCache = true
	svc := &fakeAIService{explainResult: cached}
	budget := &fakeBudgetTracker{}
	e := ai.NewEnricher(svc, budget, &fakeSuggestionRepo{}, nil)

	e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityMedium)})

	if budget.reserved != budget.released {
		t.Errorf("reserved = %d, released = %d — a cache hit cost nothing real and should be fully released", budget.reserved, budget.released)
	}
}

// TestEnricher_EnrichScan_ExplainCallFails_ReleasesReservationAndCountsAsFailed
// is the near-miss half of the budget-vs-failure distinction: a genuine
// provider error must still degrade gracefully for the finding itself
// (doc §9 — no suggestion persisted, the scan isn't affected) but must be
// counted as a real Failed, not silently folded into SkippedForBudget —
// otherwise a real outage (every call erroring) reads identically to
// ordinary, expected budget management in the logs.
func TestEnricher_EnrichScan_ExplainCallFails_ReleasesReservationAndCountsAsFailed(t *testing.T) {
	svc := &fakeAIService{explainErr: errors.New("gemini: 503")}
	budget := &fakeBudgetTracker{}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, budget, store, nil)

	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityHigh)})

	if result.Failed != 1 {
		t.Errorf("result = %+v, want Failed:1 — a genuine provider error is not the same as an ordinary budget skip", result)
	}
	if result.SkippedForBudget != 0 {
		t.Errorf("result = %+v, want SkippedForBudget:0", result)
	}
	if budget.reserved != budget.released {
		t.Errorf("reserved = %d, released = %d — a failed call must give its reservation back", budget.reserved, budget.released)
	}
	if len(store.upserted) != 0 {
		t.Errorf("upserted = %v, want none", store.upserted)
	}
}

// TestEnricher_EnrichScan_BudgetExhaustion_CountedSeparatelyFromFailure is
// the true-positive half — an actual ErrBudgetExhausted must land in
// SkippedForBudget, not Failed, since it's the expected, non-error outcome
// §8 documents.
func TestEnricher_EnrichScan_BudgetExhaustion_CountedSeparatelyFromFailure(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK()}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, &fakeBudgetTracker{exhausted: true}, store, nil)

	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityHigh)})

	if result.SkippedForBudget != 1 {
		t.Errorf("result = %+v, want SkippedForBudget:1", result)
	}
	if result.Failed != 0 {
		t.Errorf("result = %+v, want Failed:0 — budget exhaustion is not a failure", result)
	}
}

// TestEnricher_EnrichScan_PatchFails_StillPersistsExplanationOnly is the
// partial-enrichment case: a critical/high finding's explanation succeeded
// but its patch call didn't — the explanation is still worth persisting
// rather than discarding everything.
func TestEnricher_EnrichScan_PatchFails_StillPersistsExplanationOnly(t *testing.T) {
	svc := &fakeAIService{explainResult: explainOK(), patchErr: errors.New("gemini: 503")}
	store := &fakeSuggestionRepo{}
	e := ai.NewEnricher(svc, &fakeBudgetTracker{}, store, nil)

	result := e.EnrichScan(context.Background(), uuid.New(), []domain.Finding{testFinding(domain.SeverityCritical)})

	if result.Enriched != 1 {
		t.Fatalf("result = %+v, want Enriched:1 (the explanation half still succeeded)", result)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("len(upserted) = %d, want 1", len(store.upserted))
	}
	s := store.upserted[0]
	if s.Explanation == "" {
		t.Error("Explanation is empty, want it persisted despite the patch failure")
	}
	if s.PatchDiff != "" {
		t.Errorf("PatchDiff = %q, want empty — the patch call failed", s.PatchDiff)
	}
	if s.PatchStatus != "not_applicable" {
		t.Errorf("PatchStatus = %q, want not_applicable", s.PatchStatus)
	}
}
