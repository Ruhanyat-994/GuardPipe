package scoring_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
)

func finding(engine domain.EngineID, sev domain.Severity, ruleID string) domain.Finding {
	return domain.Finding{
		ID:       uuid.New(),
		Engine:   engine,
		RuleID:   ruleID,
		Severity: sev,
		Status:   domain.StatusOpen,
	}
}

func withStatus(f domain.Finding, status domain.Status) domain.Finding {
	f.Status = status
	return f
}

func succeeded(engines ...domain.EngineID) []domain.ScanJob {
	jobs := make([]domain.ScanJob, len(engines))
	for i, e := range engines {
		jobs[i] = domain.ScanJob{ID: uuid.New(), Engine: e, Status: domain.JobStatusSucceeded}
	}
	return jobs
}

// TestCompute_WorkedExample replicates documentation/11-risk-scoring-and-
// severity.md §4 exactly, down to the rounded intermediate values, so a
// future change to a constant that breaks the documented example fails
// here rather than silently drifting from the spec.
func TestCompute_WorkedExample(t *testing.T) {
	var findings []domain.Finding
	add := func(engine domain.EngineID, sev domain.Severity, n int) {
		for range n {
			findings = append(findings, finding(engine, sev, string(engine)+".example.rule"))
		}
	}
	add(domain.EngineCodeScan, domain.SeverityCritical, 1)
	add(domain.EngineCodeScan, domain.SeverityHigh, 4)
	add(domain.EngineCodeScan, domain.SeverityMedium, 9)
	add(domain.EngineCodeScan, domain.SeverityLow, 4)
	add(domain.EngineCodeScan, domain.SeverityInformational, 2)

	add(domain.EngineDepScan, domain.SeverityCritical, 1)
	add(domain.EngineDepScan, domain.SeverityHigh, 3)
	add(domain.EngineDepScan, domain.SeverityMedium, 5)
	add(domain.EngineDepScan, domain.SeverityLow, 2)
	add(domain.EngineDepScan, domain.SeverityInformational, 3)

	add(domain.EngineK8sScan, domain.SeverityHigh, 2)
	add(domain.EngineK8sScan, domain.SeverityMedium, 6)
	add(domain.EngineK8sScan, domain.SeverityLow, 6)
	add(domain.EngineK8sScan, domain.SeverityInformational, 1)

	add(domain.EngineCICDScan, domain.SeverityMedium, 1)
	add(domain.EngineCICDScan, domain.SeverityLow, 2)
	add(domain.EngineCICDScan, domain.SeverityInformational, 1)

	jobs := succeeded(domain.EngineCodeScan, domain.EngineDepScan, domain.EngineK8sScan, domain.EngineCICDScan)
	// containerscan skipped (no Dockerfile), pentest not requested — neither
	// appears in jobs at all, matching how an unrequested/inapplicable
	// engine never gets a job row. docreview failed.
	jobs = append(jobs, domain.ScanJob{ID: uuid.New(), Engine: domain.EngineDocReview, Status: domain.JobStatusFailed})

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score != 70 {
		t.Errorf("Score = %d, want 70", got.Score)
	}
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %q, want block", got.Verdict)
	}
	if !got.IsPartial {
		t.Errorf("IsPartial = false, want true (docreview failed)")
	}
	wantEngineScores := map[domain.EngineID]int{
		domain.EngineCodeScan: 86,
		domain.EngineDepScan:  70,
		domain.EngineK8sScan:  42,
		domain.EngineCICDScan: 5,
	}
	for engine, want := range wantEngineScores {
		if got.EngineScores[engine] != want {
			t.Errorf("EngineScores[%s] = %d, want %d", engine, got.EngineScores[engine], want)
		}
	}
	if len(got.EngineScores) != len(wantEngineScores) {
		t.Errorf("EngineScores has %d entries, want %d: %v", len(got.EngineScores), len(wantEngineScores), got.EngineScores)
	}

	if len(got.Breakdown) != 6 { // floor + 4 engines + partial
		t.Fatalf("Breakdown has %d entries, want 6: %+v", len(got.Breakdown), got.Breakdown)
	}
	if got.Breakdown[0].Reason != scoring.ReasonCriticalFloor {
		t.Errorf("Breakdown[0].Reason = %q, want critical_floor_applied", got.Breakdown[0].Reason)
	}
	if got.Breakdown[0].Detail != "2 critical findings set a minimum score of 70" {
		t.Errorf("Breakdown[0].Detail = %q", got.Breakdown[0].Detail)
	}
	last := got.Breakdown[len(got.Breakdown)-1]
	if last.Reason != scoring.ReasonPartialScan {
		t.Errorf("last Breakdown entry Reason = %q, want partial_scan", last.Reason)
	}
	if last.Detail != "docreview failed — verdict cannot be pass" {
		t.Errorf("partial_scan Detail = %q", last.Detail)
	}
}

