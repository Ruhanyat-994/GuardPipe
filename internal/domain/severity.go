package domain

// Severity is one of the five closed severity levels every finding carries.
// The ranking is worst-first so a plain numeric sort matches how the UI and
// the database (severity is a Postgres enum declared worst-first) both order
// findings — see documentation/06-database-design.md §3.
type Severity string

const (
	SeverityCritical      Severity = "critical"
	SeverityHigh          Severity = "high"
	SeverityMedium        Severity = "medium"
	SeverityLow           Severity = "low"
	SeverityInformational Severity = "informational"
)

// severityRank gives each severity a sort position, critical first.
var severityRank = map[Severity]int{
	SeverityCritical:      0,
	SeverityHigh:          1,
	SeverityMedium:        2,
	SeverityLow:           3,
	SeverityInformational: 4,
}

// Rank returns the sort position of s, critical = 0. An invalid Severity
// sorts after every valid one instead of colliding with SeverityInformational.
func (s Severity) Rank() int {
	if r, ok := severityRank[s]; ok {
		return r
	}
	return len(severityRank)
}

// Valid reports whether s is one of the five declared severities.
func (s Severity) Valid() bool {
	_, ok := severityRank[s]
	return ok
}

func (s Severity) String() string {
	return string(s)
}
