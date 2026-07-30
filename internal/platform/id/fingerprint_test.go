package id_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// TestFingerprint_GoldenVector pins the exact algorithm from
// documentation/06-database-design.md §6:
// SHA256(rule_id + "\x00" + normalized_location + "\x00" + normalized_evidence).
// If this ever fails after a refactor, the hash construction changed —
// which would silently invalidate every fingerprint already stored in a
// database, making every open finding look brand new.
func TestFingerprint_GoldenVector(t *testing.T) {
	got := id.Fingerprint(
		"codescan.injection.sql-string-concat",
		"internal/db/user.go:GetUser",
		"SELECT * FROM users WHERE name = <str>",
	)
	want := "e79b6c6eed39951e6f33c0e5397d7ce25b169598f990f02f748264608b2b0339"
	if got != want {
		t.Errorf("Fingerprint() = %s, want %s", got, want)
	}
}

func TestFingerprint_IsDeterministic(t *testing.T) {
	a := id.Fingerprint("rule.a", "loc.a", "ev.a")
	b := id.Fingerprint("rule.a", "loc.a", "ev.a")
	if a != b {
		t.Errorf("Fingerprint() is not deterministic: %s != %s", a, b)
	}
}

func TestFingerprint_IsHexSHA256Length(t *testing.T) {
	got := id.Fingerprint("rule.a", "loc.a", "ev.a")
	const sha256HexLen = 64
	if len(got) != sha256HexLen {
		t.Errorf("Fingerprint() length = %d, want %d (hex-encoded SHA-256)", len(got), sha256HexLen)
	}
}

// TestFingerprint_SensitiveToEachComponent is the near-miss half: changing
// only one of the three inputs must change the fingerprint, or two unrelated
// findings could collide.
func TestFingerprint_SensitiveToEachComponent(t *testing.T) {
	base := id.Fingerprint("rule.a", "loc.a", "ev.a")

	tests := []struct {
		name                                   string
		ruleID, normalizedLocation, normalized string
	}{
		{"different rule ID", "rule.b", "loc.a", "ev.a"},
		{"different location", "rule.a", "loc.b", "ev.a"},
		{"different evidence", "rule.a", "loc.a", "ev.b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := id.Fingerprint(tt.ruleID, tt.normalizedLocation, tt.normalized)
			if got == base {
				t.Errorf("changing %s did not change the fingerprint", tt.name)
			}
		})
	}
}

// TestFingerprint_StableAcrossLineNumberChanges documents the entire point
// of "normalized" location: the caller is responsible for excluding volatile
// details like line numbers before calling Fingerprint, so that a one-line
// insertion elsewhere in the file does not make an existing finding look new
// (documentation/06-database-design.md §6, "deliberately excluded" column).
func TestFingerprint_StableAcrossLineNumberChanges(t *testing.T) {
	// Two calls simulating "the same finding, one line lower after an
	// unrelated edit above it" — the caller already normalized both to the
	// same location string, so Fingerprint must treat them as identical.
	before := id.Fingerprint("codescan.injection.sql-string-concat", "internal/db/user.go:GetUser", "SELECT ... <str>")
	after := id.Fingerprint("codescan.injection.sql-string-concat", "internal/db/user.go:GetUser", "SELECT ... <str>")
	if before != after {
		t.Errorf("identical normalized inputs produced different fingerprints: %s != %s", before, after)
	}
}
