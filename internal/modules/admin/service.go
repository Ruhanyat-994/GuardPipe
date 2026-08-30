package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// Service is the platform-operator control plane
// (BUILD_GUIDE.md Phase 14). Every method here is reachable only through
// the `RequirePlatformOperator` middleware (transport/http/middleware) —
// this package does not re-check operator status itself, the same "route
// gates, service re-verifies resource ownership" split every other module
// follows, just with "is this caller even an operator" standing in for
// resource ownership, since there is no per-resource owner to check against
// a cross-org read.
//
// platform_operators grants themselves are never created or revoked through
// this Service — see cmd/guardpipe/admin.go's CLI subcommand and this
// package's own doc note at the top of BUILD_GUIDE.md's Phase 14 for why
// that is a deliberate anti-escalation control, not an oversight.
type Service interface {
	IsOperator(ctx context.Context, userID uuid.UUID) (bool, error)

	ListOrganizations(ctx context.Context, search string, page Page) ([]OrganizationSummary, int, error)
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*OrganizationDetail, error)
	SuspendOrganization(ctx context.Context, operator domain.Actor, orgID uuid.UUID, reason string) error
	ReinstateOrganization(ctx context.Context, operator domain.Actor, orgID uuid.UUID) error

	SuspendUser(ctx context.Context, operator domain.Actor, userID uuid.UUID, reason string) error
	ReinstateUser(ctx context.Context, operator domain.Actor, userID uuid.UUID) error

	// CreateFlag is deliberately not operator-only — any authenticated user
	// can report that a target scanned infrastructure they own (see
	// CreateFlagInput's own doc comment).
	CreateFlag(ctx context.Context, reporter domain.Actor, in CreateFlagInput) (*PentestFlag, error)
	ListFlags(ctx context.Context, status *FlagStatus, page Page) ([]PentestFlagDetail, int, error)
	// ResolveFlag transitioning to FlagConfirmedMisuse cascades to revoking
	// the target (project.Service.AdminRevokeTarget) — the concrete point
	// where an attributed report becomes an actual block, not just a label.
	ResolveFlag(ctx context.Context, operator domain.Actor, flagID uuid.UUID, in ResolveFlagInput) (*PentestFlag, error)

	ListAuditLog(ctx context.Context, filter audit.ListFilter, page Page) ([]audit.Entry, int, error)

	SystemHealth(ctx context.Context) (*SystemHealth, error)
}

// OrganizationRepository is defined by this package; implementation lives
// in internal/store/repo, on the same OrganizationRepo type that already
// implements identity.OrganizationRepository — one struct satisfying two
// modules' interfaces against the same table, the same pattern
// UserRepo.GetDisplayName already establishes for `project`.
type OrganizationRepository interface {
	ListAll(ctx context.Context, search string, page Page) ([]OrganizationSummary, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*OrganizationSummary, error)
	SetSuspended(ctx context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error
}

// UserRepository is defined by this package; implementation lives in
// internal/store/repo, on the same UserRepo type that already implements
// identity.UserRepository.
type UserRepository interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]UserSummary, error)
	// GetSummaryByID is named distinctly from identity.UserRepository's own
	// GetByID (which returns a full identity.User) — both interfaces are
	// implemented by the same store/repo.UserRepo struct, and Go doesn't
	// allow two methods of the same name with different signatures on one
	// type.
	GetSummaryByID(ctx context.Context, id uuid.UUID) (*UserSummary, error)
	SetSuspended(ctx context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error
}

// PlatformOperatorRepository is defined by this package; implementation
// lives in internal/store/repo. No Grant/Revoke method here — those are
// cmd/guardpipe/admin.go-only, wired directly against this same repository
// type but never exposed through Service.
type PlatformOperatorRepository interface {
	IsOperator(ctx context.Context, userID uuid.UUID) (bool, error)
}

