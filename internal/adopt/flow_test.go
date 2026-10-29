package adopt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

// fakeGateway is a minimal nectar-gateway admin API.
type fakeGateway struct {
	mu       sync.Mutex
	routes   map[int64]map[string]any
	next     int64
	settings gateway.Settings
	certs    map[int64]string
	blocked  map[string]string
	hits     []gateway.FallbackHit
}

func newFakeGateway(t *testing.T) (*fakeGateway, *httptest.Server) {
	g := &fakeGateway{routes: map[int64]map[string]any{}, certs: map[int64]string{}, blocked: map[string]string{}}
	mux := http.NewServeMux()
	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		js(w, map[string]any{"version": "test", "api_version": 1})
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) { g.mu.Lock(); defer g.mu.Unlock(); js(w, g.settings) })
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		json.NewDecoder(r.Body).Decode(&g.settings)
		js(w, g.settings)
	})
	mux.HandleFunc("GET /api/routes", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		out := []map[string]any{}
		for _, rt := range g.routes {
			if mb := r.URL.Query().Get("managed_by"); r.URL.Query().Has("managed_by") && rt["managed_by"] != mb {
				continue
			}
			out = append(out, rt)
		}
		js(w, out)
	})
	mux.HandleFunc("POST /api/routes", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		var rt map[string]any
		json.NewDecoder(r.Body).Decode(&rt)
		for _, o := range g.routes {
			if o["domain"] == rt["domain"] && o["path_prefix"] == rt["path_prefix"] {
				w.WriteHeader(http.StatusConflict)
				js(w, map[string]string{"error": "exists"})
				return
			}
		}
		if rt["force_ssl"] == true && rt["acme"] != true {
			w.WriteHeader(http.StatusBadRequest)
			js(w, map[string]string{"error": "force_ssl requires a certificate"})
			return
		}
		g.next++
		rt["id"] = float64(g.next)
		g.routes[g.next] = rt
		w.WriteHeader(http.StatusCreated)
		js(w, rt)
	})
	mux.HandleFunc("PUT /api/routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var rt map[string]any
		json.NewDecoder(r.Body).Decode(&rt)
		rt["id"] = float64(id)
		if rt["force_ssl"] == true && rt["acme"] != true && g.certs[id] == "" {
			w.WriteHeader(http.StatusBadRequest)
			js(w, map[string]string{"error": "force_ssl requires a certificate"})
			return
		}
		g.routes[id] = rt
		js(w, rt)
	})
	mux.HandleFunc("DELETE /api/routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		delete(g.routes, id)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/routes/{id}/cert", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		g.certs[id] = in["cert_pem"]
		g.routes[id]["cert_source"] = "upload"
		js(w, g.routes[id])
	})
	mux.HandleFunc("POST /api/blocked", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		g.blocked[in["ip"].(string)] = in["reason"].(string)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/fallback-hits", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		js(w, map[string]any{"hosts": g.hits})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return g, ts
}

func (g *fakeGateway) domains() map[string]map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]map[string]any{}
	for _, r := range g.routes {
		out[fmt.Sprint(r["domain"], r["path_prefix"])] = r
	}
	return out
}

