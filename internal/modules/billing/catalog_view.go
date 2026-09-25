package billing

import (
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// CatalogView is GET /billing/catalog — everything the pricing page and
// the scan launcher need to show prices, computed from the catalogue so
// the frontend never keeps its own copy.
type CatalogView struct {
	PriceVersion      string
	Plans             []Plan
	Packs             []Pack
	EnginePrices      map[domain.EngineID]int64
	PentestMultiplier map[domain.PentestPreset]float64
	LiveDiscount      float64
	AIPrices          map[AIAction]int64
	Examples          []CatalogExample
	Guarantee         Guarantee
}

// CatalogExample is one worked "what does this cost" row.
type CatalogExample struct {
	Key     string
	Label   string
	Tokens  int64
	PerPro  int64 // how many fit in one Pro month
	Engines []domain.EngineID
}

// Guarantee is the monthly allowance Pro is sized for.
type Guarantee struct {
	Pentests       int
	FullScans      int
	LivePushes     int
	TokensNeeded   int64
	MonthlyAllowed int64
}

// BuildCatalog assembles the public price list.
func BuildCatalog() CatalogView {
	v := CatalogView{
		PriceVersion: PriceVersion, EnginePrices: EnginePrice, PentestMultiplier: PentestMultiplier,
		LiveDiscount: LiveDiscount, AIPrices: AIPrice,
	}
	for _, c := range PlanOrder {
		v.Plans = append(v.Plans, Plans[c])
	}
	for _, c := range PackOrder {
		v.Packs = append(v.Packs, Packs[c])
	}
	pro := Plans[PlanProMonthly].MonthlyGrant
	pentest := []domain.EngineID{domain.EnginePentest}
	fullPlusPentest := append(append([]domain.EngineID{}, FullScanEngines...), domain.EnginePentest)
	examples := []struct {
		key, label string
		engines    []domain.EngineID
		preset     domain.PentestPreset
		trigger    domain.TriggerSource
	}{
		{"full_scan", "Full supply-chain scan (6 engines, no pentest)", FullScanEngines, "", domain.TriggerManual},
		{"full_plus_pentest", "Full scan + standard pentest", fullPlusPentest, domain.PentestPresetStandard, domain.TriggerManual},
		{"live_push", "Live scan on push (codescan + depscan, 50% off)", TypicalLiveEngines, "", domain.TriggerWebhookPush},
		{"pentest_stealth", "Pentest, stealth", pentest, domain.PentestPresetStealth, domain.TriggerManual},
		{"pentest_standard", "Pentest, standard", pentest, domain.PentestPresetStandard, domain.TriggerManual},
		{"pentest_deep", "Pentest, deep", pentest, domain.PentestPresetDeep, domain.TriggerManual},
	}
	for _, e := range examples {
		q := mustQuote(e.engines, e.preset, e.trigger)
		v.Examples = append(v.Examples, CatalogExample{Key: e.key, Label: e.label, Tokens: q.Total, PerPro: pro / q.Total, Engines: e.engines})
	}
	v.Guarantee = Guarantee{
		Pentests: GuaranteedPentests, FullScans: GuaranteedFullScans, LivePushes: GuaranteedLivePushes,
		TokensNeeded:   GuaranteeTokens(),
		MonthlyAllowed: pro,
	}
	return v
}

// GuaranteeTokens is what the Pro allowance promise costs at today's
// prices: 5 standard pentests + 10 full scans + 52 live pushes.
func GuaranteeTokens() int64 {
	pentest := mustQuote([]domain.EngineID{domain.EnginePentest}, domain.PentestPresetStandard, domain.TriggerManual).Total
	full := mustQuote(FullScanEngines, "", domain.TriggerManual).Total
	live := mustQuote(TypicalLiveEngines, "", domain.TriggerWebhookPush).Total
	return GuaranteedPentests*pentest + GuaranteedFullScans*full + GuaranteedLivePushes*live
}
