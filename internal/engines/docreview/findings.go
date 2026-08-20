package docreview

import (
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// classifyRuleID validates an AI response's rule_id against Rules' known
// category set (categoryRuleIDs, rules.go) — unlike cicdscan (where every
// AI finding is pinned to one generic anchor because AI only supplements a
// deterministic rule pass there), docreview's AI output *is* the rule
// taxonomy, so the model's own rule_id is what gets persisted, not a
// fabricated stand-in. A rule_id the model invents that isn't in this known
// set is not coerced into a plausible-looking ID — the finding is dropped
// instead (aipass.go's caller), on the same "precision over false
// completeness" reasoning documentation/15-testing-strategy.md's testing
// philosophy states for every other engine's rule table.
func classifyRuleID(ruleID string) (domain.RuleMeta, bool) {
	meta, ok := rulesByID[ruleID]
	if !ok || ruleID == "docreview.security.prompt-injection-attempt" {
		// The injection-defence anchor is never a rule_id the model itself
		// should return — modules/ai raises it directly on a discarded
		// response — so a model response naming it is treated the same as
		// any other unrecognised rule_id: dropped, not trusted.
		return domain.RuleMeta{}, false
	}
	return meta, true
}

// aiFinding converts one validated ai.DocumentReviewFinding into a
// domain.Finding. Confidence is always Medium regardless of what the model
// implies — every docreview finding is AI-authored, and this repository's
// convention (cicdscan.aiFinding) is that an AI-sourced finding never
// carries more than medium confidence. Suggestion/LocationHint — the
// model's answer to "where should this be fixed" — are preserved on
// Metadata rather than folded into Description, so the UI can render them
// as their own distinct field (BUILD_GUIDE.md Phase 11's "Done when").
func aiFinding(scanID uuid.UUID, docPath string, meta domain.RuleMeta, f ai.DocumentReviewFinding) domain.Finding {
	normLoc := docPath + "|" + f.LocationHint
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDocReview, RuleID: meta.ID,
		Fingerprint: id.Fingerprint(meta.ID, normLoc, f.Excerpt),
		Title:       f.Title + " (" + docPath + ")",
		Description: f.Description,
		Severity:    domain.Severity(f.Severity), Confidence: domain.ConfidenceMedium,
		CWE:         meta.CWE,
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: docPath},
		Evidence:    []domain.Evidence{{Kind: domain.EvidenceKindManifestExcerpt, Value: f.Excerpt}},
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Source:      domain.FindingSourceAI,
		Metadata:    map[string]any{"suggestion": f.Suggestion, "location_hint": f.LocationHint},
	}
}