// k3sCluster is a stock k3s: bundled Traefik behind ServiceLB, one app with an Ingress.
func k3sCluster(t *testing.T) *kubefake.Fake {
	ctx := context.Background()
	k := kubefake.New()
	apply := func(o kube.Object) {
		t.Helper()
		if o["apiVersion"] == nil {
			o["apiVersion"] = "v1"
		}
		if _, err := k.Apply(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	for _, ns := range []string{"kube-system", "default", "shop"} {
		apply(obj("Namespace", "", ns, nil))
	}
	apply(obj("Node", "", "srv1", map[string]any{"status": map[string]any{
		"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
		"addresses":  []any{map[string]any{"type": "InternalIP", "address": "10.0.0.5"}}}}))
	apply(obj("HelmChart", "kube-system", "traefik", map[string]any{"spec": map[string]any{"chart": "traefik"}}))
	apply(obj("Deployment", "kube-system", "traefik", map[string]any{"spec": map[string]any{"replicas": float64(1), "template": map[string]any{
		"metadata": map[string]any{"labels": map[string]any{"app.kubernetes.io/name": "traefik"}},
		"spec":     map[string]any{"containers": []any{map[string]any{"name": "traefik", "image": "rancher/mirrored-library-traefik:3.3.6"}}}}}}))
	apply(obj("Service", "kube-system", "traefik", map[string]any{"spec": map[string]any{"type": "LoadBalancer",
		"selector": map[string]any{"app.kubernetes.io/name": "traefik"},
		"ports": []any{map[string]any{"name": "web", "port": float64(80), "nodePort": float64(31080)},
			map[string]any{"name": "websecure", "port": float64(443), "nodePort": float64(31443)}}},
		"status": map[string]any{"loadBalancer": map[string]any{"ingress": []any{map[string]any{"ip": "10.0.0.5"}}}}}))
	ds := obj("DaemonSet", "kube-system", "svclb-traefik-abc", map[string]any{"spec": map[string]any{"template": map[string]any{
		"spec": map[string]any{"containers": []any{map[string]any{"name": "lb-tcp-80", "image": "rancher/klipper-lb:v0.4.13"}}}}}})
	ds["metadata"].(map[string]any)["labels"] = map[string]any{"svccontroller.k3s.cattle.io/svcname": "traefik", "svccontroller.k3s.cattle.io/svcnamespace": "kube-system"}
	apply(ds)
	apply(withAnn(obj("IngressClass", "", "traefik", map[string]any{"spec": map[string]any{"controller": "traefik.io/ingress-controller"}}),
		map[string]any{"ingressclass.kubernetes.io/is-default-class": "true"}))
	apply(obj("Deployment", "shop", "web", map[string]any{"spec": map[string]any{"replicas": float64(2), "template": map[string]any{
		"spec": map[string]any{"containers": []any{map[string]any{"name": "web", "image": "nginx:1.27"}}}}}}))
	apply(svc("shop", "web", "http", 80))
	apply(secret("shop", "shop-tls", "kubernetes.io/tls", map[string]string{"tls.crt": testCertPEM(t), "tls.key": testKeyPEM}))
	apply(ingress("shop", "web", "", []any{map[string]any{"hosts": []any{"shop.example.com"}, "secretName": "shop-tls"}},
		rule("shop.example.com", path("/", "Prefix", "web", 80))))
	apply(ingress("shop", "legacy", "", nil, rule("legacy.example.com", path("/", "ImplementationSpecific", "web", 80))))
	k.Get("Ingress", "shop", "legacy")["metadata"].(map[string]any)["annotations"] = map[string]any{"traefik.ingress.kubernetes.io/router.middlewares": "shop-rewrite@kubernetescrd"}
	return k
}

func TestDiscoverK3sTraefik(t *testing.T) {
	k := k3sCluster(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"/etc/hostname": "srv1\n", "/var/lib/rancher/k3s/server/db/state.db": "x",
		"/etc/rancher/k3s/config.yaml": "write-kubeconfig-mode: \"0644\"\ndisable:\n  - metrics-server\n"})
	h := Host{Root: root, Runner: func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "k3s" {
			return "k3s version v1.34.1+k3s1 (abc)\ngo version go1.24", nil
		}
		return "", errors.New("not found")
	}}
	rep, err := Discover(context.Background(), k, h, "nectar-system")
	if err != nil {
		t.Fatal(err)
	}
	c := rep.EdgeController()
	if c == nil || c.Kind != "traefik" || !c.K3sBundled || !c.ServiceLB || c.Service != "kube-system/traefik" || len(c.Classes) != 1 {
		t.Fatalf("controller: %+v", rep.Controllers)
	}
	if rep.Edge != "controller:traefik" || rep.K3s.Version != "k3s version v1.34.1+k3s1 (abc)" || rep.K3s.Datastore != "sqlite" ||
		len(rep.K3s.Disabled) != 1 || rep.K3s.Nodes[0].InternalIP != "10.0.0.5" {
		t.Fatalf("report: edge=%s k3s=%+v", rep.Edge, rep.K3s)
	}
	if rep.Recommended() != StrategyMigrate || len(rep.Blockers) != 0 {
		t.Fatalf("strategy: %+v blockers %v", rep.Strategies, rep.Blockers)
	}
	m := byKey(rep.Routes)
	if !m["shop.example.com/"].Migratable() || m["legacy.example.com/"].Status != StatusFallback {
		t.Fatalf("routes: %+v", rep.Routes)
	}
	if len(rep.Hosts) != 1 || rep.TLSSecrets != 1 {
		t.Fatalf("hosts %v secrets %d", rep.Hosts, rep.TLSSecrets)
	}
	md := rep.Markdown()
	for _, want := range []string{"migration report", "shop.example.com", "✗", "`migrate` **(recommended)**", "install.sh --adopt --strategy=migrate"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}

	// extra ports on the Traefik Service block the cutover
	k.Get("Service", "kube-system", "traefik")["spec"].(map[string]any)["ports"] = append(
		k.Get("Service", "kube-system", "traefik")["spec"].(map[string]any)["ports"].([]any), map[string]any{"name": "pg", "port": float64(5432)})
	rep2, _ := Discover(context.Background(), k, h, "nectar-system")
	if len(rep2.Blockers) != 1 || rep2.Recommended() != StrategyKeep {
		t.Fatalf("extra port blocker: %v %+v", rep2.Blockers, rep2.Strategies)
	}
}

