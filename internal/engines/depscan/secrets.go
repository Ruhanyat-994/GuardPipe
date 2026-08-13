package depscan

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// secretMatch is one hit from the pattern-based sweep — normalisePath.go
// turns these into Findings.
type secretMatch struct {
	Pattern   string
	Path      string // relative to the workspace root
	LineStart int
	Matched   string // the raw matched text — never stored verbatim, see redactSecret
}

type secretPattern struct {
	name    string
	pattern *regexp.Regexp
}

// secretPatterns is depscan's own secret ruleset, built now because
// codescan (this ruleset's eventual permanent home — "reuses codescan's
// secret rule set via a shared internal package, the rules live once" per
// documentation/05-module-specifications.md) doesn't exist yet (Phase 7).
// Kept in this package until then rather than left unbuilt — the same
// build-ahead-of-the-consumer sequencing Phase 4's ai module and Phase 5's
// RuleRegistry both used.
var secretPatterns = []secretPattern{
	{"AWS Access Key ID", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"GitHub Token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36}\b`)},
	{"Slack Token", regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z-]{10,48}\b`)},
	{"Private Key", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA |)PRIVATE KEY-----`)},
	// The catch-all: name-based, not signature-based, so it needs the
	// strongest near-miss discrimination — see isLikelyPlaceholder.
	{"Hardcoded credential assignment", regexp.MustCompile(`(?i)\b(password|secret|api[_-]?key|access[_-]?token|auth[_-]?token)\s*[:=]\s*['"]([^'"]{12,})['"]`)},
}

// placeholderMarkers suppresses the generic assignment pattern's most
// common near-misses: template/example values that are not real secrets.
// This is the discrimination that keeps depscan.secrets.committed-credential
// usable rather than a noise generator on every .env.example and README
// in the fixture (documentation/15-testing-strategy.md: "the near-miss half
// is the valuable half").
var placeholderMarkers = []string{
	"changeme", "change_me", "your-", "your_", "example", "placeholder",
	"xxxxxxxx", "dummy", "test-", "test_", "<", "${", "{{",
}

func isLikelyPlaceholder(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range placeholderMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// skipDirs are never walked — noise, not source: dependency trees, VCS
// internals, and build output.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".venv": true, "__pycache__": true,
}

// envFilePattern matches .env, .env.local, .env.production, etc. — but not
// .env.example/.env.sample/.env.template, the near-miss (a committed
// template is the documented, safe convention, not a leak).
var envFilePattern = regexp.MustCompile(`^\.env(\.[A-Za-z0-9_-]+)?$`)

var envFileNearMiss = map[string]bool{".env.example": true, ".env.sample": true, ".env.template": true}

var keyFileSuffixes = []string{".pem", ".key", ".p12", ".pfx"}

// sweepResult is what walkForSecrets reports — three independent rule
// signals from one filesystem walk, rather than three separate walks.
type sweepResult struct {
	SecretMatches []secretMatch
	EnvFiles      []string // relative paths
	KeyFiles      []string // relative paths
}

// sweepSecrets walks the whole checkout — depscan.secrets.* Core rules 3-5
// (documentation/05-module-specifications.md: "The whole checkout, not just
// manifests"). Read-only, never executes anything it finds.
func sweepSecrets(workspaceDir string) (sweepResult, error) {
	var result sweepResult

	err := filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		base := d.Name()

		if envFilePattern.MatchString(base) && !envFileNearMiss[base] {
			result.EnvFiles = append(result.EnvFiles, rel)
		}
		if base == "id_rsa" || hasKeyFileSuffix(base) {
			result.KeyFiles = append(result.KeyFiles, rel)
		}

		matches, err := scanFileForSecrets(path, rel)
		if err != nil {
			return nil // an unreadable file (permissions, binary, race) is skipped, not fatal to the sweep
		}
		result.SecretMatches = append(result.SecretMatches, matches...)
		return nil
	})
	if err != nil {
		return sweepResult{}, err
	}
	return result, nil
}

func hasKeyFileSuffix(name string) bool {
	return slices.ContainsFunc(keyFileSuffixes, func(suffix string) bool {
		return strings.HasSuffix(name, suffix)
	})
}

// binaryProbeSize is how many leading bytes scanFileForSecrets inspects to
// decide "binary, skip" — matching the module spec's "text segments only"
// scope for git-tracked binaries.
const binaryProbeSize = 512

func scanFileForSecrets(path, relPath string) ([]secretMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	probe := make([]byte, binaryProbeSize)
	n, _ := f.Read(probe)
	if looksBinary(probe[:n]) {
		return nil, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	var matches []secretMatch
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		for _, sp := range secretPatterns {
			m := sp.pattern.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			matched := m[0]
			if sp.name == "Hardcoded credential assignment" && isLikelyPlaceholder(m[2]) {
				continue
			}
			matches = append(matches, secretMatch{Pattern: sp.name, Path: relPath, LineStart: lineNum, Matched: matched})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}

func looksBinary(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

// redactSecret returns a shape indicator, never the real value —
// documentation/06-database-design.md §4.12: "a finding that detects a
// hardcoded API key must store the location and shape of the secret, never
// the secret itself."
func redactSecret(matched string) string {
	if len(matched) <= 8 {
		return strings.Repeat("•", len(matched))
	}
	return matched[:4] + strings.Repeat("•", len(matched)-8) + matched[len(matched)-4:]
}