// PentestFlagRepository is defined by this package; implementation lives in
// internal/store/repo.
type PentestFlagRepository interface {
	Create(ctx context.Context, f *PentestFlag) error
	GetByID(ctx context.Context, id uuid.UUID) (*PentestFlag, error)
	List(ctx context.Context, status *FlagStatus, page Page) ([]PentestFlag, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status FlagStatus, resolvedBy *uuid.UUID, resolvedAt *time.Time) error
}

// TargetReader is the narrow, read-only slice of `project`'s data this
// module needs — display context for a pentest-misuse flag. Implemented in
// internal/store/repo against the same table project.TargetRepository
// already owns (a read-only join, not a business operation — see
// TargetInfo's own doc comment).
type TargetReader interface {
	GetTargetInfo(ctx context.Context, targetID uuid.UUID) (*TargetInfo, error)
}

// TargetRevoker is the one piece of business logic this module triggers on
// another module's owned table — implemented by project.Service itself
// (AdminRevokeTarget), not by a repository, because revoking a target is a
// state transition project.Service already owns the rules for
// (RevokeTarget's existing body), not a plain column write.
type TargetRevoker interface {
	AdminRevokeTarget(ctx context.Context, targetID uuid.UUID) error
}

// EngineJobStatsReader is the last-24h engine success/failure signal for
// SystemHealth — implemented in internal/store/repo against `scan_jobs`,
// which every engine's job already writes to regardless of AI/Docker/
// Redis availability, so this is always real.
type EngineJobStatsReader interface {
	EngineJobStatsSince(ctx context.Context, since time.Time) ([]EngineJobStats, error)
}

// QueueHealthReader is optional — nil when no queue was wired (never true
// in production, but keeps a unit test from needing a real Redis).
type QueueHealthReader interface {
	JobsInFlight(ctx context.Context) (int, error)
}

// SandboxHealthReader is optional — nil when no Docker client was wired.
type SandboxHealthReader interface {
	RunningSandboxCount(ctx context.Context) (int, error)
}

// GeminiPoolReader is optional — nil when AI is disabled.
type GeminiPoolReader interface {
	PoolStatus() GeminiPoolStatus
}

// AICacheReader is optional — nil when AI is disabled.
type AICacheReader interface {
	CacheStats() AICacheStatus
}

type service struct {
	orgs      OrganizationRepository
	users     UserRepository
	operators PlatformOperatorRepository
	flags     PentestFlagRepository
	targets   TargetReader
	revoker   TargetRevoker
	audit     audit.Service

	engineStats EngineJobStatsReader
	queue       QueueHealthReader   // optional
	sandbox     SandboxHealthReader // optional
	gemini      GeminiPoolReader    // optional
	aiCache     AICacheReader       // optional
}

// NewService wires the admin module. queue/sandbox/gemini/aiCache may all
// be nil — SystemHealth degrades each corresponding field to "unavailable"
// rather than failing the whole call, matching this package's own
// SystemHealth doc comment.
func NewService(
	orgs OrganizationRepository,
	users UserRepository,
	operators PlatformOperatorRepository,
	flags PentestFlagRepository,
	targets TargetReader,
	revoker TargetRevoker,
	auditSvc audit.Service,
	engineStats EngineJobStatsReader,
	queue QueueHealthReader,
	sandbox SandboxHealthReader,
	gemini GeminiPoolReader,
	aiCache AICacheReader,
) Service {
	return &service{
		orgs: orgs, users: users, operators: operators, flags: flags,
		targets: targets, revoker: revoker, audit: auditSvc,
		engineStats: engineStats, queue: queue, sandbox: sandbox, gemini: gemini, aiCache: aiCache,
	}
}

func (s *service) IsOperator(ctx context.Context, userID uuid.UUID) (bool, error) {
	ok, err := s.operators.IsOperator(ctx, userID)
	if err != nil {
		return false, apperrors.Internal(fmt.Errorf("check platform operator status: %w", err))
	}
	return ok, nil
}

func (s *service) ListOrganizations(ctx context.Context, search string, page Page) ([]OrganizationSummary, int, error) {
	orgs, total, err := s.orgs.ListAll(ctx, strings.TrimSpace(search), page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list organizations: %w", err))
	}
	return orgs, total, nil
}

