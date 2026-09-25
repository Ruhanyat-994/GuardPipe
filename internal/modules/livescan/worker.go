package livescan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// trigger is one decided-on scan, waiting out its debounce window (push) or
// firing straight away (pull request).
type trigger struct {
	WebhookID  uuid.UUID            `json:"webhook_id"`
	Source     domain.TriggerSource `json:"source"`
	Branch     string               `json:"branch"`
	Ref        string               `json:"ref"`
	Actor      string               `json:"actor"`
	CommitSHA  string               `json:"commit_sha"`
	DeliveryID string               `json:"delivery_id"`
}

// prActionsThatScan are the pull_request actions that mean "there's new
// code to look at".
var prActionsThatScan = []string{"opened", "synchronize", "reopened"}

func (s *service) HandleQueuedEvent(ctx context.Context, raw []byte) error {
	var ev queuedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("livescan: decode queued event: %w", err)
	}
	w, ok := s.activeWebhook(ctx, ev.WebhookID)
	if !ok {
		return nil
	}

	// The hook is registered on one repository, but the project's repository
	// can be replaced afterwards. Events from anything but the project's
	// current repository are dropped rather than scanning the wrong code.
	repoURL, _, _, err := s.projects.GetCloneInfo(ctx, w.ProjectID)
	if err != nil {
		s.dropped(ctx, w, ev.DeliveryID, "no_repository")
		return nil
	}
	ref, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return fmt.Errorf("livescan: parse stored repository URL: %w", err)
	}
	fullName := ref.Owner + "/" + ref.Name

	switch ev.Event {
	case "push":
		e, err := github.ParsePushEvent(ev.Body)
		if err != nil {
			return err
		}
		if !strings.EqualFold(e.Repository.FullName, fullName) {
			s.dropped(ctx, w, ev.DeliveryID, "repository_mismatch")
			return nil
		}
		branch := e.Branch()
		if e.Deleted || branch == "" || !slices.Contains(w.WatchedBranches, branch) {
			return nil // branch deletion, a tag, or a branch nobody asked to watch
		}
		t := trigger{
			WebhookID: w.ID, Source: domain.TriggerWebhookPush, Branch: branch, Ref: branch,
			Actor: e.Sender.Login, CommitSHA: e.After, DeliveryID: ev.DeliveryID,
		}
		payload, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("livescan: encode trigger: %w", err)
		}
		// One pending scan per project+branch: pushes inside the window
		// collapse into it, and it scans whatever the branch holds when the
		// window ends.
		key := w.ProjectID.String() + ":" + branch
		return s.coord.ArmPending(ctx, key, payload, time.Now().UTC().Add(s.cfg.Debounce))

	case "pull_request":
		e, err := github.ParsePullRequestEvent(ev.Body)
		if err != nil {
			return err
		}
		if !slices.Contains(prActionsThatScan, e.Action) {
			return nil
		}
		if !strings.EqualFold(e.Repository.FullName, fullName) {
			s.dropped(ctx, w, ev.DeliveryID, "repository_mismatch")
			return nil
		}
		if !slices.Contains(w.WatchedBranches, e.PullRequest.Base.Ref) {
			return nil // a PR into a branch nobody asked to watch
		}
		// A fork's branch can't be cloned from this repository, and it's
		// someone outside the repository spending this org's scan quota.
		if e.FromFork() {
			s.dropped(ctx, w, ev.DeliveryID, "fork_pull_request")
			return nil
		}
		// No debounce: someone iterating on a PR wants each push's result.
		return s.fire(ctx, w, trigger{
			WebhookID: w.ID, Source: domain.TriggerWebhookPullRequest, Branch: e.PullRequest.Head.Ref,
			Ref: fmt.Sprintf("refs/pull/%d/head", e.Number), Actor: e.Sender.Login,
			CommitSHA: e.PullRequest.Head.SHA, DeliveryID: ev.DeliveryID,
		})
	}
	return nil
}

