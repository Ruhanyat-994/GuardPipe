//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func TestAISuggestionRepo_Upsert_RoundTrips(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	suggestions := repo.NewAISuggestionRepo(pool)
	seedRule(t, pool, "codescan.injection.sql-string-concat")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, RuleID: "codescan.injection.sql-string-concat",
		Fingerprint: "fp-suggestion", Title: "t", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "app.py"},
		Remediation: "use parameterised queries", Status: domain.StatusOpen,
	}
	_, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	generatedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, suggestions.Upsert(ctx, ai.Suggestion{
		FindingID: finding.ID, Explanation: "this is exploitable via string concatenation", PatchDiff: "--- a\n+++ b\n",
		PatchStatus: "unverified", Model: "gemini-2.5-flash", PromptVersion: "v1", InputHash: "abc123",
		TokensIn: 120, TokensOut: 60, GeneratedAt: generatedAt,
	}))

	var explanation, patchDiff, patchStatus, model, promptVersion, inputHash string
	var tokensIn, tokensOut int
	row := pool.QueryRow(ctx, `SELECT explanation, patch_diff, patch_status, model, prompt_version, input_hash, tokens_in, tokens_out FROM ai_suggestions WHERE finding_id = $1`, finding.ID)
	require.NoError(t, row.Scan(&explanation, &patchDiff, &patchStatus, &model, &promptVersion, &inputHash, &tokensIn, &tokensOut))
	require.Equal(t, "this is exploitable via string concatenation", explanation)
	require.Equal(t, "--- a\n+++ b\n", patchDiff)
	require.Equal(t, "unverified", patchStatus)
	require.Equal(t, "gemini-2.5-flash", model)
	require.Equal(t, "v1", promptVersion)
	require.Equal(t, "abc123", inputHash)
	require.Equal(t, 120, tokensIn)
	require.Equal(t, 60, tokensOut)
}

// TestAISuggestionRepo_Upsert_IsIdempotentOnFindingID proves the ON
// CONFLICT upsert: re-enriching the same finding (a redelivered job
// re-triggering enrichment, or a later on-demand explain call) replaces
// the row rather than violating the UNIQUE(finding_id) constraint.
func TestAISuggestionRepo_Upsert_IsIdempotentOnFindingID(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	suggestions := repo.NewAISuggestionRepo(pool)
	seedRule(t, pool, "codescan.injection.sql-string-concat")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, RuleID: "codescan.injection.sql-string-concat",
		Fingerprint: "fp-idempotent", Title: "t", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "app.py"},
		Remediation: "r", Status: domain.StatusOpen,
	}
	_, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	require.NoError(t, suggestions.Upsert(ctx, ai.Suggestion{FindingID: finding.ID, Explanation: "first", PatchStatus: "not_applicable", Model: "m1", PromptVersion: "v1", InputHash: "h1", GeneratedAt: time.Now().UTC()}))
	require.NoError(t, suggestions.Upsert(ctx, ai.Suggestion{FindingID: finding.ID, Explanation: "second", PatchStatus: "not_applicable", Model: "m2", PromptVersion: "v1", InputHash: "h2", GeneratedAt: time.Now().UTC()}))

	var explanation string
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM ai_suggestions WHERE finding_id = $1`, finding.ID).Scan(&count))
	require.Equal(t, 1, count, "exactly one row per finding_id, never two")
	require.NoError(t, pool.QueryRow(ctx, `SELECT explanation FROM ai_suggestions WHERE finding_id = $1`, finding.ID).Scan(&explanation))
	require.Equal(t, "second", explanation, "the second Upsert must replace, not duplicate")
}
