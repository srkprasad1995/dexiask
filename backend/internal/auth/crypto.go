package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// TokenCipher encrypts and decrypts GitHub OAuth tokens for storage at rest using
// AES-GCM. The key is provided as a hex string of 32, 48, or 64 chars (AES-128/
// 192/256).
type TokenCipher struct {
	gcm cipher.AEAD
}

// NewTokenCipher builds a TokenCipher from a hex-encoded key.
func NewTokenCipher(hexKey string) (*TokenCipher, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("token enc key must be hex: %w", err)
	}
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("token enc key must be 16/24/32 bytes (32/48/64 hex chars), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &TokenCipher{gcm: gcm}, nil
}

// Encrypt seals plaintext and returns a hex string of nonce||ciphertext.
func (c *TokenCipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (c *TokenCipher) Decrypt(hexCiphertext string) (string, error) {
	raw, err := hex.DecodeString(hexCiphertext)
	if err != nil {
		return "", err
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plaintext, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
