// Package config parses GuardPipe's entire configuration surface from
// environment variables, once, at startup, into a typed struct — no config
// files, no runtime mutation (documentation/13-devops-and-environments.md
// §5). Load fails fast: a missing required variable, a short JWT secret, or
// a wrong-length encryption key aborts startup naming the variable, rather
// than booting a half-configured security product.
package config

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
)

// Role is the value of GUARDPIPE_ROLE — the same binary runs as HTTP
// server, worker pool, or both.
type Role string

const (
	RoleAll    Role = "all"
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
)

// Config is the full, typed configuration surface, grouped to match
// documentation/13-devops-and-environments.md §5.
type Config struct {
	Core     Core
	Data     Data
	Security Security
	Scanning Scanning
	Pentest  Pentest
	AI       AI
	External External
	Gate     Gate
}

// Core — §5.1.
type Core struct {
	Env      string // "development" | "production"
	Role     Role
	HTTPPort string
	LogLevel string
	BaseURL  string
}

// Data — §5.2.
type Data struct {
	DatabaseURL    string
	DBMaxConns     int
	RedisURL       string
	MigrateOnStart bool
}

// Security — §5.3. JWTSecret and EncryptionKey are already validated by the
// time Load returns: JWTSecret is at least 32 bytes, EncryptionKeyRaw
// decodes from base64 to exactly crypto.KeySize bytes.
type Security struct {
	JWTSecret        string
	EncryptionKeyRaw []byte
	AccessTokenTTL   time.Duration
	// RefreshTokenTTL doubles as the session idle timeout: Refresh resets a
	// token's ExpiresAt another RefreshTokenTTL forward on every rotation
	// (identity.service's issueTokenPairInFamily), so an actively-used
	// session never hits it and an abandoned one dies exactly this long
	// after the last request (BUILD_GUIDE.md Phase 14).
	RefreshTokenTTL time.Duration
	// SessionAbsoluteTTL (BUILD_GUIDE.md Phase 14) is the ceiling on top of
	// RefreshTokenTTL's sliding idle timeout — a session dies this long
	// after its original login regardless of activity, closing the gap
	// where a continuously-refreshed session (legitimate or a stolen-cookie
	// replay) never expired at all.
	SessionAbsoluteTTL time.Duration
	CORSOrigins        []string

	// AuthRateLimit/AuthRateWindow bound /auth/register and /auth/login,
	// shared one bucket per client IP (documentation/07-api-specification.md
	// §1.6 specifies 5/min — that's the default here too, so a default
	// deployment stays spec-compliant without reading either var). Was
	// hardcoded in cmd/guardpipe/main.go until a single local dev IP running
	// both a browser session and API testing/automation against the same
	// backend kept exhausting the shared 5/min bucket within normal use —
	// exposing it as env vars lets a local .env raise it for that kind of
	// testing without touching the documented production default.
	AuthRateLimit  int
	AuthRateWindow time.Duration

	// SecureCookies gates the refresh-token cookie's Secure flag
	// (transport/http/router.go's AuthHandler wiring) — defaults to
	// Core.Env == "production" but is independently overridable via
	// GUARDPIPE_SECURE_COOKIES, since "production" doesn't always mean
	// "TLS is terminated in front of this deployment yet."
	SecureCookies bool
}

// Scanning — §5.4.
type Scanning struct {
	WorkerCount     int
	WorkspaceRoot   string
	MaxRepoMB       int
	SandboxMax      int
	SandboxImage    string
	DockerHost      string
	DockerNetwork   string // Compose network name a sibling container (e.g. codescan's sonar-scanner) joins to reach other services by name — see docker-compose.yml's networks.default.name
	WorkspaceVolume string // Named Docker volume backing WorkspaceRoot, as seen by the daemon — see docker-compose.yml's volumes.workspace.name; a sibling container (e.g. codescan's sonar-scanner) must mount it by this name, since WorkspaceRoot is a path inside *this* container's mount namespace, not the daemon host's
	EngineTimeouts  map[domain.EngineID]time.Duration

	// SandboxBackend picks which pentest.Runner implementation
	// cmd/guardpipe/main.go wires up: "docker" (default — adapters/sandbox +
	// adapters/pentestsandbox, a Docker socket, local dev/Compose) or
	// "kubernetes" (adapters/k8spentestsandbox, one-shot Jobs via the
	// in-cluster API, no Docker socket needed — the EKS deployment, which
	// has none). SandboxImage still means the Docker path's image in either
	// case; K8sSandboxImage is the "kubernetes" backend's own image
	// (internal/scripts/pentest/Dockerfile, pushed to the
	// guardpipe-pentest-sandbox ECR repo by deploy.yml), a separate build —
	// see that Dockerfile's own comment on why it bakes scripts in rather
	// than relying on a runtime-mounted volume the way the Docker path's
	// ScriptMount does.
	SandboxBackend  string
	K8sSandboxImage string
	K8sSandboxNS    string

	// Trivy — containerscan (Phase 8, ADR-0012). No API URL/token here
	// unlike SonarQube's External fields — Trivy is a local CLI invocation,
	// not a service with an endpoint to authenticate against.
	TrivyImage    string
	TrivyDBUpdate bool
	// TrivyCacheVolume, mounted by name into each Trivy sibling container the
	// same way WorkspaceVolume is (see docker-compose.yml's volumes.trivy_cache),
	// persists the vulnerability database across scans instead of
	// re-downloading it from scratch on every containerscan run.
	TrivyCacheVolume string
}

// Pentest — §5.5.
type Pentest struct {
	Enabled             bool
	AllowPrivateTargets bool
	Denylist            []string
	RateLimit           int
	Ports               string
}

// AI — §5.6.
type AI struct {
	Enabled            bool
	GeminiAPIKey       string   // single-key form, kept working as a one-key alias
	GeminiAPIKeys      []string // GUARDPIPE_GEMINI_API_KEYS — comma-separated rotation pool (BUILD_GUIDE.md Phase 4)
	ModelFast          string
	ModelSmart         string
	TokenBudgetPerScan int
	CacheTTL           time.Duration
}

// KeyPool returns the full set of configured Gemini API keys, in rotation
// order: GUARDPIPE_GEMINI_API_KEYS first (if set), then the singular
// GUARDPIPE_GEMINI_API_KEY appended if it isn't already in the list — so a
// developer who sets both doesn't get a key rotated to twice. Empty entries
// are never present (Load already trims/drops them via getCSV, and the
// singular form is checked for blank separately).
func (a AI) KeyPool() []string {
	pool := make([]string, 0, len(a.GeminiAPIKeys)+1)
	seen := make(map[string]bool, len(a.GeminiAPIKeys)+1)
	for _, k := range a.GeminiAPIKeys {
		if k != "" && !seen[k] {
			pool = append(pool, k)
			seen[k] = true
		}
	}
	if a.GeminiAPIKey != "" && !seen[a.GeminiAPIKey] {
		pool = append(pool, a.GeminiAPIKey)
	}
	return pool
}

// External — §5.7.
type External struct {
	OSVAPIURL    string
	OSVCacheTTL  time.Duration
	GitHubAPIURL string

	// SonarQube — codescan (Phase 7, ADR-0011): self-hosted CE, not
	// SonarCloud. SonarQubeToken is a user token generated once in
	// SonarQube's own UI on first boot (BUILD_GUIDE.md Phase 7) — there is
	// no default, codescan simply can't run without one.
	SonarQubeAPIURL          string
	SonarQubeToken           string
	SonarQubeAnalysisTimeout time.Duration
}

// Gate — §5.8.
type Gate struct {
	Warn  int
	Block int
}

