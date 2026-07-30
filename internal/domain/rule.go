package domain

// Tier distinguishes rules that must ship (Core) from rules that are
// valuable but expendable under the 4-week timeline (Stretch) — see
// documentation/05-module-specifications.md's per-module Core/Stretch tables.
type Tier string

const (
	TierCore    Tier = "core"
	TierStretch Tier = "stretch"
)

func (t Tier) Valid() bool {
	switch t {
	case TierCore, TierStretch:
		return true
	default:
		return false
	}
}

// RuleMeta is what every rule declares once, at registration
// (documentation/05-module-specifications.md §2.3). RuleMeta.ID is the same
// permanent, namespaced identifier that appears on every Finding.RuleID and
// in users' suppression comments — renaming one is a breaking change.
//
// Remediation must be set and must be independent of AI: if Gemini is down,
// a finding still has to tell the user what to do (FR-AI-011).
type RuleMeta struct {
	ID          string
	Title       string
	Description string
	Severity    Severity // default; a rule may raise it contextually
	Confidence  Confidence
	CWE         []string
	OWASP       []string
	Remediation string
	References  []string
	Tier        Tier
}
