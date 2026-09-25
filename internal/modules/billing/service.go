package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// Auditor is the subset of audit.Service billing writes to.
type Auditor interface {
	Log(ctx context.Context, e audit.Entry)
}

// Feature names for RequireFeature.
const (
	FeatureLiveScanning = "live_scanning"
	FeatureSchedules    = "schedules"
)

// Service is the billing business logic. All balance changes happen inside
// Repository.WithOrgLock, so they're atomic and serialised per org.
type Service struct {
	repo     Repository
	provider PaymentProvider
	audit    Auditor
	mode     Mode
	log      *slog.Logger
	now      func() time.Time
}

func NewService(repo Repository, provider PaymentProvider, auditor Auditor, mode Mode, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, provider: provider, audit: auditor, mode: mode, log: log,
		now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the clock — tests only.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Mode reports GUARDPIPE_BILLING_MODE.
func (s *Service) Mode() Mode { return s.mode }

func addMonths(t time.Time, n int) time.Time { return t.AddDate(0, n, 0) }

func periodEnd(start time.Time, p Period) time.Time {
	if p == PeriodYear {
		return start.AddDate(1, 0, 0)
	}
	return addMonths(start, 1)
}

func freshSubscription(orgID uuid.UUID, now time.Time) Subscription {
	return Subscription{
		OrgID: orgID, PlanCode: PlanFree, Status: StatusActive,
		PeriodStart: now, PeriodEnd: addMonths(now, 1), NextGrantAt: addMonths(now, 1),
	}
}

// withOrg locks the org and, the first time billing ever sees it, gives it
// a free subscription and its first monthly grant ("lazy provisioning" —
// no registration hook or backfill needed).
func (s *Service) withOrg(ctx context.Context, orgID uuid.UUID, fn func(tx Tx, sub *Subscription, now time.Time) error) error {
	now := s.now()
	return s.repo.WithOrgLock(ctx, orgID, freshSubscription(orgID, now), func(tx Tx, sub *Subscription, created bool) error {
		if created {
			if err := s.grantPlanTokens(ctx, tx, sub, now, "grant:"+orgID.String()+":initial", "welcome"); err != nil {
				return err
			}
		}
		return fn(tx, sub, now)
	})
}

func planOf(sub *Subscription) Plan {
	if p, ok := Plans[sub.PlanCode]; ok {
		return p
	}
	return Plans[PlanFree]
}

func requireOrgBilling(actor domain.Actor) error {
	if actor.ProjectID != nil {
		// A single-project collaborator session can't see or change the
		// owning organisation's billing.
		return apperrors.Forbidden("billing.not_available", "billing isn't available in a shared-project session")
	}
	return nil
}

func requireBillingAdmin(actor domain.Actor) error {
	if err := requireOrgBilling(actor); err != nil {
		return err
	}
	if actor.Role != domain.RoleAdmin {
		return apperrors.Forbidden("billing.admin_required", "only an organisation admin can manage billing")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Balance changes. Every one is idempotent on its ledger key.

// addBucket creates a new bucket and its ledger row, unless key was already
// written (then it does nothing).
func (s *Service) addBucket(ctx context.Context, tx Tx, now time.Time, b Bucket, e LedgerEntry) error {
	exists, err := tx.LedgerKeyExists(ctx, e.IdempotencyKey)
	if err != nil || exists {
		return err
	}
	b.ID = id.New()
	b.Remaining = b.Amount
	if err := tx.InsertBucket(ctx, &b); err != nil {
		return err
	}
	bal, err := tx.Balance(ctx, now)
	if err != nil {
		return err
	}
	e.ID, e.OrgID, e.Delta, e.BalanceAfter, e.BucketID = id.New(), b.OrgID, b.Amount, bal, &b.ID
	_, err = tx.InsertLedger(ctx, &e)
	return err
}

// moveTokens changes one bucket by delta and writes its ledger row, unless
// key was already written.
func (s *Service) moveTokens(ctx context.Context, tx Tx, now time.Time, bucketID uuid.UUID, delta int64, e LedgerEntry) (bool, error) {
	exists, err := tx.LedgerKeyExists(ctx, e.IdempotencyKey)
	if err != nil || exists {
		return false, err
	}
	if err := tx.AddToBucket(ctx, bucketID, delta); err != nil {
		return false, err
	}
	bal, err := tx.Balance(ctx, now)
	if err != nil {
		return false, err
	}
	e.ID, e.Delta, e.BalanceAfter, e.BucketID = id.New(), delta, bal, &bucketID
	return tx.InsertLedger(ctx, &e)
}

// expireBucket zeroes a bucket's leftover tokens, recording them as expired.
func (s *Service) expireBucket(ctx context.Context, tx Tx, now time.Time, b Bucket, reason string) error {
	if b.Remaining <= 0 {
		return nil
	}
	_, err := s.moveTokens(ctx, tx, now, b.ID, -b.Remaining, LedgerEntry{
		OrgID: b.OrgID, Kind: KindExpire, Reason: reason,
		IdempotencyKey: fmt.Sprintf("expire:%s:%d", b.ID, now.UnixNano()),
	})
	return err
}

// grantPlanTokens replaces whatever is left of the previous plan grant with
// this plan's fresh monthly grant, valid until the next grant is due.
func (s *Service) grantPlanTokens(ctx context.Context, tx Tx, sub *Subscription, now time.Time, key, reason string) error {
	if exists, err := tx.LedgerKeyExists(ctx, key); err != nil || exists {
		return err
	}
	old, err := tx.BucketsBySource(ctx, SourcePlanGrant)
	if err != nil {
		return err
	}
	for _, b := range old {
		if err := s.expireBucket(ctx, tx, now, b, "plan_period_ended"); err != nil {
			return err
		}
	}
	plan := planOf(sub)
	expires := sub.NextGrantAt
	if !expires.After(now) {
		expires = addMonths(now, 1)
	}
	return s.addBucket(ctx, tx, now,
		Bucket{OrgID: sub.OrgID, Source: SourcePlanGrant, Amount: plan.MonthlyGrant, ExpiresAt: expires},
		LedgerEntry{Kind: KindGrant, Reason: reason, IdempotencyKey: key})
}

// ---------------------------------------------------------------------------
// Reads.

// Catalog is the public price list.
func (s *Service) Catalog() CatalogView { return BuildCatalog() }

func (s *Service) summaryLocked(ctx context.Context, tx Tx, sub *Subscription, now time.Time) (*Summary, error) {
	buckets, err := tx.SpendableBuckets(ctx, now)
	if err != nil {
		return nil, err
	}
	out := &Summary{
		Plan: planOf(sub), Status: sub.Status, PeriodStart: sub.PeriodStart, PeriodEnd: sub.PeriodEnd,
		NextGrantAt: sub.NextGrantAt, CancelAtPeriodEnd: sub.CancelAtPeriodEnd, Mode: s.mode,
	}
	for _, b := range buckets {
		out.Balance += b.Remaining
		if b.Source == SourcePlanGrant {
			out.PlanGrantRemaining += b.Remaining
			continue
		}
		out.TopupRemaining += b.Remaining
		if out.TopupExpiresAt == nil || b.ExpiresAt.Before(*out.TopupExpiresAt) {
			t := b.ExpiresAt
			out.TopupExpiresAt = &t
		}
	}
	usage, err := tx.UsageSince(ctx, addMonths(sub.NextGrantAt, -1))
	if err != nil {
		return nil, err
	}
	out.Usage = usage
	for _, u := range usage {
		out.UsedThisCycle += u.Tokens
	}
	return out, nil
}

// Summary is the caller's org's plan, balance and usage this cycle.
func (s *Service) Summary(ctx context.Context, actor domain.Actor) (*Summary, error) {
	if err := requireOrgBilling(actor); err != nil {
		return nil, err
	}
	var out *Summary
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, sub *Subscription, now time.Time) error {
		var err error
		out, err = s.summaryLocked(ctx, tx, sub, now)
		return err
	})
	if err != nil {
		return nil, wrapInternal(err)
	}
	return out, nil
}

// Ledger pages through the caller's org's history, newest first.
func (s *Service) Ledger(ctx context.Context, actor domain.Actor, before *time.Time, limit int) ([]LedgerEntry, error) {
	if err := requireOrgBilling(actor); err != nil {
		return nil, err
	}
	return s.ledger(ctx, actor.OrgID, before, limit)
}

func (s *Service) ledger(ctx context.Context, orgID uuid.UUID, before *time.Time, limit int) ([]LedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var out []LedgerEntry
	err := s.withOrg(ctx, orgID, func(tx Tx, _ *Subscription, _ time.Time) error {
		var err error
		out, err = tx.ListLedger(ctx, before, limit)
		return err
	})
	return out, wrapInternal(err)
}

// ScanTokens is what one scan cost the caller's org. Another org's scan
// simply has no rows here, so nothing leaks.
func (s *Service) ScanTokens(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*ScanTokens, error) {
	if err := requireOrgBilling(actor); err != nil {
		return nil, err
	}
	out := &ScanTokens{}
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, _ *Subscription, _ time.Time) error {
		entries, err := tx.LedgerForScan(ctx, scanID)
		if err != nil {
			return err
		}
		out.Entries = entries
		for _, e := range entries {
			switch e.Kind {
			case KindDebit:
				out.Charged -= e.Delta
			case KindRefund:
				out.Refunded += e.Delta
			}
		}
		return nil
	})
	return out, wrapInternal(err)
}

