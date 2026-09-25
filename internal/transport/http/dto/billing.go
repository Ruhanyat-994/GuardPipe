package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
)

// --- catalog ---

type BillingPlan struct {
	Code                 string   `json:"code"`
	Name                 string   `json:"name"`
	PriceCents           int      `json:"price_cents"`
	Period               string   `json:"period"`
	MonthlyGrant         int64    `json:"monthly_grant"`
	Engines              []string `json:"engines"`
	LiveScanning         bool     `json:"live_scanning"`
	Schedules            bool     `json:"schedules"`
	TopupDiscountPercent int      `json:"topup_discount_percent"`
}

type BillingPack struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Tokens     int64  `json:"tokens"`
	PriceCents int    `json:"price_cents"`
}

type BillingExample struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Tokens      int64    `json:"tokens"`
	PerProMonth int64    `json:"per_pro_month"`
	Engines     []string `json:"engines"`
}

type BillingGuarantee struct {
	Pentests         int   `json:"pentests"`
	FullScans        int   `json:"full_scans"`
	LivePushes       int   `json:"live_pushes"`
	TokensNeeded     int64 `json:"tokens_needed"`
	MonthlyAllowance int64 `json:"monthly_allowance"`
}

type BillingCatalogResponse struct {
	PriceVersion       string             `json:"price_version"`
	Plans              []BillingPlan      `json:"plans"`
	Packs              []BillingPack      `json:"packs"`
	EnginePrices       map[string]int64   `json:"engine_prices"`
	PentestMultipliers map[string]float64 `json:"pentest_multipliers"`
	LiveDiscount       float64            `json:"live_discount"`
	AIPrices           map[string]int64   `json:"ai_prices"`
	Examples           []BillingExample   `json:"examples"`
	Guarantee          BillingGuarantee   `json:"guarantee"`
}

func enginesToStrings(es []domain.EngineID) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = string(e)
	}
	return out
}

func FromBillingPlan(p billing.Plan) BillingPlan {
	return BillingPlan{
		Code: string(p.Code), Name: p.Name, PriceCents: p.PriceCents, Period: string(p.Period),
		MonthlyGrant: p.MonthlyGrant, Engines: enginesToStrings(p.Engines), LiveScanning: p.LiveScanning,
		Schedules: p.Schedules, TopupDiscountPercent: p.TopupDiscount,
	}
}

func FromBillingCatalog(v billing.CatalogView) BillingCatalogResponse {
	out := BillingCatalogResponse{
		PriceVersion: v.PriceVersion, LiveDiscount: v.LiveDiscount,
		EnginePrices: map[string]int64{}, PentestMultipliers: map[string]float64{}, AIPrices: map[string]int64{},
		Guarantee: BillingGuarantee{
			Pentests: v.Guarantee.Pentests, FullScans: v.Guarantee.FullScans, LivePushes: v.Guarantee.LivePushes,
			TokensNeeded: v.Guarantee.TokensNeeded, MonthlyAllowance: v.Guarantee.MonthlyAllowed,
		},
	}
	for _, p := range v.Plans {
		out.Plans = append(out.Plans, FromBillingPlan(p))
	}
	for _, p := range v.Packs {
		out.Packs = append(out.Packs, BillingPack{Code: p.Code, Name: p.Name, Tokens: p.Tokens, PriceCents: p.PriceCents})
	}
	for e, t := range v.EnginePrices {
		out.EnginePrices[string(e)] = t
	}
	for a, t := range v.AIPrices {
		out.AIPrices[string(a)] = t
	}
	for p, m := range v.PentestMultiplier {
		out.PentestMultipliers[string(p)] = m
	}
	for _, e := range v.Examples {
		out.Examples = append(out.Examples, BillingExample{Key: e.Key, Label: e.Label, Tokens: e.Tokens, PerProMonth: e.PerPro, Engines: enginesToStrings(e.Engines)})
	}
	return out
}

// --- summary / ledger ---

