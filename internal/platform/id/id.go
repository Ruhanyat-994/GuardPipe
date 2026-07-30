// Package id provides the two identity primitives every other module needs:
// generating a new UUIDv4 primary key, and computing the stable fingerprint
// that lets a finding be tracked across repeated scans.
package id

import "github.com/google/uuid"

// New returns a random (version 4) UUID, matching the UUIDv4 primary key
// convention in documentation/06-database-design.md §1.
//
// uuid.NewRandom only fails if the system's entropy source (crypto/rand)
// fails, which is not a condition any caller can meaningfully recover from —
// so, like the standard library's own uuid.New, this panics instead of
// forcing every call site to handle an error that is never actually returned
// in practice.
func New() uuid.UUID {
	return uuid.Must(uuid.NewRandom())
}