func (s *service) FireDue(ctx context.Context, now time.Time) error {
	payloads, err := s.coord.PopDuePending(ctx, now)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	for _, raw := range payloads {
		var t trigger
		if err := json.Unmarshal(raw, &t); err != nil {
			errs = append(errs, fmt.Errorf("livescan: decode pending trigger: %w", err))
			continue
		}
		// Re-read the webhook: it may have been turned off or paused while
		// the push was waiting out its window.
		w, ok := s.activeWebhook(ctx, t.WebhookID)
		if !ok {
			continue
		}
		if err := s.fire(ctx, w, t); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// fire applies the rate limit and creates the scan.
func (s *service) fire(ctx context.Context, w *Webhook, t trigger) error {
	orgID, err := s.projects.GetOrgID(ctx, w.ProjectID)
	if err != nil {
		return fmt.Errorf("livescan: resolve org: %w", err)
	}
	now := time.Now().UTC()

	engines := safeEngines(w.Engines)
	if len(engines) == 0 {
		s.dropped(ctx, w, t.DeliveryID, "no_allowed_engines")
		return nil
	}

	limit := int64(s.cfg.MaxScansPerProjectPerHour)
	if limit > 0 {
		n, err := s.coord.IncrWindowCount(ctx, "project:"+w.ProjectID.String(), time.Hour)
		if err != nil {
			return fmt.Errorf("livescan: count triggers: %w", err)
		}
		if n > limit {
			// Dropped quietly: GitHub already got its 202 long ago, and an
			// error back to GitHub would only push it toward disabling the
			// hook. The audit entry is what explains it later.
			s.recordDelivery(ctx, w.ID, now, "rate_limited")
			s.auditForProject(ctx, w, w.EnabledBy, "webhook.rate_limited", map[string]any{
				"delivery_id": t.DeliveryID, "branch": t.Branch, "limit_per_hour": limit,
			})
			if n >= breakerFactor*limit {
				reason := fmt.Sprintf("paused automatically: %d scan triggers within an hour (limit %d)", n, limit)
				if err := s.repo.Pause(ctx, w.ID, reason, now); err != nil {
					return fmt.Errorf("livescan: pause: %w", err)
				}
				s.log.Warn("livescan: circuit breaker paused live scanning", "project_id", w.ProjectID, "triggers", n)
				s.auditForProject(ctx, w, w.EnabledBy, "webhook.paused", map[string]any{"reason": reason})
			}
			return nil
		}
	}

	// Tokens: the plan must include live scanning, and paying for this scan
	// must leave the balance at or above the project's floor. Dropped like
	// the hourly cap — never an error back to GitHub — and it resumes on
	// its own once there are tokens again.
	if s.tokens != nil {
		ok, reason, err := s.tokens.LiveScanAllowed(ctx, orgID, engines, w.MinBalancePercent)
		if err != nil {
			return fmt.Errorf("livescan: token check: %w", err)
		}
		if !ok {
			s.recordDelivery(ctx, w.ID, now, reason)
			s.auditForProject(ctx, w, w.EnabledBy, "webhook."+reason, map[string]any{
				"delivery_id": t.DeliveryID, "branch": t.Branch, "min_balance_percent": w.MinBalancePercent,
			})
			return nil
		}
	}

	// The same commit with the same engines is only scanned (and paid for)
	// once: a re-delivery, or a PR opened on an already-pushed commit.
	if t.CommitSHA != "" {
		key := fmt.Sprintf("commit:%s:%s:%s", w.ProjectID, t.CommitSHA, enginesKey(engines))
		if first, err := s.coord.MarkDeliverySeen(ctx, key, commitSeenTTL); err == nil && !first {
			s.recordDelivery(ctx, w.ID, now, "duplicate_commit")
			return nil
		}
	}

	// Attributed to whoever turned live scanning on — the same shape
	// orchestrator.TriggerSchedule uses for a schedule's creator. Their
	// confirmation when enabling is the authorisation for this scan.
	actor := domain.Actor{OrgID: orgID, Role: domain.RoleAdmin}
	if w.EnabledBy != nil {
		actor.UserID = *w.EnabledBy
	}
	detail, err := s.scans.CreateScan(ctx, actor, w.ProjectID, orchestrator.CreateScanInput{
		Type: domain.ScanTypePartial, Engines: engines, Branch: t.Branch,
		TriggerSource: t.Source, TriggerRef: t.Ref, TriggerActor: t.Actor,
	})
	if err != nil {
		code := "scan_create_failed"
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Code != "" {
			code = appErr.Code
		}
		status := "scan_failed"
		switch code {
		case "billing.insufficient_tokens":
			status = "insufficient_tokens" // lost a race with another scan
		case "billing.plan_required":
			status = "plan_required"
		}
		s.log.Error("livescan: create scan", "project_id", w.ProjectID, "branch", t.Branch, "error", err)
		s.recordDelivery(ctx, w.ID, now, status)
		s.auditForProject(ctx, w, w.EnabledBy, "webhook.delivery_failed", map[string]any{
			"delivery_id": t.DeliveryID, "branch": t.Branch, "reason": code,
		})
		return nil
	}

	s.log.Info("livescan: scan triggered", "project_id", w.ProjectID, "scan_id", detail.ID, "source", t.Source, "branch", t.Branch)
	s.recordDelivery(ctx, w.ID, now, "scan_triggered")
	s.auditForProject(ctx, w, w.EnabledBy, "webhook.scan_triggered", map[string]any{
		"scan_id": detail.ID.String(), "delivery_id": t.DeliveryID, "trigger_source": string(t.Source),
		"branch": t.Branch, "ref": t.Ref, "github_actor": t.Actor, "commit_sha": t.CommitSHA,
	})
	return nil
}

// activeWebhook loads a webhook that exists and isn't paused.
func (s *service) activeWebhook(ctx context.Context, webhookID uuid.UUID) (*Webhook, bool) {
	w, err := s.repo.GetByID(ctx, webhookID)
	if err != nil {
		if !isNotFound(err) {
			s.log.Error("livescan: load webhook", "webhook_id", webhookID, "error", err)
		}
		return nil, false // turned off since the event arrived
	}
	if w.PausedReason != nil {
		return nil, false
	}
	return w, true
}

// dropped records an event that was deliberately not scanned.
func (s *service) dropped(ctx context.Context, w *Webhook, deliveryID, reason string) {
	s.recordDelivery(ctx, w.ID, time.Now().UTC(), "skipped_"+reason)
	s.auditForProject(ctx, w, w.EnabledBy, "webhook.skipped", map[string]any{"delivery_id": deliveryID, "reason": reason})
}

// Worker runs the live-scanning event loop under GUARDPIPE_ROLE=worker/all,
// next to orchestrator.Pool and orchestrator.Scheduler.
type Worker struct {
	Service     Service
	Coordinator Coordinator
	Log         *slog.Logger
	// PollInterval bounds both how long one wait for an event blocks and
	// how late a debounced push can fire. Defaults to one second.
	PollInterval time.Duration
}

// Start blocks until ctx is cancelled.
func (wk *Worker) Start(ctx context.Context) {
	interval := wk.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	log := wk.Log
	if log == nil {
		log = slog.Default()
	}
	for ctx.Err() == nil {
		raw, err := wk.Coordinator.PopEvent(ctx, interval)
		if err != nil && ctx.Err() == nil {
			log.Error("livescan: pop event", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval): // Redis down — don't spin
			}
		}
		if raw != nil {
			if err := wk.Service.HandleQueuedEvent(ctx, raw); err != nil {
				log.Error("livescan: handle event", "error", err)
			}
		}
		if err := wk.Service.FireDue(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			log.Error("livescan: fire due scans", "error", err)
		}
	}
}

// enginesKey is a stable key for an engine set, for commit de-duplication.
func enginesKey(engines []domain.EngineID) string {
	parts := make([]string, len(engines))
	for i, e := range engines {
		parts[i] = string(e)
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}
