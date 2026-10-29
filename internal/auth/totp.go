package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit TOTP secret (base32, as authenticator apps expect it).
func NewTOTPSecret() string {
	var b [20]byte
	_, _ = rand.Read(b[:])
	return b32.EncodeToString(b[:])
}

// TOTPURL is the otpauth:// URL for QR codes.
func TOTPURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTPCode returns the 6-digit code of a 30-second step (RFC 6238, HMAC-SHA1).
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0F
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7FFFFFFF
	return fmt.Sprintf("%06d", v%1_000_000), nil
}

// VerifyTOTP checks a code against the current step ±1 (clock skew) and returns the matching step, so
// callers can refuse a code that was used already. step 0 means no match.
func VerifyTOTP(secret, code string, now time.Time) int64 {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0
	}
	cur := now.Unix() / 30
	for _, s := range []int64{cur, cur - 1, cur + 1} {
		want, err := TOTPCode(secret, s)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s
		}
	}
	return 0
}

// NewRecoveryCodes returns n one-time codes (xxxx-xxxx) and their hashes for storage.
func NewRecoveryCodes(n int) (plain, hashes []string) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	for range n {
		var b [8]byte
		_, _ = rand.Read(b[:])
		c := make([]byte, 0, 9)
		for i, x := range b {
			if i == 4 {
				c = append(c, '-')
			}
			c = append(c, alphabet[int(x)%len(alphabet)])
		}
		plain = append(plain, string(c))
		hashes = append(hashes, HashRecoveryCode(string(c)))
	}
	return plain, hashes
}

// HashRecoveryCode normalizes and hashes a recovery code.
func HashRecoveryCode(code string) string {
	c := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if len(c) == 8 {
		c = c[:4] + "-" + c[4:]
	}
	sum := sha256.Sum256([]byte("nectar-recovery:" + c))
	return hex.EncodeToString(sum[:])
}
