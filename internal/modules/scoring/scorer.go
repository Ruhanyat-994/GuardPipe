package scoring

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// countedStatuses are the finding statuses that still count toward the
// score (§5): suppressed, false_positive, and fixed findings are excluded
// (visible in the UI, but never scored — acknowledging a risk does not
// remove it, so acknowledged still counts).
var countedStatuses = map[domain.Status]bool{
	domain.StatusOpen:         true,
	domain.StatusAcknowledged: true,
}

// scoredSeverities is the fixed processing order for the saturating sum
// (§3.3). informational is never included — its weight is always 0, so
// summing it would be a no-op that only costs a map lookup.
var scoredSeverities = []domain.Severity{
	domain.SeverityCritical,
	domain.SeverityHigh,
	domain.SeverityMedium,
	domain.SeverityLow,
}

// Scorer computes RiskAssessments under one fixed Config. Stateless beyond
// that config — safe to share across concurrent Compute calls.
type Scorer struct {
	cfg Config
}

func NewScorer(cfg Config) *Scorer {
	return &Scorer{cfg: cfg}
}

func NewDefaultScorer() *Scorer {
	return NewScorer(DefaultConfig())
}

// Compute derives a RiskAssessment from one scan's findings and the
// terminal status of each of its jobs (documentation/11-risk-scoring-and-
// severity.md §3). Pure: no I/O, no clock, no randomness — the same two
// slices always produce an identical result (FR-SCR-006). Findings are
// sorted before processing and every downstream aggregation iterates in a
// fixed order, so Go's randomised map iteration never leaks into the
// result.
func (s *Scorer) Compute(findings []domain.Finding, jobs []domain.ScanJob) RiskAssessment {
	counted := countedFindings(findings)
	sort.Slice(counted, func(i, j int) bool { return counted[i].ID.String() < counted[j].ID.String() })

	ran, failed := jobOutcomes(jobs)
	counts := countBySeverity(counted)

	engineScores := make(map[domain.EngineID]int, len(ran))
	for e := range ran {
		engineScores[e] = s.subscore(s.rawScore(counts[e]))
	}

	scoreWeighted, engineContributions := s.weightedAggregate(ran, engineScores)
	floor, floorDetail := s.criticalFloor(counted)

	finalScore := int(math.Round(math.Max(scoreWeighted, float64(floor))))

	breakdown := make([]Contribution, 0, len(engineContributions)+2)
	if floor > 0 && float64(floor) > scoreWeighted {
		breakdown = append(breakdown, Contribution{
			Reason: ReasonCriticalFloor,
			Detail: floorDetail,
			Impact: math.Round(float64(floor) - scoreWeighted),
		})
	}
	breakdown = append(breakdown, engineContributions...)

	isPartial := len(failed) > 0
	if isPartial {
		names := make([]string, len(failed))
		for i, e := range failed {
			names[i] = string(e)
		}
		breakdown = append(breakdown, Contribution{
			Reason: ReasonPartialScan,
			Detail: fmt.Sprintf("%s failed — verdict cannot be pass", strings.Join(names, ", ")),
			Impact: 0,
		})
	}

	return RiskAssessment{
		Score:          finalScore,
		Verdict:        s.verdict(finalScore, isPartial),
		EngineScores:   engineScores,
		Breakdown:      breakdown,
		IsPartial:      isPartial,
		FormulaVersion: s.cfg.FormulaVersion,
	}
}

// countedFindings filters to the statuses §5 says still count toward the
// score.
func countedFindings(in []domain.Finding) []domain.Finding {
	out := make([]domain.Finding, 0, len(in))
	for _, f := range in {
		if countedStatuses[f.Status] {
			out = append(out, f)
		}
	}
	return out
}

// jobOutcomes splits jobs into the engines that actually ran (succeeded —
// §3.5's renormalisation only credits/blames an engine that produced a
// result) and the engines that failed outright (§3.7 — as opposed to
// skipped, which is neither ran nor failed).
func jobOutcomes(jobs []domain.ScanJob) (ran map[domain.EngineID]bool, failed []domain.EngineID) {
	ran = make(map[domain.EngineID]bool)
	for _, j := range jobs {
		switch j.Status {
		case domain.JobStatusSucceeded:
			ran[j.Engine] = true
		case domain.JobStatusFailed:
			failed = append(failed, j.Engine)
		}
	}
	slices.Sort(failed)
	return ran, failed
}

func countBySeverity(findings []domain.Finding) map[domain.EngineID]map[domain.Severity]int {
	out := make(map[domain.EngineID]map[domain.Severity]int)
	for _, f := range findings {
		m, ok := out[f.Engine]
		if !ok {
			m = make(map[domain.Severity]int)
			out[f.Engine] = m
		}
		m[f.Severity]++
	}
	return out
}

