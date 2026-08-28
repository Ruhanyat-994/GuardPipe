package reporting

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// MinSuppressionReasonLength is FR-RPT-005's justification-length floor —
// suppressing a finding is a permanent-looking decision recorded in the
// exported report, so it has to come with a real reason, not a rubber
// stamp.
const MinSuppressionReasonLength = 20

// validTransitions is the triage state machine
// (documentation/05-module-specifications.md §15's mermaid diagram).
// fixed -> open is the regression path: a finding auto-marked fixed
// because it disappeared from a scan, then reappearing, reopens rather
// than staying silently closed.
var validTransitions = map[domain.Status][]domain.Status{
	domain.StatusOpen:          {domain.StatusAcknowledged, domain.StatusSuppressed, domain.StatusFalsePositive},
	domain.StatusAcknowledged:  {domain.StatusFixed, domain.StatusSuppressed},
	domain.StatusSuppressed:    {domain.StatusOpen},
	domain.StatusFalsePositive: {domain.StatusOpen},
	domain.StatusFixed:         {domain.StatusOpen},
}

var (
	// ErrInvalidTransition is returned for any (from, to) pair not listed in
	// validTransitions — including a same-status no-op, which is neither a
	// real state change nor something finding_status_history should record
	// a row for.
	ErrInvalidTransition = errors.New("reporting: invalid status transition")
	// ErrReasonTooShort is returned only when to is suppressed and reason
	// doesn't meet MinSuppressionReasonLength.
	ErrReasonTooShort = errors.New("reporting: suppression requires at least 20 characters of justification")
)

// ValidateTransition checks from -> to is a legal move in the triage state
// machine and, only when to is suppressed, that reason is long enough
// (FR-RPT-005). Pure — no I/O, easy to test exhaustively against the state
// diagram directly.
func ValidateTransition(from, to domain.Status, reason string) error {
	if !slices.Contains(validTransitions[from], to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	if to == domain.StatusSuppressed && len(strings.TrimSpace(reason)) < MinSuppressionReasonLength {
		return ErrReasonTooShort
	}
	return nil
}
