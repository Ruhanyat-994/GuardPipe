package domain

// Confidence expresses how sure an engine is that a Finding is a true
// positive, independent of Severity (a low-confidence finding can still be
// critical if true).
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

func (c Confidence) Valid() bool {
	switch c {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return true
	default:
		return false
	}
}

func (c Confidence) String() string {
	return string(c)
}