// saturate is §3.3's saturating curve: the first finding of a class counts
// almost fully, the tenth counts very little.
func saturate(n, k float64) float64 {
	return k * (1 - math.Exp(-n/k))
}

// rawScore is §3.3's per-engine raw score: a saturating weighted sum over
// every scored severity.
func (s *Scorer) rawScore(counts map[domain.Severity]int) float64 {
	var raw float64
	for _, sev := range scoredSeverities {
		n := counts[sev]
		if n == 0 {
			continue
		}
		w := s.cfg.SeverityWeights[sev]
		k := s.cfg.SaturationConstants[sev]
		raw += w * saturate(float64(n), k)
	}
	return raw
}

// subscore is §3.4's normalisation to 0-100, rounded here (not just at the
// very end) so it matches §4's worked example, where the rounded per-engine
// sub-score — not the raw float — is what feeds the weighted aggregate.
// Rounding it exactly once, at this one step, and then reusing that same
// rounded value everywhere downstream (the aggregate and the breakdown) is
// what keeps the breakdown summing to the score without drift, honouring
// §9's rounding-discipline requirement.
func (s *Scorer) subscore(raw float64) int {
	sub := 100 * raw / s.cfg.RMax
	if sub > 100 {
		sub = 100
	}
	return int(math.Round(sub))
}

// weightedAggregate is §3.5: renormalise engine weights across only the
// engines that ran, then sum. Returns the unrounded weighted score (rounded
// once, at the very end, by Compute) plus one ordered Contribution per
// engine, heaviest weight first.
func (s *Scorer) weightedAggregate(ran map[domain.EngineID]bool, engineScores map[domain.EngineID]int) (float64, []Contribution) {
	type weighted struct {
		id     domain.EngineID
		weight float64
	}
	list := make([]weighted, 0, len(ran))
	var sumWeight float64
	for e := range ran {
		w := s.cfg.EngineWeights[e]
		list = append(list, weighted{e, w})
		sumWeight += w
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		return list[i].id < list[j].id
	})

	if sumWeight == 0 {
		return 0, nil
	}

	var scoreWeighted float64
	contributions := make([]Contribution, 0, len(list))
	for _, we := range list {
		normWeight := we.weight / sumWeight
		sub := engineScores[we.id]
		impact := normWeight * float64(sub)
		scoreWeighted += impact
		contributions = append(contributions, Contribution{
			Reason: ReasonEngineContribution,
			Engine: we.id,
			Detail: fmt.Sprintf("%s contributed a sub-score of %d, weighted %.0f%% after renormalisation", we.id, sub, normWeight*100),
			Impact: math.Round(impact*10) / 10,
		})
	}
	return scoreWeighted, contributions
}

// criticalFloor is §3.6: a weighted average can dilute a single
// catastrophic finding, so a floor overrides it. The three conditions are
// evaluated worst-first since only the highest applicable floor matters.
func (s *Scorer) criticalFloor(counted []domain.Finding) (floor int, detail string) {
	var criticalCount int
	var hasSecretCritical bool
	for _, f := range counted {
		if f.Severity != domain.SeverityCritical {
			continue
		}
		criticalCount++
		// Rule IDs are namespaced "<engine>.<category>.<rule>" — a secrets
		// rule always carries ".secrets." as its category segment.
		if strings.Contains(f.RuleID, ".secrets.") {
			hasSecretCritical = true
		}
	}

	switch {
	case hasSecretCritical:
		return 90, "a live credential secret sets a minimum score of 90"
	case criticalCount >= 3:
		return 85, fmt.Sprintf("%d critical findings set a minimum score of 85", criticalCount)
	case criticalCount >= 1:
		plural := "s"
		if criticalCount == 1 {
			plural = ""
		}
		return 70, fmt.Sprintf("%d critical finding%s set a minimum score of 70", criticalCount, plural)
	default:
		return 0, ""
	}
}

// verdict is §3.8, with §3.7's partial-scan cap: a scan where any engine
// failed outright may never read "pass" — an unknown is not a zero.
func (s *Scorer) verdict(score int, isPartial bool) domain.Verdict {
	var v domain.Verdict
	switch {
	case score >= s.cfg.Thresholds.Block:
		v = domain.VerdictBlock
	case score >= s.cfg.Thresholds.Warn:
		v = domain.VerdictWarn
	default:
		v = domain.VerdictPass
	}
	if isPartial && v == domain.VerdictPass {
		v = domain.VerdictWarn
	}
	return v
}
