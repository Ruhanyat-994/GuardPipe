package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// KeySize is the required AES-256 key length in bytes.
// GUARDPIPE_ENCRYPTION_KEY is documented as "base64 of exactly 32 bytes"
// (documentation/13-devops-and-environments.md §5.3) — platform/config
// decodes it before it ever reaches this package.
const KeySize = 32

// NonceSize is the standard GCM nonce length. `project_credentials.nonce`
// is documented as "12 bytes, unique per row"
// (documentation/06-database-design.md §4.6).
const NonceSize = 12

// ErrInvalidKeySize is returned when key is not exactly KeySize bytes.
var ErrInvalidKeySize = fmt.Errorf("crypto: key must be exactly %d bytes for AES-256", KeySize)

// Encrypt encrypts plaintext under key with AES-256-GCM, using a fresh
// random nonce. It returns the ciphertext and the nonce separately, matching
// the `project_credentials` table's separate `ciphertext`/`nonce` columns —
// the caller stores both, and must pass the same nonce back to Decrypt.
func Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt reverses Encrypt. It fails if key, nonce, or ciphertext were
// tampered with or don't match — GCM authenticates the ciphertext, it does
// not just decrypt it.
func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("crypto: invalid nonce size")
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
