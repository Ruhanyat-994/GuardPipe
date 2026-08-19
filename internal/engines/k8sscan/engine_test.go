package k8sscan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestEngine_Applicable_WithRawManifest(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: default}
spec: {template: {spec: {containers: [{name: app, image: nginx}]}}}`)

	e := New()
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if !ok {
		t.Errorf("Applicable() = false (%q), want true", reason)
	}
}

func TestEngine_Applicable_WithHelmChartOnly(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, dir, "apiVersion: v2\nname: api\nversion: 0.1.0\n", "",
		"deployment.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {}")

	e := New()
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if !ok {
		t.Errorf("Applicable() = false (%q), want true — a Helm chart alone is enough", reason)
	}
}

// TestEngine_Applicable_NoManifestsOrCharts is the near-miss: a repository
// with real YAML files, just none of them Kubernetes-shaped, must not
// falsely report itself applicable — matching containerscan's own "skips
// cleanly if no Dockerfile" precedent.
func TestEngine_Applicable_NoManifestsOrCharts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docker-compose.yml", "version: \"3.8\"\nservices: {web: {image: nginx}}")

	e := New()
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if ok {
		t.Error("Applicable() = true, want false — no Kubernetes manifests or Helm charts exist")
	}
	if reason == "" {
		t.Error("Applicable() reason is empty, want an explanation")
	}
}

// TestEngine_Run_FullPipeline exercises raw manifests, a Helm chart, and a
// chart with an unresolved dependency all in one workspace — the
// "everything the module spec's failure-mode table promises" integration
// check, on top of the narrower per-family/per-mechanism tests elsewhere in
// this package.
func TestEngine_Run_FullPipeline(t *testing.T) {
	dir := t.TempDir()

	// A raw manifest with a planted RBAC violation.
	writeTestFile(t, dir, "rbac/role.yaml", `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: bad-role}
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]`)

	// A Helm chart with a planted workload violation.
	writeChart(t, dir+"/charts/api",
		"apiVersion: v2\nname: api\nversion: 0.1.0\n", "privileged: true\n",
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

	// A chart with a dependency that was never vendored — must be skipped,
	// not fail the whole engine.
	writeChart(t, dir+"/charts/broken",
		"apiVersion: v2\nname: broken\nversion: 0.1.0\ndependencies:\n- name: missing\n  version: \"1.0.0\"\n  repository: \"https://example.invalid\"\n",
		"", "deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: broken-web, namespace: default}
spec: {template: {spec: {containers: [{name: app, image: nginx}]}}}`)

	e := New()
	ctx := context.Background()
	in := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: dir}

	ok, _ := e.Applicable(ctx, in)
	if !ok {
		t.Fatal("Applicable() = false, want true")
	}

	var findings []domain.Finding
	result, err := e.Run(ctx, in, func(f domain.Finding) { findings = append(findings, f) })
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !hasRuleID(findings, "k8sscan.rbac.wildcard-verbs") {
		t.Error("expected the raw-manifest RBAC violation to fire")
	}
	if !hasRuleID(findings, "k8sscan.workload.privileged") {
		t.Error("expected the Helm-rendered workload violation to fire")
	}
	for _, f := range findings {
		if f.Engine != domain.EngineK8sScan {
			t.Errorf("finding %s has Engine = %q, want %q", f.RuleID, f.Engine, domain.EngineK8sScan)
		}
		if f.ScanID != in.ScanID {
			t.Errorf("finding %s has ScanID = %v, want %v", f.RuleID, f.ScanID, in.ScanID)
		}
	}

	foundBrokenChartSkip := false
	for _, s := range result.Skipped {
		if strings.Contains(s.Reason, "helm_dependency_unresolved") {
			foundBrokenChartSkip = true
		}
	}
	if !foundBrokenChartSkip {
		t.Errorf("expected a helm_dependency_unresolved skip reason in %+v", result.Skipped)
	}

	if result.Stats["charts_rendered"] != 1 {
		t.Errorf("charts_rendered = %v, want 1 (api rendered, broken skipped)", result.Stats["charts_rendered"])
	}
	if result.Stats["charts_skipped"] != 1 {
		t.Errorf("charts_skipped = %v, want 1", result.Stats["charts_skipped"])
	}
}

// --- golden fixtures (documentation/15-testing-strategy.md §5) ---
//
// testdata/fixtures/{fixture-vulnerable,fixture-clean} are shared across
// every engine's own planted content in the same repo — EXPECTED.yaml's
// rules list mixes depscan.* and k8sscan.* entries (and eventually every
// other engine's), so both engines' tests filter to their own prefix
// (depscan's own copy of this pattern: internal/engines/depscan/engine_test.go).

type expectedFixture struct {
	Rules []struct {
		ID    string `yaml:"id"`
		Count int    `yaml:"count"`
	} `yaml:"rules"`
}

func loadExpectedFixture(t *testing.T, fixtureDir string) expectedFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "EXPECTED.yaml"))
	if err != nil {
		t.Fatalf("read EXPECTED.yaml: %v", err)
	}
	var expected expectedFixture
	if err := yaml.Unmarshal(raw, &expected); err != nil {
		t.Fatalf("parse EXPECTED.yaml: %v", err)
	}
	return expected
}

func runEngineAgainstFixtureDir(t *testing.T, fixtureDir string) []domain.Finding {
	t.Helper()
	e := New()
	ctx := context.Background()
	in := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: fixtureDir}
	ok, reason := e.Applicable(ctx, in)
	if !ok {
		t.Fatalf("Applicable() = false (%q), want true", reason)
	}
	var findings []domain.Finding
	if _, err := e.Run(ctx, in, func(f domain.Finding) { findings = append(findings, f) }); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return findings
}

// TestGoldenFixture_Vulnerable_MatchesCatalogue is the detection-rate gate
// documentation/15-testing-strategy.md §5 names: every planted k8sscan.*
// case in fixture-vulnerable (a raw manifest set under k8s/ plus a Helm
// chart under charts/api) must fire exactly as many times as EXPECTED.yaml
// says.
func TestGoldenFixture_Vulnerable_MatchesCatalogue(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-vulnerable")
	expected := loadExpectedFixture(t, fixtureDir)
	findings := runEngineAgainstFixtureDir(t, fixtureDir)

	counts := map[string]int{}
	for _, f := range findings {
		counts[f.RuleID]++
	}
	for _, rule := range expected.Rules {
		if !strings.HasPrefix(rule.ID, "k8sscan.") {
			continue
		}
		if counts[rule.ID] != rule.Count {
			t.Errorf("rule %s: got %d findings, want %d", rule.ID, counts[rule.ID], rule.Count)
		}
	}
}

// TestGoldenFixture_Clean_ProducesOnlyInformationalFindings is the other
// gate: fixture-clean's k8s/deployment.yaml is deliberately hardened
// against every Core rule, so nothing but the always-present informational
// psa-level summary should fire.
func TestGoldenFixture_Clean_ProducesOnlyInformationalFindings(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-clean")
	findings := runEngineAgainstFixtureDir(t, fixtureDir)

	for _, f := range findings {
		if f.Severity != domain.SeverityInformational {
			t.Errorf("fixture-clean produced a non-informational finding: %s (%s)", f.RuleID, f.Severity)
		}
	}
}
