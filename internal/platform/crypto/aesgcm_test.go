package crypto_test

import (
	"bytes"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
)

func testKey(t *testing.T, fill byte) []byte {
	t.Helper()
	key := make([]byte, crypto.KeySize)
	for i := range key {
		key[i] = fill
	}
	return key
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := testKey(t, 0x01)
	plaintext := []byte("ghp_1234567890abcdefghijklmnopqrstuv")

	ciphertext, nonce, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(nonce) != crypto.NonceSize {
		t.Errorf("nonce length = %d, want %d", len(nonce), crypto.NonceSize)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Error("ciphertext contains the plaintext in the clear")
	}

	got, err := crypto.Decrypt(key, ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	right := testKey(t, 0x01)
	wrong := testKey(t, 0x02)
	ciphertext, nonce, err := crypto.Encrypt(right, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if _, err := crypto.Decrypt(wrong, ciphertext, nonce); err == nil {
		t.Error("Decrypt() with the wrong key succeeded, want an error")
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	key := testKey(t, 0x03)
	ciphertext, nonce, err := crypto.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	tampered := bytes.Clone(ciphertext)
	tampered[0] ^= 0xFF

	if _, err := crypto.Decrypt(key, tampered, nonce); err == nil {
		t.Error("Decrypt() of tampered ciphertext succeeded, want an error — GCM must authenticate, not just decrypt")
	}
}

func TestDecrypt_WrongNonceFails(t *testing.T) {
	key := testKey(t, 0x04)
	ciphertext, _, err := crypto.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	wrongNonce := make([]byte, crypto.NonceSize)

	if _, err := crypto.Decrypt(key, ciphertext, wrongNonce); err == nil {
		t.Error("Decrypt() with the wrong nonce succeeded, want an error")
	}
}

func TestEncrypt_RejectsWrongKeySize(t *testing.T) {
	tooShort := make([]byte, 16) // AES-128 length, not AES-256
	if _, _, err := crypto.Encrypt(tooShort, []byte("secret")); err == nil {
		t.Error("Encrypt() with a 16-byte key succeeded, want ErrInvalidKeySize")
	}
}

func TestEncrypt_NonceIsRandomPerCall(t *testing.T) {
	key := testKey(t, 0x05)
	_, nonceA, err := crypto.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	_, nonceB, err := crypto.Encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if bytes.Equal(nonceA, nonceB) {
		t.Error("two Encrypt() calls produced the same nonce — nonce reuse breaks GCM's guarantees")
	}
}