// Estimate prices a scan against the org's balance and plan, without
// charging anything.
func (s *Service) Estimate(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID, preset domain.PentestPreset, trigger domain.TriggerSource) (*Estimate, error) {
	q, err := QuoteScan(engines, preset, trigger)
	if err != nil {
		return nil, apperrors.Validation("billing.invalid_engines", err.Error(), nil)
	}
	out := &Estimate{Quote: q}
	err = s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
		plan := planOf(sub)
		out.BlockedEngines = blockedEngines(plan, engines)
		out.PlanAllowed = len(out.BlockedEngines) == 0
		if !out.PlanAllowed {
			out.RequiredPlan = PlanProMonthly
		}
		bal, err := tx.Balance(ctx, now)
		if err != nil {
			return err
		}
		out.Balance = bal
		out.BalanceAfter = bal - q.Total
		out.Affordable = bal >= q.Total
		return nil
	})
	if err != nil {
		return nil, wrapInternal(err)
	}
	return out, nil
}

func blockedEngines(plan Plan, engines []domain.EngineID) []domain.EngineID {
	var out []domain.EngineID
	for _, e := range engines {
		if !plan.Allows(e) {
			out = append(out, e)
		}
	}
	return out
}

// AllowedEngines filters engines down to what the org's plan may run —
// used for "run everything available" scans, so a Free org's full scan
// quietly leaves pentest out instead of failing.
func (s *Service) AllowedEngines(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID) ([]domain.EngineID, error) {
	var out []domain.EngineID
	err := s.withOrg(ctx, orgID, func(_ Tx, sub *Subscription, _ time.Time) error {
		plan := planOf(sub)
		for _, e := range engines {
			if plan.Allows(e) {
				out = append(out, e)
			}
		}
		return nil
	})
	return out, wrapInternal(err)
}

