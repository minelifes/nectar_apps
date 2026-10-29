package observe

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// fakeCluster answers the few Kubernetes paths the package uses.
func fakeCluster(t *testing.T, rx *uint64) *kube.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/nodes":
			io.WriteString(w, `{"items":[{"metadata":{"name":"n1"},"status":{"allocatable":{"cpu":"2","memory":"4Gi"},"capacity":{"cpu":"2","memory":"4Gi"}}}]}`)
		case r.URL.Path == "/apis/metrics.k8s.io/v1beta1/nodes":
			io.WriteString(w, `{"items":[{"metadata":{"name":"n1"},"usage":{"cpu":"500m","memory":"1Gi"}}]}`)
		case r.URL.Path == "/api/v1/nodes/n1/proxy/stats/summary":
			fmt.Fprintf(w, `{"node":{"fs":{"capacityBytes":100000,"usedBytes":25000},"network":{"rxBytes":%d,"txBytes":%d}}}`, *rx, *rx/2)
		case r.URL.Path == "/apis/metrics.k8s.io/v1beta1/pods":
			io.WriteString(w, `{"items":[
				{"metadata":{"namespace":"prj-shop","labels":{"nectar.dev/app":"api"}},"containers":[{"usage":{"cpu":"100m","memory":"128Mi"}}]},
				{"metadata":{"namespace":"prj-shop","labels":{"nectar.dev/app":"api"}},"containers":[{"usage":{"cpu":"50m","memory":"64Mi"}}]},
				{"metadata":{"namespace":"other","labels":{"nectar.dev/app":"api"}},"containers":[{"usage":{"cpu":"9","memory":"9Gi"}}]}]}`)
		case r.URL.Path == "/api/v1/namespaces/prj-shop/pods":
			io.WriteString(w, `{"items":[
				{"metadata":{"name":"api-b","creationTimestamp":"2026-10-28T10:00:00Z","labels":{"nectar.dev/role":"workload"}},"status":{"phase":"Running","containerStatuses":[{"restartCount":2}]}},
				{"metadata":{"name":"api-a","creationTimestamp":"2026-10-28T09:00:00Z"},"status":{"phase":"Running"}}]}`)
		case strings.HasSuffix(r.URL.Path, "/pods/api-a/log"):
			io.WriteString(w, "2026-10-28T10:00:01.000000000Z started a\n2026-10-28T10:00:03.000000000Z third\n")
		case strings.HasSuffix(r.URL.Path, "/pods/api-b/log"):
			if r.URL.Query().Get("previous") == "true" {
				io.WriteString(w, "2026-10-28T09:59:59Z panic: boom\n")
				return
			}
			io.WriteString(w, "2026-10-28T10:00:02.000000000Z second\nno timestamp\n")
		case r.URL.Path == "/api/v1/namespaces/prj-shop/events":
			io.WriteString(w, `{"items":[
				{"type":"Warning","reason":"BackOff","message":"Back-off restarting","involvedObject":{"kind":"Pod","name":"api-b"},"count":3,"lastTimestamp":"2026-10-28T10:05:00Z"},
				{"type":"Normal","reason":"ScalingReplicaSet","message":"Scaled up","involvedObject":{"kind":"Deployment","name":"api"},"firstTimestamp":"2026-10-28T09:00:00Z","lastTimestamp":"2026-10-28T09:00:00Z"},
				{"type":"Normal","reason":"Pulled","message":"x","involvedObject":{"kind":"Pod","name":"apiary-1"},"lastTimestamp":"2026-10-28T11:00:00Z"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	k, err := kube.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCollectorAndHistory(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "m.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image"})
	idle, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "idle", Name: "Idle", Kind: "worker", RuntimeID: "image"})

	rx := uint64(1000)
	c := &Collector{Kube: fakeCluster(t, &rx), Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	t0 := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	if err := c.Collect(ctx, t0); err != nil {
		t.Fatal(err)
	}
	rx = 1000 + 15*2000 // 2000 B/s over 15 s
	if err := c.Collect(ctx, t0.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}

	node, err := st.MetricHistory(ctx, "node:n1", t0.Add(-time.Minute), t0.Add(time.Minute), 100)
	if err != nil || len(node) != 2 {
		t.Fatalf("node points: %v %v", node, err)
	}
	if node[1].CPUm != 500 || node[1].MemB != 1<<30 || node[1].DiskUsed != 25000 || node[1].DiskTotal != 100000 || node[1].RxBps != 2000 || node[1].TxBps != 1000 {
		t.Fatalf("node sample: %+v", node[1])
	}
	if node[0].RxBps != 0 {
		t.Fatalf("first sample has no rate: %+v", node[0])
	}
	pts, _ := st.MetricHistory(ctx, fmt.Sprintf("app:%d", app.ID), t0.Add(-time.Minute), t0.Add(time.Minute), 100)
	if len(pts) != 2 || pts[0].CPUm != 150 || pts[0].MemB != 192<<20 {
		t.Fatalf("app usage (two pods, other namespace ignored): %+v", pts)
	}
	if z, _ := st.MetricHistory(ctx, fmt.Sprintf("app:%d", idle.ID), t0.Add(-time.Minute), t0.Add(time.Minute), 100); len(z) != 2 || z[0].CPUm != 0 {
		t.Fatalf("idle app gets zeros: %+v", z)
	}
	// 7 days → rollups: both samples are in one 15-minute bucket, averaged
	week, _ := st.MetricHistory(ctx, "node:n1", t0.Add(-7*24*time.Hour), t0.Add(time.Hour), 100)
	if len(week) != 1 || week[0].RxBps != 1000 {
		t.Fatalf("rollup: %+v", week)
	}
	// pruning: raw points older than 24 h go away, rollups stay
	if err := st.PruneMetrics(ctx, t0.Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if raw, _ := st.MetricHistory(ctx, "node:n1", t0.Add(-time.Minute), t0.Add(time.Minute), 100); len(raw) != 0 {
		t.Fatalf("raw not pruned: %+v", raw)
	}
	if week, _ := st.MetricHistory(ctx, "node:n1", t0.Add(-7*24*time.Hour), t0.Add(time.Hour), 100); len(week) != 1 {
		t.Fatal("rollup pruned too early")
	}
}

// Without metrics-server (404 on metrics.k8s.io) node and app usage come from the kubelet summary.
func TestCollectorWithoutMetricsServer(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "m.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes":
			io.WriteString(w, `{"items":[{"metadata":{"name":"n1"},"status":{"allocatable":{"cpu":"2","memory":"4Gi"}}}]}`)
		case "/api/v1/nodes/n1/proxy/stats/summary":
			io.WriteString(w, `{"node":{"cpu":{"usageNanoCores":250000000},"memory":{"workingSetBytes":1073741824},"fs":{"capacityBytes":100,"usedBytes":50}},
				"pods":[{"podRef":{"name":"api-1","namespace":"prj-shop"},"cpu":{"usageNanoCores":40000000},"memory":{"workingSetBytes":1048576}},
				{"podRef":{"name":"api-2","namespace":"prj-shop"},"cpu":{"usageNanoCores":10400000},"memory":{"workingSetBytes":1048576}},
				{"podRef":{"name":"stranger","namespace":"prj-shop"},"cpu":{"usageNanoCores":900000000},"memory":{"workingSetBytes":1}}]}`)
		case "/api/v1/pods":
			io.WriteString(w, `{"items":[{"metadata":{"name":"api-1","namespace":"prj-shop","labels":{"nectar.dev/app":"api"}}},
				{"metadata":{"name":"api-2","namespace":"prj-shop","labels":{"nectar.dev/app":"api"}}}]}`)
		default:
			http.NotFound(w, r) // metrics.k8s.io is not served
		}
	}))
	defer srv.Close()
	k, err := kube.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := k.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].Usage == nil || nodes[0].Usage.CPUMillicores != 250 || nodes[0].Usage.MemoryBytes != 1<<30 {
		t.Fatalf("node usage from the summary: %+v %v", nodes, err)
	}
	c := &Collector{Kube: k, Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	t0 := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := c.Collect(ctx, t0); err != nil {
		t.Fatal(err)
	}
	node, _ := st.MetricHistory(ctx, "node:n1", t0.Add(-time.Minute), t0.Add(time.Minute), 10)
	if len(node) != 1 || node[0].CPUm != 250 || node[0].DiskUsed != 50 {
		t.Fatalf("node sample: %+v", node)
	}
	pts, _ := st.MetricHistory(ctx, fmt.Sprintf("app:%d", app.ID), t0.Add(-time.Minute), t0.Add(time.Minute), 10)
	if len(pts) != 1 || pts[0].CPUm != 50 || pts[0].MemB != 2<<20 {
		t.Fatalf("app usage from the summary (unlabelled pod ignored): %+v", pts)
	}
}

func TestLogsAndEvents(t *testing.T) {
	ctx := context.Background()
	rx := uint64(0)
	k := fakeCluster(t, &rx)

	pods, err := AppPods(ctx, k, "prj-shop", "api")
	if err != nil || len(pods) != 2 || pods[0].Name != "api-a" || pods[1].Restarts != 2 || pods[1].Role != "workload" {
		t.Fatalf("pods: %+v %v", pods, err)
	}
	lines, err := TailLogs(ctx, k, "prj-shop", []string{"api-a", "api-b"}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range lines {
		got = append(got, l.Pod+":"+l.Line)
	}
	// the untimestamped line sorts first (zero time); the rest is merged by time
	if strings.Join(got, "|") != "api-b:no timestamp|api-a:started a|api-b:second|api-a:third" {
		t.Fatalf("merged: %v", got)
	}
	if last, _ := TailLogs(ctx, k, "prj-shop", []string{"api-a", "api-b"}, 2, false); len(last) != 2 || last[1].Line != "third" {
		t.Fatalf("tail cut: %+v", last)
	}
	prev, _ := TailLogs(ctx, k, "prj-shop", []string{"api-b"}, 10, true)
	if len(prev) != 1 || prev[0].Line != "panic: boom" {
		t.Fatalf("previous: %+v", prev)
	}

	evs, err := AppEvents(ctx, k, "prj-shop", "api", 10)
	if err != nil || len(evs) != 2 || evs[0].Reason != "BackOff" || evs[0].Count != 3 || evs[1].Object != "Deployment/api" {
		t.Fatalf("events (apiary-1 is another app): %+v %v", evs, err)
	}
	if p := LogPath("ns", "p", true, false, 50, time.Time{}); !strings.Contains(p, "follow=true") || !strings.Contains(p, "tailLines=50") || !strings.Contains(p, "timestamps=true") {
		t.Fatalf("log path: %s", p)
	}
}
