package livescan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// hookEvents is what the GitHub hook subscribes to.
var hookEvents = []string{"push", "pull_request"}

// Service is live scanning's whole surface: the three settings endpoints,
// the webhook receiver, and the worker's two entry points.
type Service interface {
	// Get returns the project's live-scanning settings, or nil when it's off.
	Get(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*Webhook, error)
	// Enable turns live scanning on, or changes its settings (and resumes it
	// if paused) when it's already on.
	Enable(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in EnableInput) (*Webhook, error)
	Disable(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*DisableResult, error)

	// Receive verifies and queues one webhook delivery. It never creates a
	// scan itself: GitHub gives up on a delivery after ~10s and disables
	// hooks that keep failing, so the HTTP handler must answer fast.
	Receive(ctx context.Context, d Delivery) error

	// HandleQueuedEvent and FireDue are the worker's entry points (worker.go).
	HandleQueuedEvent(ctx context.Context, raw []byte) error
	FireDue(ctx context.Context, now time.Time) error
}

type service struct {
	cfg      Config
	repo     Repository
	hooks    HookClient
	projects ProjectAccess
	scans    ScanCreator
	coord    Coordinator
	audit    audit.Service
	log      *slog.Logger
	tokens   TokenGate
}

// SetTokenGate turns on token billing for svc (built by NewService).
func SetTokenGate(svc Service, t TokenGate) {
	if s, ok := svc.(*service); ok {
		s.tokens = t
	}
}

func NewService(cfg Config, repo Repository, hooks HookClient, projects ProjectAccess, scans ScanCreator, coord Coordinator, auditSvc audit.Service, log *slog.Logger) Service {
	if log == nil {
		log = slog.Default()
	}
	return &service{cfg: cfg, repo: repo, hooks: hooks, projects: projects, scans: scans, coord: coord, audit: auditSvc, log: log}
}

func (s *service) Get(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*Webhook, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, err // already 404-not-403 cross-org
	}
	w, err := s.repo.GetByProjectID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, apperrors.Internal(fmt.Errorf("get live scanning: %w", err))
	}
	return w, nil
}

func (s *service) Enable(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in EnableInput) (*Webhook, error) {
	detail, err := s.projects.Get(ctx, actor, projectID)
	if err != nil {
		return nil, err
	}
	if !in.Confirmed {
		return nil, apperrors.Validation("livescan.confirmation_required", "you must confirm you authorise automatic scanning of this repository", nil)
	}
	if s.tokens != nil {
		if err := s.tokens.RequireFeature(ctx, detail.OrgID, "live_scanning"); err != nil {
			return nil, err
		}
	}
	if in.MinBalancePercent != nil && (*in.MinBalancePercent < 0 || *in.MinBalancePercent > 90) {
		return nil, apperrors.Validation("livescan.invalid_input", "min_balance_percent must be between 0 and 90", nil)
	}
	engines, err := validateEngines(in.Engines)
	if err != nil {
		return nil, err
	}
	if detail.Repository == nil {
		return nil, apperrors.Unprocessable("livescan.no_repository", "attach a GitHub repository to this project before turning on live scanning")
	}
	repoURL, defaultBranch, token, err := s.projects.GetCloneInfo(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, apperrors.Unprocessable("livescan.credential_required", "live scanning needs a GitHub token attached to this project that can manage webhooks (admin:repo_hook scope, or Webhooks: read & write)")
	}
	branches, err := normalizeBranches(in.WatchedBranches, defaultBranch)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	enabledBy := actor.UserID

	existing, err := s.repo.GetByProjectID(ctx, projectID)
	if err != nil && !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("get live scanning: %w", err))
	}
	if existing != nil {
		existing.Engines, existing.WatchedBranches = engines, branches
		existing.EnabledBy, existing.AttestedAt = &enabledBy, now
		existing.PausedReason, existing.PausedAt = nil, nil
		if in.MinBalancePercent != nil {
			existing.MinBalancePercent = *in.MinBalancePercent
		}
		if err := s.repo.UpdateConfig(ctx, existing); err != nil {
			return nil, apperrors.Internal(fmt.Errorf("update live scanning: %w", err))
		}
		s.log.Info("livescan: settings updated", "project_id", projectID)
		s.logAudit(ctx, actor.OrgID, &actor.UserID, "webhook.updated", existing.ID, map[string]any{
			"project_id": projectID.String(), "engines": engines, "watched_branches": branches,
		})
		return existing, nil
	}

	ref, err := github.ParseRepoURL(repoURL)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("parse stored repository URL: %w", err))
	}
	secret, err := newSecret()
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	ciphertext, nonce, err := crypto.Encrypt(s.cfg.EncryptionKey, []byte(secret))
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("encrypt webhook secret: %w", err))
	}

	w := &Webhook{
		ID: id.New(), ProjectID: projectID, SecretCiphertext: ciphertext, SecretNonce: nonce,
		Engines: engines, WatchedBranches: branches, EnabledBy: &enabledBy, AttestedAt: now,
		MinBalancePercent: defaultMinBalancePercent,
	}
	if in.MinBalancePercent != nil {
		w.MinBalancePercent = *in.MinBalancePercent
	}
	hookID, err := s.hooks.CreateHook(ctx, ref.Owner, ref.Name, token, s.deliveryURL(w.ID), secret, hookEvents)
	if err != nil {
		return nil, err
	}
	w.GitHubHookID = hookID

	if err := s.repo.Create(ctx, w); err != nil {
		// Don't leave a hook on GitHub that nothing on our side knows about.
		if delErr := s.hooks.DeleteHook(ctx, ref.Owner, ref.Name, token, hookID); delErr != nil {
			s.log.Error("livescan: remove hook after failed save", "project_id", projectID, "hook_id", hookID, "error", delErr)
		}
		return nil, apperrors.Internal(fmt.Errorf("save live scanning: %w", err))
	}
	s.log.Info("livescan: enabled", "project_id", projectID, "hook_id", hookID)
	s.logAudit(ctx, actor.OrgID, &actor.UserID, "webhook.enabled", w.ID, map[string]any{
		"project_id": projectID.String(), "engines": engines, "watched_branches": branches, "github_hook_id": hookID,
	})
	return w, nil
}