// installNectar adds what install.sh deploys in shadow mode.
func installNectar(t *testing.T, k *kubefake.Fake, adminURL string) {
	ctx := context.Background()
	k.Apply(ctx, obj("Namespace", "", "nectar-system", map[string]any{"apiVersion": "v1"}))
	k.Apply(ctx, secret("nectar-system", "nectar-secrets", "Opaque", map[string]string{"gateway-api-token": "tok"}))
	k.Apply(ctx, obj("Deployment", "nectar-system", "nectar-gateway", map[string]any{"apiVersion": "apps/v1", "spec": map[string]any{
		"replicas": float64(1), "template": map[string]any{"spec": map[string]any{"hostNetwork": true, "containers": []any{map[string]any{
			"name": "gateway", "image": "ghcr.io/minelifes/nectar-gateway:1.2.0", "env": []any{
				map[string]any{"name": "PROXY_ADDR", "value": ":18080"}, map[string]any{"name": "HTTPS_ADDR", "value": ":18443"},
				map[string]any{"name": "ADMIN_ADDR", "value": ":" + adminURL[strings.LastIndex(adminURL, ":")+1:]},
				map[string]any{"name": "GATEWAY_API_TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "nectar-secrets", "key": "gateway-api-token"}}},
			}}}}}}}))
}

// probe simulator: what answers on each port depends on the simulated phase.
type world struct {
	mu        sync.Mutex
	gwPorts   [2]int // gateway listening ports
	oldOn     bool   // old proxy still holds 80/443
	brokenNew bool   // gateway answers 502 (to test the rollback)
	calls     int
}

func (w *world) probe(ctx context.Context, scheme, ip string, port int, host, path string) Response {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	old := Response{Status: 200, ContentType: "text/html", BodySHA: "aa"}
	if strings.HasPrefix(host, "legacy") {
		old = Response{Status: 404, ContentType: "text/plain", BodySHA: "bb"}
	}
	gwServes := port == w.gwPorts[0] || port == w.gwPorts[1]
	switch {
	case (port == 80 || port == 443) && w.oldOn:
		return old
	case gwServes && w.brokenNew && !strings.HasPrefix(host, "legacy"):
		return Response{Status: 502}
	case gwServes:
		return old // the gateway proxies the same backend; legacy goes through the fallback to the old proxy
	}
	return Response{Error: "connection refused"}
}

func TestMigrateFlowTraefik(t *testing.T) {
	ctx := context.Background()
	k := k3sCluster(t)
	fg, ts := newFakeGateway(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"/etc/hostname": "srv1\n", "/var/lib/rancher/k3s/server/db/state.db": "sqlite-data",
		"/etc/rancher/k3s/config.yaml": "disable: [metrics-server]\n"})
	h := Host{Root: root, Runner: func(ctx context.Context, name string, args ...string) (string, error) { return "", nil }}
	w := &world{gwPorts: [2]int{ShadowHTTPPort, ShadowHTTPSPort}, oldOn: true}
	env := &Env{Kube: k, Host: h, SystemNamespace: "nectar-system", StateDir: "/var/lib/nectar", ProbeIP: "127.0.0.1",
		Prober: w.probe, Logf: t.Logf, Timeout: time.Minute}
	// k3s simulation: sleeping lets "k3s" react (helm-controller applies the HelmChartConfig, ServiceLB releases ports)
	env.Sleep = func(time.Duration) {
		if hc := k.Get("HelmChartConfig", "kube-system", "traefik"); hc != nil && strings.Contains(kube.Str(hc, "spec", "valuesContent"), "ClusterIP") {
			k.Get("Service", "kube-system", "traefik")["spec"].(map[string]any)["type"] = "ClusterIP"
			k.Delete(ctx, "DaemonSet", "kube-system", "svclb-traefik-abc")
			w.mu.Lock()
			w.oldOn = false
			w.mu.Unlock()
		}
	}
	env.Now = func() time.Time { return time.Now().UTC() }

	rep, err := Discover(ctx, k, h, "nectar-system")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := env.LoadState()
	if err := env.Backup(ctx, rep, st); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"k3s-state.db", "objects/Ingress.json", "objects/Secret.json", "files/etc/rancher/k3s/config.yaml", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(root, st.BackupDir, f)); err != nil {
			t.Fatalf("backup misses %s: %v", f, err)
		}
	}

	installNectar(t, k, ts.URL)
	env.Gateway = gateway.New(ts.URL, "tok")
	// the panel's Ingress sync (running in the cluster) pushes the Kubernetes routes
	sync := &IngressSync{Kube: k, Gateway: env.Gateway, Options: rep.Options, Log: discardLogger()}
	if err := sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	got := fg.domains()["shop.example.com/"]
	if got == nil || got["target_ip"] != "web.shop.svc.cluster.local" || got["managed_by"] != IngressSyncManagedBy {
		t.Fatalf("synced route: %v", fg.domains())
	}
	if fg.certs[int64(got["id"].(float64))] == "" {
		t.Fatal("TLS Secret not uploaded")
	}
	if _, ok := fg.domains()["legacy.example.com/"]; ok {
		t.Fatal("fallback host must not get a route")
	}
	// with an edge address, served Ingresses get it in their status (not the one left on the fallback)
	sk := &statusKube{Fake: k}
	withStatus := &IngressSync{Kube: sk, Gateway: env.Gateway, Options: rep.Options, Log: discardLogger(),
		Address: func(context.Context) string { return "203.0.113.7" }}
	if err := withStatus.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sk.patches) != 1 || !strings.HasSuffix(sk.patches[0], "/namespaces/shop/ingresses/web/status "+`{"status":{"loadBalancer":{"ingress":[{"ip":"203.0.113.7"}]}}}`) {
		t.Fatalf("ingress status patches: %q", sk.patches)
	}
	if err := env.Shadow(ctx, rep, st); err != nil {
		t.Fatal(err)
	}
	if fg.settings.FallbackHTTP != "traefik.kube-system.svc.cluster.local:80" || fg.settings.FallbackTLS != "traefik.kube-system.svc.cluster.local:443" {
		t.Fatalf("fallback: %+v", fg.settings)
	}
	v := env.Verify(ctx, rep, st)
	if v.Failed != 0 || v.OK != 3 { // shop http+https, legacy http (fallback)
		t.Fatalf("verify: %s", v.Text())
	}
	if err := env.SaveState(ctx, st, rep); err != nil {
		t.Fatal(err)
	}
	if cm := k.Get("ConfigMap", "nectar-system", ConfigMapName); cm == nil || !strings.Contains(kube.Str(cm, "data", "state.json"), `"verified"`) {
		t.Fatalf("configmap: %v", cm)
	}

	// cutover: gateway first, then Traefik → ClusterIP
	w.mu.Lock()
	w.gwPorts = [2]int{80, 443}
	w.mu.Unlock()
	if err := env.Cutover(ctx, rep, st, CutoverOptions{GateDelay: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if st.Phase != PhaseSoaking || containerEnv(k.Get("Deployment", "nectar-system", "nectar-gateway"), "PROXY_ADDR") != ":80" {
		t.Fatalf("after cutover: %s", st.Phase)
	}
	if vc := kube.Str(k.Get("HelmChartConfig", "kube-system", "traefik"), "spec", "valuesContent"); !strings.Contains(vc, "type: ClusterIP") {
		t.Fatalf("helmchartconfig: %q", vc)
	}

	// finalize refuses while a host still uses the fallback
	fg.mu.Lock()
	fg.hits = []gateway.FallbackHit{{Host: "legacy.example.com", HTTP: 3, LastSeen: time.Now().UTC()}}
	fg.mu.Unlock()
	if err := env.Finalize(ctx, rep, st, FinalizeOptions{}); err == nil || !strings.Contains(err.Error(), "legacy.example.com") {
		t.Fatalf("finalize should refuse: %v", err)
	}
	// rollback restores everything
	if err := env.Rollback(ctx, rep, st, false); err != nil {
		t.Fatal(err)
	}
	if k.Has("HelmChartConfig", "kube-system", "traefik") || containerEnv(k.Get("Deployment", "nectar-system", "nectar-gateway"), "PROXY_ADDR") != ":18080" {
		t.Fatal("rollback did not restore the HelmChartConfig / gateway ports")
	}
	if fg.settings.FallbackHTTP != "" {
		t.Fatalf("gateway settings not restored: %+v", fg.settings)
	}
}

func TestCutoverHealthGateRollsBack(t *testing.T) {
	ctx := context.Background()
	k := k3sCluster(t)
	_, ts := newFakeGateway(t)
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"/etc/hostname": "srv1\n", "/var/lib/rancher/k3s/server/db/state.db": "x"})
	h := Host{Root: root, Runner: func(ctx context.Context, name string, args ...string) (string, error) { return "", nil }}
	w := &world{gwPorts: [2]int{80, 443}, oldOn: true, brokenNew: true}
	env := &Env{Kube: k, Host: h, SystemNamespace: "nectar-system", StateDir: "/var/lib/nectar", ProbeIP: "127.0.0.1", Prober: w.probe, Timeout: time.Minute}
	env.Sleep = func(time.Duration) {
		hc := k.Get("HelmChartConfig", "kube-system", "traefik")
		w.mu.Lock()
		defer w.mu.Unlock()
		if hc != nil {
			k.Get("Service", "kube-system", "traefik")["spec"].(map[string]any)["type"] = "ClusterIP"
			k.Delete(ctx, "DaemonSet", "kube-system", "svclb-traefik-abc")
			w.oldOn = false
		} else {
			k.Get("Service", "kube-system", "traefik")["spec"].(map[string]any)["type"] = "LoadBalancer"
			w.oldOn = true
		}
	}
	rep, _ := Discover(ctx, k, h, "nectar-system")
	st, _ := env.LoadState()
	env.Backup(ctx, rep, st)
	installNectar(t, k, ts.URL)
	env.Gateway = gateway.New(ts.URL, "tok")
	st.Phase = PhaseVerified
	err := env.Cutover(ctx, rep, st, CutoverOptions{GateTries: 2, GateDelay: time.Millisecond})
	if !errors.Is(err, ErrRolledBack) || !strings.Contains(err.Error(), "health gate") || !strings.Contains(err.Error(), "status 200 → 502") {
		t.Fatalf("expected a health-gate rollback, got %v", err)
	}
	if st.Phase != PhaseRolledBack || k.Has("HelmChartConfig", "kube-system", "traefik") ||
		containerEnv(k.Get("Deployment", "nectar-system", "nectar-gateway"), "HTTPS_ADDR") != ":18443" {
		t.Fatalf("not rolled back: %s", st.Phase)
	}
}

