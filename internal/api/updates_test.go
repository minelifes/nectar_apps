package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nectar_apps/internal/update"
)

func TestUpdatesAPI(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	if code, _, _ := e.do(o, "GET", "/api/updates", nil); code != 503 {
		t.Fatalf("without the manager: %d", code)
	}
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(update.Feed{Schema: 1, Components: map[string]map[string][]update.Release{
			"panel": {"stable": {{Version: "0.2.0", Image: "ghcr.io/minelifes/nectar-apps@sha256:2", Notes: "- feat: updates", Migrations: true}}}}})
	}))
	defer feed.Close()
	e.api.updates = &update.Manager{Store: e.api.store, Fetcher: &update.Fetcher{}, Namespace: "nectar-system",
		PanelVersion: "0.1.0", PanelAPI: 1}

	code, v, _ := e.do(o, "GET", "/api/updates", nil)
	if code != 200 || v["settings"].(map[string]any)["panel"].(map[string]any)["policy"] != "notify" {
		t.Fatalf("get: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "PUT", "/api/updates/settings", map[string]any{"panel": map[string]any{"policy": "sometimes"}}); code != 400 {
		t.Fatalf("bad policy: %d", code)
	}
	code, set, _ := e.do(o, "PUT", "/api/updates/settings", map[string]any{
		"panel":  map[string]any{"policy": "patch", "channel": "stable", "feed_url": feed.URL + "/updates.json"},
		"window": map[string]any{"start": "03:00", "end": "05:00", "days": []string{"sat", "sun"}}, "interval_hours": 12})
	if code != 200 || set["interval_hours"].(float64) != 12 || set["k3s"].(map[string]any)["policy"] != "off" {
		t.Fatalf("put: %d %v", code, set)
	}
	code, st, _ := e.do(o, "POST", "/api/updates/check", nil)
	pc := st["components"].(map[string]any)["panel"].(map[string]any)
	if code != 200 || len(pc["available"].([]any)) != 1 {
		t.Fatalf("check: %d %v", code, st)
	}
	// 0.1.0 → 0.2.0 is a minor release: not covered by "patch"
	if pc["auto"] != nil || pc["waiting"] == "" {
		t.Errorf("auto %v waiting %v", pc["auto"], pc["waiting"])
	}
	code, plan, _ := e.do(o, "POST", "/api/updates/plan", map[string]any{"panel": "0.2.0"})
	if code != 200 || plan["summary"] != "panel 0.1.0 → 0.2.0" || len(plan["warnings"].([]any)) != 2 || len(plan["releases"].([]any)) != 1 {
		t.Fatalf("plan: %d %v", code, plan)
	}
	if code, _, _ := e.do(o, "POST", "/api/updates/plan", map[string]any{"panel": "9.9.9"}); code != 409 {
		t.Fatalf("unknown version: %d", code)
	}
	// without Kubernetes nothing can be installed
	if code, _, _ := e.do(o, "POST", "/api/updates/apply", map[string]any{"panel": "0.2.0"}); code != 502 {
		t.Fatalf("apply without kubernetes: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/updates/rollback", nil); code != 409 {
		t.Fatalf("nothing to roll back: %d", code)
	}
	if code, _, list := e.do(o, "GET", "/api/updates/runs", nil); code != 200 || len(list) != 0 {
		t.Fatalf("runs: %d %v", code, list)
	}
	if code, _, _ := e.do(o, "POST", "/api/updates/k3s", map[string]any{"version": "1.31"}); code != 400 {
		t.Fatalf("bad k3s version: %d", code)
	}
	// a user without roles sees nothing
	e.do(o, "POST", "/api/users", map[string]string{"email": "nobody@x.io", "name": "N", "password": "nobody-pass1"})
	viewer := e.client()
	e.do(viewer, "POST", "/api/auth/login", map[string]string{"email": "nobody@x.io", "password": "nobody-pass1"})
	if code, _, _ := e.do(viewer, "GET", "/api/updates", nil); code != 403 {
		t.Fatalf("no roles get: %d", code)
	}
	if code, _, _ := e.do(viewer, "POST", "/api/updates/check", nil); code != 403 {
		t.Fatalf("viewer check: %d", code)
	}
}
