package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// Cloner is the subset of modules/vcs.Service the worker needs — defined
// here (the consumer) so tests substitute a fake instead of a real clone.
type Cloner interface {
	ShallowClone(ctx context.Context, rawURL, token, destDir string) error
}

// CloneInfoProvider is the subset of modules/project.Service the worker
// needs. Separate from ProjectAccess (service.go's ownership-check
// dependency) because this is a different capability, used from a
// different code path (background processing, not a per-request check).
type CloneInfoProvider interface {
	GetCloneInfo(ctx context.Context, projectID uuid.UUID) (repoURL, branch, token string, err error)
	// MarkCredentialInvalid is called the moment a clone comes back 401/403
	// with the stored credential — see prepareWorkspace below.
	MarkCredentialInvalid(ctx context.Context, projectID uuid.UUID, reason string) error
}

// DocumentProvider is the subset of modules/project.Service the docreview
// engine's job needs — same no-actor reasoning as CloneInfoProvider (the
// worker isn't handling a per-request authorization check, it's processing
// a scan job whose creation was already authorized). project.Document
// itself carries file bytes and DB bookkeeping docreview has no use for;
// DocumentRef (domain/engine.go) is the trimmed shape an Engine actually
// receives.
type DocumentProvider interface {
	GetDocuments(ctx context.Context, projectID uuid.UUID) ([]project.Document, error)
}

// TargetProvider is the subset of modules/project.Service the worker needs
// to run a pentest job — same no-actor, background-job shape as
// CloneInfoProvider/DocumentProvider above.
type TargetProvider interface {
	GetAttestedTarget(ctx context.Context, projectID uuid.UUID) (*project.Target, error)
}

// pinnedIPStrings converts project.Target's netip.Addr slice (pgx/v5's own
// mapping for Postgres inet[], documentation/06-database-design.md §4.7)
// into the plain []string domain.PentestTarget carries — domain stays free
// of a pgx-shaped type, and engines/pentest compares these against
// validate.ResolveTarget's net.IP-derived strings at execution time.
func pinnedIPStrings(ips []netip.Addr) []string {
	out := make([]string, len(ips))
	for i, ip := range ips {
		out[i] = ip.String()
	}
	return out
}

// errCredentialInvalid is what prepareWorkspace wraps its returned error
// with when a clone failed specifically because the stored credential was
// rejected (as opposed to a network blip, an oversized repo, or any other
// clone failure) — processJob uses this to persist the specific
// "credential_invalid" job reason instead of the generic
// "workspace_unavailable", so a PartialResultBanner (and, from now on, the
// project itself) can tell the user exactly what to fix. By the time this
// is returned, CloneInfoProvider.MarkCredentialInvalid has already been
// called — the project's repository row carries the same signal even
// between scans, not just as one job's failure reason.
var errCredentialInvalid = errors.New("orchestrator: repository credential invalid")

// isNoRepositoryAttached reports whether err is prepareWorkspace failing
// specifically because the project has no repository attached at all —
// project.Service.GetCloneInfo's own "project.repository_not_found", not any
// other clone failure (bad credential, oversized repo, network error). Kept
// narrow on purpose: only docreview treats this as "nothing to add from a
// repo checkout" (processJob, above); every other engine genuinely needs a
// repository, so this must not turn every "no repo attached" into a silent
// pass for them too.
func isNoRepositoryAttached(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Code == "project.repository_not_found"
}

// claimPollTimeout is how long each worker's Claim call blocks waiting for
// a job before looping back to check ctx.Done() — bounds shutdown latency
// without busy-polling.
const claimPollTimeout = 5 * time.Second

