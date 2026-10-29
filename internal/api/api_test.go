package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/config"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/wsock"
)

type testEnv struct {
	t    *testing.T
	srv  *httptest.Server
	api  *Server
	kube *kubefake.Fake
	dep  *deploy.Deployer
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	cfg := config.Config{Dev: true, SetupToken: "setup-123", MasterKey: bytes.Repeat([]byte{9}, 32), MaxArtifactBytes: 10 << 20}
	st := openTestStore(t)

	// fake Kubernetes API for /api/server/nodes
	k8s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			w.Write([]byte(`{"gitVersion":"v1.34.1+k3s1"}`))
		case "/api/v1/nodes":
			w.Write([]byte(`{"items":[{"metadata":{"name":"n1"},"status":{"capacity":{"cpu":"2","memory":"4Gi"},"conditions":[{"type":"Ready","status":"True"}]}}]}`))
		case "/api/v1/namespaces/prj-shop/pods":
			w.Write([]byte(`{"items":[{"metadata":{"name":"api-1","creationTimestamp":"2026-10-28T10:00:00Z"},"status":{"phase":"Running"}}]}`))
		case "/api/v1/namespaces/prj-shop/pods/api-1/log":
			w.Write([]byte("2026-10-28T10:00:01Z hello from api\n"))
			if r.URL.Query().Get("follow") == "true" {
				w.(http.Flusher).Flush()
				w.Write([]byte("2026-10-28T10:00:02Z live line\n"))
				w.(http.Flusher).Flush()
				<-r.Context().Done() // a follow stream stays open
			}
		case "/api/v1/namespaces/prj-shop/pods/api-1/exec":
			if r.URL.Query().Get("tty") != "true" || r.URL.Query().Get("container") != "app" || len(r.URL.Query()["command"]) != 3 {
				http.Error(w, "bad exec request", 400)
				return
			}
			c, err := wsock.Upgrade(w, r)
			if err != nil {
				return
			}
			defer c.Close()
			for {
				op, msg, err := c.ReadMessage()
				if err != nil || op != wsock.Binary || len(msg) == 0 {
					return
				}
				switch msg[0] {
				case kube.ExecStdin:
					if strings.Contains(string(msg[1:]), "exit") {
						c.WriteMessage(wsock.Binary, append([]byte{kube.ExecError}, `{"status":"Failure","message":"command terminated with non-zero exit code","details":{"causes":[{"reason":"ExitCode","message":"3"}]}}`...))
						return
					}
					c.WriteMessage(wsock.Binary, append([]byte{kube.ExecStdout}, append([]byte("echo:"), msg[1:]...)...))
				case kube.ExecResize:
					c.WriteMessage(wsock.Binary, append([]byte{kube.ExecStdout}, append([]byte("size:"), msg[1:]...)...))
				}
			}
		case "/api/v1/namespaces/prj-shop/events":
			w.Write([]byte(`{"items":[{"type":"Warning","reason":"BackOff","message":"restarting","involvedObject":{"kind":"Pod","name":"api-1"},"lastTimestamp":"2026-10-28T10:05:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(k8s.Close)
	kc, _ := kube.New(k8s.URL)

	cat, _ := runtimes.LoadBuiltin()
	arts, _ := artifacts.New(t.TempDir())
	box, _ := secretbox.New(bytes.Repeat([]byte{3}, 32), "env")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := kubefake.New()
	dep := deploy.New(st, fake, cat, box, logger, deploy.Options{InternalURL: "http://panel.svc", SigningKey: cfg.MasterKey,
		PollInterval: 5 * time.Millisecond, RolloutTimeout: 2 * time.Second, JobTimeout: 2 * time.Second})
	a := New(Deps{Config: cfg, Store: st, Kube: kc, Deployer: dep, Catalog: cat, Artifacts: arts, EnvBox: box, Log: logger})
	if err := a.SyncBuiltinRoles(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return &testEnv{t: t, srv: srv, api: a, kube: fake, dep: dep}
}

// client returns an HTTP client with its own cookie jar (one browser).
func (e *testEnv) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (e *testEnv) do(c *http.Client, method, path string, body any, hdr ...string) (int, map[string]any, []any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var obj map[string]any
	var arr []any
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &arr)
	} else {
		_ = json.Unmarshal(raw, &obj)
	}
	return resp.StatusCode, obj, arr
}

func TestSetupLoginAndPermissions(t *testing.T) {
	e := newEnv(t)
	anon := e.client()

	// public endpoints
	if code, v, _ := e.do(anon, "GET", "/api/version", nil); code != 200 || v["api_version"].(float64) != 1 {
		t.Fatalf("version: %d %v", code, v)
	}
	if code, v, _ := e.do(anon, "GET", "/api/health", nil); code != 200 || v["kubernetes"] != "ok" {
		t.Fatalf("health: %d %v", code, v)
	}
	if _, v, _ := e.do(anon, "GET", "/api/setup", nil); v["required"] != true {
		t.Fatal("setup should be required")
	}
	if code, _, _ := e.do(anon, "GET", "/api/me", nil); code != 401 {
		t.Fatalf("anonymous /me: %d", code)
	}

	// setup: wrong token, weak password, then success
	if code, _, _ := e.do(anon, "POST", "/api/setup", map[string]string{"token": "nope", "email": "o@x.io", "name": "O", "password": "longenough1"}); code != 403 {
		t.Fatalf("bad setup token: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "short"}); code != 400 {
		t.Fatalf("weak password: %d", code)
	}
	owner := e.client()
	code, v, _ := e.do(owner, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "Owner@X.io", "name": "Owner", "password": "owner-password"})
	if code != 201 || v["user"].(map[string]any)["is_owner"] != true {
		t.Fatalf("setup: %d %v", code, v)
	}
	if code, _, _ := e.do(anon, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "z@x.io", "name": "Z", "password": "owner-password"}); code != 409 {
		t.Fatalf("second setup: %d", code)
	}

	// the setup response logged the owner in (cookie)
	if code, v, _ := e.do(owner, "GET", "/api/me", nil); code != 200 || v["email"] != "owner@x.io" {
		t.Fatalf("/me after setup: %d %v", code, v)
	}
	if _, v, _ := e.do(owner, "GET", "/api/me/permissions", nil); v["owner"] != true {
		t.Fatalf("owner permissions: %v", v)
	}
	if code, _, arr := e.do(owner, "GET", "/api/server/nodes", nil); code != 200 || len(arr) != 1 {
		t.Fatalf("nodes: %d %v", code, arr)
	}

	// CSRF: cookie-authenticated POST from another origin is rejected
	if code, _, _ := e.do(owner, "POST", "/api/users", map[string]string{"email": "c@x.io", "name": "C", "password": "password-123"}, "Origin", "https://evil.example"); code != 403 {
		t.Fatalf("cross-origin POST: %d", code)
	}

	// roles: built-ins exist and are read-only
	_, _, roles := e.do(owner, "GET", "/api/roles", nil)
	roleID := map[string]float64{}
	for _, r := range roles {
		m := r.(map[string]any)
		roleID[m["builtin_key"].(string)] = m["id"].(float64)
	}
	if len(roleID) != 4 {
		t.Fatalf("built-in roles: %v", roles)
	}
	if code, _, _ := e.do(owner, "PUT", "/api/roles/"+itoa(roleID["viewer"]), map[string]any{"name": "V", "permissions": []string{}}); code != 403 {
		t.Fatalf("editing builtin role: %d", code)
	}
	if code, _, _ := e.do(owner, "POST", "/api/roles", map[string]any{"name": "Bad", "permissions": []string{"app.fly"}}); code != 400 {
		t.Fatalf("unknown permission accepted: %d", code)
	}
	code, mgrRole, _ := e.do(owner, "POST", "/api/roles", map[string]any{"name": "User manager", "permissions": []string{"user.manage", "app.view"}})
	if code != 201 {
		t.Fatalf("create role: %d", code)
	}

	// users: create a user manager and a dev
	code, mgr, _ := e.do(owner, "POST", "/api/users", map[string]string{"email": "mgr@x.io", "name": "Mgr", "password": "manager-pass"})
	if code != 201 {
		t.Fatalf("create user: %d", code)
	}
	if code, _, _ := e.do(owner, "POST", "/api/users", map[string]string{"email": "MGR@x.io", "name": "Dup", "password": "manager-pass"}); code != 409 {
		t.Fatalf("duplicate user: %d", code)
	}
	_, dev, _ := e.do(owner, "POST", "/api/users", map[string]string{"email": "dev@x.io", "name": "Dev", "password": "developer-pass"})
	mgrID, devID := itoa(mgr["id"].(float64)), itoa(dev["id"].(float64))
	if code, _, _ := e.do(owner, "PUT", "/api/users/"+mgrID+"/bindings", []map[string]any{{"role_id": mgrRole["id"]}}); code != 200 {
		t.Fatalf("bind manager: %d", code)
	}

	// mobile login for the manager (bearer tokens)
	code, login, _ := e.do(anon, "POST", "/api/auth/login", map[string]string{"email": "mgr@x.io", "password": "manager-pass", "client": "mobile"})
	if code != 200 || !strings.HasPrefix(login["access_token"].(string), "nxa_") {
		t.Fatalf("mobile login: %d %v", code, login)
	}
	bearer := []string{"Authorization", "Bearer " + login["access_token"].(string)}
	if code, _, _ := e.do(anon, "GET", "/api/users", nil, bearer...); code != 200 {
		t.Fatalf("manager lists users: %d", code)
	}
	if code, _, _ := e.do(anon, "GET", "/api/server/nodes", nil, bearer...); code != 403 {
		t.Fatalf("manager without server.view: %d", code)
	}

	// escalation: the manager cannot grant Admin (it lacks most of Admin's permissions)…
	if code, v, _ := e.do(anon, "PUT", "/api/users/"+devID+"/bindings", []map[string]any{{"role_id": roleID["admin"]}}, bearer...); code != 403 {
		t.Fatalf("escalation allowed: %d %v", code, v)
	}
	// …nor give itself Admin, nor touch the owner
	if code, _, _ := e.do(anon, "PUT", "/api/users/"+mgrID+"/bindings", []map[string]any{{"role_id": roleID["admin"]}}, bearer...); code != 403 {
		t.Fatalf("self-escalation allowed: %d", code)
	}
	if code, _, _ := e.do(anon, "PATCH", "/api/users/1", map[string]any{"disabled": true}, bearer...); code != 403 {
		t.Fatalf("manager modified owner: %d", code)
	}

	// refresh rotates the access token; the old one stops working
	code, ref, _ := e.do(anon, "POST", "/api/auth/refresh", map[string]string{"refresh_token": login["refresh_token"].(string)})
	if code != 200 || ref["access_token"] == login["access_token"] {
		t.Fatalf("refresh: %d %v", code, ref)
	}
	if code, _, _ := e.do(anon, "GET", "/api/me", nil, bearer...); code != 401 {
		t.Fatalf("old access token still valid: %d", code)
	}

	// disabling a user ends their sessions
	newBearer := []string{"Authorization", "Bearer " + ref["access_token"].(string)}
	if code, _, _ := e.do(owner, "PATCH", "/api/users/"+mgrID, map[string]any{"disabled": true}); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	if code, _, _ := e.do(anon, "GET", "/api/me", nil, newBearer...); code != 401 {
		t.Fatalf("disabled user still authenticated: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/login", map[string]string{"email": "mgr@x.io", "password": "manager-pass"}); code != 401 {
		t.Fatalf("disabled user logged in: %d", code)
	}

	// last-owner protection and self-protection
	if code, _, _ := e.do(owner, "DELETE", "/api/users/1", nil); code != 400 {
		t.Fatalf("delete self: %d", code)
	}
	if code, _, _ := e.do(owner, "PATCH", "/api/users/1", map[string]any{"is_owner": false}); code != 409 {
		t.Fatalf("revoke last owner: %d", code)
	}

	// audit log has entries; logout ends the session
	if code, _, arr := e.do(owner, "GET", "/api/audit", nil); code != 200 || len(arr) < 5 {
		t.Fatalf("audit: %d %d", code, len(arr))
	}
	if code, _, _ := e.do(owner, "POST", "/api/auth/logout", nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code, _, _ := e.do(owner, "GET", "/api/me", nil); code != 401 {
		t.Fatalf("after logout: %d", code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	for i := 0; i < 10; i++ {
		if code, _, _ := e.do(c, "POST", "/api/auth/login", map[string]string{"email": "x@x.io", "password": "whatever-123"}); code != 401 {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code, _, _ := e.do(c, "POST", "/api/auth/login", map[string]string{"email": "x@x.io", "password": "whatever-123"}); code != 429 {
		t.Fatalf("expected 429, got %d", code)
	}
}

func TestUnknownAPIRouteAndBadJSON(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	if code, _, _ := e.do(c, "GET", "/api/nope", nil); code != 404 {
		t.Fatalf("unknown route: %d", code)
	}
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/login", strings.NewReader(`{"email":"a","password":"b","extra":1}`))
	resp, _ := c.Do(req)
	if resp.StatusCode != 400 {
		t.Fatalf("unknown field: %d", resp.StatusCode)
	}
}

func itoa(f float64) string { return strings.TrimSuffix(strings.TrimSuffix(jsonNum(f), ".0"), ".") }

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }
