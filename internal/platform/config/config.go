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
	RefreshTokenTTL  time.Duration
	CORSOrigins      []string
}

// Scanning — §5.4.
type Scanning struct {
	WorkerCount    int
	WorkspaceRoot  string
	MaxRepoMB      int
	SandboxMax     int
	SandboxImage   string
	DockerHost     string
	EngineTimeouts map[domain.EngineID]time.Duration
}

// Pentest — §5.5.
type Pentest struct {
	Enabled             bool
	AllowPrivateTargets bool
	Allowlist           []string
	RateLimit           int
	Ports               string
}

// AI — §5.6.
type AI struct {
	Enabled            bool
	GeminiAPIKey       string
	ModelFast          string
	ModelSmart         string
	TokenBudgetPerScan int
	CacheTTL           time.Duration
}

// External — §5.7.
type External struct {
	OSVAPIURL    string
	OSVCacheTTL  time.Duration
	GitHubAPIURL string
}

// Gate — §5.8.
type Gate struct {
	Warn  int
	Block int
}

// defaultEngineTimeouts matches documentation/04-backend-architecture.md
// §6.3 exactly.
var defaultEngineTimeouts = map[domain.EngineID]time.Duration{
	domain.EngineDocReview:     5 * time.Minute,
	domain.EngineCodeScan:      5 * time.Minute,
	domain.EngineDepScan:       3 * time.Minute,
	domain.EngineContainerScan: 8 * time.Minute,
	domain.EngineK8sScan:       2 * time.Minute,
	domain.EngineCICDScan:      3 * time.Minute,
	domain.EnginePentest:       15 * time.Minute,
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

	cfg := &Config{
		Core: Core{
			Env:      getString("GUARDPIPE_ENV", "development"),
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
			JWTSecret:       requireString("GUARDPIPE_JWT_SECRET", p),
			AccessTokenTTL:  getDuration("GUARDPIPE_ACCESS_TOKEN_TTL", 15*time.Minute, p),
			RefreshTokenTTL: getDuration("GUARDPIPE_REFRESH_TOKEN_TTL", 168*time.Hour, p),
			CORSOrigins:     getCSV("GUARDPIPE_CORS_ORIGINS", []string{"http://localhost:5173"}),
		},
		Scanning: Scanning{
			WorkerCount:    getInt("GUARDPIPE_WORKER_COUNT", 4, p),
			WorkspaceRoot:  getString("GUARDPIPE_WORKSPACE_ROOT", "/var/lib/guardpipe/workspace"),
			MaxRepoMB:      getInt("GUARDPIPE_MAX_REPO_MB", 500, p),
			SandboxMax:     getInt("GUARDPIPE_SANDBOX_MAX", 2, p),
			SandboxImage:   getString("GUARDPIPE_SANDBOX_IMAGE", ""),
			DockerHost:     getString("GUARDPIPE_DOCKER_HOST", "unix:///var/run/docker.sock"),
			EngineTimeouts: loadEngineTimeouts(p),
		},
		Pentest: Pentest{
			Enabled:             getBool("GUARDPIPE_PENTEST_ENABLED", true, p),
			AllowPrivateTargets: getBool("GUARDPIPE_ALLOW_PRIVATE_TARGETS", false, p),
			Allowlist:           getCSV("GUARDPIPE_PENTEST_ALLOWLIST", nil),
			RateLimit:           getInt("GUARDPIPE_PENTEST_RATE_LIMIT", 10, p),
			Ports:               getString("GUARDPIPE_PENTEST_PORTS", "top100"),
		},
		AI: AI{
			Enabled:            getBool("GUARDPIPE_AI_ENABLED", true, p),
			GeminiAPIKey:       getString("GUARDPIPE_GEMINI_API_KEY", ""),
			ModelFast:          getString("GUARDPIPE_GEMINI_MODEL_FAST", "gemini-2.5-flash"),
			ModelSmart:         getString("GUARDPIPE_GEMINI_MODEL_SMART", "gemini-2.5-pro"),
			TokenBudgetPerScan: getInt("GUARDPIPE_AI_TOKEN_BUDGET_PER_SCAN", 100000, p),
			CacheTTL:           getDuration("GUARDPIPE_AI_CACHE_TTL", 168*time.Hour, p),
		},
		External: External{
			OSVAPIURL:    getString("GUARDPIPE_OSV_API_URL", "https://api.osv.dev"),
			OSVCacheTTL:  getDuration("GUARDPIPE_OSV_CACHE_TTL", 24*time.Hour, p),
			GitHubAPIURL: getString("GUARDPIPE_GITHUB_API_URL", "https://api.github.com"),
		},
		Gate: Gate{
			Warn:  getInt("GUARDPIPE_GATE_WARN", 30, p),
			Block: getInt("GUARDPIPE_GATE_BLOCK", 70, p),
		},
	}

	validateSecurity(cfg, p)
	if cfg.AI.Enabled && strings.TrimSpace(cfg.AI.GeminiAPIKey) == "" {
		p.add("GUARDPIPE_GEMINI_API_KEY is required when GUARDPIPE_AI_ENABLED is true")
	}
	if cfg.Core.Role != RoleAll && cfg.Core.Role != RoleAPI && cfg.Core.Role != RoleWorker {
		p.add("GUARDPIPE_ROLE must be one of \"all\", \"api\", \"worker\", got %q", string(cfg.Core.Role))
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
