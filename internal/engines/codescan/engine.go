// Package codescan wraps a self-hosted SonarQube Community Edition
// instance for static analysis (documentation/05-module-specifications.md
// §6, BUILD_GUIDE.md Phase 7, ADR-0011) instead of implementing GuardPipe's
// own SAST engine. It triggers analysis, polls for completion, filters
// SonarQube's output to security-relevant findings only
// (VULNERABILITY issues + SECURITY_HOTSPOTs), and normalises them into
// domain.Finding.
package codescan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sonarqube"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// SonarQubeClient is the subset of adapters/sonarqube.Client this package
// needs — defined here (the consumer), so tests substitute a fake instead of
// making a real HTTP call (documentation/15-testing-strategy.md: "no mocking
// framework ... hand-written fakes", "Tests never call ... live").
type SonarQubeClient interface {
	GetTask(ctx context.Context, taskID string) (sonarqube.Task, error)
	SearchIssues(ctx context.Context, projectKey string) ([]sonarqube.Issue, error)
	SearchHotspots(ctx context.Context, projectKey string) ([]sonarqube.Hotspot, error)
	GetRule(ctx context.Context, ruleKey string) (sonarqube.RuleInfo, error)
}

// Scanner is the subset of adapters/sonarqube.Scanner this package needs.
type Scanner interface {
	Analyze(ctx context.Context, workspaceDir, projectKey string) (taskID string, err error)
}

// RuleRegistrar is the subset of modules/advisory.Service this package
// needs — just per-rule upsert, not the OSV-lookup surface depscan uses.
// SonarQube's own rule catalogue has thousands of rules and isn't
// enumerable at compile time the way depscan's Rules slice is, so codescan
// registers each rule the first time it actually fires a finding, not as a
// static batch at startup.
type RuleRegistrar interface {
	UpsertRule(ctx context.Context, rm domain.RuleMeta) error
}

// Engine implements domain.Engine by wrapping SonarQube.
type Engine struct {
	sonarqube       SonarQubeClient
	scanner         Scanner
	rules           RuleRegistrar
	analysisTimeout time.Duration
	pollInterval    time.Duration
}

// New builds the codescan Engine. analysisTimeout is
// GUARDPIPE_SONARQUBE_ANALYSIS_TIMEOUT — the bound on the /api/ce/task poll
// loop specifically, independent of (though in practice smaller than) the
// overall per-engine ctx deadline the orchestrator already applies
// (GUARDPIPE_ENGINE_TIMEOUT_CODESCAN).
func New(sonarqubeClient SonarQubeClient, scanner Scanner, rules RuleRegistrar, analysisTimeout time.Duration) *Engine {
	if analysisTimeout <= 0 {
		analysisTimeout = 5 * time.Minute
	}
	return &Engine{
		sonarqube: sonarqubeClient, scanner: scanner, rules: rules,
		analysisTimeout: analysisTimeout, pollInterval: 3 * time.Second,
	}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineCodeScan
}

// sourceExtensions is the same five-language floor the old in-house spec
// declared (documentation/05-module-specifications.md §6's language-support
// table) — SonarQube CE analyses all five out of the box.
var sourceExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".java": true, ".php": true,
}

// skipDirs mirrors engines/depscan's own list (unexported there, small
// enough to duplicate rather than extract a shared package for one map).
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".venv": true, "__pycache__": true,
}

