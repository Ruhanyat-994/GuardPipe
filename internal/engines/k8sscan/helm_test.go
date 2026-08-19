package k8sscan

import (
	"path/filepath"
	"testing"
)

func writeChart(t *testing.T, root, chartYAML, valuesYAML, templateFile, templateYAML string) {
	t.Helper()
	writeTestFile(t, root, "Chart.yaml", chartYAML)
	if valuesYAML != "" {
		writeTestFile(t, root, "values.yaml", valuesYAML)
	}
	writeTestFile(t, root, filepath.Join("templates", templateFile), templateYAML)
}

func TestRenderedChartManifests_RendersAndTagsHelmSource(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, dir,
		"apiVersion: v2\nname: api\nversion: 0.1.0\n",
		"privileged: true\n",
		"deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-web
  namespace: default
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
        securityContext:
          privileged: {{ .Values.privileged }}
`)

	manifests, skip, err := renderedChartManifests(dir, "charts/api")
	if err != nil {
		t.Fatalf("renderedChartManifests() error = %v", err)
	}
	if skip != nil {
		t.Fatalf("renderedChartManifests() skip = %+v, want none", skip)
	}
	if len(manifests) != 1 {
		t.Fatalf("got %d manifests, want 1", len(manifests))
	}
	m := manifests[0]
	if !m.FromHelm || m.ChartName != "api" || m.TemplateFile != "templates/deployment.yaml" {
		t.Errorf("got FromHelm=%v ChartName=%q TemplateFile=%q, want true/api/templates/deployment.yaml", m.FromHelm, m.ChartName, m.TemplateFile)
	}
	// DocLine must be cleared, not just left however the rendered-output
	// parse happened to set it — it would otherwise silently claim to be a
	// real line in TemplateFile when it's actually a line in ephemeral
	// rendered text (see Manifest.DocLine's own doc comment).
	if m.DocLine != 0 {
		t.Errorf("m.DocLine = %d, want 0 — a Helm-sourced manifest's DocLine must never be presented as a real TemplateFile line", m.DocLine)
	}
	if m.Pod == nil || len(m.Pod.Containers) != 1 || m.Pod.Containers[0].SecurityContext == nil ||
		m.Pod.Containers[0].SecurityContext.Privileged == nil || !*m.Pod.Containers[0].SecurityContext.Privileged {
		t.Errorf("rendered chart's planted privileged:true did not survive parsing: %+v", m.Pod)
	}

	// And the rule engine actually fires on it, same as a raw manifest —
	// no separate Helm rule set.
	graph := newResourceGraph()
	graph.add(m)
	if !hasRuleID(evaluateAll(graph), "k8sscan.workload.privileged") {
		t.Error("k8sscan.workload.privileged did not fire on a Helm-rendered manifest")
	}
}

func TestRenderedChartManifests_UnresolvedDependencyIsSkipped(t *testing.T) {
	dir := t.TempDir()
	// Declares a dependency on "redis" but never vendors it under charts/ —
	// the near-miss for this is the vendored-dependency test below.
	writeChart(t, dir,
		"apiVersion: v2\nname: api\nversion: 0.1.0\ndependencies:\n- name: redis\n  version: \"1.0.0\"\n  repository: \"https://example.invalid\"\n",
		"", "deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: default}
spec: {template: {spec: {containers: [{name: app, image: nginx}]}}}`)

	manifests, skip, err := renderedChartManifests(dir, "charts/api")
	if err != nil {
		t.Fatalf("renderedChartManifests() error = %v", err)
	}
	if skip == nil || skip.Reason != "helm_dependency_unresolved" {
		t.Fatalf("renderedChartManifests() skip = %+v, want helm_dependency_unresolved", skip)
	}
	if len(manifests) != 0 {
		t.Errorf("got %d manifests from a chart with an unresolved dependency, want 0", len(manifests))
	}
}

// TestRenderedChartManifests_VendoredDependencyRendersFine is the near-miss
// for the unresolved-dependency case above: the same declared dependency,
// but actually vendored under charts/ (as `helm dependency vendor` would
// leave it) — this must render normally, offline, with no network access
// attempted for the already-satisfied dependency.
func TestRenderedChartManifests_VendoredDependencyRendersFine(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, dir,
		"apiVersion: v2\nname: api\nversion: 0.1.0\ndependencies:\n- name: redis\n  version: \"1.0.0\"\n  repository: \"https://example.invalid\"\n",
		"", "deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: default}
spec: {template: {spec: {containers: [{name: app, image: nginx}]}}}`)
	writeChart(t, filepath.Join(dir, "charts", "redis"),
		"apiVersion: v2\nname: redis\nversion: 1.0.0\n",
		"", "statefulset.yaml", `apiVersion: apps/v1
kind: StatefulSet
metadata: {name: redis, namespace: default}
spec: {template: {spec: {containers: [{name: redis, image: redis}]}}}`)

	manifests, skip, err := renderedChartManifests(dir, "charts/api")
	if err != nil {
		t.Fatalf("renderedChartManifests() error = %v", err)
	}
	if skip != nil {
		t.Fatalf("renderedChartManifests() skip = %+v, want none — the dependency is vendored", skip)
	}
	if len(manifests) != 2 {
		t.Errorf("got %d manifests, want 2 (parent Deployment + vendored redis StatefulSet)", len(manifests))
	}
}

func TestFindChartRoots_ExcludesVendoredSubcharts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "charts/api/Chart.yaml", "apiVersion: v2\nname: api\nversion: 0.1.0\n")
	writeTestFile(t, dir, "charts/api/charts/redis/Chart.yaml", "apiVersion: v2\nname: redis\nversion: 1.0.0\n")

	roots, err := findChartRoots(dir)
	if err != nil {
		t.Fatalf("findChartRoots() error = %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("got %d chart roots, want 1 (only the top-level chart, not its vendored subchart): %v", len(roots), roots)
	}
}

func TestIsHelmHelperOrEmpty(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    bool
	}{
		{"helper template", "_helpers.tpl", "some content", true},
		{"NOTES.txt", "NOTES.txt", "install notes", true},
		{"empty rendered output (conditional false)", "templates/optional.yaml", "   \n  ", true},
		{"a real manifest", "templates/deployment.yaml", "apiVersion: apps/v1\nkind: Deployment", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHelmHelperOrEmpty(tt.file, tt.content); got != tt.want {
				t.Errorf("isHelmHelperOrEmpty(%q, ...) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
