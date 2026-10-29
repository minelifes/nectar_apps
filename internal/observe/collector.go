// Package observe collects resource usage history and reads logs and events for the panel (PLAN.md §6).
package observe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"sync"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// Kube is the part of the Kubernetes client the collector needs.
type Kube interface {
	ListNodes(ctx context.Context) ([]kube.Node, error)
	Get(ctx context.Context, path string, out any) error
}

// Collector samples node and app usage every Interval into the metric history.
type Collector struct {
	Kube     Kube
	Store    *store.Store
	Log      *slog.Logger
	Interval time.Duration // default 15 s

	mu      sync.Mutex
	prevNet map[string]netCounter // node → last network counters
}

type netCounter struct {
	at     time.Time
	rx, tx uint64
}

// Run samples until ctx ends. Old points are pruned every hour.
func (c *Collector) Run(ctx context.Context) {
	if c.Interval <= 0 {
		c.Interval = store.MetricRaw * time.Second
	}
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	lastPrune := time.Time{}
	for {
		now := time.Now()
		if err := c.Collect(ctx, now); err != nil && ctx.Err() == nil {
			c.Log.Debug("metrics: collect", "err", err)
		}
		if now.Sub(lastPrune) > time.Hour {
			if err := c.Store.PruneMetrics(ctx, now); err != nil {
				c.Log.Warn("metrics: prune", "err", err)
			}
			lastPrune = now
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Collect takes one sample of every node and every app.
func (c *Collector) Collect(ctx context.Context, now time.Time) error {
	var samples []store.MetricSample

	nodes, err := c.Kube.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("nodes: %w", err)
	}
	for _, n := range nodes {
		if n.Usage == nil {
			continue // no metrics-server
		}
		m := store.MetricSample{Series: "node:" + n.Name, CPUm: n.Usage.CPUMillicores, MemB: n.Usage.MemoryBytes}
		c.nodeStats(ctx, n.Name, now, &m)
		samples = append(samples, m)
	}

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	apps, err := c.appUsage(ctx, names)
	if err != nil {
		c.Log.Debug("metrics: pods", "err", err)
	}
	samples = append(samples, apps...)
	return c.Store.AddMetricSamples(ctx, now, samples)
}

// nodeStats adds disk and network numbers from the kubelet summary API (best effort).
func (c *Collector) nodeStats(ctx context.Context, node string, now time.Time, m *store.MetricSample) {
	var sum struct {
		Node struct {
			Fs struct {
				CapacityBytes *uint64 `json:"capacityBytes"`
				UsedBytes     *uint64 `json:"usedBytes"`
			} `json:"fs"`
			Network struct {
				RxBytes *uint64 `json:"rxBytes"`
				TxBytes *uint64 `json:"txBytes"`
			} `json:"network"`
		} `json:"node"`
	}
	if err := c.Kube.Get(ctx, "/api/v1/nodes/"+url.PathEscape(node)+"/proxy/stats/summary", &sum); err != nil {
		return
	}
	if fs := sum.Node.Fs; fs.CapacityBytes != nil && fs.UsedBytes != nil {
		m.DiskTotal, m.DiskUsed = clampInt(*fs.CapacityBytes), clampInt(*fs.UsedBytes)
	}
	nw := sum.Node.Network
	if nw.RxBytes == nil || nw.TxBytes == nil {
		return
	}
	cur := netCounter{at: now, rx: *nw.RxBytes, tx: *nw.TxBytes}
	c.mu.Lock()
	if c.prevNet == nil {
		c.prevNet = map[string]netCounter{}
	}
	prev, ok := c.prevNet[node]
	c.prevNet[node] = cur
	c.mu.Unlock()
	dt := cur.at.Sub(prev.at).Seconds()
	if !ok || dt <= 0 || cur.rx < prev.rx || cur.tx < prev.tx { // first sample or counter reset
		return
	}
	m.RxBps = int64(math.Round(float64(cur.rx-prev.rx) / dt))
	m.TxBps = int64(math.Round(float64(cur.tx-prev.tx) / dt))
}

// appUsage sums the pod metrics of every app (pods labelled nectar.dev/app in a project namespace).
// Without metrics-server it falls back to the kubelet summaries of the given nodes.
func (c *Collector) appUsage(ctx context.Context, nodes []string) ([]store.MetricSample, error) {
	ids, err := c.appIndex(ctx)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	var pm struct {
		Items []struct {
			Metadata struct {
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			} `json:"metadata"`
			Containers []struct {
				Usage map[string]string `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	sums := map[int64]*store.MetricSample{}
	add := func(ns, slug string, cpuM, memB int64) {
		id, ok := ids[ns+"/"+slug]
		if !ok {
			return
		}
		s := sums[id]
		if s == nil {
			s = &store.MetricSample{Series: "app:" + strconv.FormatInt(id, 10)}
			sums[id] = s
		}
		s.CPUm += cpuM
		s.MemB += memB
	}
	err = c.Kube.Get(ctx, "/apis/metrics.k8s.io/v1beta1/pods?labelSelector="+url.QueryEscape("nectar.dev/app"), &pm)
	var se *kube.StatusError
	switch {
	case err == nil:
		for _, it := range pm.Items {
			var cpuM, memB int64
			for _, ct := range it.Containers {
				if q, err := kube.ParseQuantity(ct.Usage["cpu"]); err == nil {
					cpuM += int64(math.Round(q * 1000))
				}
				if q, err := kube.ParseQuantity(ct.Usage["memory"]); err == nil {
					memB += int64(q)
				}
			}
			add(it.Metadata.Namespace, it.Metadata.Labels["nectar.dev/app"], cpuM, memB)
		}
	case errors.As(err, &se): // no metrics-server
		if err := c.summaryUsage(ctx, nodes, add); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	// apps without running pods get an explicit zero, so charts show the gap as 0 instead of nothing
	out := make([]store.MetricSample, 0, len(ids))
	for _, id := range ids {
		if s := sums[id]; s != nil {
			out = append(out, *s)
		} else {
			out = append(out, store.MetricSample{Series: "app:" + strconv.FormatInt(id, 10)})
		}
	}
	return out, nil
}

// summaryUsage reads pod usage from each node's kubelet summary and maps pods to apps by their label.
func (c *Collector) summaryUsage(ctx context.Context, nodes []string, add func(ns, slug string, cpuM, memB int64)) error {
	var pods struct {
		Items []struct {
			Metadata struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := c.Kube.Get(ctx, "/api/v1/pods?labelSelector="+url.QueryEscape("nectar.dev/app"), &pods); err != nil {
		return err
	}
	slug := map[string]string{}
	for _, p := range pods.Items {
		slug[p.Metadata.Namespace+"/"+p.Metadata.Name] = p.Metadata.Labels["nectar.dev/app"]
	}
	for _, n := range nodes {
		var sum kube.Summary
		if err := c.Kube.Get(ctx, "/api/v1/nodes/"+url.PathEscape(n)+"/proxy/stats/summary", &sum); err != nil {
			continue // node down: its apps read 0 like pods without metrics
		}
		for _, u := range sum.PodUsage() {
			if s, ok := slug[u.Namespace+"/"+u.Name]; ok {
				add(u.Namespace, s, u.CPUMillicores, u.MemoryBytes)
			}
		}
	}
	return nil
}

// appIndex maps "namespace/slug" to app id.
func (c *Collector) appIndex(ctx context.Context) (map[string]int64, error) {
	projects, err := c.Store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, p := range projects {
		apps, err := c.Store.ListApps(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range apps {
			out[p.Namespace+"/"+a.Slug] = a.ID
		}
	}
	return out, nil
}

func clampInt(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
