package livescan_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// fakeGate stands in for billing.Service.
type fakeGate struct {
	planOK    bool
	tokensOK  bool
	lastFloor int
	checks    int
}

func (g *fakeGate) RequireFeature(context.Context, uuid.UUID, string) error {
	if !g.planOK {
		return apperrors.Forbidden("billing.plan_required", "Live scanning needs the Pro plan")
	}
	return nil
}

func (g *fakeGate) LiveScanAllowed(_ context.Context, _ uuid.UUID, _ []domain.EngineID, floor int) (bool, string, error) {
	g.checks++
	g.lastFloor = floor
	switch {
	case !g.planOK:
		return false, "plan_required", nil
	case !g.tokensOK:
		return false, "insufficient_tokens", nil
	}
	return true, "", nil
}

func newBilledHarness(t *testing.T) (*harness, *fakeGate) {
	h := newHarness(t, 10)
	gate := &fakeGate{planOK: true, tokensOK: true}
	livescan.SetTokenGate(h.svc, gate)
	return h, gate
}

func TestEnable_FreePlanIsPlanRequired(t *testing.T) {
	h, gate := newBilledHarness(t)
	gate.planOK = false
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true,
	})
	requireCode(t, err, "billing.plan_required")
	require.Empty(t, h.hooks.created, "no hook registered on GitHub")
}

func TestEnable_StoresTheTokenFloor(t *testing.T) {
	h, _ := newBilledHarness(t)
	w := h.enable(t)
	require.Equal(t, 10, w.MinBalancePercent, "default floor")

	floor := 25
	w, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true, MinBalancePercent: &floor,
	})
	require.NoError(t, err)
	require.Equal(t, 25, w.MinBalancePercent)

	bad := 95
	_, err = h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true, MinBalancePercent: &bad,
	})
	requireCode(t, err, "livescan.invalid_input")
}

// Out of tokens: no scan, an audited "insufficient_tokens" delivery, and
// no error back to GitHub. The next push after a top-up just works — no
// pause to undo.
func TestPush_InsufficientTokensDropsQuietlyAndResumesByItself(t *testing.T) {
	h, gate := newBilledHarness(t)
	h.enable(t)
	gate.tokensOK = false
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "sha1", "octocat")))
	h.drain(t)

	require.Empty(t, h.scans.calls)
	require.Equal(t, "insufficient_tokens", h.repo.only().LastDeliveryStatus)
	require.NotNil(t, h.audit.last("webhook.insufficient_tokens"))
	require.Nil(t, h.repo.only().PausedReason, "running out of tokens never pauses live scanning")
	require.Equal(t, 10, gate.lastFloor, "the project's floor is what's checked")

	gate.tokensOK = true
	require.NoError(t, h.deliver(t, "push", "d2", pushBody("acme/payments-api", "main", "sha2", "octocat")))
	h.drain(t)
	require.Len(t, h.scans.calls, 1)
}

func TestPush_PlanLapsedDropsWithPlanRequired(t *testing.T) {
	h, gate := newBilledHarness(t)
	h.enable(t)
	gate.planOK = false
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "sha1", "octocat")))
	h.drain(t)
	require.Empty(t, h.scans.calls)
	require.Equal(t, "plan_required", h.repo.only().LastDeliveryStatus)
}

// The same commit is only scanned (and paid for) once, even when it
// arrives under a different delivery ID.
func TestPush_SameCommitIsNotScannedTwice(t *testing.T) {
	h, _ := newBilledHarness(t)
	h.enable(t)
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "abc123", "octocat")))
	h.drain(t)
	require.NoError(t, h.deliver(t, "push", "d2", pushBody("acme/payments-api", "main", "abc123", "octocat")))
	h.drain(t)

	require.Len(t, h.scans.calls, 1)
	require.Equal(t, "duplicate_commit", h.repo.only().LastDeliveryStatus)

	// A new commit is scanned normally.
	require.NoError(t, h.deliver(t, "push", "d3", pushBody("acme/payments-api", "main", "def456", "octocat")))
	h.drain(t)
	require.Len(t, h.scans.calls, 2)
}

func TestPush_LostRaceWithAnotherScanIsInsufficientTokensNotScanFailed(t *testing.T) {
	h, _ := newBilledHarness(t)
	h.enable(t)
	h.scans.err = apperrors.PaymentRequired("billing.insufficient_tokens", "not enough tokens")
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "sha1", "octocat")))
	h.drain(t)
	require.Equal(t, "insufficient_tokens", h.repo.only().LastDeliveryStatus)
}
