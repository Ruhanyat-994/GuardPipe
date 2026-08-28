//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// seedCompletedScan creates a scan row already in `completed` status —
// GetPreviousScore only ever looks at completed scans, so tests need a way
// to seed one directly rather than driving a real job to completion.
func seedCompletedScan(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID) uuid.UUID {
	t.Helper()
	scans := repo.NewScanRepo(pool)
	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued}
	require.NoError(t, scans.Create(context.Background(), scan))
	_, err := pool.Exec(context.Background(), `UPDATE scans SET status = 'completed', finished_at = now() WHERE id = $1`, scan.ID)
	require.NoError(t, err)
	return scan.ID
}

func TestRiskAssessmentRepo_CreateThenGetByScanID_RoundTrips(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scanID := seedCompletedScan(t, pool, projectID)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	previous := 42
	rec := orchestrator.RiskAssessmentRecord{
		ScanID:  scanID,
		Score:   70,
		Verdict: domain.VerdictBlock,
		EngineScores: map[domain.EngineID]int{
			domain.EngineCodeScan: 86,
			domain.EngineDepScan:  70,
		},
		Breakdown: []scoring.Contribution{
			{Reason: scoring.ReasonCriticalFloor, Detail: "2 critical findings set a minimum score of 70", Impact: 16},
			{Reason: scoring.ReasonEngineContribution, Engine: domain.EngineCodeScan, Detail: "codescan contributed", Impact: 24.9},
		},
		PreviousScore:  &previous,
		IsPartial:      true,
		FormulaVersion: "1.0",
	}
	require.NoError(t, repoUnderTest.Create(ctx, rec))

	got, err := repoUnderTest.GetByScanID(ctx, scanID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, scanID, got.ScanID)
	require.Equal(t, 70, got.Score)
	require.Equal(t, domain.VerdictBlock, got.Verdict)
	require.Equal(t, 86, got.EngineScores[domain.EngineCodeScan])
	require.Equal(t, 70, got.EngineScores[domain.EngineDepScan])
	require.True(t, got.IsPartial)
	require.Equal(t, "1.0", got.FormulaVersion)
	require.NotNil(t, got.PreviousScore)
	require.Equal(t, 42, *got.PreviousScore)
	require.False(t, got.ComputedAt.IsZero())

	require.Len(t, got.Breakdown, 2)
	require.Equal(t, scoring.ReasonCriticalFloor, got.Breakdown[0].Reason)
	require.Equal(t, "2 critical findings set a minimum score of 70", got.Breakdown[0].Detail)
	require.InDelta(t, 16.0, got.Breakdown[0].Impact, 0.001)
	require.Equal(t, domain.EngineID(""), got.Breakdown[0].Engine, "the floor contribution isn't about one specific engine")
	require.Equal(t, scoring.ReasonEngineContribution, got.Breakdown[1].Reason)
	require.Equal(t, domain.EngineCodeScan, got.Breakdown[1].Engine)
	require.InDelta(t, 24.9, got.Breakdown[1].Impact, 0.001)
}

func TestRiskAssessmentRepo_GetByScanID_NoRowReturnsNilNotError(t *testing.T) {
	pool := setupTestDB(t)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	got, err := repoUnderTest.GetByScanID(context.Background(), id.New())
	require.NoError(t, err)
	require.Nil(t, got, "a scan with no assessment yet is a normal case, not an error")
}

// TestRiskAssessmentRepo_Create_IsIdempotentOnScanID proves the ON CONFLICT
// upsert: a second Create for the same scan_id (a redelivered job
// re-triggering finalization — see JobResultRepository's own doc comment)
// replaces the row rather than erroring or duplicating it.
func TestRiskAssessmentRepo_Create_IsIdempotentOnScanID(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scanID := seedCompletedScan(t, pool, projectID)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	require.NoError(t, repoUnderTest.Create(ctx, orchestrator.RiskAssessmentRecord{
		ScanID: scanID, Score: 40, Verdict: domain.VerdictWarn, FormulaVersion: "1.0",
	}))
	require.NoError(t, repoUnderTest.Create(ctx, orchestrator.RiskAssessmentRecord{
		ScanID: scanID, Score: 90, Verdict: domain.VerdictBlock, FormulaVersion: "1.0",
	}))

	got, err := repoUnderTest.GetByScanID(ctx, scanID)
	require.NoError(t, err)
	require.Equal(t, 90, got.Score, "the second Create must replace, not duplicate")
	require.Equal(t, domain.VerdictBlock, got.Verdict)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM risk_assessments WHERE scan_id = $1`, scanID).Scan(&count))
	require.Equal(t, 1, count, "exactly one row per scan_id, never two")
}

func TestRiskAssessmentRepo_GetPreviousScore_MostRecentCompletedScanExcludingCurrent(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	olderScanID := seedCompletedScan(t, pool, projectID)
	require.NoError(t, repoUnderTest.Create(ctx, orchestrator.RiskAssessmentRecord{ScanID: olderScanID, Score: 30, Verdict: domain.VerdictWarn, FormulaVersion: "1.0"}))

	// created_at defaults to now() on insert (documentation/06-database-design.md's
	// convention) — a second scan row for the same project a moment later is
	// still ordered after the first by created_at, matching real sequential
	// scan creation without needing to fake the clock.
	newerScanID := seedCompletedScan(t, pool, projectID)
	require.NoError(t, repoUnderTest.Create(ctx, orchestrator.RiskAssessmentRecord{ScanID: newerScanID, Score: 60, Verdict: domain.VerdictWarn, FormulaVersion: "1.0"}))

	currentScanID := seedCompletedScan(t, pool, projectID)

	got, err := repoUnderTest.GetPreviousScore(ctx, projectID, currentScanID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 60, *got, "must be the newer of the two prior scans, not the older one")
}

func TestRiskAssessmentRepo_GetPreviousScore_NoPriorScanReturnsNil(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	currentScanID := seedCompletedScan(t, pool, projectID)

	got, err := repoUnderTest.GetPreviousScore(ctx, projectID, currentScanID)
	require.NoError(t, err)
	require.Nil(t, got)
}

// TestRiskAssessmentRepo_GetPreviousScore_IgnoresNonCompletedScans: a
// cancelled or still-running scan never had a real score computed, so it
// must never surface as "the previous score" even if it's the most recent
// scan by created_at.
func TestRiskAssessmentRepo_GetPreviousScore_IgnoresNonCompletedScans(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	repoUnderTest := repo.NewRiskAssessmentRepo(pool)

	completedScanID := seedCompletedScan(t, pool, projectID)
	require.NoError(t, repoUnderTest.Create(ctx, orchestrator.RiskAssessmentRecord{ScanID: completedScanID, Score: 25, Verdict: domain.VerdictPass, FormulaVersion: "1.0"}))

	// A newer scan that's still queued — no risk assessment, and must not
	// be mistaken for "no previous score exists" by any join weirdness.
	scans := repo.NewScanRepo(pool)
	stillQueued := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued}
	require.NoError(t, scans.Create(ctx, stillQueued))

	currentScanID := seedCompletedScan(t, pool, projectID)

	got, err := repoUnderTest.GetPreviousScore(ctx, projectID, currentScanID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 25, *got, "the still-queued scan must be skipped, falling back to the one completed scan")
}
