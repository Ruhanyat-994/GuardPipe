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
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func TestProjectWebhookRepo_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	r := repo.NewProjectWebhookRepo(pool)

	w := &livescan.Webhook{
		ID: id.New(), ProjectID: projectID, GitHubHookID: 123456,
		SecretCiphertext: []byte("ciphertext"), SecretNonce: []byte("nonce-12byte"),
		Engines:         []domain.EngineID{domain.EngineCodeScan, domain.EngineK8sScan},
		WatchedBranches: []string{"main", "develop"}, AttestedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	require.NoError(t, r.Create(ctx, w))

	got, err := r.GetByProjectID(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, w.ID, got.ID)
	require.EqualValues(t, 123456, got.GitHubHookID)
	require.Equal(t, w.Engines, got.Engines)
	require.Equal(t, w.WatchedBranches, got.WatchedBranches)
	require.Nil(t, got.PausedReason)

	now := time.Now().UTC()
	require.NoError(t, r.RecordDelivery(ctx, w.ID, now, "accepted"))
	require.NoError(t, r.Pause(ctx, w.ID, "too many triggers", now))
	got, err = r.GetByID(ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, "accepted", got.LastDeliveryStatus)
	require.NotNil(t, got.PausedReason)

	got.Engines = []domain.EngineID{domain.EngineDepScan}
	require.NoError(t, r.UpdateConfig(ctx, got))
	got, err = r.GetByID(ctx, w.ID)
	require.NoError(t, err)
	require.Nil(t, got.PausedReason, "re-confirming clears the pause")
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, got.Engines)

	require.NoError(t, r.Delete(ctx, w.ID))
	_, err = r.GetByID(ctx, w.ID)
	require.Error(t, err)
}

// The database itself refuses a live-scanning row that would run pentest —
// the last line of defence behind the service's own checks.
func TestProjectWebhookRepo_DatabaseRejectsPentest(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	r := repo.NewProjectWebhookRepo(pool)

	err := r.Create(ctx, &livescan.Webhook{
		ID: id.New(), ProjectID: projectID, GitHubHookID: 1,
		SecretCiphertext: []byte("c"), SecretNonce: []byte("n"),
		Engines:         []domain.EngineID{domain.EngineCodeScan, domain.EnginePentest},
		WatchedBranches: []string{"main"}, AttestedAt: time.Now(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "project_webhooks_no_pentest")
}

func TestScanRepo_TriggerColumnsRoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	ref, actor := "refs/pull/12/head", "octocat"
	s := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: domain.ScanTypePartial, Status: domain.ScanStatusQueued,
		RequestedEngines: []domain.EngineID{domain.EngineCodeScan}, FindingCounts: map[domain.Severity]int{},
		TriggerSource: domain.TriggerWebhookPullRequest, TriggerRef: &ref, TriggerActor: &actor,
	}
	require.NoError(t, scans.Create(ctx, s))

	got, err := scans.GetByID(ctx, s.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TriggerWebhookPullRequest, got.TriggerSource)
	require.Equal(t, ref, *got.TriggerRef)
	require.Equal(t, actor, *got.TriggerActor)

	// A scan with no recorded origin (pre-migration shape) stays empty.
	plain := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: domain.ScanTypePartial, Status: domain.ScanStatusQueued,
		RequestedEngines: []domain.EngineID{domain.EngineCodeScan}, FindingCounts: map[domain.Severity]int{},
	}
	require.NoError(t, scans.Create(ctx, plain))
	got, err = scans.GetByID(ctx, plain.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TriggerSource(""), got.TriggerSource)
	require.Nil(t, got.TriggerRef)
}
