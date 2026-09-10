package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// ScanProfile is "how to run this scan," reused verbatim from
// CreateScanInput's own shape (minus SourceIP, which is per-request, not
// storable) — BUILD_GUIDE.md Phase 15 names this explicitly so scheduling
// doesn't invent a second shape for the same concept.
type ScanProfile struct {
	Type          domain.ScanType
	Engines       []domain.EngineID
	Branch        string
	PentestConfig *domain.PentestScanConfig
}

// ScanSchedule mirrors one row of `scan_schedules` (migration 00021,
// BUILD_GUIDE.md Phase 15).
type ScanSchedule struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	CronExpression string
	Profile        ScanProfile
	AssignedTo     *uuid.UUID
	CreatedBy      *uuid.UUID
	Enabled        bool
	NextRunAt      time.Time
	LastRunAt      *time.Time
	LastRunStatus  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateScheduleInput is CreateSchedule's input
// (`POST /projects/{id}/schedules`).
type CreateScheduleInput struct {
	CronExpression string
	Profile        ScanProfile
	AssignedTo     *uuid.UUID
}

// UpdateScheduleInput is UpdateSchedule's input — nil fields are left
// unchanged. ClearAssignedTo removes an existing assignee (a plain nil
// AssignedTo can't distinguish "leave unchanged" from "unassign").
type UpdateScheduleInput struct {
	CronExpression  *string
	Enabled         *bool
	AssignedTo      *uuid.UUID
	ClearAssignedTo bool
}

// ScanScheduleRepository is defined by this package; implementation lives
// in internal/store/repo.
type ScanScheduleRepository interface {
	Create(ctx context.Context, s *ScanSchedule) error
	GetByID(ctx context.Context, id uuid.UUID) (*ScanSchedule, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]ScanSchedule, error)
	Update(ctx context.Context, s *ScanSchedule) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ListDue is the scheduler ticker's one query — every enabled schedule
	// whose next_run_at has arrived.
	ListDue(ctx context.Context, at time.Time) ([]ScanSchedule, error)
	// RecordRun advances a schedule after it fires — see Scheduler.tick's
	// own doc comment for why this is a separate write from Update.
	RecordRun(ctx context.Context, id uuid.UUID, ranAt time.Time, status string, nextRunAt time.Time) error
}

// MembershipChecker is defined by this package — CreateSchedule's "assignee
// must already be an org member" rule, identical in shape and intent to
// project.MembershipChecker (see that interface's own doc comment); kept as
// its own local interface rather than importing project's, the same "no
// module reaches into another module's package just to reuse an interface
// shape" convention this codebase follows elsewhere (ProjectAccess, above,
// is the one deliberate exception, and it imports project for its concrete
// types anyway).
type MembershipChecker interface {
	IsOrgMember(ctx context.Context, orgID, userID uuid.UUID) (bool, error)
}

// minScheduleInterval is the "no more than once/hour" ceiling BUILD_GUIDE.md
// Phase 15 asks for — the same "a ceiling no client or custom value can
// exceed" pattern Phase 12 already established for pentest scan intensity,
// so a schedule can't accidentally DOS the worker pool. Not configurable —
// unlike the pentest rate limit, no requirement doc names an operator
// override for this one.
const minScheduleInterval = time.Hour

func (s *service) CreateSchedule(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in CreateScheduleInput) (*ScanSchedule, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, err // already 404-not-403 per project.Service's own rule
	}
	if !in.Profile.Type.Valid() {
		return nil, apperrors.Validation("schedule.invalid_input", "profile.type must be a recognised scan type", nil)
	}
	sched, err := parseAndClampCron(in.CronExpression)
	if err != nil {
		return nil, err
	}
	if in.AssignedTo != nil {
		if err := s.requireOrgMember(ctx, actor.OrgID, *in.AssignedTo); err != nil {
			return nil, err
		}
	}

	createdBy := actor.UserID
	now := time.Now().UTC()
	row := &ScanSchedule{
		ID: id.New(), ProjectID: projectID, CronExpression: strings.TrimSpace(in.CronExpression),
		Profile: in.Profile, AssignedTo: in.AssignedTo, CreatedBy: &createdBy,
		Enabled: true, NextRunAt: sched.Next(now),
	}
	if err := s.schedules.Create(ctx, row); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create schedule: %w", err))
	}
	if s.audit != nil {
		s.audit.Log(ctx, audit.Entry{
			OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "schedule.created",
			ResourceType: strPtr("scan_schedule"), ResourceID: &row.ID,
			Detail: map[string]any{"project_id": projectID.String(), "cron": row.CronExpression},
		})
	}
	return row, nil
}

func (s *service) GetSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID) (*ScanSchedule, error) {
	row, err := s.getOwnedSchedule(ctx, actor, scheduleID)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *service) ListSchedules(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ScanSchedule, error) {
	if _, err := s.projects.Get(ctx, actor, projectID); err != nil {
		return nil, err
	}
	rows, err := s.schedules.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list schedules: %w", err))
	}
	return rows, nil
}

func (s *service) UpdateSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID, in UpdateScheduleInput) (*ScanSchedule, error) {
	row, err := s.getOwnedSchedule(ctx, actor, scheduleID)
	if err != nil {
		return nil, err
	}

	if in.CronExpression != nil {
		sched, err := parseAndClampCron(*in.CronExpression)
		if err != nil {
			return nil, err
		}
		row.CronExpression = strings.TrimSpace(*in.CronExpression)
		row.NextRunAt = sched.Next(time.Now().UTC())
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.ClearAssignedTo {
		row.AssignedTo = nil
	} else if in.AssignedTo != nil {
		if err := s.requireOrgMember(ctx, actor.OrgID, *in.AssignedTo); err != nil {
			return nil, err
		}
		row.AssignedTo = in.AssignedTo
	}

	if err := s.schedules.Update(ctx, row); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("update schedule: %w", err))
	}
	return row, nil
}