// Pool is the worker pool documentation/04-backend-architecture.md §6.2
// describes: a fixed number of goroutines claiming jobs from the Redis
// queue, running the matching registered engine, and persisting whatever
// it emits in one transaction per job (JobResults, below).
type Pool struct {
	Size           int
	Queue          *jobQueueClaimer
	Registry       *Registry
	Scans          ScanRepository
	Jobs           ScanJobRepository
	JobResults     JobResultRepository
	Projects       CloneInfoProvider
	Documents      DocumentProvider
	Targets        TargetProvider
	Cloner         Cloner
	WorkspaceRoot  string
	EngineTimeouts map[domain.EngineID]time.Duration
	DefaultTimeout time.Duration
	// Progress is the live store an engine's ScanInput.ReportProgress
	// writes to (nil is fine — a nil store just means ReportProgress calls
	// are silently dropped, same as never calling it). Service.GetProgress
	// reads the same instance; wire the identical pointer into both at
	// construction time (main.go).
	Progress *LiveProgress
	Log      *slog.Logger

	workspaces workspaceCache
}

// jobQueueClaimer is the subset of adapters/queue.JobQueue the worker
// needs — narrower than Enqueuer (service.go's dependency), since claiming
// and acking are worker-only operations.
type jobQueueClaimer struct {
	Claim func(ctx context.Context, timeout time.Duration) (string, error)
	Ack   func(ctx context.Context, jobID string) error
}

// NewJobQueueClaimer adapts a *queue.JobQueue (or any type with matching
// methods) into the narrow shape Pool needs — kept as a plain struct of
// funcs rather than an interface so a test can construct one from two
// closures without a whole fake type.
func NewJobQueueClaimer(claim func(context.Context, time.Duration) (string, error), ack func(context.Context, string) error) *jobQueueClaimer {
	return &jobQueueClaimer{Claim: claim, Ack: ack}
}

// Start spawns Size worker goroutines and blocks until ctx is cancelled,
// then waits for in-flight jobs to reach a safe stopping point (each
// worker only checks ctx.Done() between jobs, matching the claim-poll
// timeout above as the shutdown latency bound).
func (p *Pool) Start(ctx context.Context) {
	done := make(chan struct{}, p.Size)
	for i := 0; i < p.Size; i++ {
		go func(workerID int) {
			defer func() { done <- struct{}{} }()
			p.runWorker(ctx, workerID)
		}(i)
	}
	for i := 0; i < p.Size; i++ {
		<-done
	}
}

func (p *Pool) runWorker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		jobID, err := p.Queue.Claim(ctx, claimPollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.Log.Error("orchestrator: claim failed", "worker", workerID, "error", err)
			continue
		}
		if jobID == "" {
			continue // timed out waiting — loop back and check ctx.Done()
		}

		p.processJob(ctx, jobID)
		if err := p.Queue.Ack(ctx, jobID); err != nil {
			p.Log.Error("orchestrator: ack failed", "job_id", jobID, "error", err)
		}
	}
}

