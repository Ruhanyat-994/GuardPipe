//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// TestOrganizationRepo_AdminMethods_RoundTrip exercises admin.OrganizationRepository
// against store/repo.OrganizationRepo — the same struct that already
// implements identity.OrganizationRepository against the same table
// (BUILD_GUIDE.md Phase 14).
func TestOrganizationRepo_AdminMethods_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	orgs := repo.NewOrganizationRepo(pool)

	summary, err := orgs.GetByID(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, orgID, summary.ID)
	require.Equal(t, 1, summary.MemberCount)
	require.Nil(t, summary.SuspendedAt)

	list, total, err := orgs.ListAll(ctx, "", admin.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 1)
	require.NotEmpty(t, list)

	reason := "ToS violation"
	now := time.Now().UTC()
	require.NoError(t, orgs.SetSuspended(ctx, orgID, &now, &reason))

	suspended, err := orgs.GetByID(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, suspended.SuspendedAt)
	require.Equal(t, reason, *suspended.SuspendedReason)

	require.NoError(t, orgs.SetSuspended(ctx, orgID, nil, nil))
	reinstated, err := orgs.GetByID(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, reinstated.SuspendedAt)

	_ = userID
}

func TestUserRepo_AdminMethods_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	users := repo.NewUserRepo(pool)

	members, err := users.ListByOrg(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, userID, members[0].ID)

	summary, err := users.GetSummaryByID(ctx, userID)
	require.NoError(t, err)
	require.Nil(t, summary.SuspendedAt)

	reason := "abusive behaviour"
	now := time.Now().UTC()
	require.NoError(t, users.SetSuspended(ctx, userID, &now, &reason))

	suspended, err := users.GetSummaryByID(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, suspended.SuspendedAt)
	require.Equal(t, reason, *suspended.SuspendedReason)
}

// TestUserRepo_Suspension_VisibleThroughIdentityGetByID confirms the two
// interfaces this struct implements (identity.UserRepository and
// admin.UserRepository) really do read/write the same underlying columns —
// this is the one thing a unit test with fakes cannot prove.
func TestUserRepo_Suspension_VisibleThroughIdentityGetByID(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	_, userID := seedOrgAndUser(t, pool)
	users := repo.NewUserRepo(pool)

	reason := "ToS violation"
	now := time.Now().UTC()
	require.NoError(t, users.SetSuspended(ctx, userID, &now, &reason))

	identityUser, err := users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, identityUser.SuspendedAt)
	require.Equal(t, reason, *identityUser.SuspendedReason)
}

func TestPlatformOperatorRepo_GrantIsOperatorRevoke_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	_, userID := seedOrgAndUser(t, pool)
	operators := repo.NewPlatformOperatorRepo(pool)

	isOp, err := operators.IsOperator(ctx, userID)
	require.NoError(t, err)
	require.False(t, isOp)

	_, err = operators.Grant(ctx, userID, nil, "founding team member")
	require.NoError(t, err)

	isOp, err = operators.IsOperator(ctx, userID)
	require.NoError(t, err)
	require.True(t, isOp)

	require.NoError(t, operators.Revoke(ctx, userID))
	isOp, err = operators.IsOperator(ctx, userID)
	require.NoError(t, err)
	require.False(t, isOp)

	err = operators.Revoke(ctx, userID)
	require.ErrorIs(t, err, repo.ErrNotAnOperator)
}

func TestTargetRepo_GetTargetInfo_JoinsProjectAndOrg(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)
	targets := repo.NewTargetRepo(pool)

	p := &project.Project{ID: id.New(), OrgID: orgID, Name: "Payments API", Status: project.StatusActive, CreatedBy: &userID}
	require.NoError(t, projects.Create(ctx, p))

	tgt := &project.Target{
		ID: id.New(), ProjectID: p.ID, TargetInput: "https://staging.acme.example",
		NormalizedHost: "staging.acme.example",
		PinnedIPs:      []netip.Addr{netip.MustParseAddr("203.0.113.10")},
		Status:         project.TargetAwaitingAttestation, LastResolvedAt: time.Now().UTC(),
	}
	require.NoError(t, targets.Create(ctx, tgt))

	info, err := targets.GetTargetInfo(ctx, tgt.ID)
	require.NoError(t, err)
	require.Equal(t, "staging.acme.example", info.Host)
	require.Equal(t, p.ID, info.ProjectID)
	require.Equal(t, "Payments API", info.ProjectName)
	require.Equal(t, orgID, info.OrgID)
}

