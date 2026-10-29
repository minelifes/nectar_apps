package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/auth"
)

func TestTwoFactorLogin(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password"})

	if code, v, _ := e.do(c, "POST", "/api/me/mfa/enable", map[string]string{"code": "000000"}); code != 409 {
		t.Fatalf("enable before setup: %d %v", code, v)
	}
	code, setup, _ := e.do(c, "POST", "/api/me/mfa/setup", nil)
	secret, _ := setup["secret"].(string)
	if code != 200 || len(secret) != 32 || !strings.HasPrefix(setup["otpauth_url"].(string), "otpauth://totp/") {
		t.Fatalf("setup: %d %v", code, setup)
	}
	if code, _, _ := e.do(c, "POST", "/api/me/mfa/enable", map[string]string{"code": "123456"}); code != 400 {
		t.Fatalf("wrong code: %d", code)
	}
	now, _ := auth.TOTPCode(secret, time.Now().Unix()/30)
	code, en, _ := e.do(c, "POST", "/api/me/mfa/enable", map[string]string{"code": now})
	if code != 200 || len(en["recovery_codes"].([]any)) != 10 {
		t.Fatalf("enable: %d %v", code, en)
	}
	recovery := en["recovery_codes"].([]any)[0].(string)
	if _, me, _ := e.do(c, "GET", "/api/me", nil); me["mfa_enabled"] != true {
		t.Fatalf("me: %v", me)
	}

	// login now needs the second factor
	b := e.client()
	code, v, _ := e.do(b, "POST", "/api/auth/login", map[string]string{"email": "o@x.io", "password": "owner-password"})
	tok, _ := v["mfa_token"].(string)
	if code != 200 || v["mfa_required"] != true || tok == "" {
		t.Fatalf("login: %d %v", code, v)
	}
	if code, _, _ := e.do(b, "GET", "/api/me", nil); code != 401 {
		t.Fatalf("session before the code: %d", code)
	}
	if code, _, _ := e.do(b, "POST", "/api/auth/mfa", map[string]string{"mfa_token": tok, "code": "000000"}); code != 401 {
		t.Fatalf("wrong code: %d", code)
	}
	// the code used for enabling cannot be replayed; the next step's code works
	next, _ := auth.TOTPCode(secret, time.Now().Unix()/30+1)
	if code, _, _ := e.do(b, "POST", "/api/auth/mfa", map[string]string{"mfa_token": tok, "code": now}); code != 401 {
		t.Fatalf("replayed code: %d", code)
	}
	if code, v, _ := e.do(b, "POST", "/api/auth/mfa", map[string]string{"mfa_token": tok, "code": next}); code != 200 || v["user"] == nil {
		t.Fatalf("mfa: %d %v", code, v)
	}
	if code, _, _ := e.do(b, "GET", "/api/me", nil); code != 200 {
		t.Fatalf("session after the code: %d", code)
	}

	// a recovery code works once
	m := e.client()
	_, v, _ = e.do(m, "POST", "/api/auth/login", map[string]string{"email": "o@x.io", "password": "owner-password", "client": "mobile"})
	tok = v["mfa_token"].(string)
	if code, v, _ := e.do(m, "POST", "/api/auth/mfa", map[string]string{"mfa_token": tok, "code": strings.ToUpper(recovery)}); code != 200 || v["access_token"] == nil {
		t.Fatalf("recovery: %d %v", code, v)
	}
	_, v, _ = e.do(m, "POST", "/api/auth/login", map[string]string{"email": "o@x.io", "password": "owner-password"})
	if code, _, _ := e.do(m, "POST", "/api/auth/mfa", map[string]string{"mfa_token": v["mfa_token"].(string), "code": recovery}); code != 401 {
		t.Fatalf("recovery code reused: %d", code)
	}
	if _, st, _ := e.do(c, "GET", "/api/me/mfa", nil); st["recovery_codes_left"].(float64) != 9 {
		t.Fatalf("status: %v", st)
	}

	// turning it off needs the password
	if code, _, _ := e.do(c, "POST", "/api/me/mfa/disable", map[string]string{"password": "nope"}); code != 403 {
		t.Fatalf("disable with a wrong password: %d", code)
	}
	if code, _, _ := e.do(c, "POST", "/api/me/mfa/disable", map[string]string{"password": "owner-password"}); code != 204 {
		t.Fatalf("disable: %d", code)
	}
	if code, v, _ := e.do(e.client(), "POST", "/api/auth/login", map[string]string{"email": "o@x.io", "password": "owner-password"}); code != 200 || v["mfa_required"] != nil {
		t.Fatalf("login without 2FA: %d %v", code, v)
	}
}

func TestIPAllowlist(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password"})
	via := func(ip string) []string { return []string{"X-Forwarded-For", ip} }

	if code, v, _ := e.do(c, "PUT", "/api/settings/access", map[string]any{"ip_allowlist": []string{"10.9.0.0/16"}}, via("203.0.113.7")...); code != 400 || !strings.Contains(v["error"].(string), "lock yourself out") {
		t.Fatalf("lockout guard: %d %v", code, v)
	}
	if code, _, _ := e.do(c, "PUT", "/api/settings/access", map[string]any{"ip_allowlist": []string{"nonsense"}}); code != 400 {
		t.Fatalf("bad entry: %d", code)
	}
	code, v, _ := e.do(c, "PUT", "/api/settings/access", map[string]any{"ip_allowlist": []string{"203.0.113.0/24", "198.51.100.9"}}, via("203.0.113.7")...)
	if code != 200 || len(v["ip_allowlist"].([]any)) != 2 {
		t.Fatalf("save: %d %v", code, v)
	}
	if code, _, _ := e.do(c, "GET", "/api/me", nil, via("192.0.2.1")...); code != 403 {
		t.Fatalf("outside the list: %d", code)
	}
	if code, _, _ := e.do(c, "GET", "/api/me", nil, via("198.51.100.9")...); code != 200 {
		t.Fatalf("inside the list: %d", code)
	}
	if code, _, _ := e.do(&http.Client{}, "GET", "/api/health", nil, via("192.0.2.1")...); code != 200 {
		t.Fatalf("health is always reachable: %d", code)
	}
	// a direct local connection (SSH tunnel) always works, so the list can be fixed
	if code, _, _ := e.do(c, "PUT", "/api/settings/access", map[string]any{"ip_allowlist": []string{}}); code != 200 {
		t.Fatalf("clear through the tunnel: %d", code)
	}
}