// processJob is the per-job lifecycle diagram in
// documentation/04-backend-architecture.md §6.2: claim (done by the
// caller) -> timeout context -> Applicable? -> run with panic recovery ->
// persist (one transaction: findings + job status + scan finalisation).
func (p *Pool) processJob(ctx context.Context, jobIDStr string) {
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		p.Log.Error("orchestrator: job ID is not a UUID", "job_id", jobIDStr, "error", err)
		return
	}

	job, err := p.Jobs.GetByID(ctx, jobID)
	if err != nil {
		p.Log.Error("orchestrator: load job failed", "job_id", jobID, "error", err)
		return
	}

	scan, err := p.Scans.GetByID(ctx, job.ScanID)
	if err != nil {
		p.Log.Error("orchestrator: load scan failed", "scan_id", job.ScanID, "error", err)
		return
	}
	if scan.CancelRequested {
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusCancelled})
		return
	}

	engine, ok := p.Registry.Get(job.Engine)
	if !ok {
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: "engine_not_registered"})
		return
	}

	if err := p.Jobs.MarkRunning(ctx, jobID); err != nil {
		p.Log.Error("orchestrator: mark running failed", "job_id", jobID, "error", err)
		return
	}
	// Idempotent — a no-op for every job after the first one claimed for
	// this scan (MarkStarted's own WHERE guard). Best-effort: a failure here
	// shouldn't abort a job that's otherwise ready to run, just leave
	// started_at unset for this attempt.
	if err := p.Scans.MarkStarted(ctx, scan.ID); err != nil {
		p.Log.Error("orchestrator: mark scan started failed", "scan_id", scan.ID, "error", err)
	}

	var scanInput domain.ScanInput
	var releaseWorkspace func()

	if job.Engine == domain.EnginePentest {
		// pentest branches directly off scan start — it never waits on the
		// repository clone every other engine shares
		// (documentation/09-ui-ux-design-system.md §4.8; BUILD_GUIDE.md
		// Phase 12), so it skips workspace acquisition entirely rather than
		// going through prepareWorkspace/workspaces.acquire below.
		scanInput = domain.ScanInput{ScanID: scan.ID, JobID: jobID, ProjectID: scan.ProjectID}
		releaseWorkspace = func() {}

		if p.Targets == nil {
			p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: "no_pentest_target_attached"})
			return
		}
		target, err := p.Targets.GetAttestedTarget(ctx, scan.ProjectID)
		if err != nil {
			p.Log.Error("orchestrator: no attested pentest target", "job_id", jobID, "project_id", scan.ProjectID, "error", err)
			p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: "no_pentest_target_attached"})
			return
		}
		scanInput.Target = &domain.PentestTarget{Host: target.NormalizedHost, IPs: pinnedIPStrings(target.PinnedIPs)}

		cfg := scan.PentestConfig
		if cfg == nil {
			// Literal default-to-Stealth (BUILD_GUIDE.md Phase 12) — a scan
			// row persisted with no pentest_config still runs quiet, not
			// unconfigured.
			def := domain.DefaultPentestScanConfig()
			cfg = &def
		}
		scanInput.Options = map[string]any{"pentest_config": *cfg}
	} else {
		// acquire, not prepareWorkspace directly: every job for the same scan
		// shares one clone (documentation/05-module-specifications.md §5's
		// pipeline diagram draws one "workspace prep" node feeding every
		// engine) — the first job for this scan.ID actually clones, every
		// other job concurrently or later in the same scan reuses that same
		// directory instead of re-cloning the repository from scratch.
		workspaceDir, release, err := p.workspaces.acquire(ctx, scan.ID, func() (string, error) {
			dir, _, prepErr := p.prepareWorkspace(ctx, scan.ProjectID)
			return dir, prepErr
		})
		if err != nil {
			// docreview is the one engine that can run from uploaded documents
			// alone (documentation/05-module-specifications.md §11) — a project
			// with no repository attached at all isn't a failure for that job
			// specifically, even though the shared workspace acquisition still
			// reports it as one for whichever engine in this scan actually needs
			// a checkout. Every other engine keeps failing exactly as before.
			if job.Engine == domain.EngineDocReview && isNoRepositoryAttached(err) {
				workspaceDir = ""
				release = func() {}
			} else {
				reason := "workspace_unavailable"
				if errors.Is(err, errCredentialInvalid) {
					reason = "credential_invalid"
				}
				p.Log.Error("orchestrator: workspace preparation failed", "job_id", jobID, "error", err)
				p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: reason})
				return
			}
		}
		releaseWorkspace = release
		scanInput = domain.ScanInput{ScanID: scan.ID, JobID: jobID, ProjectID: scan.ProjectID, WorkspaceDir: workspaceDir}

		// Uploaded documents are project-scoped data, not part of the git
		// checkout prepareWorkspace already cloned — only docreview needs
		// them, so this read is skipped for every other engine's jobs.
		if job.Engine == domain.EngineDocReview && p.Documents != nil {
			docs, err := p.Documents.GetDocuments(ctx, scan.ProjectID)
			if err != nil {
				p.Log.Error("orchestrator: load documents failed", "job_id", jobID, "project_id", scan.ProjectID, "error", err)
			} else {
				scanInput.Documents = make([]domain.DocumentRef, len(docs))
				for i, d := range docs {
					scanInput.Documents[i] = domain.DocumentRef{Path: d.Filename, Content: string(d.Content)}
				}
			}
		}
	}
	defer releaseWorkspace()

	// Always set here (unlike a hand-built ScanInput literal in a test) so
	// every engine actually running through the real worker can call it.
	// Engines that never call it simply never populate p.Progress for this
	// job, and Service.GetProgress's own fallback (an elapsed-time estimate)
	// covers that case. A nil p.Progress (not wired, e.g. some tests) means
	// every call here is silently dropped.
	scanInput.ReportProgress = func(pct int, activity string) {
		if p.Progress != nil {
			p.Progress.Set(jobID, pct, activity)
		}
	}

	applicable, reason := engine.Applicable(ctx, scanInput)
	if !applicable {
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusSkipped, SkipReason: reason})
		return
	}

	timeout := p.EngineTimeouts[job.Engine]
	if timeout <= 0 {
		timeout = p.DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var findings []domain.Finding
	result, runErr := p.runEngine(runCtx, engine, scanInput, func(f domain.Finding) {
		f.ScanID = scan.ID
		findings = append(findings, f)
	})

	if runErr != nil {
		reason := "engine_error"
		switch {
		case runCtx.Err() != nil:
			reason = "timeout"
		case errors.Is(runErr, domain.ErrPentestDNSRebindingSuspected):
			reason = "dns_rebinding_suspected"
		}
		// The underlying error only ever reaches this log line — JobResult's
		// ErrorReason is a short machine code, not free text, so without this
		// there is no way to tell *why* an engine failed short of
		// reproducing the run by hand.
		p.Log.Error("orchestrator: engine run failed", "job_id", jobID, "engine", job.Engine, "reason", reason, "error", runErr)
		// Findings collected before the failure still get persisted — a
		// panic or timeout partway through a run shouldn't discard
		// everything already emitted.
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: reason, Findings: findings})
		return
	}

	// result.Stats carries whatever engine-specific detail the engine chose to
	// report (pentest's coverage summary, an engine's tool-version metadata,
	// etc.) — merged under the two universal counters rather than discarded,
	// so a clean run still has something real to show beyond "0 findings"
	// (previously dropped here entirely; see BUILD_GUIDE.md's pentest-report
	// coverage note for why that was a real gap, not just cosmetic).
	stats := map[string]any{"rules_evaluated": result.RulesEvaluated, "files_scanned": result.FilesScanned}
	maps.Copy(stats, result.Stats)
	p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusSucceeded, Stats: stats, Findings: findings})
}

