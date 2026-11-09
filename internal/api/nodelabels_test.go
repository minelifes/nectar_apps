package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nectar_apps/internal/cli"
	"nectar_apps/internal/kube"
)

// Node labels people set (system ones kept and refused), apps placed by them, a notice when no node has them.
func TestNodeLabels(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	var mu sync.Mutex
	labels := map[string]string{"kubernetes.io/hostname": "worker", "node-role.kubernetes.io/worker": "true", "old": "x"}
	var lastPatch map[string]any
	k8s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		lj, _ := json.Marshal(labels)
		switch {
		case r.URL.Path == "/api/v1/nodes":
			fmt.Fprintf(w, `{"items":[{"metadata":{"name":"worker","labels":%s},"status":{"conditions":[{"type":"Ready","status":"True"}]}}]}`, lj)
		case r.URL.Path == "/api/v1/nodes/worker" && r.Method == "PATCH":
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			lastPatch = p["metadata"].(map[string]any)["labels"].(map[string]any)
			for k, v := range lastPatch {
				if v == nil {
					delete(labels, k)
				} else {
					labels[k] = v.(string)
				}
			}
			fmt.Fprint(w, `{}`)
		case r.URL.Path == "/api/v1/nodes/worker":
			fmt.Fprintf(w, `{"metadata":{"name":"worker","labels":%s}}`, lj)
		default:
			http.NotFound(w, r)
		}
	}))
	defer k8s.Close()
	kc, _ := kube.New(k8s.URL)
	e.api.kube = kc

	for _, bad := range []map[string]any{
		{"node-role.kubernetes.io/gpu": "true"},
		{"nectar.dev/edge": "true"},
		{"bad key": "x"},
		{"disk": "not ok"},
		{"Example.com/disk": "ssd"},
	} {
		if code, _, _ := e.do(o, "PUT", "/api/server/nodes/worker/labels", map[string]any{"labels": bad}); code != 400 {
			t.Fatalf("%v: %d", bad, code)
		}
	}
	if code, _, _ := e.do(o, "PUT", "/api/server/nodes/nope/labels", map[string]any{"labels": map[string]any{}}); code != 404 {
		t.Fatalf("unknown node: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/server/nodes/worker/labels", map[string]any{"labels": map[string]any{"disk": "ssd", "example.com/zone": "a"}}); code != 200 {
		t.Fatalf("set: %d", code)
	}
	mu.Lock()
	if lastPatch["old"] != nil || labels["old"] != "" || labels["disk"] != "ssd" || labels["kubernetes.io/hostname"] != "worker" || labels["example.com/zone"] != "a" {
		t.Fatalf("labels: %v (patch %v)", labels, lastPatch)
	}
	mu.Unlock()
	if code, _, list := e.do(o, "GET", "/api/server/nodes", nil); code != 200 || list[0].(map[string]any)["labels"].(map[string]any)["disk"] != "ssd" {
		t.Fatalf("listed labels: %v", list)
	}

	// an app placed by labels
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web",
		"runtime": "image", "config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	id := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"node_selector": map[string]any{"disk": "a b"}}}); code != 400 {
		t.Fatalf("bad selector: %d", code)
	}
	code, a, h := e.doH(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"node_selector": map[string]any{"disk": "ssd"}}})
	if code != 200 || a["config"].(map[string]any)["node_selector"].(map[string]any)["disk"] != "ssd" || h.Get("X-Nectar-Notice") != "" {
		t.Fatalf("selector: %d %v %v", code, a, h)
	}
	if code, _, h := e.doH(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"node_selector": map[string]any{"gpu": "nvidia"}}}); code != 200 ||
		!strings.Contains(h.Get("X-Nectar-Notice"), "no ready node") {
		t.Fatalf("no node matches: %d %v", code, h)
	}
	if _, a, _ := e.do(o, "GET", "/api/apps/"+id, nil); len(a["config"].(map[string]any)["node_selector"].(map[string]any)) != 1 {
		t.Fatalf("labels merged instead of replaced: %v", a["config"])
	}
	// removing the label the app needs warns about the app
	e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"node_selector": map[string]any{"disk": "ssd"}}})
	if code, _, h := e.doH(o, "PUT", "/api/server/nodes/worker/labels", map[string]any{"labels": map[string]any{}}); code != 200 ||
		!strings.Contains(h.Get("X-Nectar-Notice"), "web") {
		t.Fatalf("notice: %d %v", code, h)
	}

	// nectar node list / label
	_, tok, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "admin"})
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := cli.Run(context.Background(), args, cli.Env{Stdout: &out, Stderr: &errb, Version: "test",
			Client: &cli.Client{URL: e.srv.URL, Token: tok["token"].(string)}})
		return code, out.String(), errb.String()
	}
	if code, out, errs := run("node", "label", "worker", "disk=nvme", "zone=b"); code != 0 || !strings.Contains(out, "disk=nvme,zone=b") {
		t.Fatalf("label: %d %s %s", code, out, errs)
	}
	if code, out, errs := run("node", "label", "worker", "zone-"); code != 0 || strings.Contains(out, "zone") {
		t.Fatalf("unlabel: %d %s %s", code, out, errs)
	}
	if code, out, _ := run("node", "list"); code != 0 || !strings.Contains(out, "worker") || !strings.Contains(out, "disk=nvme") ||
		strings.Contains(out, "hostname") {
		t.Fatalf("list: %s", out)
	}
	if code, _, errs := run("node", "label", "nope", "a=b"); code != 1 || !strings.Contains(errs, "no node") {
		t.Fatalf("unknown: %d %s", code, errs)
	}
}

// doH is do with the response headers.
func (e *testEnv) doH(c *http.Client, method, path string, body any) (int, map[string]any, http.Header) {
	e.t.Helper()
	var hdr http.Header
	var code int
	var obj map[string]any
	func() {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			e.t.Fatal(err)
		}
		defer resp.Body.Close()
		hdr, code = resp.Header, resp.StatusCode
		_ = json.NewDecoder(resp.Body).Decode(&obj)
	}()
	return code, obj, hdr
}
