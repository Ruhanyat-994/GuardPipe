package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
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

	// CreateSchedule/GetSchedule/ListSchedules/UpdateSchedule/DeleteSchedule
	// are BUILD_GUIDE.md Phase 15's cron scan scheduling — see schedule.go.
	CreateSchedule(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in CreateScheduleInput) (*ScanSchedule, error)
	GetSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID) (*ScanSchedule, error)
	ListSchedules(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ScanSchedule, error)
	UpdateSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID, in UpdateScheduleInput) (*ScanSchedule, error)
	DeleteSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID) error
	// TriggerSchedule is the scheduler ticker's own entry point (scheduler.go)
	// — never called from the HTTP layer, see its own doc comment.
	TriggerSchedule(ctx context.Context, scheduleID uuid.UUID) (*ScanDetail, error)
}

// ScanRepository is defined by this package; implementation lives in
// internal/store/repo.
type ScanRepository interface {
	Create(ctx context.Context, s *domain.Scan) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Scan, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, page Page) ([]domain.Scan, int, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, page Page) ([]OrgScanSummary, int, error)
	SetCancelRequested(ctx context.Context, id uuid.UUID) error
	// MarkStarted records the scan's transition out of `queued` — status
	// `running` and `started_at = now()` — the first time any of its jobs is
	// claimed. Idempotent (`WHERE status = 'queued'`): every later job claim
	// for the same scan calls this too, and must be a safe no-op rather than
	// clobbering the real start time or overwriting a terminal status set by
	// a fast-finishing job in the meantime. Previously nothing called this at
	// all — every scan's started_at stayed NULL forever ("Scan window: not
	// started" in a completed scan's export report is what surfaced this).
	MarkStarted(ctx context.Context, id uuid.UUID) error
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
	// ListAllByScan is the unpaginated form scoring.Compute needs — the
	// formula has to see every finding for the scan, not one page of them.
	ListAllByScan(ctx context.Context, scanID uuid.UUID) ([]domain.Finding, error)
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
//
// PersistJobResult's bool return is true exactly when this call was the one
// that finalized the scan (every job now terminal) — Pool.persist uses it
// to compute and persist a RiskAssessment exactly once per scan, from the
// orchestrator layer rather than from inside this SQL-only transaction
// (scoring is business logic; CLAUDE.md reserves that for the service/
// worker layer, never the repository).
type JobResultRepository interface {
	PersistJobResult(ctx context.Context, result JobResult) (finalized bool, err error)
}

// RiskAssessmentRepository is defined by this package; implementation lives
// in internal/store/repo. Create is called exactly once per scan (by
// Pool's finalizeScoring, when PersistJobResult signals the scan just
// finalized) but implementations should treat scan_id as idempotent —
// a redelivered/reclaimed job could plausibly trigger a second call for
// the same scan, and a second Create must replace, not duplicate.
type RiskAssessmentRepository interface {
	Create(ctx context.Context, r RiskAssessmentRecord) error
	GetByScanID(ctx context.Context, scanID uuid.UUID) (*RiskAssessmentRecord, error)
	// GetPreviousScore returns the score of the most recent prior completed
	// scan for projectID (excluding excludeScanID), or nil if there is none
	// (documentation/11-risk-scoring-and-severity.md §6's Delta source).
	GetPreviousScore(ctx context.Context, projectID, excludeScanID uuid.UUID) (*int, error)
}

// ProjectAccess is the subset of project.Service this package needs —
// defined here (the consumer) even though project.Service is already the
// concrete dependency, so a future narrower fake doesn't need the whole
// interface. GetAttestedTarget mirrors worker.go's own TargetProvider
// interface exactly (same underlying project.Service method) — resolveEngines
// needs the same "does this project have a usable pentest target" answer the
// worker already asks for at execution time, just earlier, at scan creation.
type ProjectAccess interface {
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (*project.ProjectDetail, error)
	GetAttestedTarget(ctx context.Context, projectID uuid.UUID) (*project.Target, error)
	// GetOrgID is the scheduler ticker's own read (BUILD_GUIDE.md Phase 15,
	// see schedule.go's TriggerSchedule) — mirrors project.Service's own
	// no-actor GetOrgID exactly.
	GetOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)
}

