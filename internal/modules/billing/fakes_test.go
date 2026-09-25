package billing

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// fakeRepo is an in-memory billing.Repository. One mutex stands in for the
// per-org row lock. A failing fn rolls back by restoring a snapshot, like
// a real transaction would.
type fakeRepo struct {
	mu        sync.Mutex
	subs      map[uuid.UUID]*Subscription
	buckets   map[uuid.UUID]*Bucket
	ledger    []LedgerEntry
	checkouts map[uuid.UUID]*CheckoutSession
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{subs: map[uuid.UUID]*Subscription{}, buckets: map[uuid.UUID]*Bucket{}, checkouts: map[uuid.UUID]*CheckoutSession{}}
}

type snapshot struct {
	subs      map[uuid.UUID]Subscription
	buckets   map[uuid.UUID]Bucket
	ledgerLen int
	checkouts map[uuid.UUID]CheckoutSession
}

func (r *fakeRepo) snap() snapshot {
	s := snapshot{subs: map[uuid.UUID]Subscription{}, buckets: map[uuid.UUID]Bucket{}, ledgerLen: len(r.ledger), checkouts: map[uuid.UUID]CheckoutSession{}}
	for k, v := range r.subs {
		s.subs[k] = *v
	}
	for k, v := range r.buckets {
		s.buckets[k] = *v
	}
	for k, v := range r.checkouts {
		s.checkouts[k] = *v
	}
	return s
}

func (r *fakeRepo) restore(s snapshot) {
	r.subs, r.buckets, r.checkouts = map[uuid.UUID]*Subscription{}, map[uuid.UUID]*Bucket{}, map[uuid.UUID]*CheckoutSession{}
	for k, v := range s.subs {
		v := v
		r.subs[k] = &v
	}
	for k, v := range s.buckets {
		v := v
		r.buckets[k] = &v
	}
	for k, v := range s.checkouts {
		v := v
		r.checkouts[k] = &v
	}
	r.ledger = r.ledger[:s.ledgerLen]
}

func (r *fakeRepo) WithOrgLock(ctx context.Context, orgID uuid.UUID, fresh Subscription, fn func(Tx, *Subscription, bool) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snap()
	created := false
	if _, ok := r.subs[orgID]; !ok {
		f := fresh
		r.subs[orgID] = &f
		created = true
	}
	sub := *r.subs[orgID]
	if err := fn(&fakeTx{r: r, org: orgID}, &sub, created); err != nil {
		r.restore(before)
		return err
	}
	return nil
}

