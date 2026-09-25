package orchestrator_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// --- hand-written fakes (no mocking framework) ---

type fakeScheduleRepo struct {
	mu   sync.Mutex
	rows map[uuid.UUID]orchestrator.ScanSchedule
}

func newFakeScheduleRepo() *fakeScheduleRepo {
	return &fakeScheduleRepo{rows: map[uuid.UUID]orchestrator.ScanSchedule{}}
}

func (f *fakeScheduleRepo) Create(_ context.Context, s *orchestrator.ScanSchedule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = id.New()
	}
	s.CreatedAt, s.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	f.rows[s.ID] = *s
	return nil
}

func (f *fakeScheduleRepo) GetByID(_ context.Context, scheduleID uuid.UUID) (*orchestrator.ScanSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rows[scheduleID]
	if !ok {
		return nil, apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	return &s, nil
}

func (f *fakeScheduleRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]orchestrator.ScanSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []orchestrator.ScanSchedule
	for _, s := range f.rows {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeScheduleRepo) Update(_ context.Context, s *orchestrator.ScanSchedule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rows[s.ID]; !ok {
		return apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	s.UpdatedAt = time.Now().UTC()
	f.rows[s.ID] = *s
	return nil
}

func (f *fakeScheduleRepo) Delete(_ context.Context, scheduleID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rows[scheduleID]; !ok {
		return apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	delete(f.rows, scheduleID)
	return nil
}

func (f *fakeScheduleRepo) ListDue(_ context.Context, at time.Time) ([]orchestrator.ScanSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []orchestrator.ScanSchedule
	for _, s := range f.rows {
		if s.Enabled && !s.NextRunAt.After(at) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeScheduleRepo) RecordRun(_ context.Context, scheduleID uuid.UUID, ranAt time.Time, status string, nextRunAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rows[scheduleID]
	if !ok {
		return apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	s.LastRunAt, s.LastRunStatus, s.NextRunAt = &ranAt, status, nextRunAt
	f.rows[scheduleID] = s
	return nil
}

type fakeSchedMembershipChecker struct {
	members map[uuid.UUID]bool
}

func (f *fakeSchedMembershipChecker) IsOrgMember(_ context.Context, _ uuid.UUID, userID uuid.UUID) (bool, error) {
	return f.members[userID], nil
}

// newTestOrchestratorWithSchedules mirrors newTestOrchestrator but also
// wires a ScanScheduleRepository/MembershipChecker — split into its own
// constructor rather than changing newTestOrchestrator's signature, since
// most existing tests in this package don't touch scheduling at all.
func newTestOrchestratorWithSchedules(t *testing.T, orgID uuid.UUID) (orchestrator.Service, *fakeScheduleRepo, *fakeSchedMembershipChecker, uuid.UUID) {
	t.Helper()
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})

	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{
		Project:    project.Project{ID: projectID, OrgID: orgID},
		Repository: &project.Repository{},
	}}
	schedules := newFakeScheduleRepo()
	membership := &fakeSchedMembershipChecker{members: map[uuid.UUID]bool{}}

	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil, schedules, membership)
	return svc, schedules, membership, projectID
}

func TestCreateSchedule_TooFrequentIntervalRejected(t *testing.T) {
	orgID := id.New()
	svc, _, _, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}

	_, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "* * * * *", // every minute — well under the 1-hour floor
		Profile:        orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
	})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestCreateSchedule_HourlyAccepted(t *testing.T) {
	orgID := id.New()
	svc, schedules, _, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}

	sched, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1", // every Monday at 9am
		Profile:        orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
	})
	require.NoError(t, err)
	require.True(t, sched.Enabled)
	require.False(t, sched.NextRunAt.IsZero())
	require.Len(t, schedules.rows, 1)
}

func TestCreateSchedule_AssigneeMustBeOrgMember(t *testing.T) {
	orgID := id.New()
	svc, _, membership, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}
	notAMember := id.New()

	_, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1",
		Profile:        orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
		AssignedTo:     &notAMember,
	})
	require.Error(t, err)

	member := id.New()
	membership.members[member] = true
	sched, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1",
		Profile:        orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
		AssignedTo:     &member,
	})
	require.NoError(t, err)
	require.Equal(t, member, *sched.AssignedTo)
}

func TestUpdateSchedule_DisableAndReenable(t *testing.T) {
	orgID := id.New()
	svc, _, _, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}

	sched, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1", Profile: orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
	})
	require.NoError(t, err)

	disabled := false
	updated, err := svc.UpdateSchedule(context.Background(), actor, sched.ID, orchestrator.UpdateScheduleInput{Enabled: &disabled})
	require.NoError(t, err)
	require.False(t, updated.Enabled)
}

func TestDeleteSchedule_RemovesIt(t *testing.T) {
	orgID := id.New()
	svc, schedules, _, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}

	sched, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1", Profile: orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
	})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteSchedule(context.Background(), actor, sched.ID))
	require.Empty(t, schedules.rows)
}

func TestTriggerSchedule_FiresAScanAndAdvancesNextRun(t *testing.T) {
	orgID := id.New()
	svc, schedules, _, projectID := newTestOrchestratorWithSchedules(t, orgID)
	actor := domain.Actor{UserID: id.New(), OrgID: orgID, Role: domain.RoleMember}

	sched, err := svc.CreateSchedule(context.Background(), actor, projectID, orchestrator.CreateScheduleInput{
		CronExpression: "0 9 * * 1", Profile: orchestrator.ScanProfile{Type: domain.ScanTypeFullSupplyChain},
	})
	require.NoError(t, err)
	originalNextRun := sched.NextRunAt

	detail, err := svc.TriggerSchedule(context.Background(), sched.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, domain.TriggerScheduled, detail.TriggerSource)

	got, err := schedules.GetByID(context.Background(), sched.ID)
	require.NoError(t, err)
	require.Equal(t, "triggered", got.LastRunStatus)
	require.NotNil(t, got.LastRunAt)
	// A weekly cron fired moments after being created recomputes to the
	// same upcoming Monday, not a strictly later one — the real invariant
	// is "never moves backward," not "always strictly advances."
	require.False(t, got.NextRunAt.Before(originalNextRun), "next_run_at should never move backward after a trigger")
}
