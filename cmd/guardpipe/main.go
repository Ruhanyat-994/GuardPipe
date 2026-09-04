// Command guardpipe is the entrypoint for the GuardPipe API/worker binary.
// GUARDPIPE_ROLE picks whether this process serves HTTP, runs the worker
// pool, or both (`all`, the default) — an intentional near-zero-cost path
// to splitting replicas later without a code change.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used only to run goose migrations

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/gemini"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/osv"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/pentestsandbox"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/queue"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sandbox"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sonarqube"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/cicdscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/codescan"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/containerscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/depscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/docreview"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/k8sscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/pentest"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/vcs"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/config"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/logger"
	"github.com/Ruhanyat-994/GuardPipe/internal/store"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
	transporthttp "github.com/Ruhanyat-994/GuardPipe/internal/transport/http"
)

var (
	version   = "dev"
	commitSHA = "unknown"
	buildTime = "unknown"
)

// shutdownTimeout is the hard deadline for graceful shutdown
// (documentation/04-backend-architecture.md §10).
const shutdownTimeout = 30 * time.Second

// pentestSandboxUnavailable is a pentest.Runner fallback used only when no
// GUARDPIPE_SANDBOX_IMAGE is configured — an operator who hasn't run `docker
// compose build pentest-sandbox` yet still gets the engine registered
// (visible in the live graph) rather than a silent absence, failing every
// job cleanly with a clear reason instead of a startup crash. Once an image
// is configured, pentestsandbox.Runner (Phase 12 Pass 2) is the real thing.
type pentestSandboxUnavailable struct{}

func (pentestSandboxUnavailable) Run(context.Context, pentest.RunSpec) (pentest.RawResult, error) {
	return pentest.RawResult{}, fmt.Errorf("pentest: no GUARDPIPE_SANDBOX_IMAGE configured — run `docker compose build pentest-sandbox` and set it")
}

