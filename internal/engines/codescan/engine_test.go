package codescan_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sonarqube"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/codescan"
)

// fakeSonarQubeClient is a hand-written fake — no live SonarQube in tests
// (documentation/15-testing-strategy.md: "no mocking framework ...
// hand-written fakes only", "Tests never call ... live").
type fakeSonarQubeClient struct {
	task     sonarqube.Task
	taskErr  error
	issues   []sonarqube.Issue
	hotspots []sonarqube.Hotspot
	rules    map[string]sonarqube.RuleInfo
	ruleErr  error
}

func (f *fakeSonarQubeClient) GetTask(context.Context, string) (sonarqube.Task, error) {
	return f.task, f.taskErr
}
func (f *fakeSonarQubeClient) SearchIssues(context.Context, string) ([]sonarqube.Issue, error) {
	return f.issues, nil
}
func (f *fakeSonarQubeClient) SearchHotspots(context.Context, string) ([]sonarqube.Hotspot, error) {
	return f.hotspots, nil
}
func (f *fakeSonarQubeClient) GetRule(_ context.Context, ruleKey string) (sonarqube.RuleInfo, error) {
	if f.ruleErr != nil {
		return sonarqube.RuleInfo{}, f.ruleErr
	}
	if r, ok := f.rules[ruleKey]; ok {
		return r, nil
	}
	return sonarqube.RuleInfo{Key: ruleKey}, nil
}

type fakeScanner struct {
	taskID string
	err    error
	calls  int
}

func (f *fakeScanner) Analyze(context.Context, domain.ScanInput, string) (string, error) {
	f.calls++
	return f.taskID, f.err
}

type fakeRuleRegistrar struct {
	upserted []domain.RuleMeta
	err      error
}

func (f *fakeRuleRegistrar) UpsertRule(_ context.Context, rm domain.RuleMeta) error {
	if f.err != nil {
		return f.err
	}
	f.upserted = append(f.upserted, rm)
	return nil
}

func newWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
	return dir
}

func TestEngine_Applicable_TrueWithSourceFile(t *testing.T) {
	dir := newWorkspace(t)
	e := codescan.New(&fakeSonarQubeClient{}, &fakeScanner{}, &fakeRuleRegistrar{}, time.Second)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.True(t, ok, "reason: %s", reason)
}

func TestEngine_Applicable_FalseWithNoSourceFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644))
	e := codescan.New(&fakeSonarQubeClient{}, &fakeScanner{}, &fakeRuleRegistrar{}, time.Second)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.False(t, ok)
	require.NotEmpty(t, reason)
}

func TestEngine_Applicable_SkipsVendorAndNodeModules(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte("package lib\n"), 0o644))
	e := codescan.New(&fakeSonarQubeClient{}, &fakeScanner{}, &fakeRuleRegistrar{}, time.Second)
	ok, _ := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.False(t, ok, "a .go file only inside vendor/ must not make the engine applicable")
}