func (s *service) DeleteSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID) error {
	row, err := s.getOwnedSchedule(ctx, actor, scheduleID)
	if err != nil {
		return err
	}
	if err := s.schedules.Delete(ctx, row.ID); err != nil {
		return apperrors.Internal(fmt.Errorf("delete schedule: %w", err))
	}
	return nil
}

// getOwnedSchedule resolves a schedule and confirms its project belongs to
// actor's org — the standing "404, not 403, for cross-org" rule
// (project.getOwnedProject's own precedent), applied one hop through the
// project it belongs to.
// TriggerSchedule fires one due schedule — called only by Scheduler's own
// ticker loop (scheduler.go), never from the HTTP layer. Attributes the
// resulting scan to the schedule's own created_by actor
// (BUILD_GUIDE.md Phase 15's "attributed to the schedule's created_by
// actor," the same accountability convention CLAUDE.md's "Accountability,
// not an allowlist" section already establishes for pentest scans), acting
// with domain.RoleAdmin — a scheduled trigger isn't a per-request RBAC
// decision, the schedule's own existence (created through the normal
// member+ gated endpoint) is the authorization.
func (s *service) TriggerSchedule(ctx context.Context, scheduleID uuid.UUID) (*ScanDetail, error) {
	sched, err := s.schedules.GetByID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	orgID, err := s.projects.GetOrgID(ctx, sched.ProjectID)
	if err != nil {
		_ = s.recordScheduleOutcome(ctx, sched, "failed")
		return nil, err
	}

	var userID uuid.UUID
	if sched.CreatedBy != nil {
		userID = *sched.CreatedBy
	}
	actor := domain.Actor{UserID: userID, OrgID: orgID, Role: domain.RoleAdmin}
	in := CreateScanInput{Type: sched.Profile.Type, Engines: sched.Profile.Engines, Branch: sched.Profile.Branch, PentestConfig: sched.Profile.PentestConfig}

	detail, createErr := s.CreateScan(ctx, actor, sched.ProjectID, in)
	status := "triggered"
	if createErr != nil {
		status = "failed"
	}
	if err := s.recordScheduleOutcome(ctx, sched, status); err != nil {
		return nil, err
	}
	if createErr != nil {
		return nil, createErr
	}

	if s.audit != nil {
		s.audit.Log(ctx, audit.Entry{
			OrgID: &orgID, ActorID: sched.CreatedBy, Action: "scan.scheduled_triggered",
			ResourceType: strPtr("scan_schedule"), ResourceID: &sched.ID,
			Detail: map[string]any{"scan_id": detail.ID.String(), "project_id": sched.ProjectID.String()},
		})
	}
	return detail, nil
}

// recordScheduleOutcome advances next_run_at from the schedule's own cron
// expression regardless of whether the trigger succeeded — a failing
// project (deleted, credential gone) must not spin the ticker on the same
// due schedule every tick forever; it just tries again at its next normal
// interval, the same as a healthy run would.
func (s *service) recordScheduleOutcome(ctx context.Context, sched *ScanSchedule, status string) error {
	now := time.Now().UTC()
	next := now.Add(minScheduleInterval) // safety fallback if the stored expression somehow fails to reparse
	if parsed, err := cron.ParseStandard(sched.CronExpression); err == nil {
		next = parsed.Next(now)
	}
	if err := s.schedules.RecordRun(ctx, sched.ID, now, status, next); err != nil {
		return apperrors.Internal(fmt.Errorf("record schedule run: %w", err))
	}
	return nil
}

func (s *service) getOwnedSchedule(ctx context.Context, actor domain.Actor, scheduleID uuid.UUID) (*ScanSchedule, error) {
	row, err := s.schedules.GetByID(ctx, scheduleID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("schedule.not_found", "schedule not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get schedule: %w", err))
	}
	if _, err := s.projects.Get(ctx, actor, row.ProjectID); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *service) requireOrgMember(ctx context.Context, orgID, userID uuid.UUID) error {
	if s.membership == nil {
		return nil // no MembershipChecker wired (a test that doesn't exercise this) — skip rather than panic
	}
	ok, err := s.membership.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("check org membership: %w", err))
	}
	if !ok {
		return apperrors.Validation("schedule.not_org_member", "assignee must already be a member of your organization", nil)
	}
	return nil
}

// parseAndClampCron validates a standard 5-field cron expression and
// enforces minScheduleInterval by checking the gap between the schedule's
// first two computed runs — an approximation (some expressions have
// variable spacing) that's still exactly the same "a ceiling no client or
// custom value can exceed" contract Phase 12 already established, not a
// promise every possible run gap is individually checked.
func parseAndClampCron(expr string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, apperrors.Validation("schedule.invalid_input", "cron_expression is required", nil)
	}
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, apperrors.Validation("schedule.invalid_cron", "cron_expression must be a valid 5-field cron expression", nil)
	}
	now := time.Now().UTC()
	first := sched.Next(now)
	second := sched.Next(first)
	if second.Sub(first) < minScheduleInterval {
		return nil, apperrors.Validation("schedule.interval_too_short", "a scan schedule cannot run more than once per hour", nil)
	}
	return sched, nil
}
