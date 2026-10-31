package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeOIDC is an OpenID provider with discovery, token and userinfo endpoints.
func fakeOIDC(t *testing.T, email string, groups []string) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{"authorization_endpoint": srv.URL + "/auth", "token_endpoint": srv.URL + "/token", "userinfo_endpoint": srv.URL + "/userinfo"})
		case "/token":
			r.ParseForm()
			if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" || r.Form.Get("client_secret") != "s3cret" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "at-1", "token_type": "Bearer"})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer at-1" {
				w.WriteHeader(401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"email": email, "email_verified": true, "name": "Ann Dev", "groups": groups})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSSOLogin(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	idp := fakeOIDC(t, "ann@example.com", []string{"devs"})
	e.api.ssoClient.HTTP = idp.Client()

	if code, _, _ := e.do(o, "PUT", "/api/settings/sso", []map[string]any{{"id": "corp", "kind": "oidc", "client_id": "nectar", "issuer": idp.URL,
		"auto_create": true, "enabled": true, "client_secret": "s3cret"}}); code != 400 {
		t.Fatalf("auto_create without domains accepted: %d", code)
	}
	code, v, _ := e.do(o, "PUT", "/api/settings/sso", []map[string]any{{"id": "corp", "kind": "oidc", "name": "Corp SSO", "client_id": "nectar", "issuer": idp.URL,
		"allowed_domains": []string{"example.com"}, "auto_create": true, "default_role": "viewer", "group_roles": map[string]string{"devs": "developer"},
		"enabled": true, "client_secret": "s3cret"}})
	if code != 200 || !strings.HasSuffix(v["providers"].([]any)[0].(map[string]any)["callback_url"].(string), "/api/auth/sso/corp/callback") {
		t.Fatalf("save providers: %d %v", code, v)
	}
	if _, _, body := e.raw(o, "GET", "/api/settings/sso", nil); strings.Contains(string(body), "s3cret") {
		t.Fatal("client secret returned")
	}
	if _, _, list := e.do(e.client(), "GET", "/api/auth/sso", nil); len(list) != 1 || list[0].(map[string]any)["name"] != "Corp SSO" {
		t.Fatalf("public list: %v", list)
	}

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := browser.Get(e.srv.URL + "/api/auth/sso/corp/start?next=/projects")
	if err != nil || resp.StatusCode != 302 {
		t.Fatalf("start: %v %v", err, resp)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if !strings.HasPrefix(loc.String(), idp.URL+"/auth") || loc.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL: %s", loc)
	}
	state := loc.Query().Get("state")

	// a wrong state (login CSRF) is refused
	resp, _ = browser.Get(e.srv.URL + "/api/auth/sso/corp/callback?code=good-code&state=forged")
	if !strings.Contains(resp.Header.Get("Location"), "sso_error") {
		t.Fatalf("forged state: %s", resp.Header.Get("Location"))
	}
	resp, _ = browser.Get(e.srv.URL + "/api/auth/sso/corp/start?next=/projects")
	loc, _ = url.Parse(resp.Header.Get("Location"))
	state = loc.Query().Get("state")
	resp, err = browser.Get(e.srv.URL + "/api/auth/sso/corp/callback?code=good-code&state=" + state)
	if err != nil || resp.StatusCode != 302 || resp.Header.Get("Location") != "/projects" {
		t.Fatalf("callback: %v %d %s", err, resp.StatusCode, resp.Header.Get("Location"))
	}
	code, me, _ := e.do(browser, "GET", "/api/me", nil)
	if code != 200 || me["email"] != "ann@example.com" {
		t.Fatalf("me after SSO: %d %v", code, me)
	}
	_, _, bs := e.do(o, "GET", fmt.Sprintf("/api/users/%v/bindings", me["id"]), nil)
	var roles []string
	for _, b := range bs {
		roles = append(roles, strings.ToLower(b.(map[string]any)["role_name"].(string)))
	}
	if r := strings.Join(roles, ","); !strings.Contains(r, "viewer") || !strings.Contains(r, "developer") {
		t.Fatalf("roles of the new user: %s", r)
	}
	// the same state cannot be used twice
	resp, _ = browser.Get(e.srv.URL + "/api/auth/sso/corp/callback?code=good-code&state=" + state)
	if !strings.Contains(resp.Header.Get("Location"), "sso_error") {
		t.Fatal("state reused")
	}
}

func TestSSODomainRefused(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	idp := fakeOIDC(t, "eve@evil.test", nil)
	e.api.ssoClient.HTTP = idp.Client()
	e.do(o, "PUT", "/api/settings/sso", []map[string]any{{"id": "corp", "kind": "oidc", "client_id": "nectar", "issuer": idp.URL,
		"allowed_domains": []string{"example.com"}, "auto_create": true, "enabled": true, "client_secret": "s3cret"}})
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ := browser.Get(e.srv.URL + "/api/auth/sso/corp/start")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	resp, _ = browser.Get(e.srv.URL + "/api/auth/sso/corp/callback?code=good-code&state=" + loc.Query().Get("state"))
	if !strings.Contains(resp.Header.Get("Location"), "not+allowed") {
		t.Fatalf("other domain: %s", resp.Header.Get("Location"))
	}
}
