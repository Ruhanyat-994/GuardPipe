package billing

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestQuoteScan(t *testing.T) {
	cases := []struct {
		name    string
		engines []domain.EngineID
		preset  domain.PentestPreset
		trigger domain.TriggerSource
		want    int64
	}{
		{"depscan", []domain.EngineID{domain.EngineDepScan}, "", domain.TriggerManual, 1_500},
		{"k8sscan", []domain.EngineID{domain.EngineK8sScan}, "", domain.TriggerManual, 1_500},
		{"cicdscan", []domain.EngineID{domain.EngineCICDScan}, "", domain.TriggerManual, 2_500},
		{"containerscan", []domain.EngineID{domain.EngineContainerScan}, "", domain.TriggerManual, 4_000},
		{"codescan", []domain.EngineID{domain.EngineCodeScan}, "", domain.TriggerManual, 5_000},
		{"docreview", []domain.EngineID{domain.EngineDocReview}, "", domain.TriggerManual, 6_000},
		{"full scan", FullScanEngines, "", domain.TriggerManual, 20_500},
		{"full + standard pentest", append(append([]domain.EngineID{}, FullScanEngines...), domain.EnginePentest), domain.PentestPresetStandard, domain.TriggerManual, 45_500},
		{"pentest default preset is stealth", []domain.EngineID{domain.EnginePentest}, "", domain.TriggerManual, 15_000},
		{"pentest stealth", []domain.EngineID{domain.EnginePentest}, domain.PentestPresetStealth, domain.TriggerManual, 15_000},
		{"pentest standard", []domain.EngineID{domain.EnginePentest}, domain.PentestPresetStandard, domain.TriggerManual, 25_000},
		{"pentest custom", []domain.EngineID{domain.EnginePentest}, domain.PentestPresetCustom, domain.TriggerManual, 25_000},
		{"pentest deep", []domain.EngineID{domain.EnginePentest}, domain.PentestPresetDeep, domain.TriggerManual, 50_000},
		{"live push is half price", TypicalLiveEngines, "", domain.TriggerWebhookPush, 3_250},
		{"live pull request is half price", TypicalLiveEngines, "", domain.TriggerWebhookPullRequest, 3_250},
		// Near-misses: the live discount must not leak onto other triggers.
		{"manual is full price", TypicalLiveEngines, "", domain.TriggerManual, 6_500},
		{"scheduled is full price", TypicalLiveEngines, "", domain.TriggerScheduled, 6_500},
		{"cli is full price", TypicalLiveEngines, "", domain.TriggerCLIWatch, 6_500},
		{"live depscan rounds up to 50", []domain.EngineID{domain.EngineDepScan}, "", domain.TriggerWebhookPush, 750},
		{"live cicdscan rounds up to 50", []domain.EngineID{domain.EngineCICDScan}, "", domain.TriggerWebhookPush, 1_250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := QuoteScan(tc.engines, tc.preset, tc.trigger)
			if err != nil {
				t.Fatal(err)
			}
			if q.Total != tc.want {
				t.Errorf("total = %d, want %d", q.Total, tc.want)
			}
			if len(q.Lines) != len(tc.engines) {
				t.Errorf("got %d lines for %d engines", len(q.Lines), len(tc.engines))
			}
		})
	}
}

func TestQuoteScan_UnknownEngineIsAnError(t *testing.T) {
	if _, err := QuoteScan([]domain.EngineID{"nosuchengine"}, "", domain.TriggerManual); err == nil {
		t.Fatal("expected an error for an engine with no price")
	}
}

// The pricing page promises every Pro subscriber 5 pentests, 10 full scans
// and 52 live pushes a month. A price change that breaks the promise must
// fail here, not silently on a customer's invoice.
func TestProAllowanceGuarantee(t *testing.T) {
	need := GuaranteeTokens()
	if need != 499_000 {
		t.Logf("guarantee now costs %d tokens (was 499,000)", need)
	}
	for _, code := range []PlanCode{PlanProMonthly, PlanProAnnual} {
		if grant := Plans[code].MonthlyGrant; need > grant {
			t.Errorf("%s grants %d/month but the guarantee needs %d", code, grant, need)
		}
	}
}

func TestPacksCostMoreThanTheSubscription(t *testing.T) {
	pro := Plans[PlanProMonthly]
	planRate := float64(pro.PriceCents) / float64(pro.MonthlyGrant)
	for _, p := range Packs {
		if rate := float64(p.PriceCents) / float64(p.Tokens); rate <= planRate {
			t.Errorf("%s costs %.6f¢/token, cheaper than the plan's %.6f¢", p.Code, rate, planRate)
		}
	}
}

func TestPlanGates(t *testing.T) {
	if Plans[PlanFree].Allows(domain.EnginePentest) {
		t.Error("Free must not include pentest")
	}
	if Plans[PlanFree].LiveScanning || Plans[PlanFree].Schedules {
		t.Error("Free must not include live scanning or schedules")
	}
	for _, e := range allEngines {
		if !Plans[PlanProMonthly].Allows(e) {
			t.Errorf("Pro must include %s", e)
		}
	}
}

func TestCatalogExamples(t *testing.T) {
	v := BuildCatalog()
	byKey := map[string]CatalogExample{}
	for _, e := range v.Examples {
		byKey[e.Key] = e
	}
	if got := byKey["full_scan"]; got.Tokens != 20_500 || got.PerPro != 24 {
		t.Errorf("full_scan = %+v", got)
	}
	if got := byKey["live_push"]; got.Tokens != 3_250 {
		t.Errorf("live_push = %+v", got)
	}
	if len(v.Plans) != 3 || len(v.Packs) != 3 {
		t.Errorf("catalog has %d plans, %d packs", len(v.Plans), len(v.Packs))
	}
}
