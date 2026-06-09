// Package secretbox seals small secrets (user API tokens) with AES-256-GCM
// for at-rest storage in Postgres. The random nonce is prepended to the
// ciphertext. The key comes from a K8s Secret, hex-encoded.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// Box seals and opens secrets with a fixed key.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a 64-char hex string (32-byte key).
func New(keyHex string) (*Box, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("secretbox: key must be 32 bytes (64 hex chars)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext; output is nonce || ciphertext.
func (b *Box) Seal(plaintext []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return b.aead.Seal(nonce, nonce, plaintext, nil)
}

// Open decrypts data produced by Seal.
func (b *Box) Open(data []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("secretbox: ciphertext too short")
	}
	pt, err := b.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: open: %w", err)
	}
	return pt, nil
}
