package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

type harness struct {
	svc   *Service
	repo  *fakeRepo
	audit *fakeAudit
	now   time.Time
	org   uuid.UUID
	admin domain.Actor
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{repo: newFakeRepo(), audit: &fakeAudit{}, now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC), org: uuid.New()}
	h.svc = NewService(h.repo, fakeProvider{}, h.audit, ModeDemo, nil)
	h.svc.SetClock(func() time.Time { return h.now })
	h.admin = domain.Actor{UserID: uuid.New(), OrgID: h.org, Role: domain.RoleAdmin}
	return h
}

func (h *harness) balance() int64 { return h.repo.balance(h.org, h.now) }

// buy runs a whole demo checkout for itemCode.
func (h *harness) buy(t *testing.T, itemCode string) {
	t.Helper()
	sess, redirect, err := h.svc.StartCheckout(context.Background(), h.admin, itemCode)
	if err != nil {
		t.Fatalf("start checkout %s: %v", itemCode, err)
	}
	if redirect != "/checkout/"+sess.ID.String() {
		t.Fatalf("redirect = %q", redirect)
	}
	if _, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "success"); err != nil {
		t.Fatalf("confirm %s: %v", itemCode, err)
	}
}

// adjustTo sets the balance to exactly n (tests only).
func (h *harness) adjustTo(t *testing.T, n int64) {
	t.Helper()
	if _, err := h.svc.Summary(context.Background(), h.admin); err != nil { // provision the org first
		t.Fatal(err)
	}
	cur := h.balance()
	if cur == n {
		return
	}
	if err := h.svc.AdminAdjust(context.Background(), uuid.New(), h.org, n-cur, "test"); err != nil {
		t.Fatal(err)
	}
}

func chargeReq(engines ...domain.EngineID) ChargeRequest {
	req := ChargeRequest{ScanID: uuid.New(), Trigger: domain.TriggerManual, ActorID: uuid.New()}
	for _, e := range engines {
		req.Lines = append(req.Lines, ChargeLine{JobID: uuid.New(), Engine: e})
	}
	return req
}