// RequireFeature returns billing.plan_required if the org's plan doesn't
// include feature (FeatureLiveScanning, FeatureSchedules).
func (s *Service) RequireFeature(ctx context.Context, orgID uuid.UUID, feature string) error {
	var ok bool
	err := s.withOrg(ctx, orgID, func(_ Tx, sub *Subscription, _ time.Time) error {
		plan := planOf(sub)
		switch feature {
		case FeatureLiveScanning:
			ok = plan.LiveScanning
		case FeatureSchedules:
			ok = plan.Schedules
		}
		return nil
	})
	if err != nil {
		return wrapInternal(err)
	}
	if !ok {
		return planRequired(featureLabel(feature)+" is a Pro feature", nil)
	}
	return nil
}

func featureLabel(f string) string {
	switch f {
	case FeatureLiveScanning:
		return "Live scanning"
	case FeatureSchedules:
		return "Scheduled scans"
	}
	return f
}

func planRequired(detail string, blocked []domain.EngineID) *apperrors.Error {
	ext := map[string]any{"required_plan": string(PlanProMonthly)}
	if len(blocked) > 0 {
		ext["blocked_engines"] = blocked
	}
	return apperrors.Forbidden("billing.plan_required", detail).WithExtensions(ext)
}

// LiveScanAllowed decides whether a webhook-triggered scan may run: the
// plan must include live scanning, and after paying for it the balance
// must still be at or above floorPercent of the plan's monthly grant.
// reason is "plan_required" or "insufficient_tokens" when ok is false.
func (s *Service) LiveScanAllowed(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID, floorPercent int) (ok bool, reason string, err error) {
	q, qerr := QuoteScan(engines, "", domain.TriggerWebhookPush)
	if qerr != nil {
		return false, "", qerr
	}
	err = s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
		plan := planOf(sub)
		if !plan.LiveScanning {
			reason = "plan_required"
			return nil
		}
		bal, err := tx.Balance(ctx, now)
		if err != nil {
			return err
		}
		floor := plan.MonthlyGrant * int64(floorPercent) / 100
		if bal-q.Total < floor {
			reason = "insufficient_tokens"
			return nil
		}
		ok = true
		return nil
	})
	return ok, reason, err
}