const tcpTable = "header\n   0: 00000000:0050 00000000:0000 0A 0 0 0 0 0 1111 1\n   1: 00000000:01BB 00000000:0000 0A 0 0 0 0 0 1112 1\n"

func TestHostNginxRebindFlow(t *testing.T) {
	ctx := context.Background()
	k := kubefake.New()
	k.Apply(ctx, obj("Namespace", "", "default", map[string]any{"apiVersion": "v1"}))
	fg, ts := newFakeGateway(t)
	root := t.TempDir()
	conf := "events {}\nhttp {\n  server { listen 80; server_name app.example.com; return 301 https://$host$request_uri; }\n  server {\n    listen 443 ssl;\n    server_name app.example.com;\n" +
		"    ssl_certificate /etc/ssl/app.crt;\n    ssl_certificate_key /etc/ssl/app.key;\n" +
		"    location / { proxy_pass http://127.0.0.1:3000; proxy_set_header Host $host; client_max_body_size 10m; }\n  }\n}\n"
	writeFiles(t, root, map[string]string{"/etc/nginx/nginx.conf": conf, "/var/lib/rancher/k3s/server/db/state.db": "x",
		"/etc/ssl/app.crt": testCert, "/etc/ssl/app.key": testKeyPEM,
		"/proc/net/tcp":    tcpTable,
		"/proc/900/comm":   "nginx\n",
		"/proc/900/cgroup": "0::/system.slice/nginx.service\n"})
	fd := filepath.Join(root, "proc/900/fd")
	os.MkdirAll(fd, 0o755)
	os.Symlink("socket:[1111]", filepath.Join(fd, "3"))
	os.Symlink("socket:[1112]", filepath.Join(fd, "4"))
	var cmds []string
	h := Host{Root: root, Runner: func(ctx context.Context, name string, args ...string) (string, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		if name == "systemctl" && args[0] == "reload-or-restart" {
			// nginx listens where its (possibly rewritten) config says
			b, _ := os.ReadFile(filepath.Join(root, "etc/nginx/nginx.conf"))
			if strings.Contains(string(b), "10080") {
				os.WriteFile(filepath.Join(root, "proc/net/tcp"), []byte("header\n"), 0o644)
			} else {
				os.WriteFile(filepath.Join(root, "proc/net/tcp"), []byte(tcpTable), 0o644)
			}
		}
		return "", nil
	}}
	rep, err := Discover(ctx, k, h, "nectar-system")
	if err != nil {
		t.Fatal(err)
	}
	if hp := rep.EdgeHostProxy(); hp == nil || hp.Kind != "nginx" || hp.Unit != "nginx.service" || len(rep.Hosts) != 1 {
		t.Fatalf("host proxy: %+v hosts %v", rep.HostProxies, rep.Hosts)
	}
	w := &world{gwPorts: [2]int{80, 443}, oldOn: true}
	env := &Env{Kube: k, Host: h, SystemNamespace: "nectar-system", StateDir: "/var/lib/nectar", ProbeIP: "127.0.0.1", Prober: w.probe,
		Timeout: time.Minute, Sleep: func(time.Duration) {}}
	st, _ := env.LoadState()
	if err := env.Backup(ctx, rep, st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, st.BackupDir, "files/etc/nginx/nginx.conf")); err != nil {
		t.Fatal("nginx config not backed up")
	}
	installNectar(t, k, ts.URL)
	env.Gateway = gateway.New(ts.URL, "tok")
	if err := env.Shadow(ctx, rep, st); err != nil {
		t.Fatal(err)
	}
	if len(st.ImportedRoutes) != 1 {
		t.Fatalf("imported: %v", st.ImportedRoutes)
	}
	if r := fg.domains()["app.example.com/"]; r["force_ssl"] != true || fg.certs[int64(r["id"].(float64))] == "" || r["acme"] == true {
		t.Fatalf("imported route needs the certificate and the HTTPS redirect: %v", r)
	}
	st.Phase = PhaseVerified
	w.oldOn = false // nginx leaves 80 on reload
	if err := env.Cutover(ctx, rep, st, CutoverOptions{GateDelay: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "etc/nginx/nginx.conf"))
	if !strings.Contains(string(b), "listen 127.0.0.1:10080;") || !strings.Contains(strings.Join(cmds, "\n"), "systemctl reload-or-restart nginx.service") {
		t.Fatalf("rebind: %s\n%v", b, cmds)
	}
	if err := env.Rollback(ctx, rep, st, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(root, "etc/nginx/nginx.conf"))
	if string(b) != conf {
		t.Fatalf("nginx config not restored:\n%s", b)
	}
	if l := h.Listeners(80); len(l) != 1 || l[0].Process != "nginx" {
		t.Fatalf("nginx not back on 80: %+v", l)
	}
	// a report that does not know the proxy on 80 must not be used for a cutover
	empty := &Report{Edge: "none", Edges: map[int]EdgeHolder{}}
	st2 := &State{Phase: PhaseVerified}
	if err := env.Cutover(ctx, empty, st2, CutoverOptions{}); err == nil || !strings.Contains(err.Error(), "does not know") {
		t.Fatalf("stale report accepted: %v", err)
	}
}