func code(err error) string {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func TestNewOrgGetsFreePlanAndFirstGrant(t *testing.T) {
	h := newHarness(t)
	sum, err := h.svc.Summary(context.Background(), h.admin)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Plan.Code != PlanFree || sum.Balance != 15_000 || sum.PlanGrantRemaining != 15_000 {
		t.Fatalf("summary = %+v", sum)
	}
	// Looking again must not grant again.
	if _, err := h.svc.Summary(context.Background(), h.admin); err != nil {
		t.Fatal(err)
	}
	if n := len(h.repo.ledgerOf(h.org, KindGrant)); n != 1 {
		t.Fatalf("grants = %d, want 1", n)
	}
}

func TestCharge_ExactBalanceSucceeds(t *testing.T) {
	h := newHarness(t)
	h.adjustTo(t, 20_500)
	if err := h.svc.Charge(context.Background(), h.org, chargeReq(FullScanEngines...)); err != nil {
		t.Fatal(err)
	}
	if h.balance() != 0 {
		t.Fatalf("balance = %d, want 0", h.balance())
	}
	if n := len(h.repo.ledgerOf(h.org, KindDebit)); n < 6 {
		t.Fatalf("debit rows = %d, want at least one per engine", n)
	}
}

// Near-miss: one token short must be rejected outright, with nothing spent
// — never a partial charge, never a negative balance.
func TestCharge_OneTokenShortIsRejectedAndNothingSpent(t *testing.T) {
	h := newHarness(t)
	h.adjustTo(t, 20_499)
	err := h.svc.Charge(context.Background(), h.org, chargeReq(FullScanEngines...))
	if code(err) != "billing.insufficient_tokens" {
		t.Fatalf("err = %v, want billing.insufficient_tokens", err)
	}
	var appErr *apperrors.Error
	errors.As(err, &appErr)
	if appErr.Extensions["shortfall"] != int64(1) || appErr.Extensions["required"] != int64(20_500) {
		t.Errorf("extensions = %v", appErr.Extensions)
	}
	if h.balance() != 20_499 {
		t.Fatalf("balance changed to %d", h.balance())
	}
	if n := len(h.repo.ledgerOf(h.org, KindDebit)); n != 0 {
		t.Fatalf("%d debit rows written for a rejected charge", n)
	}
}

func TestCharge_FreePlanPentestIsPlanRequiredNotInsufficient(t *testing.T) {
	h := newHarness(t)
	h.adjustTo(t, 1_000_000)
	err := h.svc.Charge(context.Background(), h.org, chargeReq(domain.EnginePentest))
	if code(err) != "billing.plan_required" {
		t.Fatalf("err = %v, want billing.plan_required", err)
	}
	h.buy(t, string(PlanProMonthly))
	if err := h.svc.Charge(context.Background(), h.org, chargeReq(domain.EnginePentest)); err != nil {
		t.Fatalf("Pro pentest: %v", err)
	}
}

func TestCharge_IsIdempotent(t *testing.T) {
	h := newHarness(t)
	req := chargeReq(domain.EngineDepScan)
	for range 3 {
		if err := h.svc.Charge(context.Background(), h.org, req); err != nil {
			t.Fatal(err)
		}
	}
	if h.balance() != 15_000-1_500 {
		t.Fatalf("balance = %d, charged more than once", h.balance())
	}
}

func TestCharge_DrainsSoonestExpiringFirstAndSplitsAcrossBuckets(t *testing.T) {
	h := newHarness(t)
	h.buy(t, "topup_100k") // expires in a year; the 15k free grant expires in a month
	if err := h.svc.Charge(context.Background(), h.org, chargeReq(domain.EngineDocReview, domain.EngineDocReview, domain.EngineDocReview)); err != nil {
		t.Fatal(err) // 18,000: all 15,000 of the grant, then 3,000 of the top-up
	}
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.PlanGrantRemaining != 0 || sum.TopupRemaining != 97_000 {
		t.Fatalf("grant left %d, top-up left %d", sum.PlanGrantRemaining, sum.TopupRemaining)
	}
	// The third job straddled both buckets: one debit row per bucket.
	var rowsPerJob = map[uuid.UUID]int{}
	for _, e := range h.repo.ledgerOf(h.org, KindDebit) {
		rowsPerJob[*e.ScanJobID]++
	}
	split := 0
	for _, n := range rowsPerJob {
		if n == 2 {
			split++
		}
	}
	if split != 1 {
		t.Fatalf("expected exactly one job split across two buckets, got %v", rowsPerJob)
	}
}

func TestCharge_ConcurrentScansCannotOverspend(t *testing.T) {
	h := newHarness(t)
	h.adjustTo(t, 100_000)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 20,000 each: codescan 5,000 ×4
			err := h.svc.Charge(context.Background(), h.org, chargeReq(domain.EngineCodeScan, domain.EngineCodeScan, domain.EngineCodeScan, domain.EngineCodeScan))
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 5 || h.balance() != 0 {
		t.Fatalf("%d charges succeeded, balance %d; want 5 and 0", ok, h.balance())
	}
}

func TestRefundJob(t *testing.T) {
	h := newHarness(t)
	req := chargeReq(domain.EngineContainerScan, domain.EngineDepScan)
	if err := h.svc.Charge(context.Background(), h.org, req); err != nil {
		t.Fatal(err)
	}
	if h.balance() != 15_000-5_500 {
		t.Fatalf("after charge: %d", h.balance())
	}
	container := req.Lines[0].JobID
	for range 2 { // the second call must be a no-op
		if err := h.svc.RefundJob(context.Background(), container, "skipped"); err != nil {
			t.Fatal(err)
		}
	}
	if h.balance() != 15_000-1_500 {
		t.Fatalf("after refund: %d, want %d", h.balance(), 15_000-1_500)
	}
	refunds := h.repo.ledgerOf(h.org, KindRefund)
	if len(refunds) != 1 || refunds[0].Reason != "skipped" || *refunds[0].Engine != domain.EngineContainerScan {
		t.Fatalf("refund rows = %+v", refunds)
	}
	// A job that was never charged: nothing happens.
	if err := h.svc.RefundJob(context.Background(), uuid.New(), "skipped"); err != nil {
		t.Fatal(err)
	}
	st, _ := h.svc.ScanTokens(context.Background(), h.admin, req.ScanID)
	if st.Charged != 5_500 || st.Refunded != 4_000 {
		t.Fatalf("scan tokens = %+v", st)
	}
}

func TestBuyProMonthly(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProMonthly))
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanProMonthly || sum.Balance != 500_000 {
		t.Fatalf("summary = plan %s balance %d", sum.Plan.Code, sum.Balance)
	}
	if !sum.PeriodEnd.Equal(h.now.AddDate(0, 1, 0)) {
		t.Fatalf("period end = %v", sum.PeriodEnd)
	}
	// The unused free grant is replaced, not added on top.
	if n := len(h.repo.ledgerOf(h.org, KindExpire)); n != 1 {
		t.Fatalf("expire rows = %d", n)
	}
}