// ---------------------------------------------------------------------------
// Charging and refunds.

// Charge pays for a scan before it's created: all of its engines, or none.
// The price comes from the catalogue. Returns billing.plan_required (403)
// or billing.insufficient_tokens (402); the balance can never go negative.
// Calling it again for the same jobs does nothing.
func (s *Service) Charge(ctx context.Context, orgID uuid.UUID, req ChargeRequest) error {
	if len(req.Lines) == 0 {
		return nil
	}
	engines := make([]domain.EngineID, len(req.Lines))
	for i, l := range req.Lines {
		engines[i] = l.Engine
	}
	q, err := QuoteScan(engines, req.Preset, req.Trigger)
	if err != nil {
		return apperrors.Internal(err)
	}
	var actorID *uuid.UUID
	if req.ActorID != uuid.Nil {
		a := req.ActorID
		actorID = &a
	}
	scanID := req.ScanID

	err = s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
		already, err := tx.DebitsForJob(ctx, req.Lines[0].JobID)
		if err != nil {
			return err
		}
		if len(already) > 0 {
			return nil // a retry of a charge that already went through
		}
		if blocked := blockedEngines(planOf(sub), engines); len(blocked) > 0 {
			return planRequired(fmt.Sprintf("%s needs the Pro plan", joinEngines(blocked)), blocked)
		}
		buckets, err := tx.SpendableBuckets(ctx, now)
		if err != nil {
			return err
		}
		var balance int64
		for _, b := range buckets {
			balance += b.Remaining
		}
		if balance < q.Total {
			return apperrors.PaymentRequired("billing.insufficient_tokens",
				fmt.Sprintf("this scan needs %d tokens and %d are left", q.Total, balance)).
				WithExtensions(map[string]any{"required": q.Total, "available": balance, "shortfall": q.Total - balance})
		}
		for i, line := range req.Lines {
			owed := q.Lines[i].Tokens
			for bi := range buckets {
				if owed == 0 {
					break
				}
				b := &buckets[bi]
				take := min(b.Remaining, owed)
				if take == 0 {
					continue
				}
				jobID, engine := line.JobID, line.Engine
				if _, err := s.moveTokens(ctx, tx, now, b.ID, -take, LedgerEntry{
					OrgID: orgID, Kind: KindDebit, ScanID: &scanID, ScanJobID: &jobID, Engine: &engine,
					TriggerSource: string(req.Trigger), PriceVersion: PriceVersion, ActorUserID: actorID,
					IdempotencyKey: fmt.Sprintf("debit:%s:%s", jobID, b.ID),
				}); err != nil {
					return err
				}
				b.Remaining -= take
				owed -= take
			}
		}
		return nil
	})
	return wrapInternal(err)
}

// aiKey is the ledger idempotency key marking "this finding's command is
// paid for" — the first bucket debit carries it; any further buckets the
// same charge drains get a numbered suffix.
func aiKey(findingID uuid.UUID, action AIAction) string {
	return fmt.Sprintf("ai:%s:%s", findingID, action)
}

// AIQuote answers, before any model call: what does this command cost,
// has this org already paid for it on this finding (then it's free), and
// how many tokens are left.
func (s *Service) AIQuote(ctx context.Context, orgID, findingID uuid.UUID, action AIAction) (price int64, paid bool, balance int64, err error) {
	price, ok := AIPrice[action]
	if !ok {
		return 0, false, 0, apperrors.Validation("billing.unknown_ai_action", "unknown AI command", nil)
	}
	err = s.withOrg(ctx, orgID, func(tx Tx, _ *Subscription, now time.Time) error {
		var err error
		if paid, err = tx.LedgerKeyExists(ctx, aiKey(findingID, action)); err != nil {
			return err
		}
		balance, err = tx.Balance(ctx, now)
		return err
	})
	return price, paid, balance, wrapInternal(err)
}