// Applicable is a cheap existence check — does the checkout have anything
// SonarQube would analyse — not a full parse.
func (e *Engine) Applicable(_ context.Context, in domain.ScanInput) (bool, string) {
	found := false
	_ = filepath.WalkDir(in.WorkspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != in.WorkspaceDir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if sourceExtensions[strings.ToLower(filepath.Ext(d.Name()))] {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if found {
		return true, ""
	}
	return false, "no supported source files (.go .py .js .jsx .ts .tsx .java .php) found anywhere in the checkout"
}

// Run triggers a SonarQube analysis, waits for it to finish, reads back
// vulnerabilities and security hotspots, and emits one Finding per result.
// Any failure here — the scanner container, the poll loop, or either search
// call — fails this job only; the orchestrator's partial-result handling
// (FR-ORC-006/NFR-REL-001) takes it from there, the rest of the scan
// continues.
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	projectKey := "guardpipe-" + in.ProjectID.String()

	taskID, err := e.scanner.Analyze(ctx, in.WorkspaceDir, projectKey)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("codescan: run sonar-scanner: %w", err)
	}

	task, err := e.pollTask(ctx, taskID)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("codescan: poll analysis task: %w", err)
	}
	if task.Status != sonarqube.TaskSuccess {
		return domain.EngineResult{}, fmt.Errorf("codescan: SonarQube analysis %s: %s", strings.ToLower(string(task.Status)), task.ErrorMsg)
	}

	issues, err := e.sonarqube.SearchIssues(ctx, projectKey)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("codescan: search issues: %w", err)
	}
	hotspots, err := e.sonarqube.SearchHotspots(ctx, projectKey)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("codescan: search hotspots: %w", err)
	}

	ruleCache := make(map[string]sonarqube.RuleInfo)
	registered := make(map[string]bool)

	for _, iss := range issues {
		if ctx.Err() != nil {
			return domain.EngineResult{}, ctx.Err()
		}
		rule := e.ruleInfo(ctx, iss.RuleKey, ruleCache)
		if err := e.registerRule(ctx, iss.RuleKey, rule, severityFromIssue(iss.Severity), registered); err != nil {
			return domain.EngineResult{}, fmt.Errorf("codescan: register rule %q: %w", iss.RuleKey, err)
		}
		emit(issueFinding(in.ScanID, projectKey, iss, rule))
	}
	for _, hs := range hotspots {
		if ctx.Err() != nil {
			return domain.EngineResult{}, ctx.Err()
		}
		rule := e.ruleInfo(ctx, hs.RuleKey, ruleCache)
		if err := e.registerRule(ctx, hs.RuleKey, rule, severityFromHotspotProbability(hs.VulnerabilityProbability), registered); err != nil {
			return domain.EngineResult{}, fmt.Errorf("codescan: register rule %q: %w", hs.RuleKey, err)
		}
		emit(hotspotFinding(in.ScanID, projectKey, hs, rule))
	}

	return domain.EngineResult{
		RulesEvaluated: len(registered),
		Stats: map[string]any{
			"vulnerabilities_found": len(issues),
			"hotspots_found":        len(hotspots),
		},
	}, nil
}

// pollTask polls GetTask until the background analysis leaves
// PENDING/IN_PROGRESS, bounded by analysisTimeout independent of the
// caller's own ctx deadline (still respected via pollCtx, which derives
// from ctx).
func (e *Engine) pollTask(ctx context.Context, taskID string) (sonarqube.Task, error) {
	pollCtx, cancel := context.WithTimeout(ctx, e.analysisTimeout)
	defer cancel()

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		task, err := e.sonarqube.GetTask(pollCtx, taskID)
		if err != nil {
			return sonarqube.Task{}, err
		}
		switch task.Status {
		case sonarqube.TaskSuccess, sonarqube.TaskFailed, sonarqube.TaskCanceled:
			return task, nil
		}
		select {
		case <-pollCtx.Done():
			return sonarqube.Task{}, pollCtx.Err()
		case <-ticker.C:
		}
	}
}

// ruleInfo fetches a rule's own metadata once per distinct key per run,
// memoized in cache. A lookup failure degrades to a bare RuleInfo (just the
// key) rather than failing the run — a missing rule description shouldn't
// cost every finding for that rule its whole result.
func (e *Engine) ruleInfo(ctx context.Context, ruleKey string, cache map[string]sonarqube.RuleInfo) sonarqube.RuleInfo {
	if r, ok := cache[ruleKey]; ok {
		return r
	}
	r, err := e.sonarqube.GetRule(ctx, ruleKey)
	if err != nil {
		r = sonarqube.RuleInfo{Key: ruleKey}
	}
	cache[ruleKey] = r
	return r
}

// registerRule upserts one SonarQube-derived rule into the rules catalogue,
// once per distinct key per run (registered memoizes). This must happen
// before any Finding referencing this rule's RuleID gets persisted —
// findings.rule_id is a real foreign key into rules
// (documentation/06-database-design.md), so an upsert failure here is
// returned as a hard error rather than swallowed: an unresolved failure
// would otherwise surface later as an opaque FK-violation error from the
// orchestrator's persist step instead of a clear one from here.
func (e *Engine) registerRule(ctx context.Context, ruleKey string, rule sonarqube.RuleInfo, severity domain.Severity, registered map[string]bool) error {
	if registered[ruleKey] {
		return nil
	}
	registered[ruleKey] = true

	rm := domain.RuleMeta{
		ID:          "codescan.sonarqube." + ruleKey,
		Title:       firstNonEmpty(rule.Name, ruleKey),
		Description: remediationText(rule.RemediationHTML),
		Severity:    severity,
		Confidence:  domain.ConfidenceHigh,
		CWE:         normalizeCWE(rule.CWE),
		Remediation: remediationText(rule.RemediationHTML),
		Tier:        domain.TierCore,
	}
	return e.rules.UpsertRule(ctx, rm)
}