func TestCompute_CleanScanScoresZeroAndPasses(t *testing.T) {
	jobs := succeeded(domain.EngineCodeScan, domain.EngineDepScan, domain.EngineK8sScan,
		domain.EngineCICDScan, domain.EngineContainerScan, domain.EnginePentest, domain.EngineDocReview)

	got := scoring.NewDefaultScorer().Compute(nil, jobs)

	if got.Score != 0 {
		t.Errorf("Score = %d, want 0 for a scan with no findings", got.Score)
	}
	if got.Verdict != domain.VerdictPass {
		t.Errorf("Verdict = %q, want pass", got.Verdict)
	}
	if got.IsPartial {
		t.Errorf("IsPartial = true, want false — nothing failed")
	}
}

// TestCompute_LiveSecretFloorsAt90 is the fixture-one-secret calibration
// case (§8): an otherwise clean project with one hardcoded credential must
// block, regardless of how good everything else looks.
func TestCompute_LiveSecretFloorsAt90(t *testing.T) {
	findings := []domain.Finding{
		finding(domain.EngineDepScan, domain.SeverityCritical, "depscan.secrets.hardcoded-aws-key"),
	}
	jobs := succeeded(domain.EngineDepScan, domain.EngineCodeScan)

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score < 90 {
		t.Errorf("Score = %d, want >= 90 for a live credential secret", got.Score)
	}
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %q, want block", got.Verdict)
	}
	if got.Breakdown[0].Reason != scoring.ReasonCriticalFloor || got.Breakdown[0].Detail == "" {
		t.Errorf("expected a critical_floor_applied entry first, got %+v", got.Breakdown)
	}
}

// TestCompute_ThreeCriticalsFloorAt85AndNoSecretStaysAt85 checks the
// middle floor tier is distinct from both the 1-critical and the
// live-secret tiers.
func TestCompute_ThreeCriticalsFloorAt85AndNoSecretStaysAt85(t *testing.T) {
	findings := []domain.Finding{
		finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.sql-string-concat"),
		finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.command-injection"),
		finding(domain.EngineDepScan, domain.SeverityCritical, "depscan.vuln.no-fix-available"),
	}
	jobs := succeeded(domain.EngineCodeScan, domain.EngineDepScan)

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score < 85 {
		t.Errorf("Score = %d, want >= 85 for 3 criticals", got.Score)
	}
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("Verdict = %q, want block", got.Verdict)
	}
}

// TestCompute_SaturationKeepsNoisyScanLow is the fixture-noisy calibration
// case (§8): 200 low-severity findings and nothing serious must not
// outrank a real problem — saturation, not a linear sum, is what makes
// that true.
func TestCompute_SaturationKeepsNoisyScanLow(t *testing.T) {
	var findings []domain.Finding
	for range 200 {
		findings = append(findings, finding(domain.EngineCodeScan, domain.SeverityLow, "codescan.style.long-line"))
	}
	jobs := succeeded(domain.EngineCodeScan)

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score > 35 {
		t.Errorf("Score = %d, want <= 35 for 200 low-severity findings and nothing else (saturation)", got.Score)
	}
	if got.Verdict == domain.VerdictBlock {
		t.Errorf("Verdict = block, 200 low-severity findings alone must never block")
	}
}

// TestCompute_SkippedEngineGetsNoWeight is §3.5's renormalisation rule: an
// engine that never ran contributes neither credit nor blame — it must not
// silently improve the score just by not running.
func TestCompute_SkippedEngineGetsNoWeight(t *testing.T) {
	findings := []domain.Finding{
		finding(domain.EngineCodeScan, domain.SeverityHigh, "codescan.injection.example"),
	}
	// Only codescan ran; k8sscan/containerscan/etc. never got a job at all
	// (e.g. no Kubernetes manifests, no Dockerfile).
	jobs := succeeded(domain.EngineCodeScan)

	withOnlyCodescan := scoring.NewDefaultScorer().Compute(findings, jobs)

	if len(withOnlyCodescan.EngineScores) != 1 {
		t.Fatalf("EngineScores = %v, want exactly one entry", withOnlyCodescan.EngineScores)
	}
	// codescan is the only engine that ran, so after renormalisation it
	// carries 100% of the weight — the weighted score equals its own
	// sub-score exactly.
	if withOnlyCodescan.Score != withOnlyCodescan.EngineScores[domain.EngineCodeScan] {
		t.Errorf("Score = %d, want it to equal codescan's own sub-score %d when codescan is the only engine that ran",
			withOnlyCodescan.Score, withOnlyCodescan.EngineScores[domain.EngineCodeScan])
	}
}

// TestCompute_PartialScanCapsVerdictAtWarnNeverPass is §3.7: even a scan
// that would otherwise score a clean "pass" may not claim that if an
// engine failed outright — an unknown is not a zero.
func TestCompute_PartialScanCapsVerdictAtWarnNeverPass(t *testing.T) {
	jobs := []domain.ScanJob{
		{ID: uuid.New(), Engine: domain.EngineCodeScan, Status: domain.JobStatusSucceeded},
		{ID: uuid.New(), Engine: domain.EngineDepScan, Status: domain.JobStatusFailed},
	}

	got := scoring.NewDefaultScorer().Compute(nil, jobs)

	if got.Score != 0 {
		t.Errorf("Score = %d, want 0 (no findings)", got.Score)
	}
	if got.Verdict != domain.VerdictWarn {
		t.Errorf("Verdict = %q, want warn — a partial scan may never read pass even at score 0", got.Verdict)
	}
}