type service struct {
	scans           ScanRepository
	jobs            ScanJobRepository
	findings        FindingRepository
	riskAssessments RiskAssessmentRepository
	projects        ProjectAccess
	queue           Enqueuer
	registry        *Registry
	pentestCeiling  domain.PentestScanConfig
	audit           audit.Service
	// progress/engineTimeouts/defaultTimeout back GetProgress's per-engine
	// percentage: progress is the live store Pool writes to as an engine
	// reports real stage progress (nil-safe — a nil store just means every
	// running job falls back to the elapsed-time estimate below); the
	// timeouts are what that estimate is computed against. All three may be
	// zero-valued in a test that doesn't exercise GetProgress.
	progress       *LiveProgress
	engineTimeouts map[domain.EngineID]time.Duration
	defaultTimeout time.Duration
	// schedules/membership back BUILD_GUIDE.md Phase 15's cron scan
	// scheduling (schedule.go) — membership may be nil (requireOrgMember
	// then skips the check, a test-only convenience; cmd/guardpipe/main.go
	// always wires a real one in production).
	schedules  ScanScheduleRepository
	membership MembershipChecker
}

// Enqueuer is the subset of adapters/queue.JobQueue this package needs —
// defined here so tests substitute a fake instead of real Redis.
type Enqueuer interface {
	Enqueue(ctx context.Context, jobID string) error
}

// NewService's pentestCeiling is the hard cap CreateScan clamps every
// pentest_config against, named or custom alike (BUILD_GUIDE.md Phase 12) —
// callers normally build it from platform/config's Pentest.RateLimit atop
// domain.PentestPresetDeepConfig(), since Deep's own numbers are meant to
// already sit at the ceiling by construction (see
// domain.ClampPentestScanConfig's doc comment).
// progress may be nil (falls back to elapsedFallbackPct for every running
// job, never a frozen constant); engineTimeouts/defaultTimeout should be
// the same values Pool itself uses so the estimate matches what the worker
// will actually enforce.
func NewService(scans ScanRepository, jobs ScanJobRepository, findings FindingRepository, riskAssessments RiskAssessmentRepository, projects ProjectAccess, q Enqueuer, registry *Registry, pentestCeiling domain.PentestScanConfig, progress *LiveProgress, engineTimeouts map[domain.EngineID]time.Duration, defaultTimeout time.Duration, auditSvc audit.Service, schedules ScanScheduleRepository, membership MembershipChecker) Service {
	return &service{
		scans: scans, jobs: jobs, findings: findings, riskAssessments: riskAssessments, projects: projects, queue: q, registry: registry, pentestCeiling: pentestCeiling,
		progress: progress, engineTimeouts: engineTimeouts, defaultTimeout: defaultTimeout, audit: auditSvc,
		schedules: schedules, membership: membership,
	}
}

func (s *service) CreateScan(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in CreateScanInput) (*ScanDetail, error) {
	detail, err := s.projects.Get(ctx, actor, projectID)
	if err != nil {
		return nil, err // already 404-not-403 per project.Service's own rule
	}
	if !in.Type.Valid() {
		return nil, apperrors.Validation("scan.invalid_input", "type must be a recognised scan type", nil)
	}

	engines, err := s.resolveEngines(ctx, projectID, detail, in)
	if err != nil {
		return nil, err
	}
	if len(engines) == 0 {
		return nil, apperrors.Unprocessable("scan.no_engines_available", "no engine is registered to run this scan yet")
	}

	scan := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: in.Type, Status: domain.ScanStatusQueued,
		RequestedEngines: engines, FindingCounts: map[domain.Severity]int{}, RequestedFromIP: in.SourceIP,
	}
	if in.Branch != "" {
		scan.Branch = &in.Branch
	}
	if actor.UserID != uuid.Nil {
		triggeredBy := actor.UserID
		scan.TriggeredBy = &triggeredBy
	}

	var clampedFields []string
	if slices.Contains(engines, domain.EnginePentest) {
		requested := domain.DefaultPentestScanConfig()
		if in.PentestConfig != nil {
			requested = *in.PentestConfig
		}
		clamped, fields := domain.ClampPentestScanConfig(requested, s.pentestCeiling)
		scan.PentestConfig = &clamped
		clampedFields = fields
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

	// Accountability record for this specific scan execution — same
	// convention target.attested already establishes (project/service.go),
	// applied per-run rather than once per target: who (actor), from where
	// (SourceIP), against what (scan.ID), doing what (which engines).
	if s.audit != nil {
		var ipAddr *netip.Addr
		if parsed, err := netip.ParseAddr(in.SourceIP); err == nil {
			ipAddr = &parsed
		}
		s.audit.Log(ctx, audit.Entry{
			OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "scan.started",
			ResourceType: strPtr("scan"), ResourceID: &scan.ID, IP: ipAddr,
			Detail: map[string]any{"project_id": projectID.String(), "type": string(in.Type), "engines": engines},
		})
	}

	return &ScanDetail{Scan: *scan, Jobs: details, PentestConfigClamped: clampedFields}, nil
}

