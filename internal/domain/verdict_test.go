package domain_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestVerdict_Valid(t *testing.T) {
	tests := []struct {
		name string
		v    domain.Verdict
		want bool
	}{
		{"pass is valid", domain.VerdictPass, true},
		{"warn is valid", domain.VerdictWarn, true},
		{"block is valid", domain.VerdictBlock, true},
		{"empty string is not valid", domain.Verdict(""), false},
		{"unknown value is not valid", domain.Verdict("fail"), false},
		{"wrong case is not valid", domain.Verdict("Pass"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Valid(); got != tt.want {
				t.Errorf("Verdict(%q).Valid() = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}