func TestSamePlanIsLockedUntilPeriodEnds(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProMonthly))
	_, _, err := h.svc.StartCheckout(context.Background(), h.admin, string(PlanProMonthly))
	if code(err) != "billing.already_subscribed" {
		t.Fatalf("err = %v, want billing.already_subscribed", err)
	}
	// Upgrading to annual is allowed.
	h.buy(t, string(PlanProAnnual))
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanProAnnual || !sum.PeriodEnd.Equal(h.now.AddDate(1, 0, 0)) {
		t.Fatalf("after upgrade: %s until %v", sum.Plan.Code, sum.PeriodEnd)
	}
	// And annual can't be "downgraded" to monthly mid-year.
	if _, _, err := h.svc.StartCheckout(context.Background(), h.admin, string(PlanProMonthly)); code(err) != "billing.already_subscribed" {
		t.Fatalf("err = %v", err)
	}
}

func TestConfirmIsIdempotent(t *testing.T) {
	h := newHarness(t)
	sess, _, err := h.svc.StartCheckout(context.Background(), h.admin, "topup_100k")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "success"); err != nil {
			t.Fatal(err)
		}
	}
	if h.balance() != 115_000 {
		t.Fatalf("balance = %d, top-up applied twice?", h.balance())
	}
}

func TestDeclinedCheckoutChargesNothing(t *testing.T) {
	h := newHarness(t)
	sess, _, _ := h.svc.StartCheckout(context.Background(), h.admin, string(PlanProMonthly))
	_, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "declined")
	if code(err) != "billing.payment_declined" {
		t.Fatalf("err = %v", err)
	}
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanFree || sum.Balance != 15_000 {
		t.Fatalf("declined payment changed the plan: %+v", sum)
	}
	// A failed session can't be confirmed afterwards.
	if _, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "success"); code(err) != "billing.checkout_closed" {
		t.Fatalf("err = %v", err)
	}
}