// TestEngine_Run_OnlySecurityRelevantFindings is the near-miss half of the
// security-relevance filter (documentation/05-module-specifications.md §6):
// SearchIssues/SearchHotspots (as implemented by adapters/sonarqube) only
// ever return VULNERABILITY issues and hotspots in the first place — this
// test's fake stands in for that filtered result and confirms the engine
// converts every one of them into a Finding, emitting nothing extra and
// nothing less.
func TestEngine_Run_OnlySecurityRelevantFindings(t *testing.T) {
	// Run() derives the SonarQube project key as "guardpipe-<ProjectID>" —
	// the fake's Component values must use that same computed key so
	// componentPath's prefix-strip has something real to strip.
	projectID := uuid.New()
	projectKey := "guardpipe-" + projectID.String()

	sq := &fakeSonarQubeClient{
		task: sonarqube.Task{Status: sonarqube.TaskSuccess, AnalysisID: "AY_1"},
		issues: []sonarqube.Issue{
			{Key: "i1", RuleKey: "go:S2076", Severity: "CRITICAL", Component: projectKey + ":cmd/main.go", Message: "OS command injection", Line: 10, LineEnd: 10},
		},
		hotspots: []sonarqube.Hotspot{
			{Key: "h1", RuleKey: "go:S5332", Component: projectKey + ":server.go", Message: "Use of cleartext protocol", Line: 20, VulnerabilityProbability: "HIGH"},
		},
		rules: map[string]sonarqube.RuleInfo{
			"go:S2076": {Key: "go:S2076", Name: "OS command injection", RemediationHTML: "<p>Use exec.Command with args.</p>", CWE: []string{"CWE-78"}},
			"go:S5332": {Key: "go:S5332", Name: "Cleartext protocol", RemediationHTML: "<p>Use TLS.</p>", CWE: []string{"330"}},
		},
	}
	scanner := &fakeScanner{taskID: "task-1"}
	registrar := &fakeRuleRegistrar{}
	e := codescan.New(sq, scanner, registrar, time.Second)

	var findings []domain.Finding
	result, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: projectID, WorkspaceDir: t.TempDir()}, func(f domain.Finding) {
		findings = append(findings, f)
	})
	require.NoError(t, err)
	require.Equal(t, 1, scanner.calls)
	require.Len(t, findings, 2, "one issue + one hotspot, nothing discarded and nothing invented")
	require.Equal(t, 2, result.RulesEvaluated)

	var issueFinding, hotspotFinding *domain.Finding
	for i := range findings {
		switch findings[i].RuleID {
		case "codescan.sonarqube.go:S2076":
			issueFinding = &findings[i]
		case "codescan.sonarqube.go:S5332":
			hotspotFinding = &findings[i]
		}
	}
	require.NotNil(t, issueFinding)
	require.NotNil(t, hotspotFinding)

	require.Equal(t, domain.SeverityCritical, issueFinding.Severity)
	require.Equal(t, domain.ConfidenceHigh, issueFinding.Confidence, "a confirmed VULNERABILITY issue is high confidence")
	require.Equal(t, []string{"CWE-78"}, issueFinding.CWE)
	require.Contains(t, issueFinding.Remediation, "exec.Command")
	require.Equal(t, "cmd/main.go", issueFinding.Location.Path, "the projectKey: prefix must be stripped")

	require.Equal(t, domain.SeverityHigh, hotspotFinding.Severity, "a HIGH-probability hotspot is capped at high, never critical — it's unconfirmed")
	require.Equal(t, domain.ConfidenceMedium, hotspotFinding.Confidence, "a hotspot is 'needs review', not a confirmed vulnerability")
	require.Equal(t, []string{"CWE-330"}, hotspotFinding.CWE, "a bare numeric CWE from SonarQube must be normalised to the CWE- form")

	require.Len(t, registrar.upserted, 2, "each distinct rule is registered exactly once")
}

func TestEngine_Run_ScannerFailure_FailsJobOnly(t *testing.T) {
	scanner := &fakeScanner{err: errors.New("sonar-scanner exited 1")}
	e := codescan.New(&fakeSonarQubeClient{}, scanner, &fakeRuleRegistrar{}, time.Second)

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: t.TempDir()}, func(domain.Finding) {})
	require.Error(t, err, "a scanner failure must surface as a Run error, so only this job fails (FR-ORC-006/NFR-REL-001), not a silent empty result masquerading as a clean scan")
}

func TestEngine_Run_AnalysisFailedStatus_ReturnsError(t *testing.T) {
	sq := &fakeSonarQubeClient{task: sonarqube.Task{Status: sonarqube.TaskFailed, ErrorMsg: "boom"}}
	scanner := &fakeScanner{taskID: "task-1"}
	e := codescan.New(sq, scanner, &fakeRuleRegistrar{}, time.Second)

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: t.TempDir()}, func(domain.Finding) {})
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestEngine_Run_PollTimeout_ReturnsError(t *testing.T) {
	sq := &fakeSonarQubeClient{task: sonarqube.Task{Status: sonarqube.TaskInProgress}}
	scanner := &fakeScanner{taskID: "task-1"}
	e := codescan.New(sq, scanner, &fakeRuleRegistrar{}, 50*time.Millisecond)

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: t.TempDir()}, func(domain.Finding) {})
	require.Error(t, err, "an analysis stuck IN_PROGRESS past GUARDPIPE_SONARQUBE_ANALYSIS_TIMEOUT must fail the job, not hang forever")
}

func TestEngine_Run_RuleRegistrationFailure_FailsCleanly(t *testing.T) {
	sq := &fakeSonarQubeClient{
		task:   sonarqube.Task{Status: sonarqube.TaskSuccess},
		issues: []sonarqube.Issue{{Key: "i1", RuleKey: "go:S2076", Severity: "HIGH", Component: "guardpipe-x:main.go", Message: "x", Line: 1}},
	}
	scanner := &fakeScanner{taskID: "task-1"}
	registrar := &fakeRuleRegistrar{err: errors.New("db unavailable")}
	e := codescan.New(sq, scanner, registrar, time.Second)

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: t.TempDir()}, func(domain.Finding) {})
	require.Error(t, err, "a rule-upsert failure must fail the run cleanly here, not surface later as an opaque findings.rule_id FK violation")
}

func TestEngine_ID(t *testing.T) {
	e := codescan.New(&fakeSonarQubeClient{}, &fakeScanner{}, &fakeRuleRegistrar{}, time.Second)
	require.Equal(t, domain.EngineCodeScan, e.ID())
}
