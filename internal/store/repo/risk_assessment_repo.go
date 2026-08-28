package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
)

// RiskAssessmentRepo implements orchestrator.RiskAssessmentRepository
// against the `risk_assessments` table (documentation/06-database-design.md
// §4.17).
type RiskAssessmentRepo struct {
	db Querier
}

func NewRiskAssessmentRepo(db Querier) *RiskAssessmentRepo {
	return &RiskAssessmentRepo{db: db}
}

var _ orchestrator.RiskAssessmentRepository = (*RiskAssessmentRepo)(nil)

// breakdownEntry is the JSONB shape for one Breakdown row — field names
// match documentation/11-risk-scoring-and-severity.md §4's worked JSON
// example (reason/detail/impact, engine present only when the contribution
// is about one specific engine) rather than scoring.Contribution's own Go
// field names, since that package deliberately carries no encoding tags of
// its own (types.go's doc comment: it's a pure computation result, not a
// wire format).
type breakdownEntry struct {
	Reason string  `json:"reason"`
	Engine string  `json:"engine,omitempty"`
	Detail string  `json:"detail"`
	Impact float64 `json:"impact"`
}

// Create is idempotent on scan_id (ON CONFLICT DO UPDATE) — a redelivered
// or reclaimed job can plausibly trigger PersistJobResult's finalization
// signal a second time for the same scan (see JobResultRepository's own
// doc comment), and a second Create must replace the row, not fail or
// duplicate it, mirroring how insertFindings already treats a re-run as a
// normal case rather than an error.
func (r *RiskAssessmentRepo) Create(ctx context.Context, rec orchestrator.RiskAssessmentRecord) error {
	engineScores := make(map[string]int, len(rec.EngineScores))
	for e, s := range rec.EngineScores {
		engineScores[string(e)] = s
	}
	engineScoresJSON, err := json.Marshal(engineScores)
	if err != nil {
		return fmt.Errorf("repo: encode engine scores: %w", err)
	}

	breakdown := make([]breakdownEntry, len(rec.Breakdown))
	for i, c := range rec.Breakdown {
		breakdown[i] = breakdownEntry{Reason: string(c.Reason), Engine: string(c.Engine), Detail: c.Detail, Impact: c.Impact}
	}
	breakdownJSON, err := json.Marshal(breakdown)
	if err != nil {
		return fmt.Errorf("repo: encode breakdown: %w", err)
	}

	const q = `
		INSERT INTO risk_assessments (scan_id, score, verdict, engine_scores, breakdown, previous_score, is_partial, formula_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (scan_id) DO UPDATE SET
			score = EXCLUDED.score, verdict = EXCLUDED.verdict, engine_scores = EXCLUDED.engine_scores,
			breakdown = EXCLUDED.breakdown, previous_score = EXCLUDED.previous_score,
			is_partial = EXCLUDED.is_partial, formula_version = EXCLUDED.formula_version, computed_at = now()`
	_, err = r.db.Exec(ctx, q, rec.ScanID, rec.Score, string(rec.Verdict), engineScoresJSON, breakdownJSON, rec.PreviousScore, rec.IsPartial, rec.FormulaVersion)
	if err != nil {
		return fmt.Errorf("repo: insert risk assessment: %w", err)
	}
	return nil
}

func (r *RiskAssessmentRepo) GetByScanID(ctx context.Context, scanID uuid.UUID) (*orchestrator.RiskAssessmentRecord, error) {
	const q = `
		SELECT scan_id, score, verdict, engine_scores, breakdown, previous_score, is_partial, formula_version, computed_at
		FROM risk_assessments WHERE scan_id = $1`

	var rec orchestrator.RiskAssessmentRecord
	var verdict string
	var engineScoresJSON, breakdownJSON []byte
	err := r.db.QueryRow(ctx, q, scanID).Scan(
		&rec.ScanID, &rec.Score, &verdict, &engineScoresJSON, &breakdownJSON,
		&rec.PreviousScore, &rec.IsPartial, &rec.FormulaVersion, &rec.ComputedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not every scan has one yet — still running, or finished before
			// scoring existed (a pre-Phase-13 row).
			return nil, nil
		}
		return nil, fmt.Errorf("repo: get risk assessment: %w", err)
	}
	rec.Verdict = domain.Verdict(verdict)

	var engineScores map[string]int
	if err := json.Unmarshal(engineScoresJSON, &engineScores); err != nil {
		return nil, fmt.Errorf("repo: decode engine scores: %w", err)
	}
	rec.EngineScores = make(map[domain.EngineID]int, len(engineScores))
	for e, s := range engineScores {
		rec.EngineScores[domain.EngineID(e)] = s
	}

	var breakdown []breakdownEntry
	if err := json.Unmarshal(breakdownJSON, &breakdown); err != nil {
		return nil, fmt.Errorf("repo: decode breakdown: %w", err)
	}
	rec.Breakdown = make([]scoring.Contribution, len(breakdown))
	for i, b := range breakdown {
		rec.Breakdown[i] = scoring.Contribution{
			Reason: scoring.ContributionReason(b.Reason), Engine: domain.EngineID(b.Engine), Detail: b.Detail, Impact: b.Impact,
		}
	}

	return &rec, nil
}

// GetPreviousScore is documentation/11-risk-scoring-and-severity.md §6's
// Delta source: the score of the most recent prior *completed* scan for
// the same project, excluding the scan being scored right now. A scan
// still in progress, cancelled, or failed outright never had a real score
// computed for it, so it can't be "the previous score" either.
func (r *RiskAssessmentRepo) GetPreviousScore(ctx context.Context, projectID, excludeScanID uuid.UUID) (*int, error) {
	const q = `
		SELECT ra.score
		FROM risk_assessments ra
		JOIN scans s ON s.id = ra.scan_id
		WHERE s.project_id = $1 AND s.id != $2 AND s.status = $3
		ORDER BY s.created_at DESC
		LIMIT 1`
	var score int
	err := r.db.QueryRow(ctx, q, projectID, excludeScanID, string(domain.ScanStatusCompleted)).Scan(&score)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("repo: get previous score: %w", err)
	}
	return &score, nil
}
