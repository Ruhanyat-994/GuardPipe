// Package scoring turns a scan's findings into the single 0-100 risk score
// and pass/warn/block verdict the rest of GuardPipe is built around
// (documentation/11-risk-scoring-and-severity.md, documentation/05-module-
// specifications.md §14). Compute is a pure function — no I/O, no clock, no
// randomness (FR-SCR-006): the same findings and job statuses always produce
// the identical RiskAssessment, which is what makes the number trustworthy
// and the package trivially unit-testable in isolation.
//
// This package does not fetch findings, does not persist a RiskAssessment,
// and does not know about scan IDs — that orchestration (loading a scan's
// findings/jobs, writing the result to risk_assessments, computing Delta
// against the previous scan) is deliberately left to whichever caller wires
// this in later; see BUILD_GUIDE.md Phase 13.
package scoring

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// ContributionReason names why one entry appears in a RiskAssessment's
// Breakdown — documentation/11-risk-scoring-and-severity.md §4's worked
// example is the source of truth for these three.
type ContributionReason string

const (
	ReasonEngineContribution ContributionReason = "engine_contribution"
	ReasonCriticalFloor      ContributionReason = "critical_floor_applied"
	ReasonPartialScan        ContributionReason = "partial_scan"
)

// Contribution is one line of the human-readable explanation rendered under
// the score gauge (§4). Engine is empty for reasons that aren't about one
// specific engine (the floor, the partial-scan note).
type Contribution struct {
	Reason ContributionReason
	Engine domain.EngineID
	Detail string
	Impact float64
}

// RiskAssessment is this package's one exported result type — the shape
// documentation/05-module-specifications.md §14's Service.Compute pseudocode
// describes, minus the fields (Delta, and persistence) that belong to the
// not-yet-built caller that has access to the previous scan's stored score.
type RiskAssessment struct {
	Score          int // 0-100, higher is worse
	Verdict        domain.Verdict
	EngineScores   map[domain.EngineID]int // rounded per-engine sub-scores, for display
	Breakdown      []Contribution
	IsPartial      bool // true if any engine job failed (§3.7)
	FormulaVersion string
}

// Thresholds are the score bands a Verdict is derived from (§3.8),
// configurable per GUARDPIPE_GATE_WARN/GUARDPIPE_GATE_BLOCK
// (internal/platform/config's Gate struct) — the only part of the formula
// with a documented environment variable. Everything else in Config is a
// calibrated constant with no override knob today.
type Thresholds struct {
	// Warn is the lowest score that produces a "warn" verdict; anything
	// below is "pass".
	Warn int
	// Block is the lowest score that produces a "block" verdict; anything
	// from Warn up to Block-1 is "warn".
	Block int
}

// Config is every constant the formula needs, injected rather than
// hardcoded so Compute stays a pure function of its arguments plus this
// value (documentation/11-risk-scoring-and-severity.md §9's "Configurable"
// requirement). DefaultConfig returns the calibrated values §3 itself
// documents; callers only override Thresholds today.
type Config struct {
	// SeverityWeights is `w` in §3.2 — informational is present at weight 0
	// so it is still shown/counted upstream but never moves the score.
	SeverityWeights map[domain.Severity]float64
	// SaturationConstants is `k` in §3.3, one per non-zero-weight severity.
	SaturationConstants map[domain.Severity]float64
	// RMax is the raw per-engine score (§3.3's output) treated as maximally
	// bad — §3.4's normalisation divisor.
	RMax float64
	// EngineWeights is §3.5's per-engine importance, summing to 1.00 across
	// all seven engines before any renormalisation.
	EngineWeights map[domain.EngineID]float64
	Thresholds    Thresholds
	// FormulaVersion is stamped onto every RiskAssessment (§7) — bump it
	// whenever any constant in this Config changes, and never recompute a
	// historical assessment under a new version.
	FormulaVersion string
}

// DefaultConfig returns the constants documentation/11-risk-scoring-and-
// severity.md §3 and §8 calibrate against the fixture repositories. Do not
// change these values without bumping FormulaVersion (§7) — a silent
// constant change makes every stored historical score mean something
// different than what its own formula_version claims.
func DefaultConfig() Config {
	return Config{
		SeverityWeights: map[domain.Severity]float64{
			domain.SeverityCritical:      40,
			domain.SeverityHigh:          15,
			domain.SeverityMedium:        4,
			domain.SeverityLow:           1,
			domain.SeverityInformational: 0,
		},
		SaturationConstants: map[domain.Severity]float64{
			domain.SeverityCritical: 2,
			domain.SeverityHigh:     5,
			domain.SeverityMedium:   15,
			domain.SeverityLow:      40,
		},
		RMax: 120,
		EngineWeights: map[domain.EngineID]float64{
			domain.EngineCodeScan:      0.22,
			domain.EngineDepScan:       0.20,
			domain.EngineK8sScan:       0.18,
			domain.EngineCICDScan:      0.16,
			domain.EngineContainerScan: 0.14,
			domain.EnginePentest:       0.08,
			domain.EngineDocReview:     0.02,
		},
		Thresholds:     Thresholds{Warn: 30, Block: 70},
		FormulaVersion: "1.0",
	}
}
