// Package auth implements password hashing and opaque session/API tokens.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Password hashes use PBKDF2-HMAC-SHA256 from the standard library (OWASP 2023+: 600k iterations).
// The encoded format carries the algorithm and cost, so the scheme can be upgraded (e.g. to argon2id)
// later: Verify reports needsRehash and the caller re-hashes on the next successful login.
//
//	pbkdf2-sha256$<iterations>$<salt b64>$<hash b64>
const (
	pbkdf2Iterations = 600_000
	saltLen          = 16
	keyLen           = 32
	algPBKDF2        = "pbkdf2-sha256"

	// MinPasswordLen is the minimum accepted password length (in characters).
	MinPasswordLen = 10
	// MaxPasswordLen protects the KDF from huge inputs.
	MaxPasswordLen = 256
)

// ErrWeakPassword is returned by ValidatePassword.
var ErrWeakPassword = fmt.Errorf("password must be %d–%d characters", MinPasswordLen, MaxPasswordLen)

// ValidatePassword checks the password policy.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen || len(pw) > MaxPasswordLen {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword returns an encoded hash of pw.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iterations, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", algPBKDF2, pbkdf2Iterations, enc.EncodeToString(salt), enc.EncodeToString(dk)), nil
}

// VerifyPassword checks pw against an encoded hash. needsRehash is true when the hash uses
// weaker parameters than the current defaults.
func VerifyPassword(pw, encoded string) (ok, needsRehash bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != algPBKDF2 {
		return false, false, errors.New("auth: unsupported password hash format")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false, false, errors.New("auth: bad iteration count")
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false, false, errors.New("auth: bad hash encoding")
	}
	if len(pw) > MaxPasswordLen {
		return false, false, nil
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false, false, err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	return true, iter < pbkdf2Iterations, nil
}

// dummyHash is verified when a login names an unknown user, so response timing does not reveal
// which emails exist.
var dummyHash, _ = HashPassword("nectar-dummy-password")

// BurnTime performs a password verification against a dummy hash.
func BurnTime(pw string) { _, _, _ = VerifyPassword(pw, dummyHash) }
