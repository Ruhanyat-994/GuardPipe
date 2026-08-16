package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
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
	Cloner         Cloner
	WorkspaceRoot  string
	EngineTimeouts map[domain.EngineID]time.Duration
	DefaultTimeout time.Duration
	Log            *slog.Logger
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
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: "cancelled"})
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

	workspaceDir, cleanup, err := p.prepareWorkspace(ctx, scan.ProjectID)
	if err != nil {
		reason := "workspace_unavailable"
		if errors.Is(err, errCredentialInvalid) {
			reason = "credential_invalid"
		}
		p.Log.Error("orchestrator: workspace preparation failed", "job_id", jobID, "error", err)
		p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusFailed, ErrorReason: reason})
		return
	}
	defer cleanup()

	scanInput := domain.ScanInput{ScanID: scan.ID, JobID: jobID, ProjectID: scan.ProjectID, WorkspaceDir: workspaceDir}

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
		if runCtx.Err() != nil {
			reason = "timeout"
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

	stats := map[string]any{"rules_evaluated": result.RulesEvaluated, "files_scanned": result.FilesScanned}
	p.persist(ctx, JobResult{JobID: jobID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: domain.JobStatusSucceeded, Stats: stats, Findings: findings})
}

func (p *Pool) persist(ctx context.Context, result JobResult) {
	if err := p.JobResults.PersistJobResult(ctx, result); err != nil {
		p.Log.Error("orchestrator: persist job result failed", "job_id", result.JobID, "status", result.Status, "error", err)
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
	if err := makeWorldReadable(dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("relax workspace permissions: %w", err)
	}
	return dir, cleanup, nil
}

func makeWorldReadable(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}
