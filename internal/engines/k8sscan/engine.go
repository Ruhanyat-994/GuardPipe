package k8sscan

import (
	"context"
	"fmt"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Engine implements domain.Engine — k8sscan's manifest discovery, Helm
// rendering, and policy rule engine, wired together per
// documentation/05-module-specifications.md §9. It needs no dependencies
// (unlike depscan, which needs modules/advisory) — every rule is a pure
// function of the manifests found in the workspace.
type Engine struct{}

func New() *Engine {
	return &Engine{}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineK8sScan
}

// Applicable is a cheap existence check: does the workspace contain any
// Helm chart or any raw Kubernetes manifest at all. A repository with
// neither is exactly as inapplicable to k8sscan as one with no Dockerfile
// is to containerscan (documentation/05-module-specifications.md's
// Applicable contract).
func (e *Engine) Applicable(ctx context.Context, in domain.ScanInput) (bool, string) {
	chartRoots, err := findChartRoots(in.WorkspaceDir)
	if err == nil && len(chartRoots) > 0 {
		return true, ""
	}
	discovery, err := discoverManifests(in.WorkspaceDir, chartRoots)
	if err == nil && len(discovery.Manifests) > 0 {
		return true, ""
	}
	return false, "no Kubernetes manifests or Helm charts found anywhere in the checkout"
}

// Run discovers every manifest (raw and Helm-rendered), builds the resource
// graph, and evaluates all three rule families against it. Never writes to
// the database — the orchestrator persists what Run emits, inside one
// transaction per job (documentation/03-architecture-overview.md §6.3).
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	chartRoots, err := findChartRoots(in.WorkspaceDir)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("k8sscan: find Helm chart roots: %w", err)
	}
	if ctx.Err() != nil {
		return domain.EngineResult{}, ctx.Err()
	}

	discovery, err := discoverManifests(in.WorkspaceDir, chartRoots)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("k8sscan: discover manifests: %w", err)
	}
	if ctx.Err() != nil {
		return domain.EngineResult{}, ctx.Err()
	}

	graph := newResourceGraph()
	for _, m := range discovery.Manifests {
		graph.add(m)
	}

	var skipped []domain.SkipReason
	for _, file := range discovery.TemplatedFiles {
		skipped = append(skipped, domain.SkipReason{RuleID: "k8sscan.*", Reason: "templated_manifest: " + file})
	}
	for file, parseErr := range discovery.ParseErrors {
		skipped = append(skipped, domain.SkipReason{RuleID: "k8sscan.*", Reason: "parse_error: " + file + ": " + parseErr.Error()})
	}

	chartsRendered := 0
	for _, root := range chartRoots {
		if ctx.Err() != nil {
			return domain.EngineResult{}, ctx.Err()
		}
		rootRel := relPath(in.WorkspaceDir, root)
		manifests, skip, renderErr := renderedChartManifests(root, rootRel)
		if renderErr != nil {
			return domain.EngineResult{}, fmt.Errorf("k8sscan: render chart %s: %w", rootRel, renderErr)
		}
		if skip != nil {
			skipped = append(skipped, domain.SkipReason{RuleID: "k8sscan.*", Reason: skip.Reason + ": " + rootRel + " (" + skip.Detail + ")"})
			continue
		}
		chartsRendered++
		for _, m := range manifests {
			graph.add(m)
		}
	}

	for _, f := range evaluateRBAC(graph, in.ScanID) {
		emit(f)
	}
	for _, f := range evaluateWorkload(graph, in.ScanID) {
		emit(f)
	}
	for _, f := range evaluateNetwork(graph, in.ScanID) {
		emit(f)
	}

	return domain.EngineResult{
		RulesEvaluated: len(Rules),
		FilesScanned:   len(discovery.Manifests) + chartsRendered,
		Skipped:        skipped,
		Stats: map[string]any{
			"manifests_found": len(graph.Manifests),
			"charts_rendered": chartsRendered,
			"charts_skipped":  len(chartRoots) - chartsRendered,
		},
	}, nil
}