// defaultEngineTimeouts matches documentation/04-backend-architecture.md
// §6.3 exactly. codescan/containerscan were raised from the doc's original
// 5/8 min (documentation/04-backend-architecture.md's revision history has
// the details): both wrap a real external tool (sonar-scanner submission +
// SonarQube's own async compute-engine processing; a real `docker build`
// plus two Trivy invocations) against this project's own, now
// multi-phase-grown, self-scan target, and 5/8 min was measured too tight
// on real hardware even after fixing codescan's sonar.exclusions gap
// (adapters/sonarqube/scanner.go) and adding containerscan's Trivy DB
// cache volume (adapters/trivy/scanner.go) — both real time sinks, but not
// the only ones; a cold `docker build` alone can still take several
// minutes.
var defaultEngineTimeouts = map[domain.EngineID]time.Duration{
	domain.EngineDocReview:     5 * time.Minute,
	domain.EngineCodeScan:      10 * time.Minute,
	domain.EngineDepScan:       3 * time.Minute,
	domain.EngineContainerScan: 15 * time.Minute,
	domain.EngineK8sScan:       2 * time.Minute,
	domain.EngineCICDScan:      3 * time.Minute,
	// Raised from 15 to 45 minutes (BUILD_GUIDE.md's "Pentest Quality
	// Upgrade" pass): Deep tier's own PhaseBudget ceiling alone grew to 10
	// minutes per individual script call (domain.PentestPresetDeepConfig),
	// and a Deep scan makes many such calls across several HTTP ports and
	// pipeline stages (a full-range nmap sweep, --script vuln, a 5,000-entry
	// ffuf pass, testssl.sh's full battery, a validated-endpoint nuclei
	// sweep) — the old 15-minute ceiling could cut off a real Deep scan
	// mid-run even when every individual tool call was behaving normally.
	domain.EnginePentest: 45 * time.Minute,
}

// engineTimeoutEnvSuffix names the GUARDPIPE_ENGINE_TIMEOUT_<SUFFIX>
// variable for each engine.
var engineTimeoutEnvSuffix = map[domain.EngineID]string{
	domain.EngineDocReview:     "DOCREVIEW",
	domain.EngineCodeScan:      "CODESCAN",
	domain.EngineDepScan:       "DEPSCAN",
	domain.EngineContainerScan: "CONTAINERSCAN",
	domain.EngineK8sScan:       "K8SSCAN",
	domain.EngineCICDScan:      "CICDSCAN",
	domain.EnginePentest:       "PENTEST",
}

