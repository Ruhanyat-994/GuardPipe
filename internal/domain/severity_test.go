package domain_test

import (
	"sort"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestSeverity_Valid(t *testing.T) {
	tests := []struct {
		name string
		sev  domain.Severity
		want bool
	}{
		{"critical is valid", domain.SeverityCritical, true},
		{"high is valid", domain.SeverityHigh, true},
		{"medium is valid", domain.SeverityMedium, true},
		{"low is valid", domain.SeverityLow, true},
		{"informational is valid", domain.SeverityInformational, true},
		{"empty string is not valid", domain.Severity(""), false},
		{"unknown value is not valid", domain.Severity("catastrophic"), false},
		{"wrong case is not valid", domain.Severity("Critical"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sev.Valid(); got != tt.want {
				t.Errorf("Severity(%q).Valid() = %v, want %v", tt.sev, got, tt.want)
			}
		})
	}
}

func TestSeverity_Rank_SortsWorstFirst(t *testing.T) {
	in := []domain.Severity{
		domain.SeverityInformational,
		domain.SeverityCritical,
		domain.SeverityLow,
		domain.SeverityHigh,
		domain.SeverityMedium,
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Rank() < in[j].Rank() })

	want := []domain.Severity{
		domain.SeverityCritical,
		domain.SeverityHigh,
		domain.SeverityMedium,
		domain.SeverityLow,
		domain.SeverityInformational,
	}
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("sorted order = %v, want %v", in, want)
		}
	}
}

func TestSeverity_Rank_InvalidSortsLast(t *testing.T) {
	if domain.Severity("bogus").Rank() <= domain.SeverityInformational.Rank() {
		t.Errorf("an invalid severity must rank after every valid one")
	}
}