func (s *service) Disable(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*DisableResult, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, err
	}
	w, err := s.repo.GetByProjectID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("livescan.not_enabled", "live scanning is not turned on for this project")
		}
		return nil, apperrors.Internal(fmt.Errorf("get live scanning: %w", err))
	}

	// Remove the hook from GitHub first. Failing to do so doesn't block
	// turning live scanning off here — a user with a revoked token must still
	// be able to switch it off — but it's reported back so they know to
	// remove the hook on GitHub by hand.
	result := &DisableResult{}
	if repoURL, _, token, err := s.projects.GetCloneInfo(ctx, projectID); err == nil {
		if ref, err := github.ParseRepoURL(repoURL); err == nil {
			if err := s.hooks.DeleteHook(ctx, ref.Owner, ref.Name, token, w.GitHubHookID); err != nil {
				s.log.Warn("livescan: could not remove hook from GitHub", "project_id", projectID, "hook_id", w.GitHubHookID, "error", err)
			} else {
				result.GitHubHookRemoved = true
			}
		}
	}

	if err := s.repo.Delete(ctx, w.ID); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("delete live scanning: %w", err))
	}
	s.log.Info("livescan: disabled", "project_id", projectID, "github_hook_removed", result.GitHubHookRemoved)
	s.logAudit(ctx, actor.OrgID, &actor.UserID, "webhook.disabled", w.ID, map[string]any{
		"project_id": projectID.String(), "github_hook_removed": result.GitHubHookRemoved,
	})
	return result, nil
}

// queuedEvent is what Receive puts on the Redis queue for the worker.
type queuedEvent struct {
	WebhookID  uuid.UUID       `json:"webhook_id"`
	Event      string          `json:"event"`
	DeliveryID string          `json:"delivery_id"`
	Body       json.RawMessage `json:"body"`
}

func (s *service) Receive(ctx context.Context, d Delivery) error {
	w, err := s.repo.GetByID(ctx, d.WebhookID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("webhook.not_found", "unknown webhook")
		}
		return apperrors.Internal(fmt.Errorf("get webhook: %w", err))
	}

	secret, err := crypto.Decrypt(s.cfg.EncryptionKey, w.SecretCiphertext, w.SecretNonce)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("decrypt webhook secret: %w", err))
	}
	if !github.VerifySignature(secret, d.Body, d.Signature) {
		s.log.Warn("livescan: webhook signature invalid", "webhook_id", w.ID, "delivery_id", d.DeliveryID)
		s.auditForProject(ctx, w, nil, "webhook.signature_invalid", map[string]any{"delivery_id": d.DeliveryID, "event": d.Event})
		return apperrors.Unauthorized("webhook.signature_invalid", "webhook signature verification failed")
	}

	now := time.Now().UTC()
	switch d.Event {
	case "ping":
		s.recordDelivery(ctx, w.ID, now, "ping_ok")
		return nil
	case "push", "pull_request":
	default:
		s.recordDelivery(ctx, w.ID, now, "ignored_event")
		return nil
	}

	if d.DeliveryID != "" {
		first, err := s.coord.MarkDeliverySeen(ctx, d.DeliveryID, deliverySeenTTL)
		if err != nil {
			return apperrors.Internal(fmt.Errorf("record delivery id: %w", err))
		}
		if !first {
			return nil // GitHub retry or a manual "Redeliver" — already handled
		}
	}

	payload, err := json.Marshal(queuedEvent{WebhookID: w.ID, Event: d.Event, DeliveryID: d.DeliveryID, Body: d.Body})
	if err != nil {
		return apperrors.Internal(fmt.Errorf("encode queued event: %w", err))
	}
	if err := s.coord.PushEvent(ctx, payload); err != nil {
		return apperrors.Internal(fmt.Errorf("queue webhook event: %w", err))
	}
	s.recordDelivery(ctx, w.ID, now, "accepted")
	return nil
}