func TestSplitPEM(t *testing.T) {
	c, k, err := splitPEM([]byte(testCertPEM(t)+testKeyPEM), []byte(testCertPEM(t)+testKeyPEM))
	if err != nil || !strings.Contains(c, "CERTIFICATE") || !strings.Contains(k, "PRIVATE KEY") || strings.Contains(c, "PRIVATE") {
		t.Fatalf("split: %v", err)
	}
}

// statusKube records the status subresource patches the Ingress sync sends.
type statusKube struct {
	*kubefake.Fake
	patches []string
}

func (s *statusKube) Do(_ context.Context, method, path, _ string, body io.Reader, _ any) error {
	b, _ := io.ReadAll(body)
	s.patches = append(s.patches, method+" "+path+" "+string(b))
	return nil
}

// A new adoption after a finished one (uninstall + install) starts from a clean state.
func TestStateRestart(t *testing.T) {
	for _, phase := range []string{PhaseRolledBack, PhaseFinalized} {
		st := &State{Phase: phase, Strategy: StrategyMigrate, Journal: []Step{{Kind: stepGatewayPorts}}, ImportedRoutes: []int64{3}, History: []string{"old"}}
		st.Fallback.HTTP = "127.0.0.1:10080"
		st.restart()
		if st.Phase != PhaseDiscovered || len(st.Journal) != 0 || st.Fallback.HTTP != "" || len(st.ImportedRoutes) != 0 || len(st.History) != 1 {
			t.Fatalf("%s: %+v", phase, st)
		}
	}
	running := &State{Phase: PhaseShadow, Journal: []Step{{Kind: stepGatewayPorts}}}
	running.restart()
	if running.Phase != PhaseShadow || len(running.Journal) != 1 {
		t.Fatal("an adoption in progress must keep its journal")
	}
}
