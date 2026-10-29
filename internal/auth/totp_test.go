package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTOTP(t *testing.T) {
	// RFC 6238 test vector (SHA1, secret "12345678901234567890"), 8 digits truncated to 6
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	if c, _ := TOTPCode(secret, 59/30); c != "287082" {
		t.Errorf("t=59: %s", c)
	}
	if c, _ := TOTPCode(secret, 1111111109/30); c != "081804" {
		t.Errorf("t=1111111109: %s", c)
	}
	now := time.Unix(1111111109, 0)
	if VerifyTOTP(secret, "081804", now) == 0 || VerifyTOTP(secret, "081 804", now.Add(30*time.Second)) == 0 {
		t.Error("valid code refused")
	}
	if VerifyTOTP(secret, "081804", now.Add(2*time.Minute)) != 0 || VerifyTOTP(secret, "12345", now) != 0 {
		t.Error("stale or short code accepted")
	}
	if s := NewTOTPSecret(); len(s) != 32 {
		t.Errorf("secret %q", s)
	}
	if u := TOTPURL("Nectar Apps", "a@b.c", "ABC"); !strings.HasPrefix(u, "otpauth://totp/Nectar%20Apps:a@b.c?") || !strings.Contains(u, "secret=ABC") {
		t.Errorf("url %s", u)
	}
	plain, hashes := NewRecoveryCodes(10)
	if len(plain) != 10 || len(plain[0]) != 9 || plain[0][4] != '-' || HashRecoveryCode(strings.ToUpper(strings.ReplaceAll(plain[0], "-", ""))) != hashes[0] {
		t.Errorf("recovery codes %v", plain)
	}
}
