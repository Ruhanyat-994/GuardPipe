package config_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/config"
)

// setRequiredEnv sets every variable Load() treats as required, so each test
// can start from a known-valid baseline and override just the one thing it's
// checking. t.Setenv automatically restores the previous value after the
// test, so tests never leak environment state into each other.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GUARDPIPE_DATABASE_URL", "postgres://user:pass@localhost:5432/guardpipe")
	t.Setenv("GUARDPIPE_REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("GUARDPIPE_JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("GUARDPIPE_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("GUARDPIPE_AI_ENABLED", "false") // avoids also requiring GUARDPIPE_GEMINI_API_KEY by default
}

func TestLoad_ValidEnvironmentSucceeds(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.Core.Env != "development" {
		t.Errorf("Core.Env = %q, want the default %q", cfg.Core.Env, "development")
	}
	if cfg.Core.Role != config.RoleAll {
		t.Errorf("Core.Role = %q, want the default %q", cfg.Core.Role, config.RoleAll)
	}
	if cfg.Data.DBMaxConns != 25 {
		t.Errorf("Data.DBMaxConns = %d, want the default 25", cfg.Data.DBMaxConns)
	}
}

func TestLoad_MissingRequiredVariablesFailFast(t *testing.T) {
	// Deliberately does not call setRequiredEnv — every required variable is
	// unset (unless already set in the OS environment, which t.Setenv above
	// in other tests always restores after itself).
	t.Setenv("GUARDPIPE_DATABASE_URL", "")
	t.Setenv("GUARDPIPE_REDIS_URL", "")
	t.Setenv("GUARDPIPE_JWT_SECRET", "")
	t.Setenv("GUARDPIPE_ENCRYPTION_KEY", "")
	t.Setenv("GUARDPIPE_AI_ENABLED", "false")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error naming the missing variables")
	}
	for _, want := range []string{
		"GUARDPIPE_DATABASE_URL", "GUARDPIPE_REDIS_URL",
		"GUARDPIPE_JWT_SECRET", "GUARDPIPE_ENCRYPTION_KEY",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
}

func TestLoad_ShortJWTSecretFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_JWT_SECRET", "too-short")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a JWT secret under 32 bytes")
	}
	if !strings.Contains(err.Error(), "GUARDPIPE_JWT_SECRET") {
		t.Errorf("error does not name GUARDPIPE_JWT_SECRET: %v", err)
	}
}

// TestLoad_JWTSecretAtExactly32BytesSucceeds is the near-miss boundary
// check: the doc says "at least 32 bytes" — exactly 32 must pass, not just
// 33+.
func TestLoad_JWTSecretAtExactly32BytesSucceeds(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_JWT_SECRET", strings.Repeat("x", 32))

	if _, err := config.Load(); err != nil {
		t.Errorf("Load() error = %v, want nil for an exactly-32-byte secret", err)
	}
}

func TestLoad_WrongLengthEncryptionKeyFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 16))) // AES-128 length

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error for a 16-byte encryption key")
	}
	if !strings.Contains(err.Error(), "GUARDPIPE_ENCRYPTION_KEY") {
		t.Errorf("error does not name GUARDPIPE_ENCRYPTION_KEY: %v", err)
	}
}

func TestLoad_NonBase64EncryptionKeyFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_ENCRYPTION_KEY", "not valid base64 !!!")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want an error for invalid base64")
	}
}

func TestLoad_AIEnabledRequiresGeminiAPIKey(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_AI_ENABLED", "true")
	t.Setenv("GUARDPIPE_GEMINI_API_KEY", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an error when AI is enabled with no API key")
	}
	if !strings.Contains(err.Error(), "GUARDPIPE_GEMINI_API_KEY") {
		t.Errorf("error does not name GUARDPIPE_GEMINI_API_KEY: %v", err)
	}
}

// TestLoad_AIDisabledDoesNotRequireGeminiAPIKey is the near-miss half: the
// master AI switch being off must not drag in an unrelated requirement.
func TestLoad_AIDisabledDoesNotRequireGeminiAPIKey(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_AI_ENABLED", "false")
	t.Setenv("GUARDPIPE_GEMINI_API_KEY", "")

	if _, err := config.Load(); err != nil {
		t.Errorf("Load() error = %v, want nil when AI is disabled", err)
	}
}

func TestLoad_InvalidRoleFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_ROLE", "supervisor")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want an error for an invalid GUARDPIPE_ROLE")
	}
}

func TestLoad_CORSOriginsSplitsOnComma(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_CORS_ORIGINS", "https://a.example.com, https://b.example.com")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(cfg.Security.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins = %v, want %v", cfg.Security.CORSOrigins, want)
	}
	for i := range want {
		if cfg.Security.CORSOrigins[i] != want[i] {
			t.Errorf("CORSOrigins[%d] = %q, want %q", i, cfg.Security.CORSOrigins[i], want[i])
		}
	}
}

func TestLoad_EngineTimeoutDefaultsMatchDocumentedValues(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// documentation/04-backend-architecture.md §6.3.
	wantMinutes := map[string]int{
		"docreview":     5,
		"codescan":      5,
		"depscan":       3,
		"containerscan": 8,
		"k8sscan":       2,
		"cicdscan":      3,
		"pentest":       15,
	}
	if len(cfg.Scanning.EngineTimeouts) != len(wantMinutes) {
		t.Fatalf("got %d engine timeouts, want %d", len(cfg.Scanning.EngineTimeouts), len(wantMinutes))
	}
	for engine, timeout := range cfg.Scanning.EngineTimeouts {
		want, ok := wantMinutes[string(engine)]
		if !ok {
			t.Errorf("unexpected engine %q in EngineTimeouts", engine)
			continue
		}
		if timeout.Minutes() != float64(want) {
			t.Errorf("EngineTimeouts[%q] = %s, want %d minutes", engine, timeout, want)
		}
	}
}

func TestLoad_EngineTimeoutOverrideIsRespected(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_ENGINE_TIMEOUT_PENTEST", "30m")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Scanning.EngineTimeouts["pentest"]; got.Minutes() != 30 {
		t.Errorf("EngineTimeouts[pentest] = %s, want 30m after override", got)
	}
}

func TestLoad_InvalidDurationFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_ACCESS_TOKEN_TTL", "not-a-duration")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want an error for an invalid duration")
	}
}

func TestLoad_InvalidIntFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_DB_MAX_CONNS", "not-a-number")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want an error for a non-integer value")
	}
}

func TestLoad_InvalidBoolFailsFast(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GUARDPIPE_MIGRATE_ON_START", "definitely")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want an error for a non-boolean value")
	}
}
