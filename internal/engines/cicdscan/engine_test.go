package cicdscan

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

func TestEngine_Applicable_WithWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yml", `
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)

	e := New(nil)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if !ok {
		t.Errorf("Applicable() = false (%q), want true", reason)
	}
}

// TestEngine_Applicable_NoWorkflows is the near-miss: a repository with no
// .github/workflows/ directory at all must not falsely report itself
// applicable — same "skips cleanly" contract every other engine's
// Applicable already follows.
func TestEngine_Applicable_NoWorkflows(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "README.md", "# hello")

	e := New(nil)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if ok {
		t.Error("Applicable() = true, want false — no .github/workflows/ exists")
	}
	if reason == "" {
		t.Error("Applicable() reason is empty, want an explanation")
	}
}

// TestEngine_Run_NilAIService confirms the documented "Gemini unavailable"
// failure mode (documentation/05-module-specifications.md §10's own failure
// table): rule findings are retained, the result is flagged
// ai_pass_unavailable, and the job succeeds rather than erroring — checked
// with aiSvc literally nil, the exact state cmd/guardpipe/main.go wires up
// when GUARDPIPE_AI_ENABLED is false.
func TestEngine_Run_NilAIService(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yml", `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`)

	e := New(nil)
	ctx := context.Background()
	in := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: dir}

	var findings []domain.Finding
	result, err := e.Run(ctx, in, func(f domain.Finding) { findings = append(findings, f) })
	if err != nil {
		t.Fatalf("Run() error = %v, want nil — a missing AI service must not fail the job", err)
	}
	if !hasRuleID(findings, "cicdscan.permissions.missing-block") {
		t.Error("expected the deterministic rule pass to still run and fire with no AI service configured")
	}
	if result.Stats["ai_pass_degraded"] != true {
		t.Errorf("Stats[ai_pass_degraded] = %v, want true", result.Stats["ai_pass_degraded"])
	}
	found := false
	for _, s := range result.Skipped {
		if strings.Contains(s.Reason, "ai_pass_unavailable") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an ai_pass_unavailable skip reason in %+v", result.Skipped)
	}
}

func TestEngine_Run_EveryFindingCarriesScanIDAndEngine(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yml", `
permissions: write-all
jobs:
  build:
    runs-on: self-hosted
    steps:
      - uses: some-random-org/some-action@v1
`)

	e := New(nil)
	ctx := context.Background()
	in := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: dir}

	var findings []domain.Finding
	if _, err := e.Run(ctx, in, func(f domain.Finding) { findings = append(findings, f) }); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding from this deliberately misconfigured workflow")
	}
	for _, f := range findings {
		if f.Engine != domain.EngineCICDScan {
			t.Errorf("finding %s has Engine = %q, want %q", f.RuleID, f.Engine, domain.EngineCICDScan)
		}
		if f.ScanID != in.ScanID {
			t.Errorf("finding %s has ScanID = %v, want %v", f.RuleID, f.ScanID, in.ScanID)
		}
		if f.Source != domain.FindingSourceRule {
			t.Errorf("finding %s has Source = %q, want %q (no AI service configured in this test)", f.RuleID, f.Source, domain.FindingSourceRule)
		}
	}
}

// --- golden fixtures (documentation/15-testing-strategy.md §5) ---
//
// testdata/fixtures/{fixture-vulnerable,fixture-clean} are shared across
// every engine's own planted content in the same repo — EXPECTED.yaml's
// rules list mixes every engine's own rule_id prefix, so this test filters
// to cicdscan.* only (the same pattern k8sscan's/depscan's own copy of this
// test already establishes).

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
	e := New(nil) // no AI service — golden fixtures assert the deterministic rule catalogue only
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

func TestGoldenFixture_Vulnerable_MatchesCatalogue(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-vulnerable")
	expected := loadExpectedFixture(t, fixtureDir)
	findings := runEngineAgainstFixtureDir(t, fixtureDir)

	counts := map[string]int{}
	for _, f := range findings {
		counts[f.RuleID]++
	}
	for _, rule := range expected.Rules {
		if !strings.HasPrefix(rule.ID, "cicdscan.") {
			continue
		}
		if counts[rule.ID] != rule.Count {
			t.Errorf("rule %s: got %d findings, want %d", rule.ID, counts[rule.ID], rule.Count)
		}
	}
}

func TestGoldenFixture_Clean_ProducesNoCICDScanFindings(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-clean")
	findings := runEngineAgainstFixtureDir(t, fixtureDir)

	for _, f := range findings {
		t.Errorf("fixture-clean produced a cicdscan finding: %s (%s)", f.RuleID, f.Severity)
	}
}
