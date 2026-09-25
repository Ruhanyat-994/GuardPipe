// Package billing is GuardPipe's token-based subscription billing
// (TOKENIZATION-ARCHITECTURE.md). Every organisation has one token balance;
// every scan engine has a fixed token price; a scan is paid for in full
// when it's created (orchestrator.CreateScan) and each engine that didn't
// get to do its work is refunded automatically (orchestrator.Pool.persist).
//
// The catalogue below is the single source of truth for prices. The
// frontend never hard-codes a price: it renders GET /billing/catalog and
// POST /billing/estimate, so the two can't drift apart.
package billing

import (
	"slices"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// PriceVersion is stamped on every ledger debit, so a later price change
// never rewrites what an old scan cost.
const PriceVersion = "2026-09-25"

// PlanCode matches billing_subscriptions.plan_code's CHECK constraint.
type PlanCode string

const (
	PlanFree       PlanCode = "free"
	PlanProMonthly PlanCode = "pro_monthly"
	PlanProAnnual  PlanCode = "pro_annual"
)

// Period is how long one payment for a plan lasts.
type Period string

const (
	PeriodMonth Period = "month"
	PeriodYear  Period = "year"
)

// Plan is one subscription tier. MonthlyGrant is granted every month, on
// annual plans too (never a year's worth up front — that invites
// buy-and-burn and breaks the monthly rhythm of the token bar).
type Plan struct {
	Code          PlanCode
	Name          string
	PriceCents    int
	Period        Period
	MonthlyGrant  int64
	Engines       []domain.EngineID
	LiveScanning  bool
	Schedules     bool
	TopupDiscount int // percent off top-up packs
}

// Allows reports whether the plan may run engine e.
func (p Plan) Allows(e domain.EngineID) bool {
	return slices.Contains(p.Engines, e)
}

// Pack is a one-time top-up.
type Pack struct {
	Code       string
	Name       string
	Tokens     int64
	PriceCents int
}

var allEngines = []domain.EngineID{
	domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan, domain.EngineContainerScan,
	domain.EngineK8sScan, domain.EngineCICDScan, domain.EnginePentest,
}

var freeEngines = []domain.EngineID{
	domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan, domain.EngineContainerScan,
	domain.EngineK8sScan, domain.EngineCICDScan,
}

// PlanOrder is the display order.
var PlanOrder = []PlanCode{PlanFree, PlanProMonthly, PlanProAnnual}

var Plans = map[PlanCode]Plan{
	PlanFree: {
		Code: PlanFree, Name: "Free", PriceCents: 0, Period: PeriodMonth,
		MonthlyGrant: 15_000, Engines: freeEngines,
	},
	PlanProMonthly: {
		Code: PlanProMonthly, Name: "Pro", PriceCents: 2900, Period: PeriodMonth,
		MonthlyGrant: 500_000, Engines: allEngines, LiveScanning: true, Schedules: true,
	},
	PlanProAnnual: {
		Code: PlanProAnnual, Name: "Pro (annual)", PriceCents: 24000, Period: PeriodYear,
		MonthlyGrant: 500_000, Engines: allEngines, LiveScanning: true, Schedules: true, TopupDiscount: 10,
	},
}

// PackOrder is the display order.
var PackOrder = []string{"topup_100k", "topup_250k", "topup_600k"}

// Packs are priced above the Pro plan's own per-token rate ($29 / 500k =
// $0.058 per 1k), so the subscription is always the better deal and packs
// are only for going over the monthly allowance.
var Packs = map[string]Pack{
	"topup_100k": {Code: "topup_100k", Name: "100k tokens", Tokens: 100_000, PriceCents: 900},
	"topup_250k": {Code: "topup_250k", Name: "250k tokens", Tokens: 250_000, PriceCents: 1900},
	"topup_600k": {Code: "topup_600k", Name: "600k tokens", Tokens: 600_000, PriceCents: 3900},
}

// TopupValidity is how long purchased top-up tokens last.
const TopupValidity = 365 * 24 * time.Hour

// EnginePrice is the token price of one engine run, set by what it really
// costs GuardPipe to run (compute, sandbox time, AI calls).
var EnginePrice = map[domain.EngineID]int64{
	domain.EngineDepScan:       1_500,
	domain.EngineK8sScan:       1_500,
	domain.EngineCICDScan:      2_500,
	domain.EngineContainerScan: 4_000,
	domain.EngineCodeScan:      5_000,
	domain.EngineDocReview:     6_000,
	domain.EnginePentest:       25_000,
}

// AIAction is one command of the finding assistant (modules/assist).
type AIAction string

const (
	AIExplain   AIAction = "explain"
	AIRemediate AIAction = "remediate"
	AIFix       AIAction = "fix"
)

// AIPrice is what each finding-assistant command costs. Priced by how much
// model work it is — an explanation is a short fast-model call, a patch is
// the long smart-model one — and small next to any scan. Each command is
// paid once per finding: asking again is free. "Where is it?" is not here
// because it never calls the model (it's free).
var AIPrice = map[AIAction]int64{
	AIExplain:   150,
	AIRemediate: 400,
	AIFix:       750,
}

// PentestMultiplier scales pentest's price by intensity preset. An empty
// preset is the server-side default, Stealth.
var PentestMultiplier = map[domain.PentestPreset]float64{
	domain.PentestPresetStealth:  0.6,
	domain.PentestPresetStandard: 1,
	domain.PentestPresetDeep:     2,
	domain.PentestPresetCustom:   1,
}

// LiveDiscount is the price factor for scans a GitHub webhook started: a
// push is usually a small incremental change, and automation should be
// cheap enough to leave on.
const LiveDiscount = 0.5

// IsLiveTrigger reports whether t gets the live-scan price.
func IsLiveTrigger(t domain.TriggerSource) bool {
	return t == domain.TriggerWebhookPush || t == domain.TriggerWebhookPullRequest
}

// checkoutTTL is how long a checkout session can be paid before it expires.
const checkoutTTL = 30 * time.Minute
