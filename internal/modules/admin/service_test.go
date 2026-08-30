package admin_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// --- hand-written fakes (no mocking framework, documentation/15-testing-strategy.md) ---

type fakeOrgRepo struct {
	mu    sync.Mutex
	orgs  map[uuid.UUID]admin.OrganizationSummary
}

func newFakeOrgRepo(orgs ...admin.OrganizationSummary) *fakeOrgRepo {
	m := map[uuid.UUID]admin.OrganizationSummary{}
	for _, o := range orgs {
		m[o.ID] = o
	}
	return &fakeOrgRepo{orgs: m}
}

func (f *fakeOrgRepo) ListAll(_ context.Context, _ string, _ admin.Page) ([]admin.OrganizationSummary, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]admin.OrganizationSummary, 0, len(f.orgs))
	for _, o := range f.orgs {
		out = append(out, o)
	}
	return out, len(out), nil
}

func (f *fakeOrgRepo) GetByID(_ context.Context, id uuid.UUID) (*admin.OrganizationSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.orgs[id]
	if !ok {
		return nil, apperrors.NotFound("admin.organization_not_found", "organization not found")
	}
	return &o, nil
}

func (f *fakeOrgRepo) SetSuspended(_ context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.orgs[id]
	if !ok {
		return apperrors.NotFound("admin.organization_not_found", "organization not found")
	}
	o.SuspendedAt, o.SuspendedReason = suspendedAt, reason
	f.orgs[id] = o
	return nil
}

type fakeUserRepo struct {
	mu    sync.Mutex
	users map[uuid.UUID]admin.UserSummary
}

func newFakeUserRepo(users ...admin.UserSummary) *fakeUserRepo {
	m := map[uuid.UUID]admin.UserSummary{}
	for _, u := range users {
		m[u.ID] = u
	}
	return &fakeUserRepo{users: m}
}

func (f *fakeUserRepo) ListByOrg(_ context.Context, orgID uuid.UUID) ([]admin.UserSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []admin.UserSummary
	for _, u := range f.users {
		if u.OrgID == orgID {
			out = append(out, u)
		}
	}
	return out, nil
}

func (f *fakeUserRepo) GetSummaryByID(_ context.Context, id uuid.UUID) (*admin.UserSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, apperrors.NotFound("admin.user_not_found", "user not found")
	}
	return &u, nil
}

func (f *fakeUserRepo) SetSuspended(_ context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return apperrors.NotFound("admin.user_not_found", "user not found")
	}
	u.SuspendedAt, u.SuspendedReason = suspendedAt, reason
	f.users[id] = u
	return nil
}

type fakeOperatorRepo struct {
	operators map[uuid.UUID]bool
}

func (f *fakeOperatorRepo) IsOperator(_ context.Context, userID uuid.UUID) (bool, error) {
	return f.operators[userID], nil
}

type fakeFlagRepo struct {
	mu    sync.Mutex
	flags map[uuid.UUID]admin.PentestFlag
}

func newFakeFlagRepo() *fakeFlagRepo {
	return &fakeFlagRepo{flags: map[uuid.UUID]admin.PentestFlag{}}
}

func (f *fakeFlagRepo) Create(_ context.Context, flag *admin.PentestFlag) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	flag.CreatedAt = time.Now().UTC()
	f.flags[flag.ID] = *flag
	return nil
}

func (f *fakeFlagRepo) GetByID(_ context.Context, id uuid.UUID) (*admin.PentestFlag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	flag, ok := f.flags[id]
	if !ok {
		return nil, apperrors.NotFound("admin.flag_not_found", "pentest flag not found")
	}
	return &flag, nil
}

func (f *fakeFlagRepo) List(_ context.Context, status *admin.FlagStatus, _ admin.Page) ([]admin.PentestFlag, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []admin.PentestFlag
	for _, flag := range f.flags {
		if status == nil || flag.Status == *status {
			out = append(out, flag)
		}
	}
	return out, len(out), nil
}

