// Package secrets wraps AES-256-GCM for at-rest encryption of opaque
// secrets (currently AI provider API keys). The key comes from
// GLOSSA_SECRETS_KEY (64-char hex = 32 bytes). Each Seal call generates
// a fresh 12-byte nonce; the caller stores ciphertext + nonce as
// distinct columns.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrKeyLength is returned by New when the hex key is the wrong size.
var ErrKeyLength = errors.New("secrets: key must be 64 hex chars (32 bytes)")

// Sealer encrypts and decrypts byte strings under a fixed AES-GCM key.
type Sealer struct {
	aead cipher.AEAD
}

// New parses a 64-char hex key and returns a ready Sealer. Returns
// ErrKeyLength if the input has the wrong size.
func New(hexKey string) (*Sealer, error) {
	if len(hexKey) != 64 {
		return nil, ErrKeyLength
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts plaintext under a fresh random nonce. Returns the
// ciphertext (including auth tag) and the nonce.
func (s *Sealer) Seal(plaintext []byte) (ct, nonce []byte, err error) {
	nonce = make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("secrets: nonce: %w", err)
	}
	ct = s.aead.Seal(nil, nonce, plaintext, nil)
	return ct, nonce, nil
}

// Open decrypts ciphertext with the given nonce.
func (s *Sealer) Open(ct, nonce []byte) ([]byte, error) {
	pt, err := s.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: open: %w", err)
	}
	return pt, nil
}
