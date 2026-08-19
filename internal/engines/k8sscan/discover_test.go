package k8sscan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverManifests_FindsAValidManifest(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: default}
spec:
  template:
    spec:
      containers: [{name: app, image: nginx}]`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 1 {
		t.Fatalf("got %d manifests, want 1", len(result.Manifests))
	}
	if result.Manifests[0].Kind != "Deployment" || result.Manifests[0].Name != "web" {
		t.Errorf("got %+v, want Deployment/web", result.Manifests[0])
	}
}

// TestDiscoverManifests_IgnoresNonKubernetesYAML is the near-miss half of
// discovery itself: a YAML file that isn't a Kubernetes resource at all
// (no apiVersion/kind) must never be mistaken for one — this is what keeps
// a repo's CI workflow YAML, a Helm values.yaml, or a docker-compose.yml
// from ever reaching a rule.
func TestDiscoverManifests_IgnoresNonKubernetesYAML(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yaml", `name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest`)
	writeTestFile(t, dir, "docker-compose.yml", `version: "3.8"
services:
  web:
    image: nginx`)
	writeTestFile(t, dir, "values.yaml", `image:
  repository: nginx
  tag: latest`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 0 {
		t.Errorf("got %d manifests, want 0 — none of these files are Kubernetes resources: %+v", len(result.Manifests), result.Manifests)
	}
}

func TestDiscoverManifests_SkipsNoiseDirectories(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "node_modules/some-pkg/k8s-lookalike.yaml", `apiVersion: v1
kind: Pod
metadata: {name: p1}
spec: {containers: [{name: c, image: nginx}]}`)
	writeTestFile(t, dir, ".git/k8s-lookalike.yaml", `apiVersion: v1
kind: Pod
metadata: {name: p2}
spec: {containers: [{name: c, image: nginx}]}`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 0 {
		t.Errorf("got %d manifests from noise directories, want 0", len(result.Manifests))
	}
}

func TestDiscoverManifests_SplitsMultiDocumentFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "all.yaml", `apiVersion: v1
kind: Namespace
metadata: {name: ns1}
---
apiVersion: v1
kind: ServiceAccount
metadata: {name: sa1, namespace: ns1}`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 2 {
		t.Fatalf("got %d manifests, want 2", len(result.Manifests))
	}
	if result.Manifests[0].Kind != "Namespace" || result.Manifests[1].Kind != "ServiceAccount" {
		t.Errorf("got kinds %s, %s, want Namespace, ServiceAccount", result.Manifests[0].Kind, result.Manifests[1].Kind)
	}
}

// TestDiscoverManifests_TracksEachDocumentsStartLine is what makes a
// "View in repository" link (frontend's FindingRow, buildRepoBlobUrl) land
// close to the right resource in a multi-document file rather than always
// the top: each document's Manifest.DocLine must be the real 1-based line
// yaml.v3 reports for that document (the first document's own first line;
// a later document's --- separator line), not a fixed guess like always 1
// or always DocIndex+1 — blank lines between documents would make either
// guess wrong.
func TestDiscoverManifests_TracksEachDocumentsStartLine(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "all.yaml", `apiVersion: v1
kind: Namespace
metadata: {name: ns1}

---

apiVersion: v1
kind: ServiceAccount
metadata: {name: sa1, namespace: ns1}`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 2 {
		t.Fatalf("got %d manifests, want 2", len(result.Manifests))
	}
	if result.Manifests[0].DocLine != 1 {
		t.Errorf("first document DocLine = %d, want 1", result.Manifests[0].DocLine)
	}
	if result.Manifests[1].DocLine != 5 {
		t.Errorf("second document DocLine = %d, want 5 (yaml.v3 positions a document node at its --- separator, not its first content line — still close enough to be a useful link)", result.Manifests[1].DocLine)
	}
}

// TestDiscoverManifests_UnrenderedHelmTemplateIsSkippedNotErrored is the
// documented failure mode (documentation/05-module-specifications.md §9): a
// loose file containing Helm/Go-template markup with no sibling Chart.yaml
// (so helm.go never renders it) is recorded as a skip, not a parse_error —
// it isn't malformed YAML, it's template source that was never meant to be
// read standalone.
func TestDiscoverManifests_UnrenderedHelmTemplateIsSkippedNotErrored(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "loose-template.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-web`)

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 0 {
		t.Errorf("got %d manifests from un-rendered template markup, want 0", len(result.Manifests))
	}
	if len(result.ParseErrors) != 0 {
		t.Errorf("got %d parse errors, want 0 — this should be a templated-file skip, not an error: %+v", len(result.ParseErrors), result.ParseErrors)
	}
	if len(result.TemplatedFiles) != 1 {
		t.Errorf("got %d templated files, want 1", len(result.TemplatedFiles))
	}
}

func TestDiscoverManifests_GenuinelyMalformedYAMLIsAParseError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken.yaml", "apiVersion: v1\nkind: [unterminated")

	result, err := discoverManifests(dir, nil)
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.ParseErrors) != 1 {
		t.Errorf("got %d parse errors, want 1", len(result.ParseErrors))
	}
}

// TestDiscoverManifests_ExcludesChartRoots confirms raw discovery never
// double-scans a Helm chart's own template files as if they were
// standalone manifests — that's helm.go's job, via a completely separate
// render+parse path.
func TestDiscoverManifests_ExcludesChartRoots(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "charts/api/Chart.yaml", "apiVersion: v2\nname: api\nversion: 0.1.0\n")
	writeTestFile(t, dir, "charts/api/templates/deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-web`)
	writeTestFile(t, dir, "raw/pod.yaml", `apiVersion: v1
kind: Pod
metadata: {name: p1}
spec: {containers: [{name: c, image: nginx}]}`)

	chartRoot := filepath.Join(dir, "charts", "api")
	result, err := discoverManifests(dir, []string{chartRoot})
	if err != nil {
		t.Fatalf("discoverManifests() error = %v", err)
	}
	if len(result.Manifests) != 1 || result.Manifests[0].Name != "p1" {
		t.Errorf("got %+v, want exactly the raw/pod.yaml manifest — the chart root's own templates must be excluded", result.Manifests)
	}
}
