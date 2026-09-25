package billing

import (
	"fmt"
	"math"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// QuoteLine is one engine's price inside a quote.
type QuoteLine struct {
	Engine domain.EngineID
	Tokens int64
	// FullTokens is the undiscounted price, so the UI can show "50% off".
	FullTokens int64
}

// Quote is the price of one scan. Lines are in the same order as the
// engines asked for.
type Quote struct {
	Lines        []QuoteLine
	Total        int64
	LiveDiscount bool
	PriceVersion string
}

// QuoteScan prices a scan. It's pure — no I/O — so the same function
// backs the estimate endpoint, the real charge, and the pricing page's
// worked examples.
func QuoteScan(engines []domain.EngineID, preset domain.PentestPreset, trigger domain.TriggerSource) (Quote, error) {
	q := Quote{PriceVersion: PriceVersion, LiveDiscount: IsLiveTrigger(trigger)}
	for _, e := range engines {
		base, ok := EnginePrice[e]
		if !ok {
			return Quote{}, fmt.Errorf("billing: no price for engine %q", e)
		}
		full := base
		if e == domain.EnginePentest {
			mult, ok := PentestMultiplier[preset]
			if !ok {
				mult = PentestMultiplier[domain.PentestPresetStealth]
			}
			full = roundUp50(float64(base) * mult)
		}
		price := full
		if q.LiveDiscount {
			price = roundUp50(float64(full) * LiveDiscount)
		}
		q.Lines = append(q.Lines, QuoteLine{Engine: e, Tokens: price, FullTokens: full})
		q.Total += price
	}
	return q, nil
}

// roundUp50 rounds up to the nearest 50 tokens, so prices stay readable.
func roundUp50(v float64) int64 {
	return int64(math.Ceil(v/50) * 50)
}

// FullScanEngines is every engine except pentest — what "a full
// supply-chain scan" means on the pricing page.
var FullScanEngines = []domain.EngineID{
	domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan,
	domain.EngineContainerScan, domain.EngineK8sScan, domain.EngineCICDScan,
}

// TypicalLiveEngines is what a typical live scan on push runs.
var TypicalLiveEngines = []domain.EngineID{domain.EngineCodeScan, domain.EngineDepScan}

// Monthly allowance the Pro plan is sized to guarantee (the pricing page's
// headline; a unit test keeps it true when prices change).
const (
	GuaranteedPentests   = 5
	GuaranteedFullScans  = 10
	GuaranteedLivePushes = 52
)

// mustQuote is for the fixed, known-good engine lists above.
func mustQuote(engines []domain.EngineID, preset domain.PentestPreset, trigger domain.TriggerSource) Quote {
	q, err := QuoteScan(engines, preset, trigger)
	if err != nil {
		panic(err)
	}
	return q
}
