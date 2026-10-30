package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/config"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/routing"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
)

const gwToken = "gw-service-token-0123456789"

// fakeGateway is an in-memory nectar-gateway admin API.
type fakeGateway struct {
	mu       sync.Mutex
	routes   map[int64]map[string]any
	next     int64
	updates  int
	settings map[string]any
	unblock  []string
	block    []map[string]any
	features []string
	certs    map[int64]map[string]any
	covered  map[string]string   // host → certificate domain (covered_by on routes)
	dns      map[string][]string // name → IPs (records the gateway created)
	foreign  map[string]bool     // names with records someone else created
}

func (f *fakeGateway) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+gwToken {
				w.WriteHeader(401)
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	normalize := func(rt map[string]any) {
		if h, ok := rt["basic_auth_htpasswd"].(string); ok {
			rt["auth_fingerprint"] = gateway.AuthFingerprint(h)
			delete(rt, "basic_auth_htpasswd")
			rt["_htpasswd"] = h
		}
	}
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		feats := append([]string{}, f.features...)
		f.mu.Unlock()
		js(w, map[string]any{"version": "test", "api_version": 1, "features": feats})
	})
	mux.HandleFunc("GET /api/routes", auth(func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, rt := range f.routes {
			if c, ok := f.covered[rt["domain"].(string)]; ok {
				rt["covered_by"], rt["covered_until"] = c, float64(time.Now().Add(80*24*time.Hour).Unix())
			}
			out = append(out, rt)
		}
		js(w, out)
	}))
	mux.HandleFunc("POST /api/routes", auth(func(w http.ResponseWriter, r *http.Request) {
		var rt map[string]any
		json.NewDecoder(r.Body).Decode(&rt)
		f.next++
		rt["id"] = float64(f.next)
		normalize(rt)
		f.routes[f.next] = rt
		if c, ok := f.covered[rt["domain"].(string)]; ok {
			rt["covered_by"], rt["covered_until"] = c, float64(time.Now().Add(80*24*time.Hour).Unix())
		}
		w.WriteHeader(201)
		js(w, rt)
	}))
	mux.HandleFunc("PUT /api/routes/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var rt map[string]any
		json.NewDecoder(r.Body).Decode(&rt)
		rt["id"] = float64(id)
		normalize(rt)
		f.routes[id] = rt
		f.updates++
		if c, ok := f.covered[rt["domain"].(string)]; ok {
			rt["covered_by"], rt["covered_until"] = c, float64(time.Now().Add(80*24*time.Hour).Unix())
		}
		js(w, rt)
	}))
	mux.HandleFunc("DELETE /api/routes/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		delete(f.routes, id)
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /api/metrics", auth(func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query().Get("domain")
		js(w, map[string]any{"enabled": true, "domains": []map[string]any{{"domain": d, "requests": 42, "p95_ms": 25}}})
	}))
	mux.HandleFunc("GET /api/access-log", auth(func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query().Get("domain")
		js(w, []map[string]any{{"time": "2026-10-18T01:00:0" + strconv.Itoa(len(d)%10) + "Z", "host": d, "path": "/", "status": 200}})
	}))
	mux.HandleFunc("GET /api/settings", auth(func(w http.ResponseWriter, r *http.Request) { js(w, f.settings) }))
	mux.HandleFunc("PUT /api/settings", auth(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		for k, v := range in {
			if s, _ := v.(string); strings.Contains(s, "://") {
				w.WriteHeader(400)
				js(w, map[string]string{"error": k + " must be host:port"})
				return
			}
			f.settings[k] = v
		}
		js(w, f.settings)
	}))
	mux.HandleFunc("GET /api/certificates", auth(func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, c := range f.certs {
			out = append(out, c)
		}
		js(w, out)
	}))
	mux.HandleFunc("POST /api/certificates", auth(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		for _, c := range f.certs {
			if c["domain"] == in["domain"] {
				w.WriteHeader(409)
				js(w, map[string]string{"error": "a certificate for this domain already exists"})
				return
			}
		}
		id := int64(len(f.certs) + 1)
		in["id"], in["status"], in["acme"] = float64(id), "pending", true
		f.certs[id] = in
		w.WriteHeader(201)
		js(w, in)
	}))
	mux.HandleFunc("POST /api/certificates/{id}/renew", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if f.certs[id] == nil {
			w.WriteHeader(404)
			js(w, map[string]string{"error": "certificate not found"})
			return
		}
		w.WriteHeader(202)
		js(w, f.certs[id])
	}))
	mux.HandleFunc("DELETE /api/certificates/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		delete(f.certs, id)
		w.WriteHeader(204)
	}))
	mux.HandleFunc("PUT /api/dns/records", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string   `json:"name"`
			IPs  []string `json:"ips"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if f.foreign[in.Name] {
			w.WriteHeader(409)
			js(w, map[string]any{"zone": "z", "records": []any{}, "conflict": in.Name + " already has records the gateway did not create"})
			return
		}
		if f.dns == nil {
			f.dns = map[string][]string{}
		}
		f.dns[in.Name] = in.IPs
		js(w, map[string]any{"zone": "z", "records": []map[string]any{{"type": "A", "name": in.Name, "content": in.IPs[0], "managed": true}}, "created": len(in.IPs)})
	}))
	mux.HandleFunc("DELETE /api/dns/records", auth(func(w http.ResponseWriter, r *http.Request) {
		n := r.URL.Query().Get("name")
		_, had := f.dns[n]
		delete(f.dns, n)
		js(w, map[string]int{"deleted": map[bool]int{true: 1}[had]})
	}))
	mux.HandleFunc("GET /api/blocked", auth(func(w http.ResponseWriter, r *http.Request) { js(w, []map[string]string{{"ip": "6.6.6.6"}}) }))
	mux.HandleFunc("POST /api/blocked", auth(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		f.block = append(f.block, in)
		w.WriteHeader(204)
	}))
	mux.HandleFunc("DELETE /api/blocked/{ip}", auth(func(w http.ResponseWriter, r *http.Request) {
		f.unblock = append(f.unblock, r.PathValue("ip"))
		w.WriteHeader(204)
	}))
	return mux
}

func (f *fakeGateway) byDomain(d string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rt := range f.routes {
		if rt["domain"] == d {
			return rt
		}
	}
	return nil
}

func newEnvGW(t *testing.T) (*testEnv, *fakeGateway) {
	t.Helper()
	cfg := config.Config{Dev: true, SetupToken: "setup-123", MasterKey: bytes.Repeat([]byte{9}, 32), MaxArtifactBytes: 10 << 20, BaseDomain: "apps.example.com"}
	st := openTestStore(t)
	fg := &fakeGateway{routes: map[int64]map[string]any{}, settings: map[string]any{"fallback_http": "", "fallback_tls": ""},
		certs: map[int64]map[string]any{}, covered: map[string]string{}}
	gws := httptest.NewServer(fg.handler(t))
	t.Cleanup(gws.Close)
	gw := gateway.New(gws.URL, gwToken)

	k8s := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(k8s.Close)
	kc, _ := kube.New(k8s.URL)
	cat, _ := runtimes.LoadBuiltin()
	arts, _ := artifacts.New(t.TempDir())
	box, _ := secretbox.New(bytes.Repeat([]byte{3}, 32), "env")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	syncer := routing.New(st, gw, logger)
	fake := kubefake.New()
	dep := deploy.New(st, fake, cat, box, logger, deploy.Options{InternalURL: "http://panel.svc", SigningKey: cfg.MasterKey,
		PollInterval: 5 * time.Millisecond, RolloutTimeout: 2 * time.Second, JobTimeout: 2 * time.Second})
	a := New(Deps{Config: cfg, Store: st, Kube: kc, Deployer: dep, Catalog: cat, Artifacts: arts, EnvBox: box, Log: logger, Gateway: gw, Syncer: syncer})
	if err := a.SyncBuiltinRoles(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return &testEnv{t: t, srv: srv, api: a, kube: fake, dep: dep}, fg
}

func TestDomainOptionsAndGatewayRelay(t *testing.T) {
	e, fg := newEnvGW(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, app, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"image": "ghcr.io/acme/grpc:1", "port": 9000}})
	aid := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d", code)
	}
	e.dep.Wait()

	// invalid options
	for _, bad := range []map[string]any{
		{"host": "x.example.com", "backend_protocol": "h3"},
		{"host": "x.example.com", "timeout_seconds": 99999},
		{"host": "x.example.com", "kind": "redirect"},
		{"host": "x.example.com", "kind": "redirect", "redirect_to": "ftp://a"},
		{"host": "x.example.com", "basic_auth_users": []map[string]string{{"user": "a"}}},
		{"host": "www.example.com", "with_www": true},
	} {
		if code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", bad); code != 400 {
			t.Errorf("accepted %v: %d %v", bad, code, v)
		}
	}

	// gRPC domain with basic auth, timeout, body limit and a www redirect
	code, d, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{
		"host": "grpc.example.com", "with_www": true, "backend_protocol": "h2c", "timeout_seconds": 300, "max_body_mb": 20,
		"basic_auth_users": []map[string]string{{"user": "qa", "password": "qa-password-1"}},
	})
	if code != 201 || d["backend_protocol"] != "h2c" || d["auth_users"].([]any)[0] != "qa" || d["basic_auth"] != nil {
		t.Fatalf("create domain: %d %v", code, d)
	}
	rt := fg.byDomain("grpc.example.com")
	if rt == nil || rt["backend_protocol"] != "h2c" || rt["timeout_seconds"].(float64) != 300 || rt["max_body_mb"].(float64) != 20 ||
		!strings.HasPrefix(rt["_htpasswd"].(string), "qa:$2") || rt["hc_enabled"] == true {
		t.Fatalf("gateway route: %v", rt)
	}
	www := fg.byDomain("www.grpc.example.com")
	if www == nil || www["mode"] != "redirect" || www["redirect_url"] != "https://grpc.example.com" {
		t.Fatalf("www redirect route: %v", www)
	}

	// a second sync must not rewrite unchanged routes (fingerprint matches)
	before := fg.updates
	if err := e.api.syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fg.updates != before {
		t.Fatalf("unchanged routes were updated %d times", fg.updates-before)
	}

	// keep the password, add a user → htpasswd changes, old hash kept
	did := itoa(d["id"].(float64))
	oldHash := strings.SplitN(fg.byDomain("grpc.example.com")["_htpasswd"].(string), "\n", 2)[0]
	code, d, _ = e.do(o, "PATCH", "/api/apps/"+aid+"/domains/"+did, map[string]any{"basic_auth_users": []map[string]string{{"user": "qa"}, {"user": "dev", "password": "dev-password-1"}}})
	if code != 200 || len(d["auth_users"].([]any)) != 2 {
		t.Fatalf("update users: %d %v", code, d)
	}
	h := fg.byDomain("grpc.example.com")["_htpasswd"].(string)
	if !strings.HasPrefix(h, oldHash+"\n") || !strings.Contains(h, "\ndev:$2") {
		t.Fatalf("htpasswd after update: %q", h)
	}
	// remove basic auth
	e.do(o, "PATCH", "/api/apps/"+aid+"/domains/"+did, map[string]any{"basic_auth_users": []any{}})
	if fg.byDomain("grpc.example.com")["_htpasswd"] != "" {
		t.Fatal("basic auth not removed")
	}

	// per-app metrics + access log (default domain + grpc + www)
	code, m, _ := e.do(o, "GET", "/api/apps/"+aid+"/metrics?minutes=15", nil)
	if code != 200 || len(m["domains"].([]any)) != 3 || m["minutes"].(float64) != 15 {
		t.Fatalf("app metrics: %d %v", code, m)
	}
	if code, _, logs := e.do(o, "GET", "/api/apps/"+aid+"/access-log", nil); code != 200 || len(logs) != 3 {
		t.Fatalf("app access log: %d %v", code, logs)
	}

	// gateway relay endpoints
	if code, v, _ := e.do(o, "GET", "/api/gateway/metrics?domain=a.io&bogus=1", nil); code != 200 || v["enabled"] != true {
		t.Fatalf("gateway metrics: %d %v", code, v)
	}
	if code, _, arr := e.do(o, "GET", "/api/gateway/blocked", nil); code != 200 || len(arr) != 1 {
		t.Fatalf("blocked: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/gateway/blocked/6.6.6.6", nil); code != 204 || fg.unblock[0] != "6.6.6.6" {
		t.Fatalf("unblock: %d %v", code, fg.unblock)
	}
	if code, _, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"fallback_tls": "http://bad"}); code != 400 {
		t.Fatalf("bad gateway setting: %d", code)
	}
	if code, v, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"fallback_tls": "traefik.kube-system.svc:443"}); code != 200 || v["fallback_tls"] != "traefik.kube-system.svc:443" {
		t.Fatalf("gateway settings: %d %v", code, v)
	}

	// deleting the app's domain removes the gateway route
	e.do(o, "DELETE", "/api/apps/"+aid+"/domains/"+did, nil)
	if fg.byDomain("grpc.example.com") != nil {
		t.Fatal("route not deleted")
	}
}

func TestWildcardCertificates(t *testing.T) {
	e, fg := newEnvGW(t)
	o := e.owner()
	// an old gateway without the certificates API
	if code, v, _ := e.do(o, "GET", "/api/gateway/certificates", nil); code != 200 || v["supported"] != false || v["base_wildcard"] != "*.apps.example.com" {
		t.Fatalf("old gateway: %d %v", code, v)
	}
	if code, v, _ := e.do(o, "POST", "/api/gateway/certificates", map[string]any{"base_domain": true}); code != 409 || !strings.Contains(v["error"].(string), "1.2.2") {
		t.Fatalf("create on an old gateway: %d %v", code, v)
	}
	// certificates but no DNS provider yet
	fg.mu.Lock()
	fg.features = []string{"certificates", "acme"}
	fg.mu.Unlock()
	if code, v, _ := e.do(o, "POST", "/api/gateway/certificates", map[string]any{"base_domain": true}); code != 409 || !strings.Contains(v["error"].(string), "DNS provider") {
		t.Fatalf("wildcard without dns: %d %v", code, v)
	}
	// the DNS token goes to the gateway, never into the audit log
	if code, _, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"dns_provider": "Cloudflare", "dns_api_token": " cf-secret "}); code != 200 {
		t.Fatalf("dns settings: %d", code)
	}
	fg.mu.Lock()
	gotTok, gotProv := fg.settings["dns_api_token"], fg.settings["dns_provider"]
	fg.features = []string{"certificates", "acme", "dns-01"}
	fg.mu.Unlock()
	if gotTok != "cf-secret" || gotProv != "cloudflare" {
		t.Fatalf("relayed settings: %v %v", gotProv, gotTok)
	}
	_, _, audit := e.raw(o, "GET", "/api/audit?limit=5", nil)
	if strings.Contains(string(audit), "cf-secret") {
		t.Fatal("DNS token in the audit log")
	}
	// one click: *.<base domain> + apex, owned by the panel
	code, c, _ := e.do(o, "POST", "/api/gateway/certificates", map[string]any{"base_domain": true})
	if code != 201 || c["domain"] != "*.apps.example.com" || c["include_apex"] != true || c["managed_by"] != "nectar-apps" || c["owner"] != "base-domain" {
		t.Fatalf("create: %d %v", code, c)
	}
	if code, _, _ := e.do(o, "POST", "/api/gateway/certificates", map[string]any{"base_domain": true}); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	if code, v, _ := e.do(o, "GET", "/api/gateway/certificates", nil); code != 200 || v["dns"] != true || len(v["certificates"].([]any)) != 1 {
		t.Fatalf("list: %d %v", code, v)
	}
	cid := itoa(c["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/gateway/certificates/"+cid+"/renew", nil); code != 202 {
		t.Fatalf("renew: %d", code)
	}

	// a domain served by the wildcard is active with HTTPS, not "waiting for the certificate"
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": "nginx:1", "port": 80}})
	aid := itoa(app["id"].(float64))
	fg.mu.Lock()
	fg.covered["web-shop.apps.example.com"] = "*.apps.example.com"
	fg.mu.Unlock()
	e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil)
	e.dep.Wait()
	if err := e.api.syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, ds := e.raw(o, "GET", "/api/apps/"+aid+"/domains", nil)
	if !strings.Contains(string(ds), "HTTPS by the certificate *.apps.example.com") || strings.Contains(string(ds), "waiting for the certificate") {
		t.Fatalf("covered domain: %s", ds)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/gateway/certificates/"+cid, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
}