type BillingUsageRow struct {
	Engine        string `json:"engine"`
	TriggerSource string `json:"trigger_source"`
	Tokens        int64  `json:"tokens"`
}

type BillingSummaryResponse struct {
	Plan               BillingPlan       `json:"plan"`
	Status             string            `json:"status"`
	PeriodStart        time.Time         `json:"period_start"`
	PeriodEnd          time.Time         `json:"period_end"`
	NextGrantAt        time.Time         `json:"next_grant_at"`
	CancelAtPeriodEnd  bool              `json:"cancel_at_period_end"`
	Balance            int64             `json:"balance"`
	PlanGrantRemaining int64             `json:"plan_grant_remaining"`
	TopupRemaining     int64             `json:"topup_remaining"`
	TopupExpiresAt     *time.Time        `json:"topup_expires_at"`
	MonthlyGrant       int64             `json:"monthly_grant"`
	UsedThisCycle      int64             `json:"used_this_cycle"`
	Usage              []BillingUsageRow `json:"usage"`
	Mode               string            `json:"mode"`
}

func FromBillingSummary(s *billing.Summary) BillingSummaryResponse {
	out := BillingSummaryResponse{
		Plan: FromBillingPlan(s.Plan), Status: s.Status, PeriodStart: s.PeriodStart, PeriodEnd: s.PeriodEnd,
		NextGrantAt: s.NextGrantAt, CancelAtPeriodEnd: s.CancelAtPeriodEnd, Balance: s.Balance,
		PlanGrantRemaining: s.PlanGrantRemaining, TopupRemaining: s.TopupRemaining, TopupExpiresAt: s.TopupExpiresAt,
		MonthlyGrant: s.Plan.MonthlyGrant, UsedThisCycle: s.UsedThisCycle, Usage: []BillingUsageRow{}, Mode: string(s.Mode),
	}
	for _, u := range s.Usage {
		out.Usage = append(out.Usage, BillingUsageRow{Engine: string(u.Engine), TriggerSource: u.TriggerSource, Tokens: u.Tokens})
	}
	return out
}

