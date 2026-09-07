//go:build integration

// The phase's literal backend "Done when" deliverable (BUILD_GUIDE.md
// Phase 6): run a scan through the real orchestrator — real Postgres, real
// Redis job queue, the real depscan engine — against the golden
// fixture-vulnerable repo, and see findings land in Postgres. Run with
// `go test ./internal/modules/orchestrator/... -tags=integration` against
// a real Docker daemon.
package orchestrator_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/queue"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/depscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"

	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used only to run goose migrations
)

// e2eNoOpAdvisory returns no advisories for anything — this test is
// proving the queue -> worker -> engine -> Postgres path, not OSV lookups
// (which modules/advisory's own tests already cover against a fake, never
// live OSV.dev).
type e2eNoOpAdvisory struct{ advisory.Service }

func (e2eNoOpAdvisory) Lookup(_ context.Context, deps []advisory.Dependency) ([]advisory.Result, error) {
	out := make([]advisory.Result, len(deps))
	for i, d := range deps {
		out[i] = advisory.Result{Dependency: d}
	}
	return out, nil
}

// e2eFixtureCloner "clones" by copying a local golden fixture directory —
// tests never call GitHub live (documentation/15-testing-strategy.md), and
// this proves the worker's workspace-preparation and engine-execution path
// without needing a real repository.
type e2eFixtureCloner struct{ fixtureDir string }

