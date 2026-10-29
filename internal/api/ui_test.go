package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The native apps run first-run setup with client=mobile and get a token pair instead of a cookie.
func TestSetupMobileClient(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	if code, _, _ := e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "desktop"}); code != 400 {
		t.Fatalf("unknown client: %d", code)
	}
	code, v, _ := e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	if code != 201 || v["access_token"] == nil || v["refresh_token"] == nil {
		t.Fatalf("mobile setup: %d %v", code, v)
	}
	plain := &http.Client{}
	if code, me, _ := e.do(plain, "GET", "/api/me", nil, "Authorization", "Bearer "+v["access_token"].(string)); code != 200 || me["email"] != "o@x.io" {
		t.Fatalf("bearer: %d %v", code, me)
	}
}

// Development mode answers CORS preflights from localhost origins (the Flutter dev server) only.
func TestDevCORS(t *testing.T) {
	e := newEnv(t) // Dev: true
	pre := func(origin string) *http.Response {
		req, _ := http.NewRequest(http.MethodOptions, e.srv.URL+"/api/me", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if r := pre("http://localhost:61234"); r.StatusCode != 204 || r.Header.Get("Access-Control-Allow-Origin") != "http://localhost:61234" ||
		r.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("localhost preflight: %d %v", r.StatusCode, r.Header)
	}
	if r := pre("https://evil.example"); r.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign origin allowed: %v", r.Header)
	}
	if !isLocalOrigin("http://127.0.0.1:5000") || isLocalOrigin("http://localhost.evil.io") || isLocalOrigin("https://localhost") {
		t.Fatal("isLocalOrigin")
	}
	e.api.cfg.Dev = false
	prod := httpHandlerStatus(t, e)
	if prod != "" {
		t.Fatalf("CORS in production: %q", prod)
	}
}

func httpHandlerStatus(t *testing.T, e *testEnv) string {
	t.Helper()
	srv := e.api.Handler() // rebuilt with Dev=false
	req, _ := http.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "http://localhost:61234")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Header().Get("Access-Control-Allow-Origin")
}
