package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/store"
)

// Right-sizing hints from the stored metrics, per app and for the whole server.
func TestSizing(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	mk := func(slug string, memMB int) string {
		_, a, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": slug, "name": slug,
			"runtime": "image", "config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80, "replicas": 2,
				"resources": map[string]any{"cpu_millicores": 500, "memory_mb": memMB}}})
		return itoa(a["id"].(float64))
	}
	big, tight, fresh := mk("big", 1024), mk("tight", 256), mk("fresh", 512)
	ctx := context.Background()
	feed := func(id string, hours int, memMB int64) {
		start := time.Now().Add(-time.Duration(hours) * time.Hour)
		for i := range hours * 4 {
			at := start.Add(time.Duration(i) * 15 * time.Minute)
			// two pods, each using memMB (the app's total is twice that)
			if err := e.api.store.AddMetricSamples(ctx, at, []store.MetricSample{{Series: "app:" + id, CPUm: 60, MemB: 2 * memMB << 20,
				Pods: 2, CPUPeak: 40, MemPeak: (memMB + 20) << 20}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	feed(big, 30, 180)
	feed(tight, 30, 240)
	feed(fresh, 5, 100)

	code, rep, _ := e.do(o, "GET", "/api/apps/"+big+"/sizing", nil)
	if code != 200 || rep["ready"] != true || rep["memory_p95_mb"].(float64) != 180 || rep["memory_peak_mb"].(float64) != 200 ||
		rep["suggest_memory_mb"].(float64) != 256 || rep["saving_memory_mb"].(float64) != 2*(1024-256) {
		t.Fatalf("big: %d %v", code, rep)
	}
	if hints := rep["hints"].([]any); len(hints) != 2 || !strings.Contains(hints[0].(map[string]any)["message"].(string), "Reserves 1.0 GB, uses 180 MB") {
		t.Fatalf("hints: %v", hints)
	}
	if _, rep, _ := e.do(o, "GET", "/api/apps/"+fresh+"/sizing", nil); rep["ready"] != false || len(rep["hints"].([]any)) != 0 {
		t.Fatalf("fresh: %v", rep)
	}

	code, srv, _ := e.do(o, "GET", "/api/server/sizing", nil)
	list := srv["apps"].([]any)
	if code != 200 || len(list) != 2 || list[0].(map[string]any)["app"] != "tight" || list[1].(map[string]any)["app"] != "big" ||
		srv["collecting"].(float64) != 1 || srv["saving_memory_mb"].(float64) != 2*(1024-256) || srv["requested_memory_mb"].(float64) != 2*(1024+256+512) {
		t.Fatalf("server: %d %v", code, srv)
	}
}