func strPtr(s string) *string { return &s }

// engineRequiresRepository is every registered engine except pentest (needs
// an attested target instead of a repository) and docreview (tolerates no
// repository at all — worker.go special-cases it to keep processing with an
// empty WorkspaceDir rather than failing "workspace_unavailable" like every
// other engine). Kept here, not in domain, since "what a project needs to
// have attached before this engine's job is even worth creating" is a
// scan-creation-time policy question, not an engine-intrinsic fact the way
// domain.Engine's own methods are.
func engineRequiresRepository(e domain.EngineID) bool {
	return e != domain.EnginePentest && e != domain.EngineDocReview
}

// hasAttestedTarget reports whether projectID has a usable pentest target,
// treating "no attested target" as false rather than an error — most
// projects simply don't have one, which is normal, not a failure.
func (s *service) hasAttestedTarget(ctx context.Context, projectID uuid.UUID) (bool, error) {
	_, err := s.projects.GetAttestedTarget(ctx, projectID)
	switch {
	case err == nil:
		return true, nil
	case isNotFound(err):
		return false, nil
	default:
		return false, apperrors.Internal(fmt.Errorf("check attested pentest target: %w", err))
	}
}

// unavailableReason explains, in the same plain language style every
// engine's own Applicable() reason already uses, why an explicitly-requested
// partial-scan engine can't run against this project's current shape.
func unavailableReason(e domain.EngineID, hasRepo, hasTarget bool) string {
	if e == domain.EnginePentest {
		return "no attested pentest target attached to this project"
	}
	if engineRequiresRepository(e) && !hasRepo {
		return "no repository attached to this project"
	}
	_ = hasTarget
	return "this engine cannot run against this project"
}

// resolveEngines picks which engines a scan runs, and — since real jobs are
// about to be created for whatever it returns — is also the actual
// enforcement behind "a URL-only project only ever runs pentest, a
// repository-only project never runs pentest": a repo-based engine's job is
// never created at all for a target-only project (rather than created and
// then hard-failing "workspace_unavailable" inside the worker, which is what
// happened before this check existed).
//
//   - pentest_only resolves to exactly [pentest], 422 if no target is
//     attested yet — documentation/07-api-specification.md §5's FR-PEN-013
//     example.
//   - full_supply_chain is every registered engine whose requirement (a
//     repository, or an attested target for pentest) this project actually
//     satisfies. pentest is included here too when a target exists — a
//     project with both a repository and a target gets everything from one
//     "Run All Scans" call, not a separate pentest_only request.
//   - partial keeps the explicit list the client asked for, but now also
//     rejects (422, not a silent drop) a named engine that structurally
//     can't run here — an explicit selection should error loudly, matching
//     the existing "not registered" case's own treatment, not vanish.
func (s *service) resolveEngines(ctx context.Context, projectID uuid.UUID, detail *project.ProjectDetail, in CreateScanInput) ([]domain.EngineID, error) {
	hasRepo := detail.Repository != nil
	hasTarget, err := s.hasAttestedTarget(ctx, projectID)
	if err != nil {
		return nil, err
	}
	runnable := func(e domain.EngineID) bool {
		if e == domain.EnginePentest {
			return hasTarget
		}
		if engineRequiresRepository(e) {
			return hasRepo
		}
		return true // docreview
	}

	switch in.Type {
	case domain.ScanTypePentestOnly:
		if !s.registry.Has(domain.EnginePentest) {
			return nil, apperrors.Unprocessable("scan.engine_unavailable", "pentest engine is not registered")
		}
		if !hasTarget {
			return nil, apperrors.Unprocessable("scan.no_pentest_target", "project has no attested pentest target")
		}
		return []domain.EngineID{domain.EnginePentest}, nil

	case domain.ScanTypePartial:
		if len(in.Engines) == 0 {
			return nil, apperrors.Validation("scan.invalid_input", "engines is required for a partial scan", nil)
		}
		for _, e := range in.Engines {
			if !s.registry.Has(e) {
				return nil, apperrors.Unprocessable("scan.engine_unavailable", fmt.Sprintf("engine %q is not registered", e))
			}
			if !runnable(e) {
				return nil, apperrors.Unprocessable("scan.engine_unavailable", fmt.Sprintf("engine %q cannot run: %s", e, unavailableReason(e, hasRepo, hasTarget)))
			}
		}
		return in.Engines, nil

	default: // ScanTypeFullSupplyChain
		var out []domain.EngineID
		for _, e := range s.registry.IDs() {
			if runnable(e) {
				out = append(out, e)
			}
		}
		return out, nil
	}
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

	risk, err := s.getRiskAssessment(ctx, scanID)
	if err != nil {
		return nil, err
	}

	return &ScanDetail{Scan: *scan, Jobs: details, Risk: risk}, nil
}