func (f *fakeFlagRepo) UpdateStatus(_ context.Context, id uuid.UUID, status admin.FlagStatus, resolvedBy *uuid.UUID, resolvedAt *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	flag, ok := f.flags[id]
	if !ok {
		return apperrors.NotFound("admin.flag_not_found", "pentest flag not found")
	}
	flag.Status, flag.ResolvedBy, flag.ResolvedAt = status, resolvedBy, resolvedAt
	f.flags[id] = flag
	return nil
}

type fakeTargetReader struct {
	infos map[uuid.UUID]admin.TargetInfo
}

func (f *fakeTargetReader) GetTargetInfo(_ context.Context, targetID uuid.UUID) (*admin.TargetInfo, error) {
	info, ok := f.infos[targetID]
	if !ok {
		return nil, apperrors.NotFound("target.not_found", "pentest target not found")
	}
	return &info, nil
}

type fakeRevoker struct {
	mu      sync.Mutex
	revoked []uuid.UUID
	err     error
}

func (f *fakeRevoker) AdminRevokeTarget(_ context.Context, targetID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, targetID)
	return nil
}

type fakeAuditService struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (f *fakeAuditService) Log(_ context.Context, e audit.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeAuditService) List(_ context.Context, _ audit.ListFilter, _ audit.Page) ([]audit.Entry, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.entries, len(f.entries), nil
}

func (f *fakeAuditService) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.entries))
	for i, e := range f.entries {
		out[i] = e.Action
	}
	return out
}

type fakeEngineStatsReader struct{ stats []admin.EngineJobStats }

func (f *fakeEngineStatsReader) EngineJobStatsSince(context.Context, time.Time) ([]admin.EngineJobStats, error) {
	return f.stats, nil
}

// --- test helpers ---

func newTestService(t *testing.T, orgs *fakeOrgRepo, users *fakeUserRepo, operators *fakeOperatorRepo, flags *fakeFlagRepo, targets *fakeTargetReader, revoker *fakeRevoker, auditSvc *fakeAuditService) admin.Service {
	t.Helper()
	if orgs == nil {
		orgs = newFakeOrgRepo()
	}
	if users == nil {
		users = newFakeUserRepo()
	}
	if operators == nil {
		operators = &fakeOperatorRepo{operators: map[uuid.UUID]bool{}}
	}
	if flags == nil {
		flags = newFakeFlagRepo()
	}
	if targets == nil {
		targets = &fakeTargetReader{infos: map[uuid.UUID]admin.TargetInfo{}}
	}
	if revoker == nil {
		revoker = &fakeRevoker{}
	}
	if auditSvc == nil {
		auditSvc = &fakeAuditService{}
	}
	return admin.NewService(orgs, users, operators, flags, targets, revoker, auditSvc,
		&fakeEngineStatsReader{}, nil, nil, nil, nil)
}

func testActor() domain.Actor {
	return domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleAdmin}
}

// --- tests ---