// ChargeAI debits a finding-assistant command, once per finding per
// command (idempotent: a repeat returns charged=0). Called only after the
// model produced a usable answer, so a failed or discarded call is never
// paid for. All-or-nothing: 402 if the balance can't cover it.
func (s *Service) ChargeAI(ctx context.Context, orgID, actorID, scanID, findingID uuid.UUID, action AIAction) (charged int64, err error) {
	price, ok := AIPrice[action]
	if !ok {
		return 0, apperrors.Validation("billing.unknown_ai_action", "unknown AI command", nil)
	}
	var actor *uuid.UUID
	if actorID != uuid.Nil {
		actor = &actorID
	}
	key := aiKey(findingID, action)
	err = s.withOrg(ctx, orgID, func(tx Tx, _ *Subscription, now time.Time) error {
		if paid, err := tx.LedgerKeyExists(ctx, key); err != nil || paid {
			return err
		}
		buckets, err := tx.SpendableBuckets(ctx, now)
		if err != nil {
			return err
		}
		var balance int64
		for _, b := range buckets {
			balance += b.Remaining
		}
		if balance < price {
			return apperrors.PaymentRequired("billing.insufficient_tokens",
				fmt.Sprintf("this needs %d tokens and %d are left", price, balance)).
				WithExtensions(map[string]any{"required": price, "available": balance, "shortfall": price - balance})
		}
		owed := price
		for i := range buckets {
			if owed == 0 {
				break
			}
			take := min(buckets[i].Remaining, owed)
			if take == 0 {
				continue
			}
			k := key
			if owed != price {
				k = fmt.Sprintf("%s:%d", key, i)
			}
			if _, err := s.moveTokens(ctx, tx, now, buckets[i].ID, -take, LedgerEntry{
				OrgID: orgID, Kind: KindDebit, ScanID: &scanID, PriceVersion: PriceVersion,
				Reason: "ai_assist:" + string(action), ActorUserID: actor, IdempotencyKey: k,
			}); err != nil {
				return err
			}
			owed -= take
		}
		charged = price
		return nil
	})
	return charged, wrapInternal(err)
}

func joinEngines(es []domain.EngineID) string {
	out := ""
	for i, e := range es {
		if i > 0 {
			out += ", "
		}
		out += string(e)
	}
	return out
}

