// Package crypto provides the two cryptographic primitives the rest of the
// system builds on: Argon2id password hashing and AES-256-GCM encryption
// for stored credentials (documentation/12-security-and-threat-model.md,
// "Crypto" row).
package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters, fixed per documentation/12-security-and-threat-model.md
// §"Password hashing": 64 MB memory, 3 iterations, parallelism 2. These are
// baked in rather than configurable — a security product should not let a
// misconfigured environment variable silently weaken password hashing.
const (
	argonMemoryKB   = 64 * 1024
	argonIterations = 3
	argonParallel   = 2
	argonSaltLen    = 16
	argonKeyLen     = 32
)

const argonVersion = argon2.Version

var (
	// ErrMalformedHash means the stored value isn't a hash this package
	// produced — a corrupted row, not a wrong password.
	ErrMalformedHash = errors.New("crypto: malformed argon2id hash")
	// ErrUnsupportedVariant means the hash was produced by a different
	// algorithm or parameter set than this package currently uses.
	ErrUnsupportedVariant = errors.New("crypto: unsupported argon2 variant or parameters")
)

// DummyHash is a valid-format Argon2id hash of a value nobody will ever type
// as a password. It exists so callers can run VerifyPassword against it on
// the "user not found" path of a login handler — Argon2id verification must
// always execute, even for unknown users, or the response-time difference
// between "unknown email" and "wrong password" becomes a user-enumeration
// timing side channel (documentation/12-security-and-threat-model.md, I9).
var DummyHash = mustHash("guardpipe-dummy-hash-do-not-use-as-a-real-password")

// HashPassword returns an Argon2id-encoded hash of password, in the
// standard PHC string format: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>.
// A fresh random salt is generated per call, so hashing the same password
// twice produces two different encoded hashes.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("crypto: generate salt: %w", err)
	}
	return encode(password, salt), nil
}

// VerifyPassword reports whether password matches encodedHash. It returns
// an error only for a malformed or unsupported hash — a simple wrong
// password is a false return, not an error, so callers can't accidentally
// treat "wrong password" and "corrupted hash" the same way.
func VerifyPassword(password, encodedHash string) (bool, error) {
	salt, want, params, err := decode(encodedHash)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, params.iterations, params.memoryKB, params.parallel, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

type argonParams struct {
	memoryKB   uint32
	iterations uint32
	parallel   uint8
}

func encode(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKB, argonParallel, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion, argonMemoryKB, argonIterations, argonParallel,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func decode(encodedHash string) (salt, hash []byte, params argonParams, err error) {
	parts := strings.Split(encodedHash, "$")
	// "" $argon2id $v=19 $m=...,t=...,p=... $salt $hash -> 6 parts, parts[0] == "".
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, argonParams{}, ErrMalformedHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, argonParams{}, ErrMalformedHash
	}
	if version != argonVersion {
		return nil, nil, argonParams{}, ErrUnsupportedVariant
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memoryKB, &params.iterations, &params.parallel); err != nil {
		return nil, nil, argonParams{}, ErrMalformedHash
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, argonParams{}, ErrMalformedHash
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, argonParams{}, ErrMalformedHash
	}
	return salt, hash, params, nil
}

func mustHash(password string) string {
	h, err := HashPassword(password)
	if err != nil {
		panic(err)
	}
	return h
}