// Load reads and validates the full configuration from the environment.
// It returns every problem it finds, joined into one error, rather than
// stopping at the first — so a developer fixes their .env once, not once
// per restart.
func Load() (*Config, error) {
	p := &problems{}

	// Read once, used both for Core.Env itself and as SecureCookies'
	// default below — "production" no longer automatically implies "TLS is
	// terminated in front of this deployment" (a production EKS deployment
	// can be sitting behind a bare HTTP ALB before a domain/ACM cert exists
	// for it), so the two are separate knobs now, not one conflated with
	// the other.
	env := getString("GUARDPIPE_ENV", "development")

	cfg := &Config{
		Core: Core{
			Env:      env,
			Role:     Role(getString("GUARDPIPE_ROLE", string(RoleAll))),
			HTTPPort: getString("GUARDPIPE_HTTP_PORT", "8080"),
			LogLevel: getString("GUARDPIPE_LOG_LEVEL", "info"),
			BaseURL:  getString("GUARDPIPE_BASE_URL", "http://localhost:8080"),
		},
		Data: Data{
			DatabaseURL:    requireString("GUARDPIPE_DATABASE_URL", p),
			DBMaxConns:     getInt("GUARDPIPE_DB_MAX_CONNS", 25, p),
			RedisURL:       requireString("GUARDPIPE_REDIS_URL", p),
			MigrateOnStart: getBool("GUARDPIPE_MIGRATE_ON_START", true, p),
		},
		Security: Security{
			JWTSecret:          requireString("GUARDPIPE_JWT_SECRET", p),
			AccessTokenTTL:     getDuration("GUARDPIPE_ACCESS_TOKEN_TTL", 15*time.Minute, p),
			RefreshTokenTTL:    getDuration("GUARDPIPE_REFRESH_TOKEN_TTL", 30*time.Minute, p),
			SessionAbsoluteTTL: getDuration("GUARDPIPE_SESSION_ABSOLUTE_TTL", 12*time.Hour, p),
			CORSOrigins:        getCSV("GUARDPIPE_CORS_ORIGINS", []string{"http://localhost:5173"}),
			AuthRateLimit:      getInt("GUARDPIPE_AUTH_RATE_LIMIT", 5, p),
			AuthRateWindow:     getDuration("GUARDPIPE_AUTH_RATE_WINDOW", time.Minute, p),
			// Defaults to env=="production" (previous behavior, unchanged
			// for anyone who hasn't hit this), but overridable independently
			// now — a deployment can be "production" while still sitting
			// behind plain HTTP (no domain/ACM cert yet). The refresh-token
			// cookie's Secure flag reads this, not Core.Env directly
			// (router.go). Browsers silently refuse to store a Secure
			// cookie set over an insecure connection, which is what broke
			// session persistence across a page reload on the first EKS
			// deployment (confirmed live 2026-09-14) — GUARDPIPE_ENV was
			// "production" but the ALB has no TLS listener yet.
			SecureCookies: getBool("GUARDPIPE_SECURE_COOKIES", env == "production", p),
		},
		Scanning: Scanning{
			WorkerCount:      getInt("GUARDPIPE_WORKER_COUNT", 4, p),
			WorkspaceRoot:    getString("GUARDPIPE_WORKSPACE_ROOT", "/var/lib/guardpipe/workspace"),
			MaxRepoMB:        getInt("GUARDPIPE_MAX_REPO_MB", 500, p),
			SandboxMax:       getInt("GUARDPIPE_SANDBOX_MAX", 2, p),
			SandboxImage:     getString("GUARDPIPE_SANDBOX_IMAGE", ""),
			DockerHost:       getString("GUARDPIPE_DOCKER_HOST", "unix:///var/run/docker.sock"),
			DockerNetwork:    getString("GUARDPIPE_DOCKER_NETWORK", "guardpipe-net"),
			WorkspaceVolume:  getString("GUARDPIPE_WORKSPACE_VOLUME", "guardpipe-workspace"),
			EngineTimeouts:   loadEngineTimeouts(p),
			TrivyImage:       getString("GUARDPIPE_TRIVY_IMAGE", ""),
			TrivyDBUpdate:    getBool("GUARDPIPE_TRIVY_DB_UPDATE", true, p),
			TrivyCacheVolume: getString("GUARDPIPE_TRIVY_CACHE_VOLUME", "guardpipe-trivy-cache"),
			SandboxBackend:   getString("GUARDPIPE_SANDBOX_BACKEND", "docker"),
			K8sSandboxImage:  getString("GUARDPIPE_K8S_SANDBOX_IMAGE", ""),
			K8sSandboxNS:     getString("GUARDPIPE_K8S_SANDBOX_NAMESPACE", "guardpipe"),
		},
		Pentest: Pentest{
			Enabled:             getBool("GUARDPIPE_PENTEST_ENABLED", true, p),
			AllowPrivateTargets: getBool("GUARDPIPE_ALLOW_PRIVATE_TARGETS", false, p),
			Denylist:            getCSV("GUARDPIPE_PENTEST_DENYLIST", nil),
			RateLimit:           getInt("GUARDPIPE_PENTEST_RATE_LIMIT", 10, p),
			Ports:               getString("GUARDPIPE_PENTEST_PORTS", "top100"),
		},
		AI: AI{
			Enabled:            getBool("GUARDPIPE_AI_ENABLED", true, p),
			GeminiAPIKey:       getString("GUARDPIPE_GEMINI_API_KEY", ""),
			GeminiAPIKeys:      getCSV("GUARDPIPE_GEMINI_API_KEYS", nil),
			ModelFast:          getString("GUARDPIPE_GEMINI_MODEL_FAST", "gemini-2.5-flash"),
			ModelSmart:         getString("GUARDPIPE_GEMINI_MODEL_SMART", "gemini-2.5-pro"),
			TokenBudgetPerScan: getInt("GUARDPIPE_AI_TOKEN_BUDGET_PER_SCAN", 100000, p),
			CacheTTL:           getDuration("GUARDPIPE_AI_CACHE_TTL", 168*time.Hour, p),
		},
		External: External{
			OSVAPIURL:                getString("GUARDPIPE_OSV_API_URL", "https://api.osv.dev"),
			OSVCacheTTL:              getDuration("GUARDPIPE_OSV_CACHE_TTL", 24*time.Hour, p),
			GitHubAPIURL:             getString("GUARDPIPE_GITHUB_API_URL", "https://api.github.com"),
			SonarQubeAPIURL:          getString("GUARDPIPE_SONARQUBE_API_URL", "http://sonarqube:9000"),
			SonarQubeToken:           getString("GUARDPIPE_SONARQUBE_TOKEN", ""),
			SonarQubeAnalysisTimeout: getDuration("GUARDPIPE_SONARQUBE_ANALYSIS_TIMEOUT", 5*time.Minute, p),
		},
		Gate: Gate{
			Warn:  getInt("GUARDPIPE_GATE_WARN", 30, p),
			Block: getInt("GUARDPIPE_GATE_BLOCK", 70, p),
		},
	}

	validateSecurity(cfg, p)
	if cfg.AI.Enabled && len(cfg.AI.KeyPool()) == 0 {
		p.add("GUARDPIPE_GEMINI_API_KEY or GUARDPIPE_GEMINI_API_KEYS is required when GUARDPIPE_AI_ENABLED is true")
	}
	if cfg.Core.Role != RoleAll && cfg.Core.Role != RoleAPI && cfg.Core.Role != RoleWorker {
		p.add("GUARDPIPE_ROLE must be one of \"all\", \"api\", \"worker\", got %q", string(cfg.Core.Role))
	}
	if cfg.Scanning.SandboxBackend != "docker" && cfg.Scanning.SandboxBackend != "kubernetes" {
		p.add("GUARDPIPE_SANDBOX_BACKEND must be \"docker\" or \"kubernetes\", got %q", cfg.Scanning.SandboxBackend)
	}
	if cfg.Scanning.SandboxBackend == "kubernetes" && cfg.Scanning.K8sSandboxImage == "" {
		p.add("GUARDPIPE_K8S_SANDBOX_IMAGE is required when GUARDPIPE_SANDBOX_BACKEND is \"kubernetes\"")
	}

	if len(p.messages) > 0 {
		errs := make([]error, len(p.messages))
		for i, m := range p.messages {
			errs[i] = errors.New(m)
		}
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

// validateSecurity enforces the two hard cryptographic requirements named
// explicitly in documentation/13-devops-and-environments.md §5.3: the JWT
// secret must be at least 32 bytes, and the encryption key must decode from
// base64 to exactly 32 bytes (AES-256).
func validateSecurity(cfg *Config, p *problems) {
	if cfg.Security.JWTSecret != "" && len(cfg.Security.JWTSecret) < 32 {
		p.add("GUARDPIPE_JWT_SECRET must be at least 32 bytes, got %d", len(cfg.Security.JWTSecret))
	}

	rawKey := getString("GUARDPIPE_ENCRYPTION_KEY", "")
	if strings.TrimSpace(rawKey) == "" {
		p.add("GUARDPIPE_ENCRYPTION_KEY is required and was not set")
		return
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		p.add("GUARDPIPE_ENCRYPTION_KEY must be valid base64: %v", err)
		return
	}
	if len(key) != crypto.KeySize {
		p.add("GUARDPIPE_ENCRYPTION_KEY must decode to exactly %d bytes, got %d", crypto.KeySize, len(key))
		return
	}
	cfg.Security.EncryptionKeyRaw = key
}

func loadEngineTimeouts(p *problems) map[domain.EngineID]time.Duration {
	timeouts := make(map[domain.EngineID]time.Duration, len(defaultEngineTimeouts))
	for engine, def := range defaultEngineTimeouts {
		key := "GUARDPIPE_ENGINE_TIMEOUT_" + engineTimeoutEnvSuffix[engine]
		timeouts[engine] = getDuration(key, def, p)
	}
	return timeouts
}
