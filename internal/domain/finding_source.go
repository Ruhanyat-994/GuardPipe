package domain

// FindingSource distinguishes a deterministic rule match from an AI-authored
// semantic finding (documentation/06-database-design.md §4.11's
// `finding_source` enum, present in the schema since migration 00001 but
// never surfaced on this type until cicdscan — the first engine to actually
// call modules/ai and need to tell the two apart). An AI-sourced finding is
// always a supplement layered on top of the deterministic rule pass, never a
// replacement for it (documentation/10-ai-integration.md §1) — the zero
// value is deliberately FindingSourceRule (Go's own zero value for an
// untyped ""), so every existing engine's finding constructors, none of
// which ever set Source, keep meaning exactly what they always meant.
type FindingSource string

const (
	FindingSourceRule FindingSource = "rule"
	FindingSourceAI   FindingSource = "ai"
)

func (s FindingSource) Valid() bool {
	switch s {
	case FindingSourceRule, FindingSourceAI, "":
		return true
	default:
		return false
	}
}

// Effective returns s if set, or the default (FindingSourceRule) if it's the
// zero value — what every caller that needs a concrete value (the
// repository layer writing a NOT NULL column, the API DTO) should use
// instead of comparing against "" directly.
func (s FindingSource) Effective() FindingSource {
	if s == "" {
		return FindingSourceRule
	}
	return s
}