func (s *service) GetOrganization(ctx context.Context, orgID uuid.UUID) (*OrganizationDetail, error) {
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("admin.organization_not_found", "organization not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get organization: %w", err))
	}
	members, err := s.users.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list organization members: %w", err))
	}
	return &OrganizationDetail{OrganizationSummary: *org, Members: members}, nil
}

func (s *service) SuspendOrganization(ctx context.Context, operator domain.Actor, orgID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return apperrors.Validation("admin.reason_required", "a reason is required to suspend an organization", nil)
	}
	if _, err := s.orgs.GetByID(ctx, orgID); err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("admin.organization_not_found", "organization not found")
		}
		return apperrors.Internal(fmt.Errorf("get organization: %w", err))
	}
	now := time.Now().UTC()
	if err := s.orgs.SetSuspended(ctx, orgID, &now, &reason); err != nil {
		return apperrors.Internal(fmt.Errorf("suspend organization: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &operator.UserID, Action: "org.suspended",
		ResourceType: strPtr("organization"), ResourceID: &orgID,
		Detail: map[string]any{"reason": reason},
	})
	return nil
}

func (s *service) ReinstateOrganization(ctx context.Context, operator domain.Actor, orgID uuid.UUID) error {
	if _, err := s.orgs.GetByID(ctx, orgID); err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("admin.organization_not_found", "organization not found")
		}
		return apperrors.Internal(fmt.Errorf("get organization: %w", err))
	}
	if err := s.orgs.SetSuspended(ctx, orgID, nil, nil); err != nil {
		return apperrors.Internal(fmt.Errorf("reinstate organization: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &operator.UserID, Action: "org.reinstated",
		ResourceType: strPtr("organization"), ResourceID: &orgID,
	})
	return nil
}

func (s *service) SuspendUser(ctx context.Context, operator domain.Actor, userID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return apperrors.Validation("admin.reason_required", "a reason is required to suspend a user", nil)
	}
	u, err := s.users.GetSummaryByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("admin.user_not_found", "user not found")
		}
		return apperrors.Internal(fmt.Errorf("get user: %w", err))
	}
	now := time.Now().UTC()
	if err := s.users.SetSuspended(ctx, userID, &now, &reason); err != nil {
		return apperrors.Internal(fmt.Errorf("suspend user: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &u.OrgID, ActorID: &operator.UserID, Action: "user.suspended",
		ResourceType: strPtr("user"), ResourceID: &userID,
		Detail: map[string]any{"reason": reason},
	})
	return nil
}

func (s *service) ReinstateUser(ctx context.Context, operator domain.Actor, userID uuid.UUID) error {
	u, err := s.users.GetSummaryByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("admin.user_not_found", "user not found")
		}
		return apperrors.Internal(fmt.Errorf("get user: %w", err))
	}
	if err := s.users.SetSuspended(ctx, userID, nil, nil); err != nil {
		return apperrors.Internal(fmt.Errorf("reinstate user: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &u.OrgID, ActorID: &operator.UserID, Action: "user.reinstated",
		ResourceType: strPtr("user"), ResourceID: &userID,
	})
	return nil
}

func (s *service) CreateFlag(ctx context.Context, reporter domain.Actor, in CreateFlagInput) (*PentestFlag, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, apperrors.Validation("admin.invalid_input", "reason is required", nil)
	}
	if !in.Source.Valid() {
		return nil, apperrors.Validation("admin.invalid_input", "source must be self_reported, external_complaint, or operator_review", nil)
	}
	if _, err := s.targets.GetTargetInfo(ctx, in.TargetID); err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("admin.target_not_found", "pentest target not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get target info: %w", err))
	}

	reporterID := reporter.UserID
	f := &PentestFlag{
		ID: id.New(), TargetID: in.TargetID, Status: FlagOpen,
		Source: in.Source, Reason: reason, ReportedBy: &reporterID,
	}
	if err := s.flags.Create(ctx, f); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create pentest flag: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &reporter.OrgID, ActorID: &reporterID, Action: "pentest_flag.created",
		ResourceType: strPtr("pentest_target"), ResourceID: &in.TargetID,
		Detail: map[string]any{"source": string(in.Source), "flag_id": f.ID.String()},
	})
	return f, nil
}

