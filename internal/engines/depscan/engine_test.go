package depscan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/depscan"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
)

// fakeAdvisoryService never calls OSV.dev — it's the "no advisories for
// anything" default used by the golden-fixture tests, which are testing
// the deterministic (file-content-only) rules, not the OSV-dependent ones.
type fakeAdvisoryService struct {
	byKey map[string]advisory.Result // "eco|name|version" -> canned result
}

func (f *fakeAdvisoryService) Lookup(_ context.Context, deps []advisory.Dependency) ([]advisory.Result, error) {
	out := make([]advisory.Result, len(deps))
	for i, d := range deps {
		key := d.Ecosystem + "|" + d.Name + "|" + d.Version
		if r, ok := f.byKey[key]; ok {
			out[i] = r
			continue
		}
		out[i] = advisory.Result{Dependency: d}
	}
	return out, nil
}
func (f *fakeAdvisoryService) SyncRules(context.Context) error { return nil }
func (f *fakeAdvisoryService) UpsertRule(context.Context, domain.RuleMeta) error {
	return nil
}
func (f *fakeAdvisoryService) ListRules(context.Context, advisory.RuleFilter, advisory.RulePage) ([]advisory.Rule, int, error) {
	return nil, 0, nil
}
func (f *fakeAdvisoryService) GetRule(context.Context, string) (*advisory.Rule, error) {
	return nil, nil
}
func (f *fakeAdvisoryService) SetRuleEnabled(context.Context, string, bool) (*advisory.Rule, error) {
	return nil, nil
}

type expectedFixture struct {
	Rules []struct {
		ID    string `yaml:"id"`
		Count int    `yaml:"count"`
	} `yaml:"rules"`
}

func runEngineAgainstFixture(t *testing.T, fixtureDir string, adv advisory.Service) []domain.Finding {
	t.Helper()
	engine := depscan.New(adv)
	scanInput := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: fixtureDir}

	applicable, reason := engine.Applicable(context.Background(), scanInput)
	require.True(t, applicable, "fixture should be Applicable: %s", reason)

	var findings []domain.Finding
	_, err := engine.Run(context.Background(), scanInput, func(f domain.Finding) {
		findings = append(findings, f)
	})
	require.NoError(t, err)
	return findings
}

func countByRule(findings []domain.Finding) map[string]int {
	counts := make(map[string]int)
	for _, f := range findings {
		counts[f.RuleID]++
	}
	return counts
}

// TestEngine_FixtureVulnerable_MatchesGoldenCatalogue is the golden-fixture
// gate documentation/15-testing-strategy.md §5 names: every planted
// deterministic rule case in testdata/fixtures/fixture-vulnerable must
// fire, exactly as many times as EXPECTED.yaml says.
func TestEngine_FixtureVulnerable_MatchesGoldenCatalogue(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-vulnerable")
	expectedBytes, err := os.ReadFile(filepath.Join(fixtureDir, "EXPECTED.yaml"))
	require.NoError(t, err)
	var expected expectedFixture
	require.NoError(t, yaml.Unmarshal(expectedBytes, &expected))

	findings := runEngineAgainstFixture(t, fixtureDir, &fakeAdvisoryService{})
	counts := countByRule(findings)

	for _, rule := range expected.Rules {
		require.Equalf(t, rule.Count, counts[rule.ID], "rule %s: got %d findings, want %d", rule.ID, counts[rule.ID], rule.Count)
	}

	// No OSV-dependent findings should appear — the fake advisory service
	// returns nothing for every dependency in this fixture.
	require.Zero(t, counts["depscan.vuln.known-cve"])
	require.Zero(t, counts["depscan.vuln.no-fix-available"])
}

// TestEngine_FixtureClean_ProducesZeroFindings is the other CI gate named
// in the same section: "zero non-informational findings on fixture-clean."
func TestEngine_FixtureClean_ProducesZeroFindings(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "fixture-clean")
	findings := runEngineAgainstFixture(t, fixtureDir, &fakeAdvisoryService{})
	require.Empty(t, findings, "fixture-clean must produce no findings at all")
}

// --- OSV-dependent rules: tested against a fake advisory.Service directly,
// not the golden fixture (its dependency versions aren't meant to reflect
// real-world vulnerability status) ---

func TestEngine_KnownCVE_And_NoFixAvailable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"vulnerable-pkg":"1.0.0","unfixed-pkg":"2.0.0"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{
		"lockfileVersion": 3,
		"packages": {
			"": {},
			"node_modules/vulnerable-pkg": {"version": "1.0.0"},
			"node_modules/unfixed-pkg": {"version": "2.0.0"}
		}
	}`), 0o644))

	fake := &fakeAdvisoryService{byKey: map[string]advisory.Result{
		"npm|vulnerable-pkg|1.0.0": {
			Advisories: []advisory.Advisory{{ID: "GHSA-1", CVE: "CVE-2024-1111", HasFix: true, FixedVersion: "1.0.1", CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}},
		},
		"npm|unfixed-pkg|2.0.0": {
			Advisories: []advisory.Advisory{{ID: "GHSA-2", HasFix: false}},
		},
	}}

	findings := runEngineAgainstFixture(t, dir, fake)
	counts := countByRule(findings)

	require.Equal(t, 2, counts["depscan.vuln.known-cve"], "known-cve fires once per advisory, for both packages")
	require.Equal(t, 1, counts["depscan.vuln.no-fix-available"], "no-fix-available additionally fires for unfixed-pkg only")

	var vulnFinding domain.Finding
	found := false
	for _, f := range findings {
		if f.RuleID == "depscan.vuln.known-cve" && len(f.CVE) > 0 && f.CVE[0] == "CVE-2024-1111" {
			vulnFinding = f
			found = true
		}
	}
	require.True(t, found, "expected a known-cve finding carrying CVE-2024-1111")
	require.Equal(t, domain.SeverityHigh, vulnFinding.Severity, "CVSS vector with high impact metrics maps to high severity")
	require.Equal(t, []string{"CVE-2024-1111"}, vulnFinding.CVE)
}

func TestEngine_Applicable_NoManifest_ReturnsFalseWithReason(t *testing.T) {
	engine := depscan.New(&fakeAdvisoryService{})
	applicable, reason := engine.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: t.TempDir()})
	require.False(t, applicable)
	require.NotEmpty(t, reason)
}
