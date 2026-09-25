//go:build integration

package repo_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/payment/demo"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// billingFixture is a real billing.Service over real Postgres.
func billingFixture(t *testing.T) (*billing.Service, uuid.UUID, domain.Actor, func() int64) {
	svc, orgID, admin, balance, _ := billingFixtureWithPool(t)
	return svc, orgID, admin, balance
}

func billingFixtureWithPool(t *testing.T) (*billing.Service, uuid.UUID, domain.Actor, func() int64, *repo.OrganizationRepo) {
	t.Helper()
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, err := repo.NewOrganizationRepo(pool).Create(ctx, "Billing Org")
	require.NoError(t, err)
	svc := billing.NewService(repo.NewBillingRepo(pool), demo.Provider{}, nil, billing.ModeDemo, nil)
	user := &identity.User{
		ID: id.New(), OrgID: orgID, Email: "billing-" + orgID.String()[:8] + "@example.com", DisplayName: "Billing Admin",
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaGhhc2g", Role: "admin",
	}
	require.NoError(t, repo.NewUserRepo(pool).Create(ctx, user))
	admin := domain.Actor{UserID: user.ID, OrgID: orgID, Role: domain.RoleAdmin}
	balance := func() int64 {
		sum, err := svc.Summary(ctx, admin)
		require.NoError(t, err)
		return sum.Balance
	}
	return svc, orgID, admin, balance, repo.NewOrganizationRepo(pool)
}

func TestBillingRepo_LazyProvisioningHappensOnce(t *testing.T) {
	svc, _, admin, balance := billingFixture(t)
	require.Equal(t, int64(15_000), balance())
	require.Equal(t, int64(15_000), balance(), "a second look must not grant again")

	ledger, err := svc.Ledger(context.Background(), admin, nil, 10)
	require.NoError(t, err)
	require.Len(t, ledger, 1)
	require.Equal(t, billing.KindGrant, ledger[0].Kind)
}

// Ten concurrent 20,000-token scans against a 100,000 balance: the row
// lock must let exactly five through and never overspend.
func TestBillingRepo_ConcurrentChargesNeverOverspend(t *testing.T) {
	svc, orgID, admin, balance := billingFixture(t)
	ctx := context.Background()
	require.NoError(t, svc.AdminAdjust(ctx, admin.UserID, orgID, 85_000, "test")) // 15,000 + 85,000

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := billing.ChargeRequest{ScanID: uuid.New(), Trigger: domain.TriggerManual}
			for range 4 {
				req.Lines = append(req.Lines, billing.ChargeLine{JobID: uuid.New(), Engine: domain.EngineCodeScan})
			}
			if svc.Charge(ctx, orgID, req) == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 5, ok)
	require.Equal(t, int64(0), balance())
}

func TestBillingRepo_ChargeRefundAndPurchaseRoundTrip(t *testing.T) {
	svc, orgID, admin, balance, orgs := billingFixtureWithPool(t)
	ctx := context.Background()

	sess, _, err := svc.StartCheckout(ctx, admin, string(billing.PlanProMonthly))
	require.NoError(t, err)
	_, err = svc.ConfirmDemo(ctx, admin, sess.ID, "success")
	require.NoError(t, err)
	_, err = svc.ConfirmDemo(ctx, admin, sess.ID, "success") // idempotent
	require.NoError(t, err)
	require.Equal(t, int64(500_000), balance())

	scanID, container := uuid.New(), uuid.New()
	req := billing.ChargeRequest{ScanID: scanID, Trigger: domain.TriggerManual, Lines: []billing.ChargeLine{
		{JobID: container, Engine: domain.EngineContainerScan},
		{JobID: uuid.New(), Engine: domain.EngineDepScan},
	}}
	require.NoError(t, svc.Charge(ctx, orgID, req))
	require.Equal(t, int64(494_500), balance())

	require.NoError(t, svc.RefundJob(ctx, container, "engine_not_applicable"))
	require.NoError(t, svc.RefundJob(ctx, container, "engine_not_applicable"))
	require.Equal(t, int64(498_500), balance())

	st, err := svc.ScanTokens(ctx, admin, scanID)
	require.NoError(t, err)
	require.Equal(t, int64(5_500), st.Charged)
	require.Equal(t, int64(4_000), st.Refunded)

	// Another org can't see this org's checkout (404, not 403) or scan costs.
	otherOrg, err := orgs.Create(ctx, "Other Org")
	require.NoError(t, err)
	other := domain.Actor{UserID: uuid.New(), OrgID: otherOrg, Role: domain.RoleAdmin}
	_, err = svc.GetCheckout(ctx, other, sess.ID)
	require.ErrorContains(t, err, "checkout not found")
	st, err = svc.ScanTokens(ctx, other, scanID)
	require.NoError(t, err)
	require.Zero(t, st.Charged)
}

// The finding assistant's charge: paid once per finding per command, a
// repeat is free, a different command is charged, and a balance that can't
// cover it is refused with nothing spent.
func TestBillingRepo_ChargeAI_OncePerFindingAndCommand(t *testing.T) {
	svc, orgID, admin, balance := billingFixture(t)
	ctx := context.Background()
	finding, scan := uuid.New(), uuid.New()

	price, paid, bal, err := svc.AIQuote(ctx, orgID, finding, billing.AIFix)
	require.NoError(t, err)
	require.Equal(t, int64(750), price)
	require.False(t, paid)
	require.Equal(t, int64(15_000), bal)

	charged, err := svc.ChargeAI(ctx, orgID, admin.UserID, scan, finding, billing.AIFix)
	require.NoError(t, err)
	require.Equal(t, int64(750), charged)
	require.Equal(t, int64(14_250), balance())

	charged, err = svc.ChargeAI(ctx, orgID, admin.UserID, scan, finding, billing.AIFix)
	require.NoError(t, err)
	require.Zero(t, charged, "asking again is free")
	_, paid, _, err = svc.AIQuote(ctx, orgID, finding, billing.AIFix)
	require.NoError(t, err)
	require.True(t, paid)

	_, err = svc.ChargeAI(ctx, orgID, admin.UserID, scan, finding, billing.AIExplain)
	require.NoError(t, err)
	require.Equal(t, int64(14_100), balance())

	require.NoError(t, svc.AdminAdjust(ctx, admin.UserID, orgID, -14_000, "test"))
	_, err = svc.ChargeAI(ctx, orgID, admin.UserID, scan, uuid.New(), billing.AIRemediate)
	require.Error(t, err, "100 left, 400 needed")
	require.Equal(t, int64(100), balance(), "nothing spent on a refused charge")
}