func TestExpiredCheckoutCannotBePaid(t *testing.T) {
	h := newHarness(t)
	sess, _, _ := h.svc.StartCheckout(context.Background(), h.admin, "topup_100k")
	h.now = h.now.Add(31 * time.Minute)
	if _, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "success"); code(err) != "billing.checkout_expired" {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckoutAuthorization(t *testing.T) {
	h := newHarness(t)
	member := h.admin
	member.Role = domain.RoleMember
	if _, _, err := h.svc.StartCheckout(context.Background(), member, "topup_100k"); code(err) != "billing.admin_required" {
		t.Fatalf("member: err = %v", err)
	}
	sess, _, _ := h.svc.StartCheckout(context.Background(), h.admin, "topup_100k")
	other := domain.Actor{UserID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	if _, err := h.svc.ConfirmDemo(context.Background(), other, sess.ID, "success"); code(err) != "billing.checkout_not_found" {
		t.Fatalf("other org: err = %v, want not found", err)
	}
	if _, err := h.svc.GetCheckout(context.Background(), other, sess.ID); code(err) != "billing.checkout_not_found" {
		t.Fatalf("other org get: err = %v, want not found", err)
	}
}

func TestConfirmOnlyInDemoMode(t *testing.T) {
	h := newHarness(t)
	sess, _, _ := h.svc.StartCheckout(context.Background(), h.admin, "topup_100k")
	h.svc.mode = ModeStripe
	if _, err := h.svc.ConfirmDemo(context.Background(), h.admin, sess.ID, "success"); code(err) != "billing.checkout_not_found" {
		t.Fatalf("err = %v, want 404 outside demo mode", err)
	}
}

func TestAnnualTopupDiscount(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProAnnual))
	sess, _, err := h.svc.StartCheckout(context.Background(), h.admin, "topup_100k")
	if err != nil {
		t.Fatal(err)
	}
	if sess.AmountCents != 810 {
		t.Fatalf("annual top-up price = %d, want 810 (10%% off 900)", sess.AmountCents)
	}
}

func TestAnnualGetsMonthlyGrantsNotAYearUpFront(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProAnnual))
	if h.balance() != 500_000 {
		t.Fatalf("annual start balance = %d, want 500,000 (one month)", h.balance())
	}
	for month := 1; month <= 11; month++ {
		h.now = h.now.AddDate(0, 1, 0)
		if _, err := h.svc.RenewDue(context.Background()); err != nil {
			t.Fatal(err)
		}
		if h.balance() != 500_000 {
			t.Fatalf("month %d: balance = %d", month, h.balance())
		}
	}
	// Running the same tick again changes nothing.
	grants := len(h.repo.ledgerOf(h.org, KindGrant))
	_, _ = h.svc.RenewDue(context.Background())
	if len(h.repo.ledgerOf(h.org, KindGrant)) != grants {
		t.Fatal("a repeated tick granted again")
	}
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanProAnnual {
		t.Fatalf("plan = %s before the year is up", sum.Plan.Code)
	}
	// After the year, with no card to charge, back to Free.
	h.now = h.now.AddDate(0, 1, 0)
	_, _ = h.svc.RenewDue(context.Background())
	sum, _ = h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanFree || sum.Balance != 15_000 {
		t.Fatalf("after the year: %s, %d", sum.Plan.Code, sum.Balance)
	}
}

func TestLeftoverPlanTokensExpireButTopupsSurvive(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProMonthly))
	h.buy(t, "topup_100k")
	if err := h.svc.Charge(context.Background(), h.org, chargeReq(domain.EngineCodeScan)); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.AddDate(0, 1, 0)
	_, _ = h.svc.RenewDue(context.Background())
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	// Monthly Pro ended (demo mode doesn't auto-charge): Free grant + the top-up.
	if sum.Plan.Code != PlanFree || sum.PlanGrantRemaining != 15_000 || sum.TopupRemaining != 100_000 {
		t.Fatalf("after month: plan %s grant %d topup %d", sum.Plan.Code, sum.PlanGrantRemaining, sum.TopupRemaining)
	}
}

func TestAdvanceCycleSimulatesPaidRenewal(t *testing.T) {
	h := newHarness(t)
	h.buy(t, string(PlanProMonthly))
	if err := h.svc.Charge(context.Background(), h.org, chargeReq(FullScanEngines...)); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.AdminAdvanceCycle(context.Background(), uuid.New(), h.org); err != nil {
		t.Fatal(err)
	}
	sum, _ := h.svc.Summary(context.Background(), h.admin)
	if sum.Plan.Code != PlanProMonthly || sum.Balance != 500_000 {
		t.Fatalf("after advance: %s, %d", sum.Plan.Code, sum.Balance)
	}
	if !sum.PeriodEnd.After(h.now) {
		t.Fatalf("period end %v not extended", sum.PeriodEnd)
	}
	h.svc.mode = ModeStripe
	if err := h.svc.AdminAdvanceCycle(context.Background(), uuid.New(), h.org); code(err) != "billing.not_found" {
		t.Fatalf("advance outside demo: %v", err)
	}
}

