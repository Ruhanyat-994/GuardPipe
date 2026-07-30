package crypto_test

import (
	"strings"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
)

func TestHashAndVerifyPassword_RoundTrip(t *testing.T) {
	hash, err := crypto.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	ok, err := crypto.VerifyPassword("correct-horse-battery-staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Error("VerifyPassword() = false, want true for the correct password")
	}
}

// TestVerifyPassword_WrongPasswordIsRejected is the near-miss half: a
// close-but-wrong password (one character off) must still fail.
func TestVerifyPassword_WrongPasswordIsRejected(t *testing.T) {
	hash, err := crypto.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	ok, err := crypto.VerifyPassword("correct-horse-battery-staplf", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if ok {
		t.Error("VerifyPassword() = true for a wrong password, want false")
	}
}

func TestHashPassword_EncodedFormat(t *testing.T) {
	hash, err := crypto.HashPassword("anything")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("HashPassword() = %q, want the documented $argon2id$v=19$m=65536,t=3,p=2$... prefix", hash)
	}
}

func TestHashPassword_SaltIsRandomPerCall(t *testing.T) {
	a, err := crypto.HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	b, err := crypto.HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if a == b {
		t.Error("hashing the same password twice produced identical output — salt is not being randomised")
	}
}

func TestVerifyPassword_MalformedHashReturnsError(t *testing.T) {
	_, err := crypto.VerifyPassword("anything", "not-a-real-hash")
	if err == nil {
		t.Error("VerifyPassword() error = nil, want an error for a malformed hash")
	}
}

// TestVerifyPassword_RejectsEachWayAHashCanBeCorrupted exercises the decode
// error paths individually — a corrupted database row must be reported as
// "malformed hash", never crash the process or be silently treated as a
// wrong password (which would make a login handler indistinguishable from a
// data-integrity bug).
func TestVerifyPassword_RejectsEachWayAHashCanBeCorrupted(t *testing.T) {
	valid, err := crypto.HashPassword("whatever")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	tests := []struct {
		name string
		hash string
	}{
		{"too few segments", "$argon2id$v=19$m=65536,t=3,p=2$onlysalt"},
		{"wrong algorithm", "$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"},
		{"unsupported version", "$argon2id$v=1$m=65536,t=3,p=2$c2FsdA$aGFzaA"},
		{"malformed params", "$argon2id$v=19$m=notanumber,t=3,p=2$c2FsdA$aGFzaA"},
		{"invalid base64 salt", "$argon2id$v=19$m=65536,t=3,p=2$not!base64$aGFzaA"},
		{"invalid base64 hash", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$not!base64"},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := crypto.VerifyPassword("whatever", tt.hash); err == nil {
				t.Errorf("VerifyPassword() error = nil for %q, want an error", tt.hash)
			}
		})
	}

	// Near-miss: a genuinely valid hash from HashPassword must not trip any
	// of the same checks.
	if _, err := crypto.VerifyPassword("whatever", valid); err != nil {
		t.Errorf("VerifyPassword() on a real hash returned an error: %v", err)
	}
}

// TestDummyHash_NeverVerifiesTrue guards the anti-timing-attack helper: it
// must always be a valid, verifiable-against hash format (so the constant
// work of Argon2id runs), but must never itself "match" — it exists to make
// a login handler's runtime constant, not to actually authenticate anyone.
func TestDummyHash_NeverVerifiesTrue(t *testing.T) {
	ok, err := crypto.VerifyPassword("whatever a real attacker might guess", crypto.DummyHash)
	if err != nil {
		t.Fatalf("VerifyPassword(DummyHash) error = %v, want a clean false", err)
	}
	if ok {
		t.Fatal("DummyHash matched an arbitrary password — it must never verify true")
	}
}