func TestIsOperator_DelegatesToRepo(t *testing.T) {
	userID := id.New()
	operators := &fakeOperatorRepo{operators: map[uuid.UUID]bool{userID: true}}
	svc := newTestService(t, nil, nil, operators, nil, nil, nil, nil)

	ok, err := svc.IsOperator(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = svc.IsOperator(context.Background(), id.New())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSuspendOrganization_RequiresReason(t *testing.T) {
	orgID := id.New()
	orgs := newFakeOrgRepo(admin.OrganizationSummary{ID: orgID, Name: "Acme"})
	svc := newTestService(t, orgs, nil, nil, nil, nil, nil, nil)

	err := svc.SuspendOrganization(context.Background(), testActor(), orgID, "   ")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestSuspendOrganization_Success_SetsStateAndLogsAudit(t *testing.T) {
	orgID := id.New()
	orgs := newFakeOrgRepo(admin.OrganizationSummary{ID: orgID, Name: "Acme"})
	auditSvc := &fakeAuditService{}
	svc := newTestService(t, orgs, nil, nil, nil, nil, nil, auditSvc)
	operator := testActor()

	err := svc.SuspendOrganization(context.Background(), operator, orgID, "ToS violation")
	require.NoError(t, err)

	org, err := orgs.GetByID(context.Background(), orgID)
	require.NoError(t, err)
	require.NotNil(t, org.SuspendedAt)
	require.Equal(t, "ToS violation", *org.SuspendedReason)
	require.Contains(t, auditSvc.actions(), "org.suspended")
}

func TestSuspendOrganization_UnknownOrg_NotFound(t *testing.T) {
	svc := newTestService(t, nil, nil, nil, nil, nil, nil, nil)
	err := svc.SuspendOrganization(context.Background(), testActor(), id.New(), "reason")
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestReinstateOrganization_ClearsSuspension(t *testing.T) {
	orgID := id.New()
	reason := "ToS violation"
	now := time.Now().UTC()
	orgs := newFakeOrgRepo(admin.OrganizationSummary{ID: orgID, Name: "Acme", SuspendedAt: &now, SuspendedReason: &reason})
	auditSvc := &fakeAuditService{}
	svc := newTestService(t, orgs, nil, nil, nil, nil, nil, auditSvc)

	err := svc.ReinstateOrganization(context.Background(), testActor(), orgID)
	require.NoError(t, err)

	org, err := orgs.GetByID(context.Background(), orgID)
	require.NoError(t, err)
	require.Nil(t, org.SuspendedAt)
	require.Contains(t, auditSvc.actions(), "org.reinstated")
}

func TestSuspendUser_RequiresReason(t *testing.T) {
	userID := id.New()
	users := newFakeUserRepo(admin.UserSummary{ID: userID, OrgID: id.New()})
	svc := newTestService(t, nil, users, nil, nil, nil, nil, nil)

	err := svc.SuspendUser(context.Background(), testActor(), userID, "")
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestCreateFlag_ValidatesSourceAndReason(t *testing.T) {
	targetID := id.New()
	targets := &fakeTargetReader{infos: map[uuid.UUID]admin.TargetInfo{targetID: {TargetID: targetID}}}
	svc := newTestService(t, nil, nil, nil, nil, targets, nil, nil)

	_, err := svc.CreateFlag(context.Background(), testActor(), admin.CreateFlagInput{
		TargetID: targetID, Source: "not_a_real_source", Reason: "scanned my server",
	})
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)

	_, err = svc.CreateFlag(context.Background(), testActor(), admin.CreateFlagInput{
		TargetID: targetID, Source: admin.FlagSourceSelfReported, Reason: "  ",
	})
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestCreateFlag_UnknownTarget_NotFound(t *testing.T) {
	svc := newTestService(t, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.CreateFlag(context.Background(), testActor(), admin.CreateFlagInput{
		TargetID: id.New(), Source: admin.FlagSourceSelfReported, Reason: "scanned my server",
	})
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestCreateFlag_Success_ReachableWithoutOperatorStatus(t *testing.T) {
	// This test documents the deliberate design: CreateFlag takes no
	// operator check at all (see Service's own interface doc comment) —
	// any actor, operator or not, can file a report.
	targetID := id.New()
	targets := &fakeTargetReader{infos: map[uuid.UUID]admin.TargetInfo{targetID: {TargetID: targetID}}}
	auditSvc := &fakeAuditService{}
	svc := newTestService(t, nil, nil, &fakeOperatorRepo{operators: map[uuid.UUID]bool{}}, nil, targets, nil, auditSvc)

	flag, err := svc.CreateFlag(context.Background(), testActor(), admin.CreateFlagInput{
		TargetID: targetID, Source: admin.FlagSourceSelfReported, Reason: "scanned my server without authorization",
	})
	require.NoError(t, err)
	require.Equal(t, admin.FlagOpen, flag.Status)
	require.Contains(t, auditSvc.actions(), "pentest_flag.created")
}

func TestResolveFlag_ConfirmedMisuse_RevokesTarget(t *testing.T) {
	targetID := id.New()
	flags := newFakeFlagRepo()
	flagID := id.New()
	flags.flags[flagID] = admin.PentestFlag{ID: flagID, TargetID: targetID, Status: admin.FlagOpen, Source: admin.FlagSourceSelfReported, Reason: "misuse"}
	revoker := &fakeRevoker{}
	auditSvc := &fakeAuditService{}
	svc := newTestService(t, nil, nil, nil, flags, nil, revoker, auditSvc)

	flag, err := svc.ResolveFlag(context.Background(), testActor(), flagID, admin.ResolveFlagInput{Status: admin.FlagConfirmedMisuse})
	require.NoError(t, err)
	require.Equal(t, admin.FlagConfirmedMisuse, flag.Status)
	require.NotNil(t, flag.ResolvedAt)
	require.Equal(t, []uuid.UUID{targetID}, revoker.revoked)
	require.Contains(t, auditSvc.actions(), "pentest_flag.resolved")
}

func TestResolveFlag_Dismissed_DoesNotRevokeTarget(t *testing.T) {
	targetID := id.New()
	flags := newFakeFlagRepo()
	flagID := id.New()
	flags.flags[flagID] = admin.PentestFlag{ID: flagID, TargetID: targetID, Status: admin.FlagOpen, Source: admin.FlagSourceSelfReported, Reason: "misuse"}
	revoker := &fakeRevoker{}
	svc := newTestService(t, nil, nil, nil, flags, nil, revoker, nil)

	flag, err := svc.ResolveFlag(context.Background(), testActor(), flagID, admin.ResolveFlagInput{Status: admin.FlagDismissed})
	require.NoError(t, err)
	require.Equal(t, admin.FlagDismissed, flag.Status)
	require.Empty(t, revoker.revoked)
}

func TestResolveFlag_RejectsReopeningToOpen(t *testing.T) {
	flags := newFakeFlagRepo()
	flagID := id.New()
	flags.flags[flagID] = admin.PentestFlag{ID: flagID, TargetID: id.New(), Status: admin.FlagInvestigating}
	svc := newTestService(t, nil, nil, nil, flags, nil, nil, nil)

	_, err := svc.ResolveFlag(context.Background(), testActor(), flagID, admin.ResolveFlagInput{Status: admin.FlagOpen})
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestSystemHealth_DegradesGracefullyWhenOptionalReadersNil(t *testing.T) {
	svc := newTestService(t, nil, nil, nil, nil, nil, nil, nil)
	health, err := svc.SystemHealth(context.Background())
	require.NoError(t, err)
	require.Nil(t, health.SandboxContainersRunning)
	require.False(t, health.Gemini.Available)
	require.False(t, health.AICache.Available)
	require.Zero(t, health.JobsInFlight)
}

func TestSystemHealth_EngineStatsAlwaysReal(t *testing.T) {
	orgs, users, operators, flags, targets, revoker, auditSvc :=
		newFakeOrgRepo(), newFakeUserRepo(), &fakeOperatorRepo{operators: map[uuid.UUID]bool{}}, newFakeFlagRepo(),
		&fakeTargetReader{infos: map[uuid.UUID]admin.TargetInfo{}}, &fakeRevoker{}, &fakeAuditService{}
	stats := []admin.EngineJobStats{{Engine: domain.EngineDepScan, Succeeded: 3, Failed: 1}}
	svc := admin.NewService(orgs, users, operators, flags, targets, revoker, auditSvc,
		&fakeEngineStatsReader{stats: stats}, nil, nil, nil, nil)

	health, err := svc.SystemHealth(context.Background())
	require.NoError(t, err)
	require.Equal(t, stats, health.EngineStats)
}