func (c e2eFixtureCloner) ShallowClone(_ context.Context, _, _, destDir string) error {
	return filepath.WalkDir(c.fixtureDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(c.fixtureDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// e2eStaticCloneInfo satisfies orchestrator.CloneInfoProvider and
// orchestrator.ProjectAccess with no real project.Service — the worker
// only needs "which project ID maps to which repo," which is fixed for
// this test.
type e2eStaticCloneInfo struct{ projectID uuid.UUID }

func (e2eStaticCloneInfo) GetCloneInfo(context.Context, uuid.UUID) (string, string, string, error) {
	return "https://example.invalid/fixture-vulnerable", "main", "", nil
}

// Get's Repository must be non-nil — resolveEngines (service.go) only
// includes an engine that requires a repository (depscan does) when the
// project actually has one attached; a nil Repository here would silently
// resolve to zero runnable engines regardless of what's registered.
func (e e2eStaticCloneInfo) Get(_ context.Context, _ domain.Actor, id uuid.UUID) (*project.ProjectDetail, error) {
	return &project.ProjectDetail{
		Project:    project.Project{ID: id},
		Repository: &project.Repository{ProjectID: id, Provider: "github", Owner: "acme", Name: "fixture-vulnerable"},
	}, nil
}

// GetAttestedTarget: this E2E test only ever requests full_supply_chain
// with depscan registered (no pentest engine in the registry), so
// resolveEngines never actually calls this — it exists purely to satisfy
// orchestrator.ProjectAccess.
func (e2eStaticCloneInfo) GetAttestedTarget(context.Context, uuid.UUID) (*project.Target, error) {
	return nil, apperrors.NotFound("project.pentest_target_not_found", "no attested pentest target attached to this project")
}

func (e2eStaticCloneInfo) GetOrgID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

// MarkCredentialInvalid: this E2E test's clone always succeeds
// (e2eFixtureCloner copies a local fixture directory, never hits GitHub),
// so this is never actually called — exists purely to satisfy
// orchestrator.CloneInfoProvider.
func (e2eStaticCloneInfo) MarkCredentialInvalid(context.Context, uuid.UUID, string) error {
	return nil
}

func setupE2EPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("guardpipe_test"), postgres.WithUsername("guardpipe"), postgres.WithPassword("guardpipe"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pgContainer.Terminate(context.Background())) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, store.Migrate(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestEndToEnd_ScanThroughOrchestrator_FindingsLandInPostgres(t *testing.T) {
	pool := setupE2EPostgres(t)
	ctx := context.Background()

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, redisContainer.Terminate(context.Background())) })
	redisConnStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)
	redisClient, err := queue.New(redisConnStr)
	require.NoError(t, err)
	defer func() { _ = redisClient.Close() }()

	// --- seed an org/user/project so the findings' FK chain is real ---
	orgID, err := repo.NewOrganizationRepo(pool).Create(ctx, "E2E Test Org")
	require.NoError(t, err)
	userID := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, org_id, email, display_name, password_hash, role) VALUES ($1, $2, $3, $4, $5, 'admin')`,
		userID, orgID, "e2e@example.com", "E2E User", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2g=")
	require.NoError(t, err)
	projectID := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectID, orgID, "E2E Project")
	require.NoError(t, err)

	// --- seed the rules catalogue exactly like cmd/guardpipe/main.go's
	// advisorySvc.SyncRules(ctx) does at real startup — findings.rule_id is
	// a foreign key into `rules`, so depscan's Core rules have to exist
	// before any of its findings can be inserted. ---
	ruleRegistry := advisory.NewRuleRegistry()
	ruleRegistry.Register(depscan.Rules...)
	ruleSvc := advisory.NewService(nil, nil, 0, repo.NewRuleRepo(pool), ruleRegistry, nil)
	require.NoError(t, ruleSvc.SyncRules(ctx))

	// --- wire the real orchestrator against real Postgres + real Redis + the real depscan engine ---
	jobQueue := queue.NewJobQueue(redisClient)
	registry := orchestrator.NewRegistry()
	registry.Register(depscan.New(e2eNoOpAdvisory{}))

	cloneInfo := e2eStaticCloneInfo{projectID: projectID}
	svc := orchestrator.NewService(
		repo.NewScanRepo(pool), repo.NewScanJobRepo(pool), repo.NewFindingRepo(pool), repo.NewRiskAssessmentRepo(pool),
		cloneInfo, jobQueue, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil, nil, nil,
	)

	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-vulnerable"))
	require.NoError(t, err)

	pool2 := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(jobQueue.Claim, jobQueue.Ack),
		Registry: registry, Scans: repo.NewScanRepo(pool), Jobs: repo.NewScanJobRepo(pool),
		JobResults: repo.NewJobResultRepo(pool), Projects: cloneInfo, Cloner: e2eFixtureCloner{fixtureDir: fixtureDir},
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 30 * time.Second,
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		Findings:        repo.NewFindingRepo(pool),
		RiskAssessments: repo.NewRiskAssessmentRepo(pool),
		Scorer:          scoring.NewDefaultScorer(),
	}

	workerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go pool2.Start(workerCtx)

	// --- the actual "Done when" flow: POST-equivalent create, then poll ---
	actor := domain.Actor{UserID: userID, OrgID: orgID, Role: domain.RoleMember}
	detail, err := svc.CreateScan(ctx, actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, detail.RequestedEngines)

	var final *orchestrator.ScanDetail
	require.Eventually(t, func() bool {
		final, err = svc.GetScan(ctx, actor, detail.ID)
		require.NoError(t, err)
		return final.Status == domain.ScanStatusCompleted
	}, 8*time.Second, 100*time.Millisecond, "scan must reach completed within the worker timeout")

	require.Len(t, final.Jobs, 1)
	require.Equal(t, domain.JobStatusSucceeded, final.Jobs[0].Status)
	require.Positive(t, final.Jobs[0].FindingCount, "the depscan job must have produced findings")

	findings, total, err := svc.ListFindings(ctx, actor, detail.ID, orchestrator.Page{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Positive(t, total)

	ruleIDs := make(map[string]bool)
	for _, f := range findings {
		ruleIDs[f.RuleID] = true
	}
	require.True(t, ruleIDs["depscan.hygiene.no-lockfile"], "fixture-vulnerable's package.json has no lockfile")
	require.True(t, ruleIDs["depscan.secrets.committed-credential"], "fixture-vulnerable's config.py has a planted AWS key")

	require.NotNil(t, final.Risk, "a completed scan must have a real, persisted risk assessment by the time GetScan returns")
	require.True(t, final.Risk.Verdict.Valid())
	require.Equal(t, "1.0", final.Risk.FormulaVersion)
	require.Positive(t, final.Risk.Score, "fixture-vulnerable's planted findings must move the score off zero")
}