// TestCompute_SkippedJobIsNotPartial: a skip is an honest "nothing to
// check here", not a failure — it must not trigger the partial-scan cap.
func TestCompute_SkippedJobIsNotPartial(t *testing.T) {
	jobs := []domain.ScanJob{
		{ID: uuid.New(), Engine: domain.EngineCodeScan, Status: domain.JobStatusSucceeded},
		{ID: uuid.New(), Engine: domain.EngineContainerScan, Status: domain.JobStatusSkipped},
	}

	got := scoring.NewDefaultScorer().Compute(nil, jobs)

	if got.IsPartial {
		t.Errorf("IsPartial = true, want false — a skipped engine is not a failure")
	}
}

// TestCompute_SuppressedAndFalsePositiveFindingsDoNotScore is §5: these
// stay visible in the UI but must never move the number.
func TestCompute_SuppressedAndFalsePositiveFindingsDoNotScore(t *testing.T) {
	findings := []domain.Finding{
		withStatus(finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.example"), domain.StatusSuppressed),
		withStatus(finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.example2"), domain.StatusFalsePositive),
	}
	jobs := succeeded(domain.EngineCodeScan)

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score != 0 {
		t.Errorf("Score = %d, want 0 — both findings are excluded from scoring", got.Score)
	}
	if got.Verdict != domain.VerdictPass {
		t.Errorf("Verdict = %q, want pass", got.Verdict)
	}
}

// TestCompute_AcknowledgedFindingsStillScore is §5's explicit rule:
// acknowledging a risk is not fixing it.
func TestCompute_AcknowledgedFindingsStillScore(t *testing.T) {
	findings := []domain.Finding{
		withStatus(finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.example"), domain.StatusAcknowledged),
	}
	jobs := succeeded(domain.EngineCodeScan)

	got := scoring.NewDefaultScorer().Compute(findings, jobs)

	if got.Score < 70 {
		t.Errorf("Score = %d, want >= 70 — an acknowledged critical still counts", got.Score)
	}
}

// TestCompute_IsDeterministicRegardlessOfInputOrder is FR-SCR-006 directly:
// the same findings and jobs, given in a different order, must produce a
// bit-for-bit identical result.
func TestCompute_IsDeterministicRegardlessOfInputOrder(t *testing.T) {
	findings := []domain.Finding{
		finding(domain.EngineCodeScan, domain.SeverityCritical, "codescan.injection.a"),
		finding(domain.EngineDepScan, domain.SeverityHigh, "depscan.vuln.b"),
		finding(domain.EngineCodeScan, domain.SeverityMedium, "codescan.style.c"),
	}
	jobs := succeeded(domain.EngineCodeScan, domain.EngineDepScan)

	reversed := make([]domain.Finding, len(findings))
	for i, f := range findings {
		reversed[len(findings)-1-i] = f
	}

	a := scoring.NewDefaultScorer().Compute(findings, jobs)
	b := scoring.NewDefaultScorer().Compute(reversed, jobs)

	if a.Score != b.Score || a.Verdict != b.Verdict {
		t.Fatalf("Compute is not order-independent: %+v vs %+v", a, b)
	}
	for engine, score := range a.EngineScores {
		if b.EngineScores[engine] != score {
			t.Errorf("EngineScores[%s] differs by input order: %d vs %d", engine, score, b.EngineScores[engine])
		}
	}
}

// TestCompute_MoreFindingsNeverLowersTheScore is requirement 3
// (monotonic): adding a finding must never make the score better.
func TestCompute_MoreFindingsNeverLowersTheScore(t *testing.T) {
	base := []domain.Finding{
		finding(domain.EngineCodeScan, domain.SeverityMedium, "codescan.style.a"),
	}
	jobs := succeeded(domain.EngineCodeScan)

	before := scoring.NewDefaultScorer().Compute(base, jobs)

	withMore := append(append([]domain.Finding{}, base...),
		finding(domain.EngineCodeScan, domain.SeverityHigh, "codescan.injection.b"))
	after := scoring.NewDefaultScorer().Compute(withMore, jobs)

	if after.Score < before.Score {
		t.Errorf("Score decreased from %d to %d after adding a finding", before.Score, after.Score)
	}
}

func TestNewDefaultScorer_UsesDocumentedThresholds(t *testing.T) {
	cfg := scoring.DefaultConfig()
	if cfg.Thresholds.Warn != 30 || cfg.Thresholds.Block != 70 {
		t.Errorf("DefaultConfig thresholds = %+v, want Warn=30 Block=70", cfg.Thresholds)
	}
	if cfg.FormulaVersion != "1.0" {
		t.Errorf("DefaultConfig.FormulaVersion = %q, want 1.0", cfg.FormulaVersion)
	}
}
