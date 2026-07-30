package id

import (
	"crypto/sha256"
	"encoding/hex"
)

// separator matches the "\x00" join character in the fingerprint formula in
// documentation/06-database-design.md §6. A null byte cannot appear in any
// of the three normalised inputs, so it can't be produced by concatenating
// two components in a way that collides with a different three-way split.
const separator = "\x00"

// Fingerprint computes the stable, cross-scan identity of a finding:
//
//	fingerprint = SHA256( rule_id ‖ "\x00" ‖ normalized_location ‖ "\x00" ‖ normalized_evidence )
//
// (documentation/06-database-design.md §6). Callers are responsible for
// normalising location and evidence themselves — what "normalised" means is
// specific to each location type (e.g. a file location's normalised form
// excludes line numbers, since those shift on every unrelated edit; a
// dependency location's excludes the version, since the point is to track
// the advisory, not the exact version string). This function only owns the
// hash combination, not the per-engine normalisation rules.
func Fingerprint(ruleID, normalizedLocation, normalizedEvidence string) string {
	h := sha256.New()
	h.Write([]byte(ruleID))
	h.Write([]byte(separator))
	h.Write([]byte(normalizedLocation))
	h.Write([]byte(separator))
	h.Write([]byte(normalizedEvidence))
	return hex.EncodeToString(h.Sum(nil))
}
