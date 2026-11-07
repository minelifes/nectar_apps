package api

import (
	"strings"
	"testing"
)

// A domain without an app: host → address:port, editing it, a redirect proxy, conflicts with app
// domains, deleting it removes the gateway route.
func TestProxyDomains(t *testing.T) {
	e, fg := newEnvGW(t)
	fg.setFeatures("web-options", "strip-prefix")
	o := e.owner()
	route := func(host, path string) map[string]any {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		for _, rt := range fg.routes {
			if rt["domain"] == host && rt["path_prefix"] == path {
				return rt
			}
		}
		return nil
	}

	for _, bad := range []map[string]any{
		{"host": "nas.example.com", "target_port": 5000},                             // no address
		{"host": "nas.example.com", "target_host": "10.0.0.5"},                       // no port
		{"host": "nas.example.com", "target_host": "10.0.0.5", "target_port": 70000}, // bad port
		{"host": "nas.example.com", "target_host": "http://x/", "target_port": 80},   // not a host
		{"host": "not a host", "target_host": "10.0.0.5", "target_port": 80},         // bad domain
		{"host": "nas.example.com", "target_host": "10.0.0.5", "target_port": 80, "kind": "app"},
	} {
		if code, _, _ := e.do(o, "POST", "/api/proxies", bad); code != 400 {
			t.Fatalf("%v: %d", bad, code)
		}
	}

	code, d, _ := e.do(o, "POST", "/api/proxies", map[string]any{"host": "NAS.example.com", "target_host": "127.0.0.1",
		"target_port": 5000, "target_https": true, "strip_prefix": false})
	if code != 201 || d["kind"] != "proxy" || d["app_id"].(float64) != 0 || d["target_host"] != "127.0.0.1" || d["status"] != "active" {
		t.Fatalf("create: %d %v", code, d)
	}
	id := itoa(d["id"].(float64))
	rt := route("nas.example.com", "/")
	if rt == nil || rt["target_ip"] != "127.0.0.1" || rt["target_port"].(float64) != 5000 || rt["target_https"] != true ||
		rt["mode"] != "proxy" || rt["acme"] != true {
		t.Fatalf("gateway route: %v", rt)
	}

	// listed under proxies, not among app domains
	if code, _, list := e.do(o, "GET", "/api/proxies", nil); code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, list)
	}
	if _, ov, _ := e.do(o, "GET", "/api/domains", nil); len(ov["domains"].([]any)) != 0 {
		t.Fatalf("proxy in the app domains overview: %v", ov["domains"])
	}

	// new target, plain HTTP
	if code, v, _ := e.do(o, "PATCH", "/api/proxies/"+id, map[string]any{"target_host": "10.25.0.12", "target_port": 8080, "target_https": false}); code != 200 ||
		v["target_host"] != "10.25.0.12" || v["target_port"].(float64) != 8080 {
		t.Fatalf("update: %d %v", code, v)
	}
	if rt := route("nas.example.com", "/"); rt["target_ip"] != "10.25.0.12" || rt["target_port"].(float64) != 8080 || rt["target_https"] == true {
		t.Fatalf("updated route: %v", rt)
	}

	// a path on the same host, prefix removed
	code, pd, _ := e.do(o, "POST", "/api/proxies", map[string]any{"host": "nas.example.com", "path_prefix": "/grafana",
		"target_host": "grafana.monitoring.svc.cluster.local", "target_port": 3000, "strip_prefix": true})
	if code != 201 {
		t.Fatalf("path proxy: %d %v", code, pd)
	}
	if rt := route("nas.example.com", "/grafana"); rt == nil || rt["web"].(map[string]any)["strip_prefix"] != true {
		t.Fatalf("path proxy route: %v", rt)
	}

	// moving it to another host replaces the route
	code, md, _ := e.do(o, "PATCH", "/api/proxies/"+itoa(pd["id"].(float64)), map[string]any{"host": "grafana.example.com", "path_prefix": "/"})
	if code != 200 || md["id"] == pd["id"] || md["target_port"].(float64) != 3000 || route("nas.example.com", "/grafana") != nil ||
		route("grafana.example.com", "/") == nil {
		t.Fatalf("move: %d %v", code, md)
	}

	// a redirect without an app
	if code, rd, _ := e.do(o, "POST", "/api/proxies", map[string]any{"host": "www.nas.example.com", "kind": "redirect",
		"redirect_to": "nas.example.com"}); code != 201 || rd["kind"] != "redirect" {
		t.Fatalf("redirect: %d %v", code, rd)
	}
	if rt := route("www.nas.example.com", "/"); rt == nil || rt["mode"] != "redirect" || rt["redirect_url"] != "https://nas.example.com" {
		t.Fatalf("redirect route: %v", rt)
	}

	// an app domain can't take a proxy's host, nor become a proxy
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, web, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web",
		"runtime": "image", "config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	appID := itoa(web["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+appID+"/domains", map[string]any{"host": "nas.example.com"}); code != 409 {
		t.Fatalf("app took a proxy's host: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/apps/"+appID+"/domains", map[string]any{"host": "x.example.com", "kind": "proxy"}); code != 400 {
		t.Fatalf("app domain of kind proxy: %d", code)
	}
	// app domain ids are not proxies
	_, ad, _ := e.do(o, "POST", "/api/apps/"+appID+"/domains", map[string]any{"host": "shop.example.com"})
	if code, _, _ := e.do(o, "DELETE", "/api/proxies/"+itoa(ad["id"].(float64)), nil); code != 404 {
		t.Fatalf("deleted an app domain through proxies: %d", code)
	}

	if code, _, _ := e.do(o, "DELETE", "/api/proxies/"+id, nil); code != 204 || route("nas.example.com", "/") != nil {
		t.Fatalf("delete: %d", code)
	}
	if _, _, list := e.do(o, "GET", "/api/audit?limit=50", nil); !strings.Contains(stringsOf(list), "proxy.create") {
		t.Fatalf("no audit entry")
	}
}
