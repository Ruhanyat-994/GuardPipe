package k8sscan

import (
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// helmSkip is one chart that couldn't be scanned, and why — surfaced as a
// domain.SkipReason on the engine's EngineResult (documentation/05-module-specifications.md
// §9's failure-mode table), never as a whole-job failure. Other charts and
// any raw manifests elsewhere in the repository are still scanned.
type helmSkip struct {
	ChartPath string // relative to the workspace root
	Reason    string // "helm_dependency_unresolved" | "helm_render_failed"
	Detail    string
}

// findChartRoots walks workspaceDir for every Chart.yaml, keeping only
// top-level roots — a vendored subchart under some other chart's own
// charts/ directory is loaded and rendered together with its parent by
// loader.Load, not as an independent root of its own (that would render
// every subchart twice: once standalone with no parent context, once
// correctly as part of its parent).
func findChartRoots(workspaceDir string) ([]string, error) {
	var all []string
	err := filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != workspaceDir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "Chart.yaml" || d.Name() == "Chart.yml" {
			all = append(all, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var roots []string
	for _, candidate := range all {
		if !isUnderAnyChartsDir(candidate, all) {
			roots = append(roots, candidate)
		}
	}
	return roots, nil
}

// isUnderAnyChartsDir reports whether candidate sits inside some other
// chart root's own "charts/<name>" vendored-dependency directory.
func isUnderAnyChartsDir(candidate string, allRoots []string) bool {
	for _, other := range allRoots {
		if other == candidate {
			continue
		}
		chartsDir := filepath.Join(other, "charts")
		if rel, err := filepath.Rel(chartsDir, candidate); err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// renderHelmChart renders one chart root offline — the library-level
// equivalent of `helm template`, never `helm install`: no cluster
// connection, no Tiller, no network access of any kind. A chart's declared
// dependencies (chart.Metadata.Dependencies) are only ever resolved from
// what loader.Load already found vendored under the chart's own charts/
// subdirectory (chart.Dependencies()) — never fetched from a repository. If
// any declared dependency isn't already vendored, the whole chart is
// skipped (helm_dependency_unresolved) rather than rendered with pieces
// missing, which would risk misleading pass/fail results from templates
// that silently no-op on a missing include.
//
// Rendering is pure Go template expansion (text/template + sprig) against
// already-loaded local files — it never shells out, never touches the
// filesystem outside the chart directory, and never makes a network call,
// which is why this runs in-process with no sandbox (documentation/05-module-specifications.md
// §9, CLAUDE.md's "pure-Go analysis reads files, never executes scanned
// code").
//
// Known limitation, not solved here: rendered content's line numbers are
// the *rendered* file's lines, not the original template's — Go's
// text/template doesn't preserve a source line map through conditionals/
// loops that expand or collapse lines. A Helm-sourced Finding still names
// the correct template file (good enough to find the right place to edit);
// pinpointing the exact template line is a real gap, left for later.
func renderHelmChart(chartRootDir string) (rendered map[string]string, skip *helmSkip, err error) {
	c, loadErr := loader.Load(chartRootDir)
	if loadErr != nil {
		return nil, &helmSkip{Reason: "helm_render_failed", Detail: loadErr.Error()}, nil
	}

	if unresolved := unresolvedDependencies(c); len(unresolved) > 0 {
		return nil, &helmSkip{
			Reason: "helm_dependency_unresolved",
			Detail: "declared but not vendored under charts/: " + strings.Join(unresolved, ", "),
		}, nil
	}

	options := chartutil.ReleaseOptions{Name: c.Name(), Namespace: "default"}
	renderValues, valErr := chartutil.ToRenderValues(c, c.Values, options, chartutil.DefaultCapabilities)
	if valErr != nil {
		return nil, &helmSkip{Reason: "helm_render_failed", Detail: valErr.Error()}, nil
	}

	out, renderErr := engine.Render(c, renderValues)
	if renderErr != nil {
		return nil, &helmSkip{Reason: "helm_render_failed", Detail: renderErr.Error()}, nil
	}
	return out, nil, nil
}

// unresolvedDependencies returns the declared dependency names
// (chart.yaml's `dependencies:`) that loader.Load did not find already
// vendored under charts/.
func unresolvedDependencies(c *chart.Chart) []string {
	if c.Metadata == nil || len(c.Metadata.Dependencies) == 0 {
		return nil
	}
	loaded := map[string]bool{}
	for _, sub := range c.Dependencies() {
		loaded[sub.Name()] = true
	}
	var missing []string
	for _, dep := range c.Metadata.Dependencies {
		if !loaded[dep.Name] {
			missing = append(missing, dep.Name)
		}
	}
	return missing
}

// helmTemplateKeyPrefix strips engine.Render's "<chartname>/" key prefix so
// callers get the template path relative to the chart root, matching what
// a real template file on disk is actually called.
func helmTemplateKeyPrefix(chartName, key string) string {
	prefix := chartName + "/"
	return strings.TrimPrefix(key, prefix)
}

// isHelmHelperOrEmpty reports whether a rendered template entry should be
// skipped before manifest parsing: Helm partial templates (_helpers.tpl,
// any file starting with "_") never render standalone content of their own
// — they're included by other templates — and NOTES.txt is
// human-readable install instructions, not a manifest. A conditionally
// excluded resource (an `{{ if }}` that evaluated false) renders to empty
// or whitespace-only content, which decodeManifest already skips via the
// apiVersion/kind check, but filtering it here avoids feeding blank text
// through the YAML decoder at all.
func isHelmHelperOrEmpty(templateFile, content string) bool {
	base := filepath.Base(templateFile)
	if strings.HasPrefix(base, "_") || strings.EqualFold(base, "NOTES.txt") {
		return true
	}
	return strings.TrimSpace(content) == ""
}

// renderedChartManifests renders chartRootDir and parses every non-helper
// output into Manifests tagged FromHelm/ChartName/TemplateFile. chartRootRel
// is the chart root's path relative to the workspace, used to build a real,
// navigable File path for each resulting Manifest.
func renderedChartManifests(chartRootDir, chartRootRel string) (manifests []Manifest, skip *helmSkip, err error) {
	rendered, skip, err := renderHelmChart(chartRootDir)
	if skip != nil || err != nil {
		return nil, skip, err
	}

	c, loadErr := loader.Load(chartRootDir)
	if loadErr != nil {
		// Already loaded successfully once above (renderHelmChart would
		// have skipped otherwise) — a failure here would mean the
		// filesystem changed mid-scan, treat as a render failure.
		return nil, &helmSkip{Reason: "helm_render_failed", Detail: loadErr.Error()}, nil
	}

	for key, content := range rendered {
		templateFile := helmTemplateKeyPrefix(c.Name(), key)
		if isHelmHelperOrEmpty(templateFile, content) {
			continue
		}
		file := chartRootRel + "/" + templateFile
		docs, templated, parseErr := parseDocuments([]byte(content), file)
		if parseErr != nil || templated {
			// A chart's own rendered output failing to parse as YAML is a
			// render-correctness problem with this specific template, not
			// grounds to fail the whole chart — record nothing for this one
			// template and keep going with the rest.
			continue
		}
		for i := range docs {
			docs[i].FromHelm = true
			docs[i].ChartName = c.Name()
			docs[i].TemplateFile = templateFile
			// parseDocuments decoded the *rendered* output, so DocLine (if
			// non-zero) names a line in that ephemeral text, not in
			// TemplateFile — cleared rather than left to look like a real,
			// clickable template line it isn't (see the known limitation in
			// renderHelmChart's doc comment, and Manifest.DocLine's own).
			docs[i].DocLine = 0
		}
		manifests = append(manifests, docs...)
	}
	return manifests, nil, nil
}
