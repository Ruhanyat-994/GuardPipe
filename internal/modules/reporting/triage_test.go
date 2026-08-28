package reporting_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

// TestValidateTransition_EveryLegalMove is the true-positive half of the
// state machine table (documentation/05-module-specifications.md §15's
// mermaid diagram) — every arrow in that diagram must be accepted.
func TestValidateTransition_EveryLegalMove(t *testing.T) {
	tests := []struct {
		name     string
		from, to domain.Status
		reasonOK string
	}{
		{"open -> acknowledged", domain.StatusOpen, domain.StatusAcknowledged, ""},
		{"open -> suppressed", domain.StatusOpen, domain.StatusSuppressed, strings.Repeat("a", 20)},
		{"open -> false_positive", domain.StatusOpen, domain.StatusFalsePositive, ""},
		{"acknowledged -> fixed", domain.StatusAcknowledged, domain.StatusFixed, ""},
		{"acknowledged -> suppressed", domain.StatusAcknowledged, domain.StatusSuppressed, strings.Repeat("a", 20)},
		{"suppressed -> open (reopen)", domain.StatusSuppressed, domain.StatusOpen, ""},
		{"false_positive -> open (reopen)", domain.StatusFalsePositive, domain.StatusOpen, ""},
		{"fixed -> open (regression)", domain.StatusFixed, domain.StatusOpen, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := reporting.ValidateTransition(tt.from, tt.to, tt.reasonOK); err != nil {
				t.Errorf("ValidateTransition(%s, %s) = %v, want nil", tt.from, tt.to, err)
			}
		})
	}
}

// TestValidateTransition_IllegalMoves is the near-miss half — every pair
// NOT in the diagram must be rejected, including same-status no-ops and
// "skipping" a state (e.g. open straight to fixed, which must go through
// acknowledged first).
func TestValidateTransition_IllegalMoves(t *testing.T) {
	tests := []struct {
		name     string
		from, to domain.Status
	}{
		{"open -> fixed (must acknowledge first)", domain.StatusOpen, domain.StatusFixed},
		{"open -> open (no-op)", domain.StatusOpen, domain.StatusOpen},
		{"suppressed -> fixed", domain.StatusSuppressed, domain.StatusFixed},
		{"suppressed -> acknowledged", domain.StatusSuppressed, domain.StatusAcknowledged},
		{"false_positive -> suppressed", domain.StatusFalsePositive, domain.StatusSuppressed},
		{"fixed -> suppressed", domain.StatusFixed, domain.StatusSuppressed},
		{"fixed -> fixed (no-op)", domain.StatusFixed, domain.StatusFixed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reporting.ValidateTransition(tt.from, tt.to, "")
			if !errors.Is(err, reporting.ErrInvalidTransition) {
				t.Errorf("ValidateTransition(%s, %s) = %v, want ErrInvalidTransition", tt.from, tt.to, err)
			}
		})
	}
}

func TestValidateTransition_SuppressionReasonLength(t *testing.T) {
	t.Run("near-miss: 19 characters is too short", func(t *testing.T) {
		err := reporting.ValidateTransition(domain.StatusOpen, domain.StatusSuppressed, strings.Repeat("a", 19))
		if !errors.Is(err, reporting.ErrReasonTooShort) {
			t.Errorf("err = %v, want ErrReasonTooShort", err)
		}
	})
	t.Run("true-positive: exactly 20 characters passes", func(t *testing.T) {
		err := reporting.ValidateTransition(domain.StatusOpen, domain.StatusSuppressed, strings.Repeat("a", 20))
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})
	t.Run("whitespace padding doesn't count toward the length", func(t *testing.T) {
		err := reporting.ValidateTransition(domain.StatusOpen, domain.StatusSuppressed, "   "+strings.Repeat("a", 10)+"   ")
		if !errors.Is(err, reporting.ErrReasonTooShort) {
			t.Errorf("err = %v, want ErrReasonTooShort (10 real characters padded with spaces)", err)
		}
	})
	t.Run("a reason is not required for any other target status", func(t *testing.T) {
		if err := reporting.ValidateTransition(domain.StatusOpen, domain.StatusAcknowledged, ""); err != nil {
			t.Errorf("err = %v, want nil — acknowledging needs no justification", err)
		}
		if err := reporting.ValidateTransition(domain.StatusOpen, domain.StatusFalsePositive, ""); err != nil {
			t.Errorf("err = %v, want nil — marking false_positive needs no justification", err)
		}
	})
}
