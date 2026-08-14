package containerscan_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/engines/containerscan"
)

// fakeScanner is a hand-written fake — no live Trivy in tests
// (documentation/15-testing-strategy.md: "no mocking framework ...
// hand-written fakes only", "Tests never call ... live").
type fakeScanner struct {
	configReport trivy.Report
	configErr    error
	imageReport  trivy.Report
	imageErr     error
	imageCalls   int
	scannedRef   string
}

func (f *fakeScanner) ScanConfig(context.Context, string) (trivy.Report, error) {
	return f.configReport, f.configErr
}

func (f *fakeScanner) ScanImage(_ context.Context, ref string) (trivy.Report, error) {
	f.imageCalls++
	f.scannedRef = ref
	return f.imageReport, f.imageErr
}

type fakeImageBuilder struct {
	buildErr    error
	buildCalls  int
	removeCalls int
	builtTag    string
}

func (f *fakeImageBuilder) BuildImage(_ context.Context, _, _, tag string) error {
	f.buildCalls++
	f.builtTag = tag
	return f.buildErr
}

func (f *fakeImageBuilder) RemoveImage(context.Context, string) error {
	f.removeCalls++
	return nil
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

func newWorkspaceWithDockerfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	return dir
}

func TestEngine_Applicable_TrueWithDockerfile(t *testing.T) {
	dir := newWorkspaceWithDockerfile(t)
	e := containerscan.New(&fakeScanner{}, &fakeImageBuilder{}, &fakeRuleRegistrar{})
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.True(t, ok, "reason: %s", reason)
}

func TestEngine_Applicable_TrueWithContainerfileOrDockerfileExtension(t *testing.T) {
	for _, name := range []string{"Containerfile", "backend.dockerfile"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("FROM alpine\n"), 0o644))
			e := containerscan.New(&fakeScanner{}, &fakeImageBuilder{}, &fakeRuleRegistrar{})
			ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
			require.True(t, ok, "reason: %s", reason)
		})
	}
}

func TestEngine_Applicable_FalseWithNoDockerfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644))
	e := containerscan.New(&fakeScanner{}, &fakeImageBuilder{}, &fakeRuleRegistrar{})
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.False(t, ok)
	require.NotEmpty(t, reason)
}

func TestEngine_Applicable_SkipsVendorAndNodeModules(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "somepkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "somepkg", "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	e := containerscan.New(&fakeScanner{}, &fakeImageBuilder{}, &fakeRuleRegistrar{})
	ok, _ := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.False(t, ok, "a Dockerfile only inside node_modules/ must not make the engine applicable")
}

// TestEngine_Run_MisconfigVulnAndSecret_AllNormalised covers all three
// finding shapes containerscan emits in one pass: a Dockerfile
// misconfiguration (from ScanConfig), an image vulnerability, and an
// image-layer secret (both from ScanImage) — confirming each is normalised
// correctly and nothing is dropped or invented.
func TestEngine_Run_MisconfigVulnAndSecret_AllNormalised(t *testing.T) {
	scanner := &fakeScanner{
		configReport: trivy.Report{Results: []trivy.Result{
			{
				Target: "Dockerfile",
				Misconfigurations: []trivy.Misconfig{
					{ID: "DS002", Title: "Image user should not be 'root'", Message: "Specify at least 1 USER command", Resolution: "Add a USER instruction", Severity: "HIGH", CauseMetadata: struct {
						StartLine int `json:"StartLine"`
						EndLine   int `json:"EndLine"`
					}{StartLine: 1, EndLine: 1}},
				},
			},
		}},
		imageReport: trivy.Report{Results: []trivy.Result{
			{
				Target: "alpine:3.19 (alpine 3.19.1)",
				Vulnerabilities: []trivy.Vulnerability{
					{VulnerabilityID: "CVE-2024-1234", PkgName: "openssl", InstalledVersion: "3.1.0", FixedVersion: "3.1.4", Severity: "CRITICAL", Title: "OpenSSL buffer overflow", CweIDs: []string{"120"}},
				},
			},
			{
				Target: "app/.env",
				Secrets: []trivy.Secret{
					{RuleID: "aws-access-key-id", Category: "AWS", Title: "AWS Access Key ID", Severity: "CRITICAL", StartLine: 2, EndLine: 2},
				},
			},
		}},
	}
	builder := &fakeImageBuilder{}
	registrar := &fakeRuleRegistrar{}
	e := containerscan.New(scanner, builder, registrar)

	var findings []domain.Finding
	scanID := uuid.New()
	result, err := e.Run(context.Background(), domain.ScanInput{ScanID: scanID, ProjectID: uuid.New(), WorkspaceDir: newWorkspaceWithDockerfile(t)}, func(f domain.Finding) {
		findings = append(findings, f)
	})
	require.NoError(t, err)
	require.Equal(t, 1, builder.buildCalls)
	require.Equal(t, 1, builder.removeCalls, "the built image must always be cleaned up")
	require.Equal(t, 1, scanner.imageCalls)
	require.Equal(t, builder.builtTag, scanner.scannedRef, "ScanImage must scan the exact tag BuildImage built")
	require.Len(t, findings, 3, "one misconfig + one vulnerability + one secret, nothing discarded and nothing invented")
	require.Equal(t, 3, result.RulesEvaluated)

	var misconfig, vuln, secret *domain.Finding
	for i := range findings {
		switch findings[i].RuleID {
		case "containerscan.trivy.DS002":
			misconfig = &findings[i]
		case "containerscan.trivy.CVE-2024-1234":
			vuln = &findings[i]
		case "containerscan.trivy.aws-access-key-id":
			secret = &findings[i]
		}
	}
	require.NotNil(t, misconfig)
	require.NotNil(t, vuln)
	require.NotNil(t, secret)

	require.Equal(t, domain.SeverityHigh, misconfig.Severity)
	require.Equal(t, domain.LocationTypeFile, misconfig.Location.Type)
	require.Equal(t, "Dockerfile", misconfig.Location.Path)
	require.Contains(t, misconfig.Remediation, "USER instruction")

	require.Equal(t, domain.SeverityCritical, vuln.Severity)
	require.Equal(t, domain.ConfidenceHigh, vuln.Confidence)
	require.Equal(t, []string{"CVE-2024-1234"}, vuln.CVE)
	require.Equal(t, []string{"CWE-120"}, vuln.CWE, "a bare numeric CWE from Trivy must be normalised to the CWE- form")
	require.Equal(t, "openssl: OpenSSL buffer overflow", vuln.Title, "the title must lead with the affected package — the same CVE routinely hits several distinct packages sharing one source package, and Trivy's own title describes only the CVE, not which package; without the prefix every one of those findings renders as an apparent duplicate in a list")
	require.Equal(t, domain.LocationTypeImage, vuln.Location.Type)
	require.Contains(t, vuln.Remediation, "3.1.4")

	require.Equal(t, domain.SeverityCritical, secret.Severity)
	require.Equal(t, domain.LocationTypeImage, secret.Location.Type)

	require.Len(t, registrar.upserted, 3, "each distinct rule is registered exactly once")
}

