package api

import (
	"strings"
	"testing"
)

// app1.com/myCustomRoute → a worker with a port, with and without the path prefix; editing a domain's
// host or path; the generated default domain keeps its name.
func TestPathRouteToAnotherApp(t *testing.T) {
	e, fg := newEnvGW(t)
	fg.setFeatures("web-options", "strip-prefix")
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, web, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "app1", "name": "App1", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	_, wk, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "app2", "name": "App2", "runtime": "image", "kind": "worker",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 8080}})
	_, cron, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "job", "name": "Job", "runtime": "image", "kind": "worker",
		"config": map[string]any{"image": "busybox:1"}})
	webID, wkID, cronID := itoa(web["id"].(float64)), itoa(wk["id"].(float64)), itoa(cron["id"].(float64))
	for _, id := range []string{webID, wkID} {
		if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/deploy", nil); code != 202 {
			t.Fatalf("deploy %s: %d", id, code)
		}
	}
	e.dep.Wait()
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

	if code, _, _ := e.do(o, "POST", "/api/apps/"+cronID+"/domains", map[string]any{"host": "x.example.com"}); code != 400 {
		t.Fatalf("worker without a port got a domain: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/apps/"+webID+"/domains", map[string]any{"host": "app1.com"}); code != 201 {
		t.Fatalf("app1.com: %d", code)
	}
	code, d, _ := e.do(o, "POST", "/api/apps/"+wkID+"/domains", map[string]any{"host": "app1.com", "path_prefix": "/myCustomRoute", "strip_prefix": true})
	if code != 201 || d["web"].(map[string]any)["strip_prefix"] != true {
		t.Fatalf("path route: %d %v", code, d)
	}
	did := itoa(d["id"].(float64))
	rt := route("app1.com", "/myCustomRoute")
	if rt == nil || !strings.Contains(rt["target_ip"].(string), "app2.") || rt["target_port"].(float64) != 8080 ||
		rt["web"].(map[string]any)["strip_prefix"] != true {
		t.Fatalf("gateway route: %v", rt)
	}
	if rt := route("app1.com", "/"); rt == nil || !strings.Contains(rt["target_ip"].(string), "app1.") {
		t.Fatalf("app1.com/ route: %v", rt)
	}
	// web options from the UI do not carry strip_prefix: it is kept; strip_prefix=false removes it
	if code, v, _ := e.do(o, "PATCH", "/api/apps/"+wkID+"/domains/"+did, map[string]any{"web": map[string]any{"compress": true}}); code != 200 ||
		v["web"].(map[string]any)["strip_prefix"] != true || v["web"].(map[string]any)["compress"] != true {
		t.Fatalf("web update lost strip_prefix: %d %v", code, v)
	}
	if code, v, _ := e.do(o, "PATCH", "/api/apps/"+wkID+"/domains/"+did, map[string]any{"strip_prefix": false}); code != 200 || v["web"].(map[string]any)["strip_prefix"] != nil {
		t.Fatalf("strip off: %d %v", code, v)
	}

	// an older gateway: a stripping route is not created at all (the app would get unexpected paths)
	fg.setFeatures("web-options")
	e.do(o, "PATCH", "/api/apps/"+wkID+"/domains/"+did, map[string]any{"strip_prefix": true})
	_, _, list := e.do(o, "GET", "/api/apps/"+wkID+"/domains", nil)
	if st := list[0].(map[string]any); st["status"] != "error" || !strings.Contains(st["status_message"].(string), "nectar-gateway") || route("app1.com", "/myCustomRoute") != nil {
		t.Fatalf("old gateway: %v", st)
	}
	fg.setFeatures("web-options", "strip-prefix")

	// a new host or path replaces the domain (fresh route)
	code, nd, _ := e.do(o, "PATCH", "/api/apps/"+wkID+"/domains/"+did, map[string]any{"host": "App1.com", "path_prefix": "/api"})
	if code != 200 || nd["path_prefix"] != "/api" || nd["id"] == d["id"] || nd["web"].(map[string]any)["strip_prefix"] != true {
		t.Fatalf("move: %d %v", code, nd)
	}
	if route("app1.com", "/myCustomRoute") != nil || route("app1.com", "/api") == nil {
		t.Fatal("the gateway routes did not follow the new path")
	}
	// another app's host+path is taken
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+itoa(nd["id"].(float64))+"/domains/0", nil); code != 404 && code != 400 {
		t.Fatalf("bad id: %d", code)
	}
	_, _, dl := e.do(o, "GET", "/api/apps/"+webID+"/domains", nil)
	for _, x := range dl {
		dm := x.(map[string]any)
		if dm["is_default"] == true {
			if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID+"/domains/"+itoa(dm["id"].(float64)), map[string]any{"host": "mine.example.com"}); code != 400 {
				t.Fatalf("default domain renamed: %d", code)
			}
		}
	}
}
