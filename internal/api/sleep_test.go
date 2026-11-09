package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
)

// An idle web app sleeps (0 replicas), its route carries the gateway's wake call, the signed wake URL
// brings it back, "Wake now" and turning the setting off wake it too.
func TestSleepWhenIdle(t *testing.T) {
	e, fg := newEnvGW(t)
	fg.setFeatures("web-options")
	e.api.cfg.InternalURL = "http://panel.svc"
	e.api.syncer.WakeURL = e.api.wakeURL
	o := e.owner()
	ctx := context.Background()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, web, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80, "health": map[string]any{"type": "http", "path": "/"}}})
	_, wk, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "wk", "name": "Worker", "runtime": "image", "kind": "worker",
		"config": map[string]any{"image": "busybox:1"}})
	id := itoa(web["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d", code)
	}
	e.dep.Wait()
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/domains", map[string]any{"host": "shop.example.com"}); code != 201 {
		t.Fatalf("domain: %d", code)
	}
	route := func() map[string]any {
		_ = e.api.syncer.Sync(ctx)
		fg.mu.Lock()
		defer fg.mu.Unlock()
		for _, rt := range fg.routes {
			if rt["domain"] == "shop.example.com" {
				return rt
			}
		}
		return nil
	}
	replicas := func() int { return int(kube.Int(e.kube.Get("Deployment", "prj-shop", "web"), "spec", "replicas")) }
	status := func() string {
		_, a, _ := e.do(o, "GET", "/api/apps/"+id, nil)
		return a["status"].(string)
	}
	setSleep := func(appID string, min int) int {
		code, _, _ := e.do(o, "PATCH", "/api/apps/"+appID, map[string]any{"config": map[string]any{"sleep_after_minutes": min}})
		return code
	}

	// a gateway without wake support can't wake it
	if code := setSleep(id, 10); code != 409 {
		t.Fatalf("without wake: %d", code)
	}
	fg.setFeatures("web-options", "wake")
	for _, bad := range []int{3, 2000, -1} {
		if code := setSleep(id, bad); code != 400 {
			t.Fatalf("%d minutes: %d", bad, code)
		}
	}
	if code := setSleep(itoa(wk["id"].(float64)), 10); code != 400 {
		t.Fatalf("a worker can sleep: %d", code)
	}
	if code := setSleep(id, 10); code != 200 {
		t.Fatalf("sleep after 10: %d", code)
	}
	rt := route()
	wake, _ := rt["web"].(map[string]any)["wake"].(map[string]any)
	if wake == nil || !strings.HasPrefix(wake["url"].(string), "http://panel.svc/internal/wake/"+id+"?sig=") || wake["timeout"].(float64) != 90 ||
		rt["hc_enabled"] == true {
		t.Fatalf("route: %v", rt)
	}

	// idle: first seen running now, nothing in the gateway's statistics
	awake := map[int64]time.Time{}
	now := time.Now()
	if slept := e.api.sleepIdle(ctx, now, awake); len(slept) != 0 {
		t.Fatalf("slept at once: %v", slept)
	}
	fg.mu.Lock()
	fg.lastSeen = map[string]time.Time{"shop.example.com": now.Add(5 * time.Minute)}
	fg.mu.Unlock()
	if slept := e.api.sleepIdle(ctx, now.Add(11*time.Minute), awake); len(slept) != 0 {
		t.Fatalf("slept with a request 6 minutes ago: %v", slept)
	}
	if slept := e.api.sleepIdle(ctx, now.Add(16*time.Minute), awake); len(slept) != 1 || slept[0] != "shop/web" {
		t.Fatalf("not asleep: %v", slept)
	}
	if replicas() != 0 || status() != "sleeping" {
		t.Fatalf("asleep: %d %s", replicas(), status())
	}
	if rt := route(); rt["maintenance"] == true {
		t.Fatalf("a sleeping app shows maintenance: %v", rt)
	}

	// the gateway's wake call: signed
	post := func(url string) int {
		res, err := http.Post(e.srv.URL+url, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("/internal/wake/" + id + "?sig=00"); code != 403 {
		t.Fatalf("unsigned wake: %d", code)
	}
	wakePath := strings.TrimPrefix(wake["url"].(string), "http://panel.svc")
	if code := post(wakePath); code != 204 || replicas() != 1 || status() != "running" {
		t.Fatalf("wake: %d %d %s", code, replicas(), status())
	}
	if code := post(wakePath); code != 204 { // already awake
		t.Fatalf("wake again: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/wake", nil); code != 409 {
		t.Fatalf("wake now while awake: %d", code)
	}

	// woken: the full time again before sleeping
	if slept := e.api.sleepIdle(ctx, now.Add(20*time.Minute), awake); len(slept) != 0 {
		t.Fatalf("slept right after waking: %v", slept)
	}
	if slept := e.api.sleepIdle(ctx, now.Add(31*time.Minute), awake); len(slept) != 1 {
		t.Fatalf("not asleep again: %v", slept)
	}
	if code, a, _ := e.do(o, "POST", "/api/apps/"+id+"/wake", nil); code != 200 || a["status"] != "running" || replicas() != 1 {
		t.Fatalf("wake now: %d %v", code, a)
	}

	// turning it off wakes it and removes the wake call
	e.api.sleepIdle(ctx, now.Add(40*time.Minute), awake)
	e.api.sleepIdle(ctx, now.Add(51*time.Minute), awake)
	if status() != "sleeping" {
		t.Fatalf("not asleep a third time: %s", status())
	}
	if code := setSleep(id, 0); code != 200 || status() != "running" || replicas() != 1 {
		t.Fatalf("off: %d %s", code, status())
	}
	if rt := route(); rt["web"] != nil && rt["web"].(map[string]any)["wake"] != nil {
		t.Fatalf("wake call kept: %v", rt)
	}
	if rt := route(); rt["hc_enabled"] != true {
		t.Fatalf("health checks not back: %v", rt)
	}
	if _, _, list := e.do(o, "GET", "/api/audit?limit=50", nil); !strings.Contains(stringsOf(list), "app.sleep") || !strings.Contains(stringsOf(list), "app.wake") {
		t.Fatalf("no audit entries")
	}
}