func TestCancelAndResume(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.SetCancelAtPeriodEnd(context.Background(), h.admin, true); code(err) != "billing.nothing_to_cancel" {
		t.Fatalf("free cancel: %v", err)
	}
	h.buy(t, string(PlanProMonthly))
	sum, err := h.svc.SetCancelAtPeriodEnd(context.Background(), h.admin, true)
	if err != nil || !sum.CancelAtPeriodEnd || sum.Plan.Code != PlanProMonthly {
		t.Fatalf("cancel: %+v %v", sum, err)
	}
	sum, _ = h.svc.SetCancelAtPeriodEnd(context.Background(), h.admin, false)
	if sum.CancelAtPeriodEnd {
		t.Fatal("resume did not clear cancel")
	}
}

func TestLiveScanAllowed(t *testing.T) {
	h := newHarness(t)
	ok, reason, _ := h.svc.LiveScanAllowed(context.Background(), h.org, TypicalLiveEngines, 10)
	if ok || reason != "plan_required" {
		t.Fatalf("free: ok=%v reason=%q", ok, reason)
	}
	h.buy(t, string(PlanProMonthly))
	h.adjustTo(t, 53_250) // floor is 10% of 500k = 50,000; a push costs 3,250
	if ok, _, _ := h.svc.LiveScanAllowed(context.Background(), h.org, TypicalLiveEngines, 10); !ok {
		t.Fatal("exactly at the floor after paying should be allowed")
	}
	h.adjustTo(t, 53_249)
	ok, reason, _ = h.svc.LiveScanAllowed(context.Background(), h.org, TypicalLiveEngines, 10)
	if ok || reason != "insufficient_tokens" {
		t.Fatalf("below floor: ok=%v reason=%q", ok, reason)
	}
}

func TestRequireFeature(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.RequireFeature(context.Background(), h.org, FeatureSchedules); code(err) != "billing.plan_required" {
		t.Fatalf("free schedules: %v", err)
	}
	h.buy(t, string(PlanProMonthly))
	if err := h.svc.RequireFeature(context.Background(), h.org, FeatureLiveScanning); err != nil {
		t.Fatal(err)
	}
}

func TestEstimate(t *testing.T) {
	h := newHarness(t)
	est, err := h.svc.Estimate(context.Background(), h.org, FullScanEngines, "", domain.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if est.Quote.Total != 20_500 || est.Affordable || est.BalanceAfter != -5_500 || !est.PlanAllowed {
		t.Fatalf("estimate = %+v", est)
	}
	est, _ = h.svc.Estimate(context.Background(), h.org, []domain.EngineID{domain.EnginePentest}, "", domain.TriggerManual)
	if est.PlanAllowed || est.RequiredPlan != PlanProMonthly {
		t.Fatalf("pentest on free: %+v", est)
	}
	if h.balance() != 15_000 {
		t.Fatal("estimate spent tokens")
	}
}

func TestSharedProjectSessionHasNoBilling(t *testing.T) {
	h := newHarness(t)
	pid := uuid.New()
	collab := domain.Actor{UserID: uuid.New(), OrgID: h.org, Role: domain.RoleMember, ProjectID: &pid}
	if _, err := h.svc.Summary(context.Background(), collab); code(err) != "billing.not_available" {
		t.Fatalf("err = %v", err)
	}
}

func TestAllowedEnginesDropsPentestOnFree(t *testing.T) {
	h := newHarness(t)
	all := append(append([]domain.EngineID{}, FullScanEngines...), domain.EnginePentest)
	got, err := h.svc.AllowedEngines(context.Background(), h.org, all)
	if err != nil || len(got) != 6 {
		t.Fatalf("got %v, %v", got, err)
	}
}