func (s *service) ListFlags(ctx context.Context, status *FlagStatus, page Page) ([]PentestFlagDetail, int, error) {
	flags, total, err := s.flags.List(ctx, status, page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list pentest flags: %w", err))
	}

	details := make([]PentestFlagDetail, len(flags))
	for i, f := range flags {
		info, err := s.targets.GetTargetInfo(ctx, f.TargetID)
		if err != nil && !isNotFound(err) {
			return nil, 0, apperrors.Internal(fmt.Errorf("get target info: %w", err))
		}
		detail := PentestFlagDetail{PentestFlag: f}
		if info != nil {
			detail.Target = *info
		}
		details[i] = detail
	}
	return details, total, nil
}

func (s *service) ResolveFlag(ctx context.Context, operator domain.Actor, flagID uuid.UUID, in ResolveFlagInput) (*PentestFlag, error) {
	if !in.Status.Valid() || in.Status == FlagOpen {
		return nil, apperrors.Validation("admin.invalid_input", "status must be investigating, dismissed, or confirmed_misuse", nil)
	}
	f, err := s.flags.GetByID(ctx, flagID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("admin.flag_not_found", "pentest flag not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get pentest flag: %w", err))
	}

	var resolvedBy *uuid.UUID
	var resolvedAt *time.Time
	if in.Status == FlagDismissed || in.Status == FlagConfirmedMisuse {
		operatorID := operator.UserID
		now := time.Now().UTC()
		resolvedBy, resolvedAt = &operatorID, &now
	}
	if err := s.flags.UpdateStatus(ctx, flagID, in.Status, resolvedBy, resolvedAt); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("update pentest flag: %w", err))
	}
	f.Status, f.ResolvedBy, f.ResolvedAt = in.Status, resolvedBy, resolvedAt

	if in.Status == FlagConfirmedMisuse {
		if err := s.revoker.AdminRevokeTarget(ctx, f.TargetID); err != nil {
			return nil, apperrors.Internal(fmt.Errorf("revoke confirmed-misuse target: %w", err))
		}
	}

	s.audit.Log(ctx, audit.Entry{
		OrgID: &operator.OrgID, ActorID: &operator.UserID, Action: "pentest_flag.resolved",
		ResourceType: strPtr("pentest_target"), ResourceID: &f.TargetID,
		Detail: map[string]any{"flag_id": flagID.String(), "status": string(in.Status)},
	})
	return f, nil
}

func (s *service) ListAuditLog(ctx context.Context, filter audit.ListFilter, page Page) ([]audit.Entry, int, error) {
	entries, total, err := s.audit.List(ctx, filter, audit.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list audit log: %w", err))
	}
	return entries, total, nil
}

func (s *service) SystemHealth(ctx context.Context) (*SystemHealth, error) {
	since := time.Now().UTC().Add(-24 * time.Hour)
	stats, err := s.engineStats.EngineJobStatsSince(ctx, since)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get engine job stats: %w", err))
	}

	health := &SystemHealth{EngineStats: stats, CheckedAt: time.Now().UTC()}

	if s.queue != nil {
		if n, err := s.queue.JobsInFlight(ctx); err == nil {
			health.JobsInFlight = n
		}
	}
	if s.sandbox != nil {
		if n, err := s.sandbox.RunningSandboxCount(ctx); err == nil {
			health.SandboxContainersRunning = &n
		}
	}
	if s.gemini != nil {
		health.Gemini = s.gemini.PoolStatus()
	}
	if s.aiCache != nil {
		health.AICache = s.aiCache.CacheStats()
	}
	return health, nil
}

func strPtr(s string) *string { return &s }

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
