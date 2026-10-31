package api

import (
	"testing"

	"nectar_apps/internal/webauthn/webauthntest"
)

func TestPasskeys(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	auth := webauthntest.New(e.srv.URL)

	// add a passkey while signed in
	_, opt, _ := e.do(o, "POST", "/api/me/passkeys/options", nil)
	opts := opt["options"].(map[string]any)
	if opts["rp"].(map[string]any)["id"] != "127.0.0.1" {
		t.Fatalf("rp: %v", opts["rp"])
	}
	cred := auth.Create(opts)
	if code, _, _ := e.do(o, "POST", "/api/me/passkeys", map[string]any{"flow": "nope", "name": "x", "credential": cred}); code != 400 {
		t.Fatalf("unknown flow: %d", code)
	}
	code, pk, _ := e.do(o, "POST", "/api/me/passkeys", map[string]any{"flow": opt["flow"], "name": "MacBook", "credential": cred})
	if code != 201 || pk["name"] != "MacBook" || pk["synced"] != true {
		t.Fatalf("add: %d %v", code, pk)
	}
	pkid := itoa(pk["id"].(float64))
	// the same authenticator is excluded next time
	_, opt2, _ := e.do(o, "POST", "/api/me/passkeys/options", nil)
	if ex := opt2["options"].(map[string]any)["excludeCredentials"].([]any); len(ex) != 1 {
		t.Fatalf("exclude: %v", ex)
	}

	// sign in with it in a fresh browser: no email, no password
	browser := e.client()
	login := func() (int, map[string]any) {
		_, lo, _ := e.do(browser, "POST", "/api/auth/passkey/options", nil)
		code, v, _ := e.do(browser, "POST", "/api/auth/passkey", map[string]any{"flow": lo["flow"], "credential": auth.Get(lo["options"].(map[string]any))})
		return code, v
	}
	if code, v := login(); code != 200 || v["user"] == nil {
		t.Fatalf("passkey login: %d %v", code, v)
	}
	if code, me, _ := e.do(browser, "GET", "/api/me", nil); code != 200 || me["is_owner"] != true {
		t.Fatalf("me: %d %v", code, me)
	}
	_, _, list := e.do(o, "GET", "/api/me/passkeys", nil)
	if len(list) != 1 || list[0].(map[string]any)["last_used_at"] == nil {
		t.Fatalf("list: %v", list)
	}
	// a replayed flow fails
	_, lo, _ := e.do(browser, "POST", "/api/auth/passkey/options", nil)
	assertion := auth.Get(lo["options"].(map[string]any))
	e.do(browser, "POST", "/api/auth/passkey", map[string]any{"flow": lo["flow"], "credential": assertion})
	if code, _, _ := e.do(browser, "POST", "/api/auth/passkey", map[string]any{"flow": lo["flow"], "credential": assertion}); code != 400 {
		t.Fatalf("replay: %d", code)
	}
	// rename, delete → the passkey no longer signs in
	if code, _, _ := e.do(o, "PATCH", "/api/me/passkeys/"+pkid, map[string]any{"name": "Work Mac"}); code != 204 {
		t.Fatalf("rename: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/me/passkeys/"+pkid, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := login(); code != 401 {
		t.Fatalf("deleted passkey: %d", code)
	}
}