// getRiskAssessment loads scanID's RiskAssessmentRecord, if one exists, and
// fills in Delta (Score - PreviousScore) — computed here rather than stored,
// since it's derivable and storing it would just be another place it could
// drift from the two numbers it's computed from. nil, nil (not an error)
// whenever no score has been computed yet — a queued/running scan, or a
// riskAssessments dependency this particular caller didn't wire (nil-safe,
// same convention Pool.finalizeScoring's own three-field nil-check uses).
func (s *service) getRiskAssessment(ctx context.Context, scanID uuid.UUID) (*RiskAssessmentRecord, error) {
	if s.riskAssessments == nil {
		return nil, nil
	}
	rec, err := s.riskAssessments.GetByScanID(ctx, scanID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get risk assessment: %w", err))
	}
	if rec == nil {
		return nil, nil
	}
	if rec.PreviousScore != nil {
		delta := rec.Score - *rec.PreviousScore
		rec.Delta = &delta
	}
	return rec, nil
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
	pctSum := 0
	for i, j := range jobs {
		count, err := s.findings.CountByJob(ctx, j.ID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("count job findings: %w", err))
		}
		pct := 0
		var activity string
		switch j.Status {
		case domain.JobStatusSucceeded, domain.JobStatusFailed, domain.JobStatusSkipped, domain.JobStatusCancelled:
			pct = 100
		case domain.JobStatusRunning:
			pct, activity = s.runningJobProgress(j)
		}
		pctSum += pct
		engines[i] = EngineProgress{Engine: j.Engine, Status: j.Status, ProgressPct: pct, Activity: activity, FindingCount: count}
	}

	// The mean of every job's own real pct — not "how many jobs are fully
	// done," which is a coarse, discontinuous signal at low job counts: a
	// pentest_only scan has exactly one job, so that measure sits frozen at
	// 0 for the scan's entire running time and then jumps straight to 100.
	// Averaging the same real per-job values engines[] already carries
	// (each one live-reported or elapsed-time-estimated, see
	// runningJobProgress) gives a genuinely moving overall number instead,
	// while still only reaching 100 once every job actually has.
	overallPct := 0
	if len(jobs) > 0 {
		overallPct = pctSum / len(jobs)
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

// runningJobProgress answers "what's actually happening right now" for one
// running job: an engine-reported live stage (pentest's real phases) if one
// exists, otherwise a real elapsed-time-against-timeout estimate — never a
// frozen constant. The activity string is empty in the fallback case; the
// frontend already has a sensible generic per-engine label for that
// (lib/engines.ts's meta.activity) and doesn't need this to invent one.
func (s *service) runningJobProgress(j domain.ScanJob) (pct int, activity string) {
	if s.progress != nil {
		if live, ok := s.progress.Get(j.ID); ok {
			return live.Pct, live.Activity
		}
	}
	if j.StartedAt == nil {
		return 5, "" // claimed but no StartedAt yet (shouldn't happen — MarkRunning sets both together) — a small non-zero value beats a misleading 0
	}
	timeout := s.engineTimeouts[j.Engine]
	if timeout <= 0 {
		timeout = s.defaultTimeout
	}
	return elapsedFallbackPct(*j.StartedAt, timeout), ""
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