func main() {
	// The distroless runtime image has no shell, curl, or wget, so a Docker
	// Compose HEALTHCHECK cannot exec a shell command inside it. It execs
	// the binary itself instead: "guardpipe healthcheck".
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	// aiprobe is a throwaway manual-verification command for Phase 4, not a
	// product feature — see cmd/guardpipe/aiprobe.go.
	if len(os.Args) > 1 && os.Args[1] == "aiprobe" {
		os.Exit(runAIProbe(os.Args[2:]))
	}
	// admin grant-operator/revoke-operator is BUILD_GUIDE.md Phase 14's
	// anti-escalation control — see cmd/guardpipe/admin.go's own doc
	// comment for why this is CLI-only, never an HTTP endpoint.
	if len(os.Args) > 1 && os.Args[1] == "admin" {
		os.Exit(runAdmin(os.Args[2:]))
	}

	if err := run(); err != nil {
		// A security product that boots half-configured is worse than one
		// that refuses to boot (documentation/13-devops-and-environments.md
		// §5, "Fail-fast validation").
		fmt.Fprintln(os.Stderr, "guardpipe: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Core.LogLevel, os.Stdout)
	log.Info("guardpipe starting", "role", string(cfg.Core.Role), "port", cfg.Core.HTTPPort, "version", version)

	if cfg.Data.MigrateOnStart {
		if err := runMigrations(cfg.Data.DatabaseURL); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		log.Info("migrations applied")
	}

	ctx := context.Background()
	db, err := repo.New(ctx, cfg.Data.DatabaseURL, int32(cfg.Data.DBMaxConns))
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	auditSvc := audit.NewService(repo.NewAuditRepo(db.Pool), log)

	// membershipRepo is shared by identity's MembershipRoleReader,
	// project's/orchestrator's MembershipChecker, and modules/organization
	// itself below — one repo struct against organization_memberships
	// satisfying every module's own narrow interface for it (BUILD_GUIDE.md
	// Phase 15).
	membershipRepo := repo.NewMembershipRepo(db.Pool)

	identitySvc := identity.NewService(
		repo.NewUserRepo(db.Pool),
		repo.NewOrganizationRepo(db.Pool),
		repo.NewRefreshTokenRepo(db.Pool),
		identity.NewTokenIssuer([]byte(cfg.Security.JWTSecret), cfg.Security.AccessTokenTTL),
		auditSvc,
		cfg.Security.AccessTokenTTL,
		cfg.Security.RefreshTokenTTL,
		cfg.Security.SessionAbsoluteTTL,
		membershipRepo,
	)

	githubClient := github.NewClient(cfg.External.GitHubAPIURL, nil)
	vcsSvc := vcs.NewService(githubClient, github.ShallowClone, cfg.Scanning.MaxRepoMB)
	projectSvc := project.NewService(
		repo.NewProjectRepo(db.Pool),
		repo.NewRepositoryRepo(db.Pool),
		repo.NewCredentialRepo(db.Pool),
		repo.NewTargetRepo(db.Pool),
		repo.NewAttestationRepo(db.Pool),
		repo.NewDocumentRepo(db.Pool),
		repo.NewProjectAssignmentRepo(db.Pool),
		membershipRepo,
		repo.NewUserRepo(db.Pool),
		vcsSvc,
		net.DefaultResolver,
		auditSvc,
		project.NewHTTPURLFetcher(),
		project.NewPDFTextExtractor(),
		cfg.Security.EncryptionKeyRaw,
		cfg.Pentest.AllowPrivateTargets,
		cfg.Pentest.Denylist,
	)

	redisClient, err := queue.New(cfg.Data.RedisURL)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer func() { _ = redisClient.Close() }()

	// Every engine's rule catalogue registers here before SyncRules runs —
	// findings.rule_id is a foreign key into `rules`, so a rule missing from
	// this registry means every finding it would produce fails to persist.
	ruleRegistry := advisory.NewRuleRegistry()
	ruleRegistry.Register(depscan.Rules...)
	ruleRegistry.Register(k8sscan.Rules...)
	ruleRegistry.Register(cicdscan.Rules...)
	ruleRegistry.Register(docreview.Rules...)
	ruleRegistry.Register(pentest.Rules...)

	osvClient := osv.NewClient(cfg.External.OSVAPIURL, nil)
	advisorySvc := advisory.NewService(
		osvClient,
		advisory.NewRedisCache(redisClient),
		cfg.External.OSVCacheTTL,
		repo.NewRuleRepo(db.Pool),
		ruleRegistry,
		log,
	)
	if err := advisorySvc.SyncRules(ctx); err != nil {
		return fmt.Errorf("sync rules catalogue: %w", err)
	}

	// codescan (Phase 7, ADR-0011) wraps a self-hosted SonarQube instance
	// instead of running its own SAST — the sonar-scanner-cli invocation
	// needs Docker directly (dockerx), not adapters/sandbox: sandbox enforces
	// a no-network-by-default policy for *untrusted* execution, and
	// sonar-scanner is trusted first-party tooling that must reach the
	// sonarqube service over GUARDPIPE_DOCKER_NETWORK to do its job at all.
	dockerClient, err := dockerx.New(cfg.Scanning.DockerHost)
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	sonarqubeClient := sonarqube.NewClient(cfg.External.SonarQubeAPIURL, cfg.External.SonarQubeToken, nil)
	sonarqubeScanner := sonarqube.NewScanner(dockerClient, sonarqube.ScannerConfig{
		Network:       cfg.Scanning.DockerNetwork,
		HostURL:       cfg.External.SonarQubeAPIURL,
		Token:         cfg.External.SonarQubeToken,
		Volume:        cfg.Scanning.WorkspaceVolume,
		WorkspaceRoot: cfg.Scanning.WorkspaceRoot,
	})

	trivyScanner := trivy.NewScanner(dockerClient, trivy.ScannerConfig{
		Image:         cfg.Scanning.TrivyImage,
		DBUpdate:      cfg.Scanning.TrivyDBUpdate,
		Volume:        cfg.Scanning.WorkspaceVolume,
		WorkspaceRoot: cfg.Scanning.WorkspaceRoot,
		CacheVolume:   cfg.Scanning.TrivyCacheVolume,
	})

	jobQueue := queue.NewJobQueue(redisClient)
	registry := orchestrator.NewRegistry()
	registry.Register(depscan.New(advisorySvc))
	// advisorySvc also satisfies codescan.RuleRegistrar/containerscan.RuleRegistrar
	// (just UpsertRule) — neither SonarQube's nor Trivy's own catalogue is
	// enumerable at compile time the way depscan.Rules is, so neither engine
	// has a static ruleRegistry.Register(...) call above; both register each
	// rule at runtime instead (their own engine.go).
	registry.Register(codescan.New(sonarqubeClient, sonarqubeScanner, advisorySvc, cfg.External.SonarQubeAnalysisTimeout))
	registry.Register(containerscan.New(trivyScanner, dockerClient, advisorySvc))
	// k8sscan (Phase 9) needs no dependencies — every rule is a pure
	// function of the manifests/Helm charts found in the workspace, unlike
	// depscan (advisory lookups) or codescan/containerscan (a wrapped tool).
	registry.Register(k8sscan.New())
	// cicdscan (Phase 10) is the first engine to call modules/ai — aiSvc is
	// nil when GUARDPIPE_AI_ENABLED is false or no Gemini key is configured,
	// which cicdscan.Engine treats as documentation/05-module-specifications.md
	// §10's own "Gemini unavailable" failure mode (rule findings only, job
	// still succeeds), never as a reason to skip registering the engine
	// itself — the 16 deterministic Core rules need no AI at all.
	// geminiClient/aiCache are kept in outer-scope vars (not just local to
	// this if-block) so modules/admin's SystemHealth (BUILD_GUIDE.md
	// Phase 14) can read the real pool/cache state below — both stay nil
	// when AI is disabled, which is exactly the "Available: false" signal
	// admin.GeminiPoolReader/AICacheReader are meant to report honestly.
	var aiSvc ai.Service
	var geminiClient *gemini.Client
	var aiCache *ai.MemoryCache
	if cfg.AI.Enabled {
		var err error
		geminiClient, err = gemini.NewClient("", nil, cfg.AI.KeyPool())
		if err != nil {
			return fmt.Errorf("create gemini client: %w", err)
		}
		aiCache = ai.NewMemoryCache()
		aiSvc = ai.NewService(geminiClient, aiCache, cfg.AI.CacheTTL, cfg.AI.ModelFast, cfg.AI.ModelSmart)
	}
	registry.Register(cicdscan.New(aiSvc))
	// docreview (Phase 11) has no deterministic fallback the way cicdscan
	// does — a nil aiSvc fails its jobs outright (engine.go's own doc
	// comment) rather than degrading to rule findings only, since AI review
	// is this engine's entire output. Still registered unconditionally: the
	// engine itself decides how to fail, not whether it exists.
	registry.Register(docreview.New(aiSvc))

	// pentest (Phase 12 Pass 2) — pentestsandbox.Runner wraps adapters/sandbox
	// with the pentest sandbox image (internal/scripts/pentest/Dockerfile),
	// firewalling each container's own egress down to the pinned target IP
	// (adapters/sandbox.NetworkTargetOnly). Falls back to
	// pentestSandboxUnavailable only when no image is configured, so a
	// dev machine that hasn't built the sandbox image yet still starts.
	var pentestRunner pentest.Runner = pentestSandboxUnavailable{}
	if cfg.Scanning.SandboxImage != "" {
		sb := sandbox.New(dockerClient)
		if n, sweepErr := sb.SweepOrphans(context.Background()); sweepErr != nil {
			log.Error("pentest sandbox: sweep orphaned containers at startup", "error", sweepErr)
		} else if n > 0 {
			log.Info("pentest sandbox: removed orphaned containers from a previous run", "count", n)
		}
		realRunner, err := pentestsandbox.New(sb, cfg.Scanning.SandboxImage, cfg.Scanning.WorkspaceVolume, cfg.Scanning.WorkspaceRoot)
		if err != nil {
			return fmt.Errorf("initialise pentest sandbox runner: %w", err)
		}
		pentestRunner = realRunner
	}
	registry.Register(pentest.New(pentestRunner, net.DefaultResolver, cfg.Pentest.AllowPrivateTargets, cfg.Pentest.Denylist, advisorySvc))

	// pentestCeiling is the hard cap CreateScan clamps every scan's
	// pentest_config against (BUILD_GUIDE.md Phase 12) — Deep's own numbers
	// already sit at the ceiling by construction (domain.ClampPentestScanConfig's
	// doc comment), with the request-rate figure sourced from the same
	// GUARDPIPE_PENTEST_RATE_LIMIT config value the (not-yet-built, Pass 2)
	// sandboxed scripts will themselves be capped by.
	pentestCeiling := domain.PentestPresetDeepConfig()
	pentestCeiling.RequestRatePerSec = cfg.Pentest.RateLimit

	// liveProgress is shared between the worker pool (writes, as an engine
	// reports real stage progress) and the API's GetProgress (reads, on
	// every ~2s poll) — one process, one in-memory store; see
	// LiveProgress's own doc comment for why this isn't Redis-backed yet.
	const defaultEngineTimeout = 5 * time.Minute
	liveProgress := orchestrator.NewLiveProgress()

	orchestratorSvc := orchestrator.NewService(
		repo.NewScanRepo(db.Pool), repo.NewScanJobRepo(db.Pool), repo.NewFindingRepo(db.Pool), repo.NewRiskAssessmentRepo(db.Pool),
		projectSvc, jobQueue, registry, pentestCeiling,
		liveProgress, cfg.Scanning.EngineTimeouts, defaultEngineTimeout, auditSvc,
		repo.NewScanScheduleRepo(db.Pool), membershipRepo,
	)

	// scorer's thresholds come from the same GUARDPIPE_GATE_WARN/BLOCK config
	// values documentation/11-risk-scoring-and-severity.md §3.8 names —
	// everything else in scoring.DefaultConfig() is a calibrated constant
	// with no environment override (see that function's own doc comment).
	scorerConfig := scoring.DefaultConfig()
	scorerConfig.Thresholds = scoring.Thresholds{Warn: cfg.Gate.Warn, Block: cfg.Gate.Block}
	scorer := scoring.NewScorer(scorerConfig)

	pool := &orchestrator.Pool{
		Size:            cfg.Scanning.WorkerCount,
		Queue:           orchestrator.NewJobQueueClaimer(jobQueue.Claim, jobQueue.Ack),
		Registry:        registry,
		Scans:           repo.NewScanRepo(db.Pool),
		Jobs:            repo.NewScanJobRepo(db.Pool),
		JobResults:      repo.NewJobResultRepo(db.Pool),
		Projects:        projectSvc,
		Documents:       projectSvc,
		Targets:         projectSvc,
		Cloner:          vcsSvc,
		WorkspaceRoot:   cfg.Scanning.WorkspaceRoot,
		EngineTimeouts:  cfg.Scanning.EngineTimeouts,
		DefaultTimeout:  defaultEngineTimeout,
		Progress:        liveProgress,
		Log:             log,
		Findings:        repo.NewFindingRepo(db.Pool),
		RiskAssessments: repo.NewRiskAssessmentRepo(db.Pool),
		Scorer:          scorer,
	}

	// GUARDPIPE_ROLE=api never runs the worker pool; GUARDPIPE_ROLE=all
	// (the default) and GUARDPIPE_ROLE=worker both do — the same binary,
	// an intentional near-zero-cost split into separate replicas later.
	var workerCtx context.Context
	var stopWorkers context.CancelFunc
	if cfg.Core.Role != config.RoleAPI {
		workerCtx, stopWorkers = context.WithCancel(context.Background())
		go pool.Start(workerCtx)
		log.Info("worker pool started", "size", cfg.Scanning.WorkerCount, "engines", registry.IDs())

		// BUILD_GUIDE.md Phase 15's cron scan scheduler — the same role
		// split as the worker pool above (never GUARDPIPE_ROLE=api), no new
		// deployment shape.
		scheduler := &orchestrator.Scheduler{Schedules: repo.NewScanScheduleRepo(db.Pool), Orchestrator: orchestratorSvc, Log: log}
		go scheduler.Start(workerCtx)
		log.Info("scan scheduler started")

		defer stopWorkers()
	}

	if cfg.Core.Role == config.RoleWorker {
		return waitForShutdown(log, func(context.Context) error {
			stopWorkers()
			return nil
		})
	}

	// modules/admin (BUILD_GUIDE.md Phase 14) — the platform-operator
	// control plane. Repository constructions are repeated here rather
	// than reusing a stored variable, matching this function's own
	// existing convention (e.g. repo.NewScanJobRepo(db.Pool) is already
	// called twice, once for orchestratorSvc and once for pool, above) —
	// each is just a struct wrapping db.Pool, cheap to construct again.
	//
	// geminiHealthReader/aiCacheHealthReader stay nil (their declared
	// interface's zero value) when AI is disabled — SystemHealth reports
	// that honestly as Available: false rather than fabricating a number.
	var geminiHealthReader admin.GeminiPoolReader
	var aiCacheHealthReader admin.AICacheReader
	if cfg.AI.Enabled {
		geminiHealthReader = geminiPoolReader{client: geminiClient}
		aiCacheHealthReader = aiCacheReader{cache: aiCache}
	}
	adminSandbox := sandbox.New(dockerClient)
	adminSvc := admin.NewService(
		repo.NewOrganizationRepo(db.Pool),
		repo.NewUserRepo(db.Pool),
		repo.NewPlatformOperatorRepo(db.Pool),
		repo.NewPentestFlagRepo(db.Pool),
		repo.NewTargetRepo(db.Pool),
		projectSvc,
		auditSvc,
		repo.NewScanJobRepo(db.Pool),
		jobQueue,
		adminSandbox,
		geminiHealthReader,
		aiCacheHealthReader,
	)

	// modules/organization (BUILD_GUIDE.md Phase 15) — multi-member orgs,
	// invites, switch-org. Depends on identitySvc for token reissuance
	// (SwitchOrg) — see organization.Service's own doc comment on the
	// dependency direction.
	orgSvc := organization.NewService(
		membershipRepo,
		repo.NewInviteRepo(db.Pool),
		repo.NewUserRepo(db.Pool),
		repo.NewOrganizationRepo(db.Pool),
		identitySvc,
		auditSvc,
	)

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		Logger:          log,
		CORSOrigins:     cfg.Security.CORSOrigins,
		IdentitySvc:     identitySvc,
		ProjectSvc:      projectSvc,
		AdvisorySvc:     advisorySvc,
		OrchestratorSvc: orchestratorSvc,
		AdminSvc:        adminSvc,
		OrgSvc:          orgSvc,
		Users:           repo.NewUserRepo(db.Pool),
		AISvc:           aiSvc,
		HealthDB:        db,
		Version:         version,
		CommitSHA:       commitSHA,
		BuildTime:       buildTime,
		SecureCookies:   cfg.Core.Env == "production",
		RefreshTokenTTL: cfg.Security.RefreshTokenTTL,
		AuthRateLimit:   cfg.Security.AuthRateLimit,
		AuthRateWindow:  cfg.Security.AuthRateWindow,
	})

	srv := &http.Server{
		Addr:    ":" + cfg.Core.HTTPPort,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "port", cfg.Core.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	return waitForShutdown(log, func(ctx context.Context) error {
		if stopWorkers != nil {
			stopWorkers()
		}
		return srv.Shutdown(ctx)
	}, serverErr)
}

// waitForShutdown blocks until SIGINT/SIGTERM, then runs shutdownFn (if any)
// with a hard deadline (documentation/04-backend-architecture.md §10). It
// also returns early if errCh (if any) delivers a server error first.
func waitForShutdown(log *slog.Logger, shutdownFn func(context.Context) error, errCh ...chan error) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var firstErrCh chan error
	if len(errCh) > 0 {
		firstErrCh = errCh[0]
	}

	select {
	case sig := <-sigCh:
		log.Info("shutdown signal received", "signal", sig.String())
	case err := <-firstErrCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}

	if shutdownFn == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := shutdownFn(ctx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("shutdown complete")
	return nil
}

func runMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database/sql connection: %w", err)
	}
	defer db.Close()
	return store.Migrate(db)
}

func runHealthcheck() int {
	port := os.Getenv("GUARDPIPE_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:" + port + "/readyz")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