// RefundJob gives back everything a scan job was charged, into the same
// buckets it came from. A job that was never charged, or was already
// refunded, is a no-op.
func (s *Service) RefundJob(ctx context.Context, jobID uuid.UUID, reason string) error {
	orgID, ok, err := s.repo.OrgForJob(ctx, jobID)
	if err != nil || !ok {
		return err
	}
	return s.withOrg(ctx, orgID, func(tx Tx, _ *Subscription, now time.Time) error {
		debits, err := tx.DebitsForJob(ctx, jobID)
		if err != nil {
			return err
		}
		for _, d := range debits {
			if d.BucketID == nil {
				continue
			}
			if _, err := s.moveTokens(ctx, tx, now, *d.BucketID, -d.Delta, LedgerEntry{
				OrgID: orgID, Kind: KindRefund, ScanID: d.ScanID, ScanJobID: d.ScanJobID, Engine: d.Engine,
				TriggerSource: d.TriggerSource, Reason: reason, PriceVersion: d.PriceVersion,
				IdempotencyKey: fmt.Sprintf("refund:%s:%s", jobID, *d.BucketID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Buying.

// StartCheckout opens a checkout session for a plan or a top-up pack and
// returns where to send the buyer.
func (s *Service) StartCheckout(ctx context.Context, actor domain.Actor, itemCode string) (*CheckoutSession, string, error) {
	if err := requireBillingAdmin(actor); err != nil {
		return nil, "", err
	}
	if s.mode == ModeOff {
		return nil, "", apperrors.NotFound("billing.disabled", "billing is turned off")
	}
	var sess *CheckoutSession
	var redirect string
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, sub *Subscription, now time.Time) error {
		amount, err := priceItem(itemCode, sub, now)
		if err != nil {
			return err
		}
		createdBy := actor.UserID
		sess = &CheckoutSession{
			ID: id.New(), OrgID: actor.OrgID, CreatedBy: &createdBy, ItemCode: itemCode,
			AmountCents: amount, Currency: "usd", Provider: s.provider.Name(), Status: CheckoutPending,
			ExpiresAt: now.Add(checkoutTTL), CreatedAt: now,
		}
		redirect, sess.ProviderRef, err = s.provider.CreateSession(ctx, *sess)
		if err != nil {
			return apperrors.External("billing.provider_error", "the payment provider could not start a checkout", err)
		}
		return tx.InsertCheckout(ctx, sess)
	})
	if err != nil {
		return nil, "", wrapInternal(err)
	}
	return sess, redirect, nil
}

// priceItem validates a purchase and returns its price in cents.
func priceItem(code string, sub *Subscription, now time.Time) (int, error) {
	if plan, ok := Plans[PlanCode(code)]; ok {
		if plan.Code == PlanFree {
			return 0, apperrors.Validation("billing.invalid_item", "the Free plan can't be bought", nil)
		}
		active := sub.Status == StatusActive && now.Before(sub.PeriodEnd)
		if active && sub.PlanCode == plan.Code {
			return 0, apperrors.Conflict("billing.already_subscribed",
				fmt.Sprintf("you're already on %s until %s", plan.Name, sub.PeriodEnd.Format("2 Jan 2006"))).
				WithExtensions(map[string]any{"period_end": sub.PeriodEnd})
		}
		if active && sub.PlanCode == PlanProAnnual && plan.Code == PlanProMonthly {
			return 0, apperrors.Conflict("billing.already_subscribed",
				fmt.Sprintf("your annual plan already covers this until %s", sub.PeriodEnd.Format("2 Jan 2006"))).
				WithExtensions(map[string]any{"period_end": sub.PeriodEnd})
		}
		return plan.PriceCents, nil
	}
	if pack, ok := Packs[code]; ok {
		price := pack.PriceCents
		if d := planOf(sub).TopupDiscount; d > 0 {
			price = price * (100 - d) / 100
		}
		return price, nil
	}
	return 0, apperrors.Validation("billing.invalid_item", "unknown plan or pack", nil)
}

// GetCheckout returns one of the caller's org's checkout sessions.
func (s *Service) GetCheckout(ctx context.Context, actor domain.Actor, sessionID uuid.UUID) (*CheckoutSession, error) {
	if err := requireBillingAdmin(actor); err != nil {
		return nil, err
	}
	var sess *CheckoutSession
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, _ *Subscription, now time.Time) error {
		var err error
		if sess, err = tx.GetCheckout(ctx, sessionID); err != nil {
			return err
		}
		if sess.Status == CheckoutPending && !now.Before(sess.ExpiresAt) {
			sess.Status = CheckoutExpired
			return tx.SetCheckoutStatus(ctx, sess.ID, CheckoutExpired, nil)
		}
		return nil
	})
	if err != nil {
		return nil, wrapInternal(err)
	}
	return sess, nil
}

// ConfirmDemo completes a demo-mode checkout. outcome "success" fulfils
// the purchase; "declined" simulates a refused card. No card data is ever
// passed in — there is nothing to pass it through.
func (s *Service) ConfirmDemo(ctx context.Context, actor domain.Actor, sessionID uuid.UUID, outcome string) (*CheckoutSession, error) {
	if s.mode != ModeDemo {
		return nil, apperrors.NotFound("billing.checkout_not_found", "checkout not found")
	}
	if err := requireBillingAdmin(actor); err != nil {
		return nil, err
	}
	var sess *CheckoutSession
	var declined bool
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, sub *Subscription, now time.Time) error {
		var err error
		if sess, err = tx.GetCheckout(ctx, sessionID); err != nil {
			return err
		}
		switch {
		case sess.Status == CheckoutPaid:
			return nil // confirmed twice: the first one already did everything
		case sess.Status != CheckoutPending:
			return apperrors.Conflict("billing.checkout_closed", "this checkout is no longer open")
		case !now.Before(sess.ExpiresAt):
			sess.Status = CheckoutExpired
			if err := tx.SetCheckoutStatus(ctx, sess.ID, CheckoutExpired, nil); err != nil {
				return err
			}
			return apperrors.Conflict("billing.checkout_expired", "this checkout expired — start again from the pricing page")
		case outcome == "declined":
			sess.Status = CheckoutFailed
			declined = true
			return tx.SetCheckoutStatus(ctx, sess.ID, CheckoutFailed, nil)
		}
		return s.fulfil(ctx, tx, sub, sess, now, actor.UserID)
	})
	if err != nil {
		return nil, wrapInternal(err)
	}
	if declined {
		return nil, apperrors.PaymentRequired("billing.payment_declined", "the card was declined (simulated) — nothing was charged")
	}
	return sess, nil
}

// fulfil applies a paid checkout: a plan starts a new period with a fresh
// grant; a pack adds a top-up bucket. The checkout's own status change is
// the idempotency guard (only a pending session gets here).
func (s *Service) fulfil(ctx context.Context, tx Tx, sub *Subscription, sess *CheckoutSession, now time.Time, actorID uuid.UUID) error {
	paidAt := now
	if err := tx.SetCheckoutStatus(ctx, sess.ID, CheckoutPaid, &paidAt); err != nil {
		return err
	}
	sess.Status, sess.PaidAt = CheckoutPaid, &paidAt

	if plan, ok := Plans[PlanCode(sess.ItemCode)]; ok {
		sub.PlanCode, sub.Status = plan.Code, StatusActive
		sub.PeriodStart, sub.PeriodEnd = now, periodEnd(now, plan.Period)
		sub.NextGrantAt, sub.CancelAtPeriodEnd = addMonths(now, 1), false
		if err := tx.UpdateSubscription(ctx, sub); err != nil {
			return err
		}
		if err := s.grantPlanTokens(ctx, tx, sub, now, "grant:purchase:"+sess.ID.String(), "purchase"); err != nil {
			return err
		}
	} else if pack, ok := Packs[sess.ItemCode]; ok {
		checkoutID := sess.ID
		if err := s.addBucket(ctx, tx, now,
			Bucket{OrgID: sub.OrgID, Source: SourceTopup, Amount: pack.Tokens, ExpiresAt: now.Add(TopupValidity), CheckoutID: &checkoutID},
			LedgerEntry{Kind: KindTopup, Reason: pack.Code, IdempotencyKey: "topup:" + sess.ID.String()}); err != nil {
			return err
		}
	}
	s.auditLog(ctx, sub.OrgID, &actorID, "billing.purchase", map[string]any{
		"item": sess.ItemCode, "amount_cents": sess.AmountCents, "checkout_id": sess.ID.String(), "provider": sess.Provider,
	})
	return nil
}

// SetCancelAtPeriodEnd cancels (true) or resumes (false) the paid plan.
// Nothing stops working until the period ends.
func (s *Service) SetCancelAtPeriodEnd(ctx context.Context, actor domain.Actor, cancel bool) (*Summary, error) {
	if err := requireBillingAdmin(actor); err != nil {
		return nil, err
	}
	var out *Summary
	err := s.withOrg(ctx, actor.OrgID, func(tx Tx, sub *Subscription, now time.Time) error {
		if sub.PlanCode == PlanFree {
			return apperrors.Conflict("billing.nothing_to_cancel", "you're on the Free plan")
		}
		sub.CancelAtPeriodEnd = cancel
		if err := tx.UpdateSubscription(ctx, sub); err != nil {
			return err
		}
		action := "billing.resumed"
		if cancel {
			action = "billing.cancelled"
		}
		actorID := actor.UserID
		s.auditLog(ctx, sub.OrgID, &actorID, action, map[string]any{"plan": string(sub.PlanCode), "period_end": sub.PeriodEnd})
		var err error
		out, err = s.summaryLocked(ctx, tx, sub, now)
		return err
	})
	if err != nil {
		return nil, wrapInternal(err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Renewal (driven by Ticker).

// RenewDue runs every due monthly grant / period end. Returns how many
// orgs it processed.
func (s *Service) RenewDue(ctx context.Context) (int, error) {
	orgs, err := s.repo.DueOrgs(ctx, s.now(), 100)
	if err != nil {
		return 0, err
	}
	for _, orgID := range orgs {
		if err := s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
			return s.renewLocked(ctx, tx, sub, now, false)
		}); err != nil {
			s.log.Error("billing: renewal failed", "org_id", orgID, "error", err)
		}
	}
	return len(orgs), nil
}

// renewLocked grants every monthly grant that's due. A paid plan whose
// period has ended falls back to Free (there is no card to charge in demo
// mode), unless demoRenew simulates a paid renewal.
func (s *Service) renewLocked(ctx context.Context, tx Tx, sub *Subscription, now time.Time, demoRenew bool) error {
	for i := 0; i < 13 && !sub.NextGrantAt.After(now); i++ {
		if sub.PlanCode != PlanFree && !sub.NextGrantAt.Before(sub.PeriodEnd) {
			plan := planOf(sub)
			if demoRenew && !sub.CancelAtPeriodEnd {
				sub.PeriodStart, sub.PeriodEnd = sub.PeriodEnd, periodEnd(sub.PeriodEnd, plan.Period)
				s.auditLog(ctx, sub.OrgID, nil, "billing.renewed", map[string]any{"plan": string(plan.Code), "demo": true})
			} else {
				s.auditLog(ctx, sub.OrgID, nil, "billing.plan_ended", map[string]any{"plan": string(plan.Code)})
				sub.PlanCode, sub.Status, sub.CancelAtPeriodEnd = PlanFree, StatusActive, false
				sub.PeriodStart, sub.PeriodEnd = sub.NextGrantAt, addMonths(sub.NextGrantAt, 1)
			}
		}
		if sub.PlanCode == PlanFree && !sub.PeriodEnd.After(sub.NextGrantAt) {
			sub.PeriodStart, sub.PeriodEnd = sub.NextGrantAt, addMonths(sub.NextGrantAt, 1)
		}
		grantKey := fmt.Sprintf("grant:%s:%d", sub.OrgID, sub.NextGrantAt.Unix())
		sub.NextGrantAt = addMonths(sub.NextGrantAt, 1)
		if err := tx.UpdateSubscription(ctx, sub); err != nil {
			return err
		}
		if err := s.grantPlanTokens(ctx, tx, sub, now, grantKey, "monthly_grant"); err != nil {
			return err
		}
	}
	expired, err := tx.ExpiredBuckets(ctx, now)
	if err != nil {
		return err
	}
	for _, b := range expired {
		if err := s.expireBucket(ctx, tx, now, b, "expired"); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Platform-operator actions.

// AdminSummary is an org's billing state for the operator panel.
func (s *Service) AdminSummary(ctx context.Context, orgID uuid.UUID) (*Summary, []LedgerEntry, error) {
	var sum *Summary
	var ledger []LedgerEntry
	err := s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
		var err error
		if sum, err = s.summaryLocked(ctx, tx, sub, now); err != nil {
			return err
		}
		ledger, err = tx.ListLedger(ctx, nil, 20)
		return err
	})
	if err != nil {
		return nil, nil, wrapInternal(err)
	}
	return sum, ledger, nil
}

// AdminAdjust gives (delta > 0) or takes (delta < 0) tokens — goodwill
// credit or a correction. Always audited.
func (s *Service) AdminAdjust(ctx context.Context, operatorID, orgID uuid.UUID, delta int64, reason string) error {
	if delta == 0 {
		return apperrors.Validation("billing.invalid_adjustment", "delta must not be zero", nil)
	}
	err := s.withOrg(ctx, orgID, func(tx Tx, _ *Subscription, now time.Time) error {
		key := "adjust:" + id.New().String()
		if delta > 0 {
			if err := s.addBucket(ctx, tx, now,
				Bucket{OrgID: orgID, Source: SourceAdjustment, Amount: delta, ExpiresAt: now.Add(TopupValidity)},
				LedgerEntry{Kind: KindAdjustment, Reason: reason, ActorUserID: &operatorID, IdempotencyKey: key}); err != nil {
				return err
			}
		} else {
			buckets, err := tx.SpendableBuckets(ctx, now)
			if err != nil {
				return err
			}
			owed := -delta
			for _, b := range buckets {
				if owed == 0 {
					break
				}
				take := min(b.Remaining, owed)
				if _, err := s.moveTokens(ctx, tx, now, b.ID, -take, LedgerEntry{
					OrgID: orgID, Kind: KindAdjustment, Reason: reason, ActorUserID: &operatorID,
					IdempotencyKey: key + ":" + b.ID.String(),
				}); err != nil {
					return err
				}
				owed -= take
			}
		}
		s.auditLog(ctx, orgID, &operatorID, "billing.adjustment", map[string]any{"delta": delta, "reason": reason})
		return nil
	})
	return wrapInternal(err)
}

// AdminAdvanceCycle (demo mode only) makes the next monthly grant due now
// and runs it, simulating a paid renewal — so a renewal can be shown live
// instead of waiting a month.
func (s *Service) AdminAdvanceCycle(ctx context.Context, operatorID, orgID uuid.UUID) error {
	if s.mode != ModeDemo {
		return apperrors.NotFound("billing.not_found", "not found")
	}
	err := s.withOrg(ctx, orgID, func(tx Tx, sub *Subscription, now time.Time) error {
		shift := sub.NextGrantAt.Sub(now)
		sub.NextGrantAt = now
		if sub.PlanCode != PlanFree {
			sub.PeriodEnd = sub.PeriodEnd.Add(-shift)
		}
		// Make the current grant expire now, so the new one visibly replaces it.
		if err := s.renewLocked(ctx, tx, sub, now, true); err != nil {
			return err
		}
		s.auditLog(ctx, orgID, &operatorID, "billing.cycle_advanced", map[string]any{"demo": true})
		return nil
	})
	return wrapInternal(err)
}

func (s *Service) auditLog(ctx context.Context, orgID uuid.UUID, actorID *uuid.UUID, action string, detail map[string]any) {
	if s.audit == nil {
		return
	}
	org := orgID
	rt := "organization"
	s.audit.Log(ctx, audit.Entry{OrgID: &org, ActorID: actorID, Action: action, ResourceType: &rt, ResourceID: &org, Detail: detail})
}

// wrapInternal passes typed errors through and wraps anything else, so a
// raw database error never reaches the client.
func wrapInternal(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperrors.Internal(err)
}