func (r *fakeRepo) OrgForJob(_ context.Context, jobID uuid.UUID) (uuid.UUID, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.ledger {
		if e.ScanJobID != nil && *e.ScanJobID == jobID && e.Kind == KindDebit {
			return e.OrgID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (r *fakeRepo) DueOrgs(_ context.Context, now time.Time, _ int) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for id, s := range r.subs {
		if !s.NextGrantAt.After(now) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, b := range r.buckets {
		if b.Remaining > 0 && !b.ExpiresAt.After(now) && !seen[b.OrgID] {
			seen[b.OrgID] = true
			out = append(out, b.OrgID)
		}
	}
	return out, nil
}

// balance is a test helper reading an org's spendable balance.
func (r *fakeRepo) balance(org uuid.UUID, now time.Time) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, b := range r.buckets {
		if b.OrgID == org && b.Remaining > 0 && b.ExpiresAt.After(now) {
			total += b.Remaining
		}
	}
	return total
}

func (r *fakeRepo) ledgerOf(org uuid.UUID, kind string) []LedgerEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []LedgerEntry
	for _, e := range r.ledger {
		if e.OrgID == org && (kind == "" || e.Kind == kind) {
			out = append(out, e)
		}
	}
	return out
}

type fakeTx struct {
	r   *fakeRepo
	org uuid.UUID
}

func (t *fakeTx) UpdateSubscription(_ context.Context, s *Subscription) error {
	c := *s
	t.r.subs[t.org] = &c
	return nil
}

func (t *fakeTx) list(pred func(*Bucket) bool) []Bucket {
	var out []Bucket
	for _, b := range t.r.buckets {
		if b.OrgID == t.org && pred(b) {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExpiresAt.Equal(out[j].ExpiresAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	})
	return out
}

func (t *fakeTx) SpendableBuckets(_ context.Context, now time.Time) ([]Bucket, error) {
	return t.list(func(b *Bucket) bool { return b.Remaining > 0 && b.ExpiresAt.After(now) }), nil
}

func (t *fakeTx) BucketsBySource(_ context.Context, source string) ([]Bucket, error) {
	return t.list(func(b *Bucket) bool { return b.Remaining > 0 && b.Source == source }), nil
}

func (t *fakeTx) ExpiredBuckets(_ context.Context, now time.Time) ([]Bucket, error) {
	return t.list(func(b *Bucket) bool { return b.Remaining > 0 && !b.ExpiresAt.After(now) }), nil
}

func (t *fakeTx) GetBucket(_ context.Context, id uuid.UUID) (*Bucket, error) {
	b, ok := t.r.buckets[id]
	if !ok || b.OrgID != t.org {
		return nil, apperrors.NotFound("billing.bucket_not_found", "not found")
	}
	c := *b
	return &c, nil
}

var fakeSeq int64

func (t *fakeTx) InsertBucket(_ context.Context, b *Bucket) error {
	fakeSeq++
	b.CreatedAt = time.Unix(fakeSeq, 0)
	c := *b
	c.OrgID = t.org
	t.r.buckets[b.ID] = &c
	return nil
}

func (t *fakeTx) AddToBucket(_ context.Context, id uuid.UUID, delta int64) error {
	b := t.r.buckets[id]
	if b.Remaining+delta < 0 || b.Remaining+delta > b.Amount {
		// the database CHECK constraint
		return apperrors.Internal(nil)
	}
	b.Remaining += delta
	return nil
}

func (t *fakeTx) InsertLedger(_ context.Context, e *LedgerEntry) (bool, error) {
	for _, x := range t.r.ledger {
		if x.IdempotencyKey == e.IdempotencyKey {
			return false, nil
		}
	}
	fakeSeq++
	e.CreatedAt = time.Unix(fakeSeq, 0)
	e.OrgID = t.org
	t.r.ledger = append(t.r.ledger, *e)
	return true, nil
}

func (t *fakeTx) LedgerKeyExists(_ context.Context, key string) (bool, error) {
	for _, x := range t.r.ledger {
		if x.IdempotencyKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (t *fakeTx) DebitsForJob(_ context.Context, jobID uuid.UUID) ([]LedgerEntry, error) {
	var out []LedgerEntry
	for _, e := range t.r.ledger {
		if e.OrgID == t.org && e.Kind == KindDebit && e.ScanJobID != nil && *e.ScanJobID == jobID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (t *fakeTx) Balance(_ context.Context, now time.Time) (int64, error) {
	var total int64
	for _, b := range t.list(func(b *Bucket) bool { return b.Remaining > 0 && b.ExpiresAt.After(now) }) {
		total += b.Remaining
	}
	return total, nil
}

func (t *fakeTx) ListLedger(_ context.Context, _ *time.Time, limit int) ([]LedgerEntry, error) {
	var out []LedgerEntry
	for i := len(t.r.ledger) - 1; i >= 0 && len(out) < limit; i-- {
		if t.r.ledger[i].OrgID == t.org {
			out = append(out, t.r.ledger[i])
		}
	}
	return out, nil
}

func (t *fakeTx) LedgerForScan(_ context.Context, scanID uuid.UUID) ([]LedgerEntry, error) {
	var out []LedgerEntry
	for _, e := range t.r.ledger {
		if e.OrgID == t.org && e.ScanID != nil && *e.ScanID == scanID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (t *fakeTx) UsageSince(_ context.Context, _ time.Time) ([]UsageRow, error) {
	agg := map[string]*UsageRow{}
	for _, e := range t.r.ledger {
		if e.OrgID != t.org || e.Engine == nil || (e.Kind != KindDebit && e.Kind != KindRefund) {
			continue
		}
		k := string(*e.Engine) + "|" + e.TriggerSource
		if agg[k] == nil {
			agg[k] = &UsageRow{Engine: *e.Engine, TriggerSource: e.TriggerSource}
		}
		agg[k].Tokens -= e.Delta
	}
	var out []UsageRow
	for _, u := range agg {
		out = append(out, *u)
	}
	return out, nil
}

func (t *fakeTx) InsertCheckout(_ context.Context, c *CheckoutSession) error {
	x := *c
	t.r.checkouts[c.ID] = &x
	return nil
}

func (t *fakeTx) GetCheckout(_ context.Context, id uuid.UUID) (*CheckoutSession, error) {
	c, ok := t.r.checkouts[id]
	if !ok || c.OrgID != t.org {
		return nil, apperrors.NotFound("billing.checkout_not_found", "checkout not found")
	}
	x := *c
	return &x, nil
}

func (t *fakeTx) SetCheckoutStatus(_ context.Context, id uuid.UUID, status string, paidAt *time.Time) error {
	c := t.r.checkouts[id]
	c.Status = status
	if paidAt != nil {
		c.PaidAt = paidAt
	}
	return nil
}

type fakeProvider struct{}

func (fakeProvider) Name() string { return "demo" }
func (fakeProvider) CreateSession(_ context.Context, s CheckoutSession) (string, string, error) {
	return "/checkout/" + s.ID.String(), "demo_" + s.ID.String(), nil
}

type fakeAudit struct {
	mu      sync.Mutex
	actions []string
}

func (a *fakeAudit) Log(_ context.Context, e audit.Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, e.Action)
}
