//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// seedProject creates the minimum row chain a scan needs: an org, a user,
// and a project (no repository attached — these tests exercise
// persistence, not cloning).
func seedProject(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, pool)

	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		projectID, orgID, "Test Project")
	require.NoError(t, err)
	return projectID
}

func TestScanRepo_CreateAndGet_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	branch := "main"
	scan := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain,
		Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan},
		Branch: &branch, FindingCounts: map[domain.Severity]int{},
	}
	require.NoError(t, scans.Create(ctx, scan))
	require.False(t, scan.QueuedAt.IsZero(), "Create() must populate QueuedAt from the DB default")
	require.Equal(t, 1, scan.ScanNumber, "the first scan for a project must be numbered 1")

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, got.Status)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, got.RequestedEngines)
	require.Equal(t, "main", *got.Branch)
	require.False(t, got.CancelRequested)
	require.Equal(t, 1, got.ScanNumber, "GetByID must return the same scan_number Create did")
}

// TestScanRepo_ListByProject_NewestFirstAndScopedToProject exercises the
// scan-history query: newest-first ordering (idx_scans_project_created)
// and that a second project's scans never leak into the first's list.
// Both projects share one seeded org/user — seedOrgAndUser hardcodes a
// single email, so seeding it twice in one test would violate the users
// table's unique-email constraint.
func TestScanRepo_ListByProject_NewestFirstAndScopedToProject(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, pool)

	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		projectID, orgID, "Test Project")
	require.NoError(t, err)

	otherProjectID := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		otherProjectID, orgID, "Other Project")
	require.NoError(t, err)

	scans := repo.NewScanRepo(pool)

	first := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, first))
	second := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, second))
	other := &domain.Scan{ID: id.New(), ProjectID: otherProjectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, other))

	list, total, err := scans.ListByProject(ctx, projectID, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID, "newest scan first")
	require.Equal(t, first.ID, list[1].ID)

	otherList, otherTotal, err := scans.ListByProject(ctx, otherProjectID, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, otherTotal)
	require.Len(t, otherList, 1)
	require.Equal(t, other.ID, otherList[0].ID)
}

// TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst is the real-Postgres
// proof for the global Scans page's cross-project history: scans from two
// different projects in the SAME org both show up (newest first, with
// their project's name attached), and a second org's scan never leaks in —
// the org-scoping is the WHERE clause itself, not a filter applied after
// the fact.
func TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	scans := repo.NewScanRepo(pool)

	orgA, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org A")
	require.NoError(t, err)
	orgB, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org B")
	require.NoError(t, err)

	projectA1 := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectA1, orgA, "Org A Project 1")
	require.NoError(t, err)
	projectA2 := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectA2, orgA, "Org A Project 2")
	require.NoError(t, err)
	projectB := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectB, orgB, "Org B Project")
	require.NoError(t, err)

	first := &domain.Scan{ID: id.New(), ProjectID: projectA1, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, first))
	second := &domain.Scan{ID: id.New(), ProjectID: projectA2, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, second))
	otherOrgScan := &domain.Scan{ID: id.New(), ProjectID: projectB, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, otherOrgScan))

	list, total, err := scans.ListByOrg(ctx, orgA, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total, "both of org A's projects' scans count, org B's does not")
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID, "newest scan first, across projects")
	require.Equal(t, "Org A Project 2", list[0].ProjectName)
	require.Equal(t, first.ID, list[1].ID)
	require.Equal(t, "Org A Project 1", list[1].ProjectName)

	otherList, otherTotal, err := scans.ListByOrg(ctx, orgB, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, otherTotal)
	require.Len(t, otherList, 1)
	require.Equal(t, otherOrgScan.ID, otherList[0].ID)
	require.Equal(t, "Org B Project", otherList[0].ProjectName)
}

func TestScanRepo_SetCancelRequested(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	require.NoError(t, scans.SetCancelRequested(ctx, scan.ID))

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.True(t, got.CancelRequested)
}

func TestScanJobRepo_CreateListAndMarkRunning(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))

	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	list, err := jobs.ListByScan(ctx, scan.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, domain.JobStatusQueued, list[0].Status)

	require.NoError(t, jobs.MarkRunning(ctx, job.ID))
	got, err := jobs.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusRunning, got.Status)
	require.NotNil(t, got.StartedAt)
	require.NotNil(t, got.ClaimedAt)
}

