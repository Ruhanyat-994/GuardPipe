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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver, used only to run goose migrations

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
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

func main() {
	// The distroless runtime image has no shell, curl, or wget, so a Docker
	// Compose HEALTHCHECK cannot exec a shell command inside it. It execs
	// the binary itself instead: "guardpipe healthcheck".
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
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

	orgID, err := repo.NewOrganizationRepo(db.Pool).EnsureDefault(ctx, "Default Organization")
	if err != nil {
		return fmt.Errorf("ensure default organisation: %w", err)
	}
	log.Info("organisation ready", "org_id", orgID)

	identitySvc := identity.NewService(
		repo.NewUserRepo(db.Pool),
		repo.NewOrganizationRepo(db.Pool),
		repo.NewRefreshTokenRepo(db.Pool),
		identity.NewTokenIssuer([]byte(cfg.Security.JWTSecret), cfg.Security.AccessTokenTTL),
		cfg.Security.AccessTokenTTL,
		cfg.Security.RefreshTokenTTL,
	)

	if cfg.Core.Role == config.RoleWorker {
		// The worker pool doesn't exist yet — it lands in Phase 6 with the
		// first engine. A worker-role process today has nothing to claim,
		// so it just idles until told to stop, rather than pretending to
		// do work it can't yet do.
		log.Warn("GUARDPIPE_ROLE=worker has no worker pool to run yet (lands in Phase 6) — idling")
		return waitForShutdown(log, nil)
	}

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		Logger:          log,
		CORSOrigins:     cfg.Security.CORSOrigins,
		IdentitySvc:     identitySvc,
		HealthDB:        db,
		Version:         version,
		CommitSHA:       commitSHA,
		BuildTime:       buildTime,
		SecureCookies:   cfg.Core.Env == "production",
		RefreshTokenTTL: cfg.Security.RefreshTokenTTL,
		AuthRateLimit:   5,
		AuthRateWindow:  time.Minute,
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
