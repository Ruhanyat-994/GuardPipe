package domain

// Verdict is the gate outcome `modules/scoring` derives from a risk score
// (documentation/11-risk-scoring-and-severity.md §3.8). Matches the
// `verdict` Postgres enum (documentation/06-database-design.md §3) exactly.
type Verdict string

const (
	VerdictPass  Verdict = "pass"
	VerdictWarn  Verdict = "warn"
	VerdictBlock Verdict = "block"
)

func (v Verdict) Valid() bool {
	switch v {
	case VerdictPass, VerdictWarn, VerdictBlock:
		return true
	default:
		return false
	}
}

func (v Verdict) String() string {
	return string(v)
}