type BillingLedgerEntry struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Delta         int64     `json:"delta"`
	BalanceAfter  int64     `json:"balance_after"`
	ScanID        *string   `json:"scan_id"`
	Engine        *string   `json:"engine"`
	TriggerSource *string   `json:"trigger_source"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func FromLedgerEntries(es []billing.LedgerEntry) []BillingLedgerEntry {
	out := make([]BillingLedgerEntry, 0, len(es))
	for _, e := range es {
		row := BillingLedgerEntry{
			ID: e.ID.String(), Kind: e.Kind, Delta: e.Delta, BalanceAfter: e.BalanceAfter,
			TriggerSource: strOrNil(e.TriggerSource), Reason: strOrNil(e.Reason), CreatedAt: e.CreatedAt,
		}
		if e.ScanID != nil {
			v := e.ScanID.String()
			row.ScanID = &v
		}
		if e.Engine != nil {
			v := string(*e.Engine)
			row.Engine = &v
		}
		out = append(out, row)
	}
	return out
}

type BillingLedgerResponse struct {
	Data []BillingLedgerEntry `json:"data"`
	// NextBefore is the cursor for the next (older) page; null at the end.
	NextBefore *time.Time `json:"next_before"`
}

type BillingScanTokensResponse struct {
	Charged  int64                `json:"charged"`
	Refunded int64                `json:"refunded"`
	Entries  []BillingLedgerEntry `json:"entries"`
}

// --- estimate ---

// BillingEstimateRequest prices a scan. With project_id + type it resolves
// engines exactly the way scan creation would; without them it prices the
// engines listed (used for the live-scanning cost forecast).
type BillingEstimateRequest struct {
	ProjectID     string                `json:"project_id" validate:"omitempty,uuid"`
	Type          string                `json:"type" validate:"omitempty,oneof=full_supply_chain partial pentest_only"`
	Engines       []string              `json:"engines" validate:"omitempty,dive,oneof=docreview codescan depscan containerscan k8sscan cicdscan pentest"`
	PentestConfig *PentestConfigRequest `json:"pentest_config,omitempty"`
	Trigger       string                `json:"trigger" validate:"omitempty,oneof=manual scheduled webhook_push webhook_pull_request cli_watch"`
}

type BillingEstimateLine struct {
	Engine     string `json:"engine"`
	Tokens     int64  `json:"tokens"`
	FullTokens int64  `json:"full_tokens"`
}

type BillingEstimateResponse struct {
	Engines        []string              `json:"engines"`
	Total          int64                 `json:"total"`
	Lines          []BillingEstimateLine `json:"lines"`
	LiveDiscount   bool                  `json:"live_discount"`
	Balance        int64                 `json:"balance"`
	BalanceAfter   int64                 `json:"balance_after"`
	Affordable     bool                  `json:"affordable"`
	PlanAllowed    bool                  `json:"plan_allowed"`
	BlockedEngines []string              `json:"blocked_engines"`
	RequiredPlan   *string               `json:"required_plan"`
}

func FromBillingEstimate(engines []domain.EngineID, e *billing.Estimate) BillingEstimateResponse {
	out := BillingEstimateResponse{
		Engines: enginesToStrings(engines), Total: e.Quote.Total, LiveDiscount: e.Quote.LiveDiscount,
		Balance: e.Balance, BalanceAfter: e.BalanceAfter, Affordable: e.Affordable, PlanAllowed: e.PlanAllowed,
		BlockedEngines: enginesToStrings(e.BlockedEngines), Lines: []BillingEstimateLine{},
	}
	for _, l := range e.Quote.Lines {
		out.Lines = append(out.Lines, BillingEstimateLine{Engine: string(l.Engine), Tokens: l.Tokens, FullTokens: l.FullTokens})
	}
	if e.RequiredPlan != "" {
		v := string(e.RequiredPlan)
		out.RequiredPlan = &v
	}
	return out
}

// --- checkout ---

type StartCheckoutRequest struct {
	ItemCode string `json:"item_code" validate:"required,max=64"`
}

// ConfirmCheckoutRequest deliberately has no card fields: the demo
// checkout page never sends card details, and there is nowhere to put them.
type ConfirmCheckoutRequest struct {
	Outcome string `json:"outcome" validate:"required,oneof=success declined"`
}

type CheckoutResponse struct {
	ID          string     `json:"id"`
	ItemCode    string     `json:"item_code"`
	ItemName    string     `json:"item_name"`
	ItemKind    string     `json:"item_kind"` // "plan" | "pack"
	Tokens      int64      `json:"tokens"`
	AmountCents int        `json:"amount_cents"`
	Currency    string     `json:"currency"`
	Provider    string     `json:"provider"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	PaidAt      *time.Time `json:"paid_at"`
	RedirectURL string     `json:"redirect_url,omitempty"`
}

func FromCheckout(c *billing.CheckoutSession, redirect string) CheckoutResponse {
	out := CheckoutResponse{
		ID: c.ID.String(), ItemCode: c.ItemCode, AmountCents: c.AmountCents, Currency: c.Currency,
		Provider: c.Provider, Status: c.Status, ExpiresAt: c.ExpiresAt, PaidAt: c.PaidAt, RedirectURL: redirect,
	}
	if p, ok := billing.Plans[billing.PlanCode(c.ItemCode)]; ok {
		out.ItemName, out.ItemKind, out.Tokens = p.Name, "plan", p.MonthlyGrant
	} else if p, ok := billing.Packs[c.ItemCode]; ok {
		out.ItemName, out.ItemKind, out.Tokens = p.Name, "pack", p.Tokens
	}
	return out
}

// --- admin ---

type AdminBillingResponse struct {
	Summary BillingSummaryResponse `json:"summary"`
	Ledger  []BillingLedgerEntry   `json:"ledger"`
}

type AdminAdjustTokensRequest struct {
	Delta  int64  `json:"delta" validate:"required,min=-10000000,max=10000000"`
	Reason string `json:"reason" validate:"required,max=200"`
}
