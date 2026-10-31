package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/kube"
)

func TestEdgeReplicas(t *testing.T) {
	e, fg := newEnvGW(t)
	o := e.owner()
	var mu sync.Mutex
	replicaLabel := map[string]bool{}
	var daemonSet map[string]any
	k8s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		node := func(name, labels, ip string) string {
			if replicaLabel[name] {
				labels = strings.TrimSuffix(labels, "}")
				if labels != "{" {
					labels += ","
				}
				labels += `"nectar.dev/edge-replica":"true"}`
			}
			return fmt.Sprintf(`{"metadata":{"name":%q,"labels":%s},"status":{"conditions":[{"type":"Ready","status":"True"}],"addresses":[{"type":"ExternalIP","address":%q}]}}`, name, labels, ip)
		}
		switch {
		case r.URL.Path == "/api/v1/nodes":
			fmt.Fprintf(w, `{"items":[%s,%s,%s]}`, node("edge", `{"nectar.dev/edge":"true"}`, "203.0.113.1"),
				node("worker", `{}`, "203.0.113.2"), node("eu", `{}`, "203.0.113.3"))
		case strings.HasPrefix(r.URL.Path, "/api/v1/nodes/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
			if r.Method == "PATCH" {
				b, _ := io.ReadAll(r.Body)
				replicaLabel[name] = strings.Contains(string(b), `"nectar.dev/edge-replica":"true"`)
				fmt.Fprint(w, `{}`)
				return
			}
			labels := `{}`
			if name == "edge" {
				labels = `{"nectar.dev/edge":"true"}`
			}
			fmt.Fprintf(w, `{"metadata":{"name":%q,"labels":%s}}`, name, labels)
		case r.URL.Path == "/apis/apps/v1/namespaces/nectar-system/deployments/nectar-gateway":
			fmt.Fprint(w, `{"spec":{"template":{"spec":{"containers":[{"name":"gateway","image":"ghcr.io/minelifes/nectar-gateway:1.4.0",
				"env":[{"name":"PROXY_ADDR","value":":80"},{"name":"ADMIN_ADDR","value":":9081"},{"name":"ACME_EMAIL","value":"a@b.c"},{"name":"DB_PATH","value":"/data/nectar.db"},
				{"name":"GATEWAY_API_TOKEN","valueFrom":{"secretKeyRef":{"name":"nectar-secrets","key":"gateway-api-token"}}}]}]}}}}`)
		case r.URL.Path == "/apis/apps/v1/namespaces/nectar-system/daemonsets/nectar-gateway-edge":
			switch r.Method {
			case "PATCH":
				json.NewDecoder(r.Body).Decode(&daemonSet)
				fmt.Fprint(w, `{}`)
			case "DELETE":
				if daemonSet == nil {
					w.WriteHeader(404)
					fmt.Fprint(w, `{"kind":"Status","code":404}`)
					return
				}
				daemonSet = nil
				fmt.Fprint(w, `{}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer k8s.Close()
	kc, _ := kube.New(k8s.URL)
	e.api.kube = kc
	e.api.cfg.SystemNamespace = "nectar-system"
	e.api.cfg.GatewayURL = "http://nectar-gateway-admin.nectar-system.svc:8081"

	// DNS points to the edge node only (not every node)
	if ips := e.api.serverIPs(t.Context(), Settings{}); strings.Join(ips, ",") != "203.0.113.1" {
		t.Fatalf("server IPs: %v", ips)
	}
	// an old gateway cannot run replicas
	if code, v, _ := e.do(o, "POST", "/api/server/nodes/eu/edge-on", nil); code != 409 || !strings.Contains(v["error"].(string), "update nectar-gateway") {
		t.Fatalf("edge-on with an old gateway: %d %v", code, v)
	}
	fg.mu.Lock()
	fg.features = append(fg.features, "replicas")
	fg.replicas = []map[string]any{{"node": "eu", "address": "10.0.0.3", "version": "1.4.0", "last_seen": time.Now().UTC()}}
	fg.mu.Unlock()
	if code, _, _ := e.do(o, "POST", "/api/server/nodes/edge/edge-on", nil); code != 409 {
		t.Fatalf("the primary as a replica: %d", code)
	}
	if code, v, _ := e.do(o, "POST", "/api/server/nodes/eu/edge-on", nil); code != 200 {
		t.Fatalf("edge-on: %d %v", code, v)
	}
	mu.Lock()
	ds, _ := json.Marshal(daemonSet)
	mu.Unlock()
	for _, want := range []string{`"image":"ghcr.io/minelifes/nectar-gateway:1.4.0"`, `"PRIMARY_URL","value":"http://nectar-gateway-admin.nectar-system.svc:8081"`,
		`"nectar.dev/edge-replica":"true"`, `"hostNetwork":true`, `"port":9081`, `"gateway-api-token"`} {
		if !strings.Contains(string(ds), want) {
			t.Fatalf("daemon set lacks %s: %s", want, ds)
		}
	}
	if strings.Contains(string(ds), "ACME_EMAIL") || strings.Contains(string(ds), "nectar.db") {
		t.Fatalf("replica would issue certificates or share the database: %s", ds)
	}
	if ips := e.api.serverIPs(t.Context(), Settings{}); strings.Join(ips, ",") != "203.0.113.1,203.0.113.3" {
		t.Fatalf("server IPs with a replica: %v", ips)
	}
	code, v, _ := e.do(o, "GET", "/api/server/edge", nil)
	reps, _ := v["replicas"].([]any)
	if code != 200 || v["primary"] != "edge" || len(reps) != 1 || reps[0].(map[string]any)["synced"] != true {
		t.Fatalf("edge status: %d %v", code, v)
	}
	// the last replica removed → the DaemonSet goes
	if code, _, _ := e.do(o, "POST", "/api/server/nodes/eu/edge-off", nil); code != 200 {
		t.Fatalf("edge-off: %d", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if daemonSet != nil {
		t.Fatal("DaemonSet kept without replicas")
	}
}