func TestEngine_Run_ConfigScanFailure_FailsJobOnly(t *testing.T) {
	scanner := &fakeScanner{configErr: errors.New("trivy config exited 1")}
	e := containerscan.New(scanner, &fakeImageBuilder{}, &fakeRuleRegistrar{})

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: newWorkspaceWithDockerfile(t)}, func(domain.Finding) {})
	require.Error(t, err, "a config-scan failure must surface as a Run error, so only this job fails (FR-ORC-006/NFR-REL-001), not a silent empty result masquerading as a clean scan")
}

func TestEngine_Run_ImageBuildFailure_FailsJobOnly(t *testing.T) {
	builder := &fakeImageBuilder{buildErr: errors.New("Dockerfile parse error on line 3")}
	e := containerscan.New(&fakeScanner{}, builder, &fakeRuleRegistrar{})

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: newWorkspaceWithDockerfile(t)}, func(domain.Finding) {})
	require.Error(t, err)
	require.Equal(t, 0, builder.removeCalls, "nothing was built, so nothing should be removed")
}

func TestEngine_Run_ImageScanFailure_FailsJobOnly(t *testing.T) {
	scanner := &fakeScanner{imageErr: errors.New("trivy image exited 1")}
	builder := &fakeImageBuilder{}
	e := containerscan.New(scanner, builder, &fakeRuleRegistrar{})

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: newWorkspaceWithDockerfile(t)}, func(domain.Finding) {})
	require.Error(t, err)
	require.Equal(t, 1, builder.removeCalls, "the built image must still be cleaned up even when the scan after it fails")
}

func TestEngine_Run_RuleRegistrationFailure_FailsCleanly(t *testing.T) {
	scanner := &fakeScanner{
		configReport: trivy.Report{Results: []trivy.Result{
			{Target: "Dockerfile", Misconfigurations: []trivy.Misconfig{{ID: "DS002", Severity: "HIGH"}}},
		}},
	}
	registrar := &fakeRuleRegistrar{err: errors.New("db unavailable")}
	e := containerscan.New(scanner, &fakeImageBuilder{}, registrar)

	_, err := e.Run(context.Background(), domain.ScanInput{ScanID: uuid.New(), ProjectID: uuid.New(), WorkspaceDir: newWorkspaceWithDockerfile(t)}, func(domain.Finding) {})
	require.Error(t, err, "a rule-upsert failure must fail the run cleanly here, not surface later as an opaque findings.rule_id FK violation")
}

func TestEngine_ID(t *testing.T) {
	e := containerscan.New(&fakeScanner{}, &fakeImageBuilder{}, &fakeRuleRegistrar{})
	require.Equal(t, domain.EngineContainerScan, e.ID())
}