func TestPentestFlagRepo_CreateGetListUpdateStatus_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)
	targets := repo.NewTargetRepo(pool)
	flags := repo.NewPentestFlagRepo(pool)

	p := &project.Project{ID: id.New(), OrgID: orgID, Name: "Payments API", Status: project.StatusActive, CreatedBy: &userID}
	require.NoError(t, projects.Create(ctx, p))
	tgt := &project.Target{
		ID: id.New(), ProjectID: p.ID, TargetInput: "https://staging.acme.example",
		NormalizedHost: "staging.acme.example",
		PinnedIPs:      []netip.Addr{netip.MustParseAddr("203.0.113.10")},
		Status:         project.TargetAwaitingAttestation, LastResolvedAt: time.Now().UTC(),
	}
	require.NoError(t, targets.Create(ctx, tgt))

	flag := &admin.PentestFlag{
		ID: id.New(), TargetID: tgt.ID, Status: admin.FlagOpen,
		Source: admin.FlagSourceSelfReported, Reason: "scanned my server without authorization",
		ReportedBy: &userID,
	}
	require.NoError(t, flags.Create(ctx, flag))

	got, err := flags.GetByID(ctx, flag.ID)
	require.NoError(t, err)
	require.Equal(t, admin.FlagOpen, got.Status)

	openStatus := admin.FlagOpen
	list, total, err := flags.List(ctx, &openStatus, admin.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)

	now := time.Now().UTC()
	require.NoError(t, flags.UpdateStatus(ctx, flag.ID, admin.FlagConfirmedMisuse, &userID, &now))

	resolved, err := flags.GetByID(ctx, flag.ID)
	require.NoError(t, err)
	require.Equal(t, admin.FlagConfirmedMisuse, resolved.Status)
	require.NotNil(t, resolved.ResolvedAt)
	require.Equal(t, userID, *resolved.ResolvedBy)
}

func TestScanJobRepo_EngineJobStatsSince_ReturnsZeroRowsWhenNoJobs(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	jobs := repo.NewScanJobRepo(pool)

	stats, err := jobs.EngineJobStatsSince(ctx, time.Now().UTC().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, stats)
}

func TestAuditRepo_List_FiltersByOrgAndAction(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, actorID := seedOrgAndUser(t, pool)
	// audit_log.org_id carries no FK constraint (migration 00010) — a
	// second real seeded org isn't needed just to prove the filter excludes
	// entries from a different organisation.
	otherOrgID := id.New()
	audits := repo.NewAuditRepo(pool)

	require.NoError(t, audits.Insert(ctx, audit.Entry{OrgID: &orgID, ActorID: &actorID, Action: "org.suspended"}))
	require.NoError(t, audits.Insert(ctx, audit.Entry{OrgID: &orgID, ActorID: &actorID, Action: "auth.login"}))
	require.NoError(t, audits.Insert(ctx, audit.Entry{OrgID: &otherOrgID, Action: "org.suspended"}))

	action := "org.suspended"
	entries, total, err := audits.List(ctx, audit.ListFilter{OrgID: &orgID, Action: &action}, audit.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, entries, 1)
	require.Equal(t, "org.suspended", entries[0].Action)
	require.Equal(t, orgID, *entries[0].OrgID)

	// No org filter at all — the cross-org view every other screen never
	// allows (audit.ListFilter's own doc comment).
	_, allTotal, err := audits.List(ctx, audit.ListFilter{Action: &action}, audit.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.GreaterOrEqual(t, allTotal, 2)
}
