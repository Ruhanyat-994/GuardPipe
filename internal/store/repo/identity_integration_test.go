//go:build integration

// Integration tests need a real Docker daemon to spin up a Postgres
// container (documentation's testing philosophy: "Real PostgreSQL/Redis via
// testcontainers-go for integration tests — mocked SQL tests verify
// nothing"). They're behind the `integration` build tag so `go test ./...`
// stays fast and Docker-independent for everyone else; run these with
// `go test ./internal/store/repo/... -tags=integration`.
package repo_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used only to run goose migrations

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// setupTestDB starts a throwaway Postgres container, applies every
// migration, and returns a ready pgxpool.Pool. It proves the migrations
// from Phase 1 and the repositories from Phase 2 actually work against a
// real database, not just against hand-written fakes.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("guardpipe_test"),
		tcpostgres.WithUsername("guardpipe"),
		tcpostgres.WithPassword("guardpipe"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(context.Background()))
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// goose applies migrations over database/sql, not pgx's own pool.
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, store.Migrate(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

func TestUserRepo_CreateAndGet_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	orgRepo := repo.NewOrganizationRepo(pool)
	orgID, err := orgRepo.EnsureDefault(ctx, "Test Org")
	require.NoError(t, err)

	users := repo.NewUserRepo(pool)
	user := &identity.User{
		ID:           id.New(),
		OrgID:        orgID,
		Email:        "nadia@example.com",
		DisplayName:  "Nadia R.",
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2g",
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, user))
	require.False(t, user.CreatedAt.IsZero(), "Create() must populate CreatedAt from the DB default, not leave it zero-valued")

	byEmail, err := users.GetByEmail(ctx, "nadia@example.com")
	require.NoError(t, err)
	require.Equal(t, user.ID, byEmail.ID)
	require.Equal(t, "admin", string(byEmail.Role))

	byID, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "nadia@example.com", byID.Email)

	count, err := users.CountAll(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestUserRepo_GetByEmail_NotFoundIsTypedError(t *testing.T) {
	pool := setupTestDB(t)
	_, err := repo.NewUserRepo(pool).GetByEmail(context.Background(), "nobody@example.com")
	require.Error(t, err)
}

func TestUserRepo_SetFailedLoginAndRecordSuccessfulLogin(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, err := repo.NewOrganizationRepo(pool).EnsureDefault(ctx, "Test Org")
	require.NoError(t, err)

	users := repo.NewUserRepo(pool)
	user := &identity.User{
		ID: id.New(), OrgID: orgID, Email: "a@example.com", DisplayName: "A",
		PasswordHash: "hash", Role: "member",
	}
	require.NoError(t, users.Create(ctx, user))

	lockedUntil := time.Now().UTC().Add(15 * time.Minute).Truncate(time.Millisecond)
	require.NoError(t, users.SetFailedLogin(ctx, user.ID, 5, &lockedUntil))

	got, err := users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 5, got.FailedLoginCount)
	require.NotNil(t, got.LockedUntil)

	require.NoError(t, users.RecordSuccessfulLogin(ctx, user.ID, time.Now().UTC()))
	got, err = users.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 0, got.FailedLoginCount)
	require.Nil(t, got.LockedUntil)
	require.NotNil(t, got.LastLoginAt)
}

func TestOrganizationRepo_EnsureDefault_IsIdempotent(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgs := repo.NewOrganizationRepo(pool)

	first, err := orgs.EnsureDefault(ctx, "Default Organization")
	require.NoError(t, err)

	second, err := orgs.EnsureDefault(ctx, "Default Organization")
	require.NoError(t, err)

	require.Equal(t, first, second, "EnsureDefault must not create a second organisation")
}

func TestRefreshTokenRepo_CreateConsumeAndRevokeFamily(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, err := repo.NewOrganizationRepo(pool).EnsureDefault(ctx, "Test Org")
	require.NoError(t, err)
	users := repo.NewUserRepo(pool)
	user := &identity.User{
		ID: id.New(), OrgID: orgID, Email: "b@example.com", DisplayName: "B",
		PasswordHash: "hash", Role: "member",
	}
	require.NoError(t, users.Create(ctx, user))

	tokens := repo.NewRefreshTokenRepo(pool)
	familyID := id.New()
	rt := &identity.RefreshToken{
		ID: id.New(), UserID: user.ID, TokenHash: "deadbeef", FamilyID: familyID,
		ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour),
	}
	require.NoError(t, tokens.Create(ctx, rt))

	got, err := tokens.GetByHash(ctx, "deadbeef")
	require.NoError(t, err)
	require.Nil(t, got.ConsumedAt)
	require.Nil(t, got.RevokedAt)

	now := time.Now().UTC()
	require.NoError(t, tokens.MarkConsumed(ctx, rt.ID, now))
	got, err = tokens.GetByHash(ctx, "deadbeef")
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)

	require.NoError(t, tokens.RevokeFamily(ctx, familyID, now))
	got, err = tokens.GetByHash(ctx, "deadbeef")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}
