package orchestrator_test

import (
	"context"
	"errors"
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

// fakeTokens records what CreateScan/the worker asked billing to do.
type fakeTokens struct {
	mu        sync.Mutex
	chargeErr error
	charges   []orchestrator.ChargeRequest
	chargeOrg []uuid.UUID
	refunds   map[uuid.UUID]string
	blocked   map[domain.EngineID]bool
}

func newFakeTokens() *fakeTokens {
	return &fakeTokens{refunds: map[uuid.UUID]string{}, blocked: map[domain.EngineID]bool{}}
}

func (f *fakeTokens) Charge(_ context.Context, orgID uuid.UUID, req orchestrator.ChargeRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chargeErr != nil {
		return f.chargeErr
	}
	f.charges = append(f.charges, req)
	f.chargeOrg = append(f.chargeOrg, orgID)
	return nil
}

func (f *fakeTokens) RefundJob(_ context.Context, jobID uuid.UUID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refunds[jobID] = reason
	return nil
}

func (f *fakeTokens) AllowedEngines(_ context.Context, _ uuid.UUID, engines []domain.EngineID) ([]domain.EngineID, error) {
	var out []domain.EngineID
	for _, e := range engines {
		if !f.blocked[e] {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeTokens) RequireFeature(context.Context, uuid.UUID, string) error { return nil }

func newBilledOrchestrator(t *testing.T, tokens *fakeTokens, enqueuer *fakeEnqueuer) (orchestrator.Service, *fakeScanRepo, *fakeScanJobRepo, uuid.UUID) {
	t.Helper()
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	registry.Register(fakeEngine{id: domain.EngineCodeScan})
	registry.Register(fakeEngine{id: domain.EnginePentest})
	orgID := id.New()
	projects := &fakeProjectAccess{
		detail: &project.ProjectDetail{Project: project.Project{ID: id.New(), OrgID: orgID}, Repository: &project.Repository{}},
		target: &project.Target{},
	}
	svc := orchestrator.NewService(scans, jobs, &fakeFindingRepo{}, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry,
		domain.PentestPresetDeepConfig(), nil, nil, 0, nil, nil, nil)
	orchestrator.SetTokenCharger(svc, tokens)
	return svc, scans, jobs, orgID
}

func TestCreateScan_ChargesEveryJobToTheProjectsOrg(t *testing.T) {
	tokens := newFakeTokens()
	svc, _, _, orgID := newBilledOrchestrator(t, tokens, &fakeEnqueuer{})
	actor := newActor()

	detail, err := svc.CreateScan(context.Background(), actor, id.New(), orchestrator.CreateScanInput{
		Type: domain.ScanTypePartial, Engines: []domain.EngineID{domain.EngineDepScan, domain.EngineCodeScan},
		TriggerSource: domain.TriggerWebhookPush,
	})
	require.NoError(t, err)
	require.Len(t, tokens.charges, 1, "charged exactly once")
	req := tokens.charges[0]
	require.Equal(t, orgID, tokens.chargeOrg[0], "the project's org pays")
	require.Equal(t, detail.ID, req.ScanID)
	require.Equal(t, domain.TriggerWebhookPush, req.Trigger, "trigger passed through for the live-scan price")
	require.Len(t, req.Lines, 2)
	for i, j := range detail.Jobs {
		require.Equal(t, j.ID, req.Lines[i].JobID, "charge lines are the real job IDs")
	}
}

func TestCreateScan_PassesPentestPresetForPricing(t *testing.T) {
	tokens := newFakeTokens()
	svc, _, _, _ := newBilledOrchestrator(t, tokens, &fakeEnqueuer{})
	cfg := domain.PentestPresetStandardConfig()
	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{
		Type: domain.ScanTypePentestOnly, PentestConfig: &cfg,
	})
	require.NoError(t, err)
	require.Equal(t, domain.PentestPresetStandard, tokens.charges[0].Preset)
}

// A scan the org can't afford is never created: no scan row, no jobs,
// nothing queued — and the billing error reaches the caller unchanged.
func TestCreateScan_ChargeRejected_CreatesNothing(t *testing.T) {
	tokens := newFakeTokens()
	tokens.chargeErr = apperrors.PaymentRequired("billing.insufficient_tokens", "not enough tokens")
	enqueuer := &fakeEnqueuer{}
	svc, scans, _, _ := newBilledOrchestrator(t, tokens, enqueuer)

	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	var appErr *apperrors.Error
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, "billing.insufficient_tokens", appErr.Code)
	require.Empty(t, scans.byID, "no scan row")
	require.Empty(t, enqueuer.enqueued, "nothing queued")
}

func TestCreateScan_EnqueueFailure_RefundsUnqueuedJobs(t *testing.T) {
	tokens := newFakeTokens()
	enqueuer := &fakeEnqueuer{err: errors.New("redis down")}
	svc, _, _, _ := newBilledOrchestrator(t, tokens, enqueuer)

	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{
		Type: domain.ScanTypePartial, Engines: []domain.EngineID{domain.EngineDepScan, domain.EngineCodeScan},
	})
	require.Error(t, err)
	require.Len(t, tokens.refunds, 2, "every job that will never run is refunded")
	for _, reason := range tokens.refunds {
		require.Equal(t, "create_failed", reason)
	}
}

func TestCreateScan_FullScanLeavesOutEnginesThePlanLacks(t *testing.T) {
	tokens := newFakeTokens()
	tokens.blocked[domain.EnginePentest] = true
	svc, _, _, _ := newBilledOrchestrator(t, tokens, &fakeEnqueuer{})

	detail, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.NotContains(t, detail.RequestedEngines, domain.EnginePentest)
	require.Len(t, detail.RequestedEngines, 2)
}

func TestCreateScan_BillingOffChargesNothing(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
}

func TestPool_RefundsSkippedAndFailedJobsButNotSucceeded(t *testing.T) {
	cases := []struct {
		name       string
		engine     *scriptedEngine
		wantRefund bool
	}{
		{"skipped (not applicable)", &scriptedEngine{id: domain.EngineDepScan, applicable: false, reason: "no manifest"}, true},
		{"failed (engine panic)", &scriptedEngine{id: domain.EngineDepScan, applicable: true, panicOnRun: true}, true},
		{"succeeded", &scriptedEngine{id: domain.EngineDepScan, applicable: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, scans, jobs, _, q := newTestPool(t, tc.engine, &fakeCloner{})
			tokens := newFakeTokens()
			pool.Tokens = tokens
			_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
			q.pending = []string{jobID.String()}

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			pool.Start(ctx)

			_, refunded := tokens.refunds[jobID]
			require.Equal(t, tc.wantRefund, refunded)
		})
	}
}

func TestPool_RefundsJobCancelledBeforeItStarted(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{})
	tokens := newFakeTokens()
	pool.Tokens = tokens
	scanID, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	require.NoError(t, scans.SetCancelRequested(context.Background(), scanID))
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	require.Equal(t, "cancelled_before_start", tokens.refunds[jobID])
}
