// Package containerscan wraps Trivy for Dockerfile-misconfiguration and
// image-vulnerability/secret analysis (documentation/05-module-specifications.md
// §8, BUILD_GUIDE.md Phase 8, ADR-0012) instead of implementing GuardPipe's
// own layer-walking analyzer. It runs `trivy config` against the cloned
// workspace, builds and scans the image if a Dockerfile is present, and
// normalises Trivy's output into domain.Finding.
package containerscan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Scanner is the subset of adapters/trivy.Scanner this package needs —
// defined here (the consumer), so tests substitute a fake instead of
// making a real Docker call (documentation/15-testing-strategy.md: "no
// mocking framework ... hand-written fakes", "Tests never call ... live").
type Scanner interface {
	ScanConfig(ctx context.Context, workspaceDir string) (trivy.Report, error)
	ScanImage(ctx context.Context, ref string) (trivy.Report, error)
}

// ImageBuilder is the subset of adapters/dockerx.Client this package
// needs to build the target repository's own Dockerfile into a scannable
// image.
type ImageBuilder interface {
	BuildImage(ctx context.Context, contextDir, dockerfilePath, tag string) error
	RemoveImage(ctx context.Context, ref string) error
}

// RuleRegistrar is the subset of modules/advisory.Service this package
// needs — just per-rule upsert. Trivy's own check/CVE catalogue isn't
// enumerable at compile time the way depscan's Rules slice is, so
// containerscan registers each rule the first time it actually fires a
// finding, not as a static batch at startup (same reasoning engines/codescan
// already applies to SonarQube's rule catalogue).
type RuleRegistrar interface {
	UpsertRule(ctx context.Context, rm domain.RuleMeta) error
}

// Engine implements domain.Engine by wrapping Trivy.
type Engine struct {
	scanner Scanner
	builder ImageBuilder
	rules   RuleRegistrar
}

func New(scanner Scanner, builder ImageBuilder, rules RuleRegistrar) *Engine {
	return &Engine{scanner: scanner, builder: builder, rules: rules}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineContainerScan
}

// dockerfileNames matches FR-CNT-001's discovery scope: `Dockerfile`,
// `*.dockerfile`, and `Containerfile`, case-insensitively.
func isDockerfileName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "dockerfile" || lower == "containerfile" || strings.HasSuffix(lower, ".dockerfile")
}

// skipDirs mirrors engines/codescan's own list (unexported there too, small
// enough to duplicate rather than extract a shared package for one map).
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".venv": true, "__pycache__": true,
}

// findDockerfile returns the first discovered Dockerfile's path relative to
// workspaceDir (Docker's `-f` expects a context-relative path), or "" if
// none exists.
func findDockerfile(workspaceDir string) string {
	found := ""
	_ = filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != workspaceDir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if isDockerfileName(d.Name()) {
			rel, relErr := filepath.Rel(workspaceDir, path)
			if relErr != nil {
				return nil
			}
			found = filepath.ToSlash(rel)
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// Applicable is a cheap existence check — does the checkout have a
// Dockerfile at all, not a full parse.
func (e *Engine) Applicable(_ context.Context, in domain.ScanInput) (bool, string) {
	if findDockerfile(in.WorkspaceDir) != "" {
		return true, ""
	}
	return false, "no Dockerfile found"
}

// Run scans the cloned workspace's Dockerfile for misconfiguration, then
// builds and scans the image itself for vulnerabilities and image-layer
// secrets. Any failure here — the config scan, the image build, or the
// image scan — fails this job only; the orchestrator's partial-result
// handling (FR-ORC-006/NFR-REL-001) takes it from there, the rest of the
// scan continues.
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	dockerfilePath := findDockerfile(in.WorkspaceDir)

	configReport, err := e.scanner.ScanConfig(ctx, in.WorkspaceDir)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("containerscan: scan config: %w", err)
	}

	registered := make(map[string]bool)
	filesScanned := 0

	for _, result := range configReport.Results {
		filesScanned++
		for _, m := range result.Misconfigurations {
			if ctx.Err() != nil {
				return domain.EngineResult{}, ctx.Err()
			}
			if err := e.registerMisconfigRule(ctx, m, registered); err != nil {
				return domain.EngineResult{}, fmt.Errorf("containerscan: register rule %q: %w", m.ID, err)
			}
			emit(misconfigFinding(in.ScanID, result.Target, m))
		}
	}

	imageRef := "guardpipe-containerscan-" + in.ScanID.String() + ":latest"
	if err := e.buildImage(ctx, in.WorkspaceDir, dockerfilePath, imageRef); err != nil {
		return domain.EngineResult{}, fmt.Errorf("containerscan: build image: %w", err)
	}
	defer func() {
		// Background context, same reasoning as the sibling-container
		// cleanup in adapters/trivy/scanner.go: a cancelled/timed-out ctx
		// must not also cancel cleanup, or the built image leaks.
		_ = e.builder.RemoveImage(context.Background(), imageRef)
	}()

	imageReport, err := e.scanner.ScanImage(ctx, imageRef)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("containerscan: scan image: %w", err)
	}

	vulnCount, secretCount := 0, 0
	for _, result := range imageReport.Results {
		for _, v := range result.Vulnerabilities {
			if ctx.Err() != nil {
				return domain.EngineResult{}, ctx.Err()
			}
			if err := e.registerVulnRule(ctx, v, registered); err != nil {
				return domain.EngineResult{}, fmt.Errorf("containerscan: register rule %q: %w", v.VulnerabilityID, err)
			}
			emit(vulnerabilityFinding(in.ScanID, imageRef, v))
			vulnCount++
		}
		for _, s := range result.Secrets {
			if ctx.Err() != nil {
				return domain.EngineResult{}, ctx.Err()
			}
			if err := e.registerSecretRule(ctx, s, registered); err != nil {
				return domain.EngineResult{}, fmt.Errorf("containerscan: register rule %q: %w", s.RuleID, err)
			}
			emit(secretFinding(in.ScanID, imageRef, result.Target, s))
			secretCount++
		}
	}

	return domain.EngineResult{
		RulesEvaluated: len(registered),
		FilesScanned:   filesScanned,
		Stats: map[string]any{
			"misconfigurations_found": len(registered) - vulnCount - secretCount,
			"vulnerabilities_found":   vulnCount,
			"secrets_found":           secretCount,
		},
	}, nil
}