func (s *service) deliveryURL(webhookID uuid.UUID) string {
	return strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/api/v1/webhooks/github/" + webhookID.String()
}

func (s *service) recordDelivery(ctx context.Context, webhookID uuid.UUID, at time.Time, status string) {
	if err := s.repo.RecordDelivery(ctx, webhookID, at, status); err != nil {
		s.log.Error("livescan: record delivery", "webhook_id", webhookID, "status", status, "error", err)
	}
}

func (s *service) logAudit(ctx context.Context, orgID uuid.UUID, actorID *uuid.UUID, action string, webhookID uuid.UUID, detail map[string]any) {
	if s.audit == nil {
		return
	}
	resType := "project_webhook"
	s.audit.Log(ctx, audit.Entry{OrgID: &orgID, ActorID: actorID, Action: action, ResourceType: &resType, ResourceID: &webhookID, Detail: detail})
}

// auditForProject logs an entry for a webhook when the org has to be looked
// up (the receiver and worker have no authenticated actor).
func (s *service) auditForProject(ctx context.Context, w *Webhook, actorID *uuid.UUID, action string, detail map[string]any) {
	orgID, err := s.projects.GetOrgID(ctx, w.ProjectID)
	if err != nil {
		s.log.Error("livescan: resolve org for audit", "project_id", w.ProjectID, "action", action, "error", err)
		return
	}
	if detail == nil {
		detail = map[string]any{}
	}
	detail["project_id"] = w.ProjectID.String()
	s.logAudit(ctx, orgID, actorID, action, w.ID, detail)
}

// validateEngines rejects an empty list, anything unknown, and — above all —
// pentest.
func validateEngines(engines []domain.EngineID) ([]domain.EngineID, error) {
	if len(engines) == 0 {
		return nil, apperrors.Validation("livescan.invalid_input", "choose at least one scan to run automatically", nil)
	}
	out := make([]domain.EngineID, 0, len(engines))
	for _, e := range engines {
		if e == domain.EnginePentest {
			return nil, apperrors.Validation("livescan.pentest_not_allowed", "penetration tests never run automatically — start them by hand", nil)
		}
		if !slices.Contains(AllowedEngines, e) {
			return nil, apperrors.Validation("livescan.invalid_input", fmt.Sprintf("unknown scan %q", e), nil)
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out, nil
}

// safeEngines is the last-moment check before a scan is created: whatever
// is stored, pentest and anything unknown are dropped.
func safeEngines(engines []domain.EngineID) []domain.EngineID {
	out := make([]domain.EngineID, 0, len(engines))
	for _, e := range engines {
		if e != domain.EnginePentest && slices.Contains(AllowedEngines, e) {
			out = append(out, e)
		}
	}
	return out
}

// normalizeBranches trims, de-duplicates and validates branch names;
// nothing given means just the repository's default branch.
func normalizeBranches(in []string, defaultBranch string) ([]string, error) {
	var out []string
	for _, b := range in {
		b = strings.TrimSpace(b)
		if b == "" || slices.Contains(out, b) {
			continue
		}
		if len(b) > 255 || strings.ContainsAny(b, " ~^:?*[\\") || strings.Contains(b, "..") || strings.HasPrefix(b, "refs/") {
			return nil, apperrors.Validation("livescan.invalid_branch", fmt.Sprintf("%q is not a valid branch name (use the short name, e.g. main)", b), nil)
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		if defaultBranch == "" {
			return nil, apperrors.Validation("livescan.invalid_branch", "choose at least one branch to watch", nil)
		}
		out = []string{defaultBranch}
	}
	if len(out) > maxWatchedBranches {
		return nil, apperrors.Validation("livescan.invalid_branch", fmt.Sprintf("watch at most %d branches", maxWatchedBranches), nil)
	}
	return out, nil
}

// newSecret is the per-hook signing secret GitHub uses for
// X-Hub-Signature-256: 32 random bytes, hex-encoded.
func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
