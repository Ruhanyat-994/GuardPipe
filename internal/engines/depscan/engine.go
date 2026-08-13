package depscan

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
)

// Engine implements domain.Engine — depscan's manifest parsers, secret
// sweep, and advisory lookup, wired together per
// documentation/05-module-specifications.md §4.
type Engine struct {
	advisory advisory.Service
}

func New(advisorySvc advisory.Service) *Engine {
	return &Engine{advisory: advisorySvc}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineDepScan
}

// manifestFiles is every filename any ecosystem parser looks for —
// Applicable is a cheap existence check, not a full parse.
var manifestFiles = []string{
	"package.json", "requirements.txt", "go.mod", "pom.xml", "composer.json",
}

// Applicable searches the whole checkout, not just its root — a project
// laid out as e.g. backend/ + frontend/ subdirectories with nothing at the
// top level is exactly as applicable as one with a manifest at the root
// (documentation/05-module-specifications.md §4: manifests must be
// discovered "anywhere in the checkout").
func (e *Engine) Applicable(ctx context.Context, in domain.ScanInput) (bool, string) {
	for _, name := range manifestFiles {
		dirs, err := findManifestDirs(in.WorkspaceDir, name)
		if err == nil && len(dirs) > 0 {
			return true, ""
		}
	}
	return false, "no recognised dependency manifest (package.json, requirements.txt, go.mod, pom.xml, composer.json) found anywhere in the checkout"
}

// Run orchestrates the whole engine: parse every ecosystem's manifests,
// sweep for secrets, look up advisories, and emit one Finding per rule hit.
// Never writes to the database — the orchestrator persists what Run emits,
// inside one transaction per job (documentation/03-architecture-overview.md
// §6.3).
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	parseResults := []parseResult{}
	for _, parse := range []func(string) ([]parseResult, error){parseNPM, parsePyPI, parseGo, parseMaven, parseComposer} {
		if ctx.Err() != nil {
			return domain.EngineResult{}, ctx.Err()
		}
		results, err := parse(in.WorkspaceDir)
		if err != nil {
			return domain.EngineResult{}, fmt.Errorf("depscan: parse manifests: %w", err)
		}
		parseResults = append(parseResults, results...)
	}

	var allDeps []Dependency
	filesScanned := 0
	for _, result := range parseResults {
		if result.ManifestFound {
			filesScanned++
		}
		allDeps = append(allDeps, result.Dependencies...)

		if result.ManifestFound && !result.LockfileFound {
			emit(e.noLockfileFinding(in.ScanID, result))
		}
	}

	for _, dep := range allDeps {
		if isWildcardRange(dep.DeclaredRange) {
			emit(e.wildcardVersionFinding(in.ScanID, dep))
		}
	}

	if ctx.Err() != nil {
		return domain.EngineResult{}, ctx.Err()
	}
	rulesEvaluated := e.emitAdvisoryFindings(ctx, in.ScanID, allDeps, emit)

	sweep, err := sweepSecrets(in.WorkspaceDir)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("depscan: sweep secrets: %w", err)
	}
	for _, m := range sweep.SecretMatches {
		emit(e.secretFinding(in.ScanID, m))
	}
	for _, path := range sweep.EnvFiles {
		emit(e.envFileFinding(in.ScanID, path))
	}
	for _, path := range sweep.KeyFiles {
		emit(e.keyFileFinding(in.ScanID, path))
	}

	return domain.EngineResult{
		RulesEvaluated: rulesEvaluated + 3, // + the 3 secret rules, always evaluated
		FilesScanned:   filesScanned,
		Skipped: []domain.SkipReason{
			{RuleID: "depscan.hygiene.unmaintained", Reason: "no package-registry client wired yet — see rules.go"},
		},
		Stats: map[string]any{
			"dependencies_found": len(allDeps),
			"secret_matches":     len(sweep.SecretMatches),
		},
	}, nil
}

// emitAdvisoryFindings batches every dependency through modules/advisory
// and emits depscan.vuln.known-cve / depscan.vuln.no-fix-available for
// whatever comes back. Returns how many rules were evaluated (2 per
// dependency actually looked up — OSV-unavailable dependencies are
// silently excluded from the count, they were not evaluated).
func (e *Engine) emitAdvisoryFindings(ctx context.Context, scanID uuid.UUID, deps []Dependency, emit func(domain.Finding)) int {
	if e.advisory == nil || len(deps) == 0 {
		return 0
	}

	lookups := make([]advisory.Dependency, len(deps))
	for i, d := range deps {
		lookups[i] = advisory.Dependency{Ecosystem: d.Ecosystem, Name: d.Name, Version: d.Version}
	}

	results, err := e.advisory.Lookup(ctx, lookups)
	if err != nil {
		return 0 // OSV-wide failure: inventory stands, advisories just aren't available this run
	}

	evaluated := 0
	for i, result := range results {
		if result.Unavailable {
			continue
		}
		evaluated += 2
		for _, adv := range result.Advisories {
			emit(e.knownCVEFinding(scanID, deps[i], adv))
			if !adv.HasFix {
				emit(e.noFixAvailableFinding(scanID, deps[i], adv))
			}
		}
	}
	return evaluated
}

// --- normalisation + fingerprinting (documentation/06-database-design.md §6) ---

var (
	reQuotedString = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	reNumber       = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
)

// normalizeEvidence collapses whitespace and replaces literals so
// reformatting doesn't create a new fingerprint, but a genuine content
// change does (documentation/06-database-design.md §6).
func normalizeEvidence(s string) string {
	s = reQuotedString.ReplaceAllString(s, "<str>")
	s = reNumber.ReplaceAllString(s, "<num>")
	return strings.Join(strings.Fields(s), " ")
}