// buildImage tries the repository root as the build context first (needed
// when a Dockerfile COPYs files from elsewhere in a monorepo), then falls
// back to the Dockerfile's own directory on failure — many single-service
// repos write their Dockerfile assuming `docker build .` is run from inside
// that same directory, with COPY paths relative to it rather than the repo
// root. Confirmed against a real repository: a Dockerfile at
// backend/Dockerfile COPYing backend/PaperPulse.Domain/*.csproj failed
// entirely under a repo-root context (those paths don't exist relative to
// the root) and succeeded once retried with backend/ itself as context.
func (e *Engine) buildImage(ctx context.Context, workspaceDir, dockerfilePath, imageRef string) error {
	err := e.builder.BuildImage(ctx, workspaceDir, dockerfilePath, imageRef)
	if err == nil {
		return nil
	}

	altContext := filepath.Join(workspaceDir, filepath.Dir(dockerfilePath))
	if altContext == workspaceDir {
		return err // the Dockerfile is already at the root — no different context to retry with
	}
	return e.builder.BuildImage(ctx, altContext, filepath.Base(dockerfilePath), imageRef)
}

// registerMisconfigRule/registerVulnRule/registerSecretRule upsert one
// Trivy-derived rule into the rules catalogue, once per distinct key per
// run (registered memoizes) — must happen before any Finding referencing
// that rule's RuleID is persisted, findings.rule_id is a real foreign key
// into rules (documentation/06-database-design.md), same reasoning
// engines/codescan's own registerRule documents. Unlike SonarQube, Trivy's
// own output already carries every field a RuleMeta needs — no separate
// "GetRule" lookup call required.
func (e *Engine) registerMisconfigRule(ctx context.Context, m trivy.Misconfig, registered map[string]bool) error {
	ruleID := "containerscan.trivy." + m.ID
	if registered[ruleID] {
		return nil
	}
	registered[ruleID] = true
	return e.rules.UpsertRule(ctx, domain.RuleMeta{
		ID: ruleID, Title: firstNonEmpty(m.Title, m.ID),
		Description: firstNonEmpty(m.Message, m.Title),
		Severity:    severityFromTrivy(m.Severity), Confidence: domain.ConfidenceHigh,
		Remediation: firstNonEmpty(m.Resolution, "See the corresponding Trivy check for detailed remediation guidance."),
		Tier:        domain.TierCore,
	})
}

func (e *Engine) registerVulnRule(ctx context.Context, v trivy.Vulnerability, registered map[string]bool) error {
	ruleID := "containerscan.trivy." + v.VulnerabilityID
	if registered[ruleID] {
		return nil
	}
	registered[ruleID] = true
	return e.rules.UpsertRule(ctx, domain.RuleMeta{
		ID: ruleID, Title: firstNonEmpty(v.Title, v.VulnerabilityID),
		Description: firstNonEmpty(v.Description, v.Title),
		Severity:    severityFromTrivy(v.Severity), Confidence: domain.ConfidenceHigh,
		CWE:         normalizeCWE(v.CweIDs),
		Remediation: "Upgrade the affected package to a fixed version, when Trivy reports one.",
		References:  nonEmptyRefs(v.PrimaryURL),
		Tier:        domain.TierCore,
	})
}

func (e *Engine) registerSecretRule(ctx context.Context, s trivy.Secret, registered map[string]bool) error {
	ruleID := "containerscan.trivy." + s.RuleID
	if registered[ruleID] {
		return nil
	}
	registered[ruleID] = true
	return e.rules.UpsertRule(ctx, domain.RuleMeta{
		ID: ruleID, Title: firstNonEmpty(s.Title, s.RuleID),
		Description: "Secret pattern (" + firstNonEmpty(s.Category, "unknown category") + ") found in a built image layer.",
		Severity:    severityFromTrivy(s.Severity), Confidence: domain.ConfidenceHigh,
		Remediation: "Remove the secret from the image and rotate it — rebuilding alone does not invalidate a credential already baked into a published layer.",
		Tier:        domain.TierCore,
	})
}

func nonEmptyRefs(url string) []string {
	if url == "" {
		return nil
	}
	return []string{url}
}
