// Package secretbox encrypts small secrets (env values, registry passwords, tokens) at rest
// with AES-256-GCM, using a key derived from the panel master key.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const prefix = "v1:"

// ErrDecrypt is returned when a value cannot be decrypted (wrong key or corrupted data).
var ErrDecrypt = errors.New("secretbox: cannot decrypt value (wrong master key or corrupted data)")

// Box seals and opens values. It is safe for concurrent use.
type Box struct{ aead cipher.AEAD }

// New derives an encryption key from masterKey (32+ bytes) for the given purpose,
// so different subsystems never share a key.
func New(masterKey []byte, purpose string) (*Box, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("secretbox: master key must be at least 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, masterKey, nil, "nectar-apps/"+purpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext and returns "v1:<base64(nonce|ciphertext)>".
func (b *Box) Seal(plaintext string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed string) (string, error) {
	if !strings.HasPrefix(sealed, prefix) {
		return "", ErrDecrypt
	}
	raw, err := base64.RawStdEncoding.DecodeString(sealed[len(prefix):])
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", ErrDecrypt
	}
	nonce, ct := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	pt, err := b.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}
