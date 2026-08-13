package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// Page is the same page/page-size shape project.Page uses, kept local per
// that package's own precedent (no shared reason to change together).
type Page struct {
	Page     int
	PageSize int
}

// Service is scan creation and read access
// (documentation/07-api-specification.md §5-6's subset this phase builds —
// risk/AI/triage fields are Phase 13's).
type Service interface {
	CreateScan(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in CreateScanInput) (*ScanDetail, error)
	GetScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*ScanDetail, error)
	ListScans(ctx context.Context, actor domain.Actor, projectID uuid.UUID, page Page) ([]domain.Scan, int, error)
	// ListOrgScans is the org-wide equivalent of ListScans — every scan
	// across every one of the actor's org's projects, newest first. No
	// projectID/ownership check needed: the query itself is already scoped
	// to actor.OrgID.
	ListOrgScans(ctx context.Context, actor domain.Actor, page Page) ([]OrgScanSummary, int, error)
	GetProgress(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*Progress, error)
	CancelScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) error
	ListFindings(ctx context.Context, actor domain.Actor, scanID uuid.UUID, page Page) ([]domain.Finding, int, error)
}

// ScanRepository is defined by this package; implementation lives in
// internal/store/repo.
type ScanRepository interface {
	Create(ctx context.Context, s *domain.Scan) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Scan, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, page Page) ([]domain.Scan, int, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, page Page) ([]OrgScanSummary, int, error)
	SetCancelRequested(ctx context.Context, id uuid.UUID) error
}

// ScanJobRepository is defined by this package. Note there is no
// MarkSucceeded/MarkFailed/MarkSkipped here — a job's terminal write always
// goes through JobResultRepository.PersistJobResult instead, so it's
// impossible to update a job's status without going through the one
// transactional path that also persists its findings and (when it's the
// scan's last job) finalises the scan.
type ScanJobRepository interface {
	CreateMany(ctx context.Context, jobs []domain.ScanJob) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ScanJob, error)
	ListByScan(ctx context.Context, scanID uuid.UUID) ([]domain.ScanJob, error)
	MarkRunning(ctx context.Context, id uuid.UUID) error
}

// FindingRepository is defined by this package — read paths only; writes
// go through JobResultRepository (see its doc comment).
type FindingRepository interface {
	ListByScan(ctx context.Context, scanID uuid.UUID, page Page) ([]domain.Finding, int, error)
	CountByScanAndSeverity(ctx context.Context, scanID uuid.UUID) (map[domain.Severity]int, error)
	CountByJob(ctx context.Context, jobID uuid.UUID) (int, error)
}

// JobResult is what one completed job reports to JobResultRepository.
// Status must be Succeeded, Failed, Skipped, or Cancelled — a terminal
// status only; MarkRunning (ScanJobRepository) is the separate, simpler,
// non-transactional write for the job's earlier "started" transition.
type JobResult struct {
	JobID       uuid.UUID
	ScanID      uuid.UUID
	ProjectID   uuid.UUID // denormalised onto every findings row, documentation/06-database-design.md §4.11
	Engine      domain.EngineID
	Status      domain.JobStatus
	ErrorReason string
	SkipReason  string
	Stats       map[string]any
	Findings    []domain.Finding
}

// JobResultRepository is the one atomic write path for a completed job —
// documentation/04-backend-architecture.md §5.2: "one transaction per job
// completion," inserting findings and updating job status together, and
// (implementation detail, inside the same transaction) finalising the
// scan's own status/finding_counts once every job for it is terminal.
// Findings insert is idempotent on (scan_id, fingerprint) — a re-run of
// the same job never double-counts (NFR-REL-002).
type JobResultRepository interface {
	PersistJobResult(ctx context.Context, result JobResult) error
}

// ProjectAccess is the subset of project.Service this package needs —
// defined here (the consumer) even though project.Service is already the
// concrete dependency, so a future narrower fake doesn't need the whole
// interface.
type ProjectAccess interface {
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (*project.ProjectDetail, error)
}

type service struct {
	scans    ScanRepository
	jobs     ScanJobRepository
	findings FindingRepository
	projects ProjectAccess
	queue    Enqueuer
	registry *Registry
}

// Enqueuer is the subset of adapters/queue.JobQueue this package needs —
// defined here so tests substitute a fake instead of real Redis.
type Enqueuer interface {
	Enqueue(ctx context.Context, jobID string) error
}

func NewService(scans ScanRepository, jobs ScanJobRepository, findings FindingRepository, projects ProjectAccess, q Enqueuer, registry *Registry) Service {
	return &service{scans: scans, jobs: jobs, findings: findings, projects: projects, queue: q, registry: registry}
}

func (s *service) CreateScan(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in CreateScanInput) (*ScanDetail, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, err // already 404-not-403 per project.Service's own rule
	}
	if !in.Type.Valid() {
		return nil, apperrors.Validation("scan.invalid_input", "type must be a recognised scan type", nil)
	}

	engines, err := s.resolveEngines(in)
	if err != nil {
		return nil, err
	}
	if len(engines) == 0 {
		return nil, apperrors.Unprocessable("scan.no_engines_available", "no engine is registered to run this scan yet")
	}

	scan := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: in.Type, Status: domain.ScanStatusQueued,
		RequestedEngines: engines, FindingCounts: map[domain.Severity]int{},
	}
	if in.Branch != "" {
		scan.Branch = &in.Branch
	}
	if actor.UserID != uuid.Nil {
		triggeredBy := actor.UserID
		scan.TriggeredBy = &triggeredBy
	}
	if err := s.scans.Create(ctx, scan); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create scan: %w", err))
	}

	jobs := make([]domain.ScanJob, len(engines))
	for i, eng := range engines {
		jobs[i] = domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: eng, Status: domain.JobStatusQueued}
	}
	if err := s.jobs.CreateMany(ctx, jobs); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create scan jobs: %w", err))
	}

	for _, j := range jobs {
		if err := s.queue.Enqueue(ctx, j.ID.String()); err != nil {
			return nil, apperrors.Internal(fmt.Errorf("enqueue job %s: %w", j.ID, err))
		}
	}

	details := make([]JobDetail, len(jobs))
	for i, j := range jobs {
		details[i] = JobDetail{ScanJob: j}
	}
	return &ScanDetail{Scan: *scan, Jobs: details}, nil
}