// TestJobResultRepo_PersistJobResult_OneJobScan_FindingsPersistAndScanFinalizes
// is the core correctness proof for documentation/04-backend-architecture.md
// §5.2's "one transaction per job completion": findings land, the job's
// status updates, and — since this is the scan's only job — the scan
// finalises to completed with the right finding_counts, all from one call.
func TestJobResultRepo_PersistJobResult_OneJobScan_FindingsPersistAndScanFinalizes(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)

	seedRule(t, pool, "depscan.hygiene.no-lockfile")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))
	require.NoError(t, jobs.MarkRunning(ctx, job.ID))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.no-lockfile",
		Fingerprint: "fp-1", Title: "no lockfile", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "package.json"},
		Remediation: "commit a lockfile", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindManifestExcerpt, Value: "no lockfile present"}},
	}

	err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Stats: map[string]any{"files_scanned": 1},
		Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	gotJob, err := jobs.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, gotJob.Status)
	require.NotNil(t, gotJob.FinishedAt)

	list, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, "depscan.hygiene.no-lockfile", list[0].RuleID)
	require.Equal(t, domain.LocationTypeFile, list[0].Location.Type)
	require.Equal(t, "package.json", list[0].Location.Path)
	require.Equal(t, "commit a lockfile", list[0].Remediation, "Remediation must round-trip through ListByScan, not just the insert path")
	require.Len(t, list[0].Evidence, 1, "finding_evidence is written by insertFindings but was never read back by ListByScan until this fix")
	require.Equal(t, domain.EvidenceKindManifestExcerpt, list[0].Evidence[0].Kind)
	require.Equal(t, "no lockfile present", list[0].Evidence[0].Value)

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, gotScan.Status, "the scan finalises once its only job is terminal")
	require.NotNil(t, gotScan.FinishedAt)
	require.Equal(t, 1, gotScan.FindingCounts[domain.SeverityMedium])
}

// TestFindingRepo_ListByScan_EvidenceDoesNotLeakBetweenFindings guards the
// batch-fetch in attachEvidence: it maps finding_evidence rows back onto
// findings by finding_id in one query, so a bug there would silently
// attach one finding's evidence to another rather than erroring.
func TestFindingRepo_ListByScan_EvidenceDoesNotLeakBetweenFindings(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	seedRule(t, pool, "depscan.secrets.committed-credential")
	seedRule(t, pool, "depscan.hygiene.wildcard-version")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	secretFinding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.secrets.committed-credential",
		Fingerprint: "fp-secret", Title: "hardcoded credential", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "login/page.tsx", LineStart: 42},
		Remediation: "move to an env var", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: "password: '••••••••'", Redacted: true, LineStart: 42, LineEnd: 42}},
	}
	wildcardFinding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.wildcard-version",
		Fingerprint: "fp-wildcard", Title: "wildcard version", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "left-pad"},
		Remediation: "pin a version", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindManifestExcerpt, Value: `"left-pad": "*"`}},
	}

	require.NoError(t, jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{secretFinding, wildcardFinding},
	}))

	list, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, list, 2)

	byRuleID := map[string]domain.Finding{}
	for _, f := range list {
		byRuleID[f.RuleID] = f
	}

	secret := byRuleID["depscan.secrets.committed-credential"]
	require.Len(t, secret.Evidence, 1)
	require.Equal(t, "password: '••••••••'", secret.Evidence[0].Value)
	require.Equal(t, 42, secret.Evidence[0].LineStart)

	wildcard := byRuleID["depscan.hygiene.wildcard-version"]
	require.Len(t, wildcard.Evidence, 1)
	require.Equal(t, `"left-pad": "*"`, wildcard.Evidence[0].Value)
}

func TestJobResultRepo_PersistJobResult_IdempotentOnRerun(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	seedRule(t, pool, "depscan.hygiene.wildcard-version")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.wildcard-version",
		Fingerprint: "same-fingerprint", Title: "t", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "left-pad"},
		Remediation: "pin a version", Status: domain.StatusOpen,
	}
	result := orchestrator.JobResult{JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding}}

	require.NoError(t, jobResults.PersistJobResult(ctx, result))
	// Re-run with a fresh finding ID but the same fingerprint — simulating
	// the same job re-executing (NFR-REL-002: re-runs are idempotent).
	finding.ID = id.New()
	result.Findings = []domain.Finding{finding}
	require.NoError(t, jobResults.PersistJobResult(ctx, result))

	_, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total, "a re-run with the same fingerprint must not double-insert")
}

func TestJobResultRepo_PersistJobResult_MultiJobScan_DoesNotFinalizeUntilLastJob(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan, domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	jobA := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	jobB := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{jobA, jobB}))

	require.NoError(t, jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobA.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded}))

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, gotScan.Status, "must not finalise while jobB is still queued")

	require.NoError(t, jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobB.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan, Status: domain.JobStatusSkipped, SkipReason: "no manifest"}))

	gotScan, err = scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, gotScan.Status, "finalises once the last job (skipped counts as terminal) is done")
}

// seedRule inserts the minimum rules row a finding's rule_id foreign key
// needs — findings.rule_id references rules(id) ON DELETE RESTRICT.
func seedRule(t *testing.T, pool *pgxpool.Pool, ruleID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rules (id, engine, category, title, description, remediation, default_severity, tier)
		VALUES ($1, 'depscan', 'test', 'test rule', 'test', 'test', 'medium', 'core')
		ON CONFLICT (id) DO NOTHING`, ruleID)
	require.NoError(t, err)
}
