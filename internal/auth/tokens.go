package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// Token prefixes make leaked tokens recognizable (e.g. by secret scanners) and show their kind.
const (
	PrefixSession = "nxs_" // web session cookie value
	PrefixAccess  = "nxa_" // mobile access token (short-lived)
	PrefixRefresh = "nxr_" // mobile refresh token
	PrefixAPI     = "nxk_" // API token for CI/CLI
)

// NewToken returns a new random token with the given prefix and its storage hash.
// Only the hash is ever stored; the plain token is shown to the client once.
func NewToken(prefix string) (plain, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	plain = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, HashToken(plain)
}

// HashToken returns the storage hash (hex SHA-256) of a token. Tokens have 256 bits of entropy,
// so a fast hash is appropriate here (unlike passwords).
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// HasPrefix reports whether token looks like a token of the given kind.
func HasPrefix(token, prefix string) bool { return strings.HasPrefix(token, prefix) }
