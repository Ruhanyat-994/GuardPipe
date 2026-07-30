package domain

// Status is the triage state of a Finding. Findings are append-only apart
// from this field group (documentation/06-database-design.md §4.11,
// immutability rule DR-003) — application code may update Status and its
// accompanying reason/actor/timestamp, and nothing else on a Finding.
type Status string

const (
	StatusOpen          Status = "open"
	StatusAcknowledged  Status = "acknowledged"
	StatusSuppressed    Status = "suppressed"
	StatusFixed         Status = "fixed"
	StatusFalsePositive Status = "false_positive"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusAcknowledged, StatusSuppressed, StatusFixed, StatusFalsePositive:
		return true
	default:
		return false
	}
}

func (s Status) String() string {
	return string(s)
}
