//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why
// (documentation's testing philosophy: "mocked SQL tests verify nothing").
package repo_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func seedOrgAndUser(t *testing.T, pool *pgxpool.Pool) (orgID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	orgID, err := repo.NewOrganizationRepo(pool).Create(ctx, "Test Org")
	require.NoError(t, err)

	user := &identity.User{
		ID:           id.New(),
		OrgID:        orgID,
		Email:        "nadia@example.com",
		DisplayName:  "Nadia R.",
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2g",
		Role:         "admin",
	}
	require.NoError(t, repo.NewUserRepo(pool).Create(ctx, user))

	return orgID, user.ID
}

func TestProjectRepo_CreateGetListUpdate_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)

	desc := "Core payment service"
	p := &project.Project{
		ID:          id.New(),
		OrgID:       orgID,
		Name:        "Payments API",
		Description: &desc,
		Status:      project.StatusActive,
		CreatedBy:   &userID,
	}
	require.NoError(t, projects.Create(ctx, p))
	require.False(t, p.CreatedAt.IsZero())

	got, err := projects.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "Payments API", got.Name)
	require.Equal(t, desc, *got.Description)

	byName, err := projects.GetByOrgAndName(ctx, orgID, "Payments API")
	require.NoError(t, err)
	require.Equal(t, p.ID, byName.ID)

	list, total, err := projects.List(ctx, orgID, project.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)

	got.Name = "Payments API v2"
	got.Status = project.StatusArchived
	require.NoError(t, projects.Update(ctx, got))
	reGot, err := projects.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "Payments API v2", reGot.Name)
	require.Equal(t, project.StatusArchived, reGot.Status)
}

func TestRepositoryRepo_UpsertReplaces(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)
	repos := repo.NewRepositoryRepo(pool)

	p := &project.Project{ID: id.New(), OrgID: orgID, Name: "Payments API", Status: project.StatusActive, CreatedBy: &userID}
	require.NoError(t, projects.Create(ctx, p))

	sizeKB := int64(18432)
	r := &project.Repository{
		ID: id.New(), ProjectID: p.ID, Provider: "github", URL: "https://github.com/acme/payments-api",
		Owner: "acme", Name: "payments-api", DefaultBranch: "main", IsPrivate: false, SizeKB: &sizeKB,
	}
	require.NoError(t, repos.Upsert(ctx, r))

	got, err := repos.GetByProjectID(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "acme", got.Owner)
	require.NotNil(t, got.LastValidatedAt)

	// Attaching a different repo to the same project replaces it (UNIQUE
	// project_id — documentation/06-database-design.md §4.5).
	r2 := &project.Repository{
		ID: id.New(), ProjectID: p.ID, Provider: "github", URL: "https://github.com/acme/payments-api-v2",
		Owner: "acme", Name: "payments-api-v2", DefaultBranch: "main", IsPrivate: true,
	}
	require.NoError(t, repos.Upsert(ctx, r2))
	got2, err := repos.GetByProjectID(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, "payments-api-v2", got2.Name)
	require.True(t, got2.IsPrivate)
}

func TestCredentialRepo_UpsertAndDecrypt_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)
	creds := repo.NewCredentialRepo(pool)

	p := &project.Project{ID: id.New(), OrgID: orgID, Name: "Payments API", Status: project.StatusActive, CreatedBy: &userID}
	require.NoError(t, projects.Create(ctx, p))

	key := make([]byte, crypto.KeySize)
	token := "ghp_abcdefghijklmnop3f9a"
	ciphertext, nonce, err := crypto.Encrypt(key, []byte(token))
	require.NoError(t, err)

	updatedAt, err := creds.Upsert(ctx, project.CredentialRow{
		ProjectID: p.ID, Kind: project.CredentialKindGitHubPAT,
		Ciphertext: ciphertext, Nonce: nonce, Hint: "ghp_••••3f9a", CreatedBy: userID,
	})
	require.NoError(t, err)
	require.False(t, updatedAt.IsZero())

	info, err := creds.GetInfo(ctx, p.ID, project.CredentialKindGitHubPAT)
	require.NoError(t, err)
	require.True(t, info.HasCredential)
	require.Equal(t, "ghp_••••3f9a", info.Hint)

	plain, err := creds.GetPlaintext(ctx, p.ID, project.CredentialKindGitHubPAT, key)
	require.NoError(t, err)
	require.Equal(t, token, plain)

	require.NoError(t, creds.Delete(ctx, p.ID, project.CredentialKindGitHubPAT))
	_, err = creds.GetInfo(ctx, p.ID, project.CredentialKindGitHubPAT)
	require.Error(t, err)
}

func TestTargetAndAttestationRepo_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projects := repo.NewProjectRepo(pool)
	targets := repo.NewTargetRepo(pool)
	attestations := repo.NewAttestationRepo(pool)
	users := repo.NewUserRepo(pool)

	p := &project.Project{ID: id.New(), OrgID: orgID, Name: "Payments API", Status: project.StatusActive, CreatedBy: &userID}
	require.NoError(t, projects.Create(ctx, p))

	pinned := []netip.Addr{netip.MustParseAddr("203.0.113.10")}
	tgt := &project.Target{
		ID: id.New(), ProjectID: p.ID, TargetInput: "https://staging.acme.example",
		NormalizedHost: "staging.acme.example", PinnedIPs: pinned,
		Status: project.TargetAwaitingAttestation, LastResolvedAt: time.Now().UTC(),
	}
	require.NoError(t, targets.Create(ctx, tgt))

	got, err := targets.GetByID(ctx, tgt.ID)
	require.NoError(t, err)
	require.Equal(t, "staging.acme.example", got.NormalizedHost)
	require.Equal(t, pinned, got.PinnedIPs)

	list, err := targets.ListByProject(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, targets.UpdateStatus(ctx, tgt.ID, project.TargetAttested))

	att := &project.Attestation{
		ID: id.New(), TargetID: tgt.ID, UserID: userID,
		AttestationTextVersion: "v1", AcceptedAt: time.Now().UTC(), SourceIP: "203.0.113.99",
	}
	require.NoError(t, attestations.Create(ctx, att))

	name, err := users.GetDisplayName(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "Nadia R.", name)

	reGot, err := targets.GetByID(ctx, tgt.ID)
	require.NoError(t, err)
	require.Equal(t, project.TargetAttested, reGot.Status)
}
