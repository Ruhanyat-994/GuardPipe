package depscan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSecretFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// --- depscan.secrets.committed-credential: true-positive + near-miss table ---

func TestSweepSecrets_CommittedCredential(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantMatch bool
	}{
		{"AWS access key", "aws_key = \"AKIAIOSFODNN7EXAMPLE\"", true},
		{"GitHub PAT", "token := \"ghp_1234567890abcdef1234567890abcdef1234\"", true},
		{"Slack token", "SLACK_TOKEN=\"xoxb-111111111-222222222-abcdefghijklmnopqrstuvwx\"", true},
		{"PEM private key header", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----", true},
		{"generic hardcoded password", `password = "S0meReallyStrongSecret!"`, true},

		// near-misses — must not fire
		{"placeholder password", `password = "changeme"`, false},
		{"example api key", `api_key = "your-api-key-here"`, false},
		{"template placeholder", `secret: "${SECRET_VALUE}"`, false},
		{"env var reference, not a literal", `password = os.Getenv("DB_PASSWORD")`, false},
		{"too short to be a real secret", `token = "abc123"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSecretFile(t, dir, "config.go", tt.content)

			result, err := sweepSecrets(dir)
			require.NoError(t, err)
			if tt.wantMatch {
				require.NotEmpty(t, result.SecretMatches, "expected a secret match")
			} else {
				require.Empty(t, result.SecretMatches, "expected no secret match (near-miss)")
			}
		})
	}
}

func TestSweepSecrets_RedactsBeforeStorage(t *testing.T) {
	redacted := redactSecret("AKIAIOSFODNN7EXAMPLE")
	require.NotContains(t, redacted, "IOSFODNN7EXAMPL")
	require.Contains(t, redacted, "AKIA") // shape (prefix) preserved, not the full value
}

// --- depscan.secrets.env-file-committed: true-positive + near-miss table ---

func TestSweepSecrets_EnvFileCommitted(t *testing.T) {
	tests := []struct {
		name       string
		fileName   string
		wantFlag   bool
		wantReason string
	}{
		{"plain .env", ".env", true, "a real .env file must be flagged"},
		{"environment-specific .env", ".env.production", true, "environment-scoped .env files must be flagged too"},
		{"local override", ".env.local", true, "local override must be flagged"},
		{"example template", ".env.example", false, "documented safe convention — must not fire"},
		{"sample template", ".env.sample", false, "documented safe convention — must not fire"},
		{"unrelated file", "environment.go", false, "must not match on substring alone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSecretFile(t, dir, tt.fileName, "SOME_VAR=value\n")

			result, err := sweepSecrets(dir)
			require.NoError(t, err)
			if tt.wantFlag {
				require.Contains(t, result.EnvFiles, tt.fileName, tt.wantReason)
			} else {
				require.Empty(t, result.EnvFiles, tt.wantReason)
			}
		})
	}
}

// --- depscan.secrets.key-file-committed: true-positive + near-miss table ---

func TestSweepSecrets_KeyFileCommitted(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantFlag bool
	}{
		{"PEM file", "server.pem", true},
		{"key file", "private.key", true},
		{"PKCS12", "cert.p12", true},
		{"PFX", "cert.pfx", true},
		{"SSH private key by name", "id_rsa", true},

		{"public key, not private", "id_rsa.pub", false},
		{"unrelated file ending in similar letters", "monkey.go", false},
		{"keychain reference in code, not a file", "notes.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSecretFile(t, dir, tt.fileName, "not a real key, just test content\n")

			result, err := sweepSecrets(dir)
			require.NoError(t, err)
			if tt.wantFlag {
				require.Contains(t, result.KeyFiles, tt.fileName)
			} else {
				require.Empty(t, result.KeyFiles)
			}
		})
	}
}

func TestSweepSecrets_SkipsNodeModulesAndGitDirs(t *testing.T) {
	dir := t.TempDir()
	writeSecretFile(t, dir, "node_modules/some-pkg/.env", "SECRET=1\n")
	writeSecretFile(t, dir, ".git/config", "AKIAIOSFODNN7EXAMPLE\n")

	result, err := sweepSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, result.EnvFiles)
	require.Empty(t, result.SecretMatches)
}

func TestSweepSecrets_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	binary := append([]byte("AKIAIOSFODNN7EXAMPLE"), 0x00, 0x01, 0x02)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.bin"), binary, 0o644))

	result, err := sweepSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, result.SecretMatches, "a binary file's text segments are out of scope")
}