func (p *Pool) persist(ctx context.Context, result JobResult) {
	if err := p.JobResults.PersistJobResult(ctx, result); err != nil {
		p.Log.Error("orchestrator: persist job result failed", "job_id", result.JobID, "status", result.Status, "error", err)
	}
	// Every call here is a job reaching a terminal status (JobResult has no
	// other caller) — clear its live-progress entry so a finished job's
	// last-reported "87%, fuzzing…" can never leak into a later read
	// (GetProgress already reports 100% for any terminal job status
	// independent of this map).
	if p.Progress != nil {
		p.Progress.Clear(result.JobID)
	}
}

// runEngine calls engine.Run behind a panic recover — a panicking engine
// fails only its own job, never the worker process
// (documentation/04-backend-architecture.md §6.4, NFR-REL-001).
func (p *Pool) runEngine(ctx context.Context, engine domain.Engine, in domain.ScanInput, emit func(domain.Finding)) (result domain.EngineResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			p.Log.Error("orchestrator: engine panicked", "engine", engine.ID(), "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("engine %s panicked: %v", engine.ID(), r)
		}
	}()
	return engine.Run(ctx, in, emit)
}

// prepareWorkspace clones one repository into a fresh temp directory. Its
// cleanup return value only ever removes a partially-written directory on a
// failure path within this function — on success, the caller (processJob,
// via workspaceCache.acquire) owns removal instead, since a workspace this
// function creates may now be shared by every job in a scan, not just the
// one that happened to trigger the clone.
func (p *Pool) prepareWorkspace(ctx context.Context, projectID uuid.UUID) (dir string, cleanup func(), err error) {
	repoURL, _, token, err := p.Projects.GetCloneInfo(ctx, projectID)
	if err != nil {
		return "", nil, fmt.Errorf("get clone info: %w", err)
	}

	if err := os.MkdirAll(p.WorkspaceRoot, 0o755); err != nil {
		return "", nil, fmt.Errorf("prepare workspace root: %w", err)
	}
	dir, err = os.MkdirTemp(p.WorkspaceRoot, "scan-*")
	if err != nil {
		return "", nil, fmt.Errorf("create workspace dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	if err := p.Cloner.ShallowClone(ctx, repoURL, token, dir); err != nil {
		cleanup()
		// A stored credential being rejected only means "this credential is
		// bad" if one was actually supplied — a 401/403 on a public repo
		// with no credential attached at all is "no credential was ever
		// configured," a different (and already-handled, at attach time)
		// state, not a credential that went bad after working before.
		if token != "" && errors.Is(err, github.ErrCloneUnauthorized) {
			if markErr := p.Projects.MarkCredentialInvalid(ctx, projectID, "github_credential_rejected"); markErr != nil {
				p.Log.Error("orchestrator: mark credential invalid failed", "project_id", projectID, "error", markErr)
			}
			return "", nil, fmt.Errorf("%w: %v", errCredentialInvalid, err)
		}
		return "", nil, fmt.Errorf("clone repository: %w", err)
	}

	// os.MkdirTemp creates dir at mode 0700 regardless of umask, and go-git's
	// own file/directory modes underneath aren't guaranteed world-readable
	// either — fine for every in-process engine (they read this tree as the
	// same uid that created it), but codescan's sonar-scanner runs as a
	// *different* uid in a sibling container (adapters/sonarqube/scanner.go
	// mounts this same directory read-only there) and gets
	// java.nio.file.AccessDeniedException walking a tree it can't read.
	// Files never need to be executed (engines only ever read scanned
	// content, never run it — see domain.Engine's own doc comment), so
	// flattening every file to 0644 and every directory to 0755 is always
	// safe here, not just for this specific clone.
	if err := hardenWorkspace(dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("harden workspace: %w", err)
	}
	return dir, cleanup, nil
}

// hardenWorkspace walks a freshly-cloned workspace once, both relaxing
// permissions (see prepareWorkspace's call site) and finally enforcing
// documentation/05-module-specifications.md §5's "Symlinks that escape the
// workspace root are removed before engines run (path-traversal defence)"
// rule — previously documented but never actually implemented, which a real
// clone surfaced two ways: os.Chmod follows symlinks (unlike WalkDir's own
// traversal, which does not), so a symlink cycle inside the cloned
// repository — e.g. github.com/madhuakula/kubernetes-goat's
// scenarios/metadata-db content — made a plain chmod pass fail outright
// with ELOOP ("too many levels of symbolic links") on every job for that
// scan, and even a well-behaved symlink pointing outside root was never
// being removed at all.
func hardenWorkspace(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return sanitizeSymlink(root, path)
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}

// sanitizeSymlink removes path if it cannot be safely resolved (a dangling
// target or a symlink cycle — either way, nothing can read it safely
// either, so an engine should never see it) or if it resolves to somewhere
// outside root. A symlink that resolves cleanly inside root is left alone
// and, deliberately, not chmod'd: on every platform this matters for, a
// symlink's own mode bits are ignored — what governs whether its target is
// readable is the target's own mode, and the target gets its turn in this
// same walk.
func sanitizeSymlink(root, path string) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return os.Remove(path)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return os.Remove(path)
	}
	return nil
}