// resolveEngines picks which engines a scan runs: an explicit list for a
// partial scan (validated against what's actually registered), or every
// registered engine for a full_supply_chain scan — "every engine" is
// deliberately registry-driven, not a hardcoded seven-engine list, since
// only depscan exists this phase.
func (s *service) resolveEngines(in CreateScanInput) ([]domain.EngineID, error) {
	if in.Type != domain.ScanTypePartial {
		return s.registry.IDs(), nil
	}
	if len(in.Engines) == 0 {
		return nil, apperrors.Validation("scan.invalid_input", "engines is required for a partial scan", nil)
	}
	for _, e := range in.Engines {
		if !s.registry.Has(e) {
			return nil, apperrors.Unprocessable("scan.engine_unavailable", fmt.Sprintf("engine %q is not registered", e))
		}
	}
	return in.Engines, nil
}

func (s *service) GetScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*ScanDetail, error) {
	scan, err := s.getOwnedScan(ctx, actor, scanID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.jobs.ListByScan(ctx, scanID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list scan jobs: %w", err))
	}
	details := make([]JobDetail, len(jobs))
	for i, j := range jobs {
		count, err := s.findings.CountByJob(ctx, j.ID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("count job findings: %w", err))
		}
		details[i] = JobDetail{ScanJob: j, FindingCount: count}
	}
	return &ScanDetail{Scan: *scan, Jobs: details}, nil
}

// ListScans powers the scan-history page — newest first, paginated. The
// project ownership check happens before the list query, same 404-not-403
// rule getOwnedScan enforces for a single scan (documentation/07-api-specification.md
// §1.4): a cross-org project ID must look identical to a nonexistent one.
func (s *service) ListScans(ctx context.Context, actor domain.Actor, projectID uuid.UUID, page Page) ([]domain.Scan, int, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, 0, err
	}
	scans, total, err := s.scans.ListByProject(ctx, projectID, page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list scans: %w", err))
	}
	return scans, total, nil
}

func (s *service) GetProgress(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*Progress, error) {
	scan, err := s.getOwnedScan(ctx, actor, scanID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.jobs.ListByScan(ctx, scanID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list scan jobs: %w", err))
	}

	engines := make([]EngineProgress, len(jobs))
	done := 0
	for i, j := range jobs {
		count, err := s.findings.CountByJob(ctx, j.ID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("count job findings: %w", err))
		}
		pct := 0
		switch j.Status {
		case domain.JobStatusSucceeded, domain.JobStatusFailed, domain.JobStatusSkipped, domain.JobStatusCancelled:
			pct = 100
			done++
		case domain.JobStatusRunning:
			pct = 50
		}
		engines[i] = EngineProgress{Engine: j.Engine, Status: j.Status, ProgressPct: pct, FindingCount: count}
	}

	overallPct := 0
	if len(jobs) > 0 {
		overallPct = done * 100 / len(jobs)
	}

	return &Progress{ScanID: scan.ID, Status: scan.Status, ProgressPct: overallPct, Engines: engines}, nil
}

// ListOrgScans powers the global Scans page's cross-project history — every
// scan across every project the actor's org owns, newest first. Unlike
// ListScans, there's no project-ownership check to make: the underlying
// query is already scoped to actor.OrgID, so a scan from another org can
// never appear here in the first place (no separate 404-vs-403 case
// applies — it's not "found but not yours," it's simply never queried).
func (s *service) ListOrgScans(ctx context.Context, actor domain.Actor, page Page) ([]OrgScanSummary, int, error) {
	scans, total, err := s.scans.ListByOrg(ctx, actor.OrgID, page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list org scans: %w", err))
	}
	return scans, total, nil
}

func (s *service) CancelScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) error {
	if _, err := s.getOwnedScan(ctx, actor, scanID); err != nil {
		return err
	}
	if err := s.scans.SetCancelRequested(ctx, scanID); err != nil {
		return apperrors.Internal(fmt.Errorf("cancel scan: %w", err))
	}
	return nil
}

func (s *service) ListFindings(ctx context.Context, actor domain.Actor, scanID uuid.UUID, page Page) ([]domain.Finding, int, error) {
	if _, err := s.getOwnedScan(ctx, actor, scanID); err != nil {
		return nil, 0, err
	}
	return s.findings.ListByScan(ctx, scanID, page)
}

// getOwnedScan loads a scan and confirms actor's organisation owns its
// project — 404, not 403, on a cross-org access attempt
// (documentation/07-api-specification.md §1.4).
func (s *service) getOwnedScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*domain.Scan, error) {
	scan, err := s.scans.GetByID(ctx, scanID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("scan.not_found", "scan not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get scan: %w", err))
	}
	if _, err := s.projects.Get(ctx, actor, scan.ProjectID); err != nil {
		return nil, apperrors.NotFound("scan.not_found", "scan not found")
	}
	return scan, nil
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
