//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func TestAuditRepo_Insert_RoundTrips(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, pool)
	audits := repo.NewAuditRepo(pool)

	actorID := id.New()
	resourceID := id.New()
	err := audits.Insert(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &actorID, Action: "project.created",
		ResourceType: strPtrForTest("project"), ResourceID: &resourceID,
		Detail: map[string]any{"name": "Payments API"},
	})
	require.NoError(t, err)

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'project.created' AND org_id = $1`, orgID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestAuditRepo_Insert_AllowsNullActorAndResource(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	audits := repo.NewAuditRepo(pool)

	// A system action — no actor, no resource, no org.
	err := audits.Insert(ctx, audit.Entry{Action: "system.startup"})
	require.NoError(t, err)

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'system.startup'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func strPtrForTest(s string) *string {
	return &s
}
