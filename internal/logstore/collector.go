package logstore

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/observe"
)

// Kube is the part of the Kubernetes client the collector needs.
type Kube interface {
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	Stream(ctx context.Context, path string) (io.ReadCloser, error)
}

// labelAppID marks app pods (deploy.LabelAppID).
const labelAppID = "nectar.dev/app-id"

// Collector follows the logs of every app pod and appends them to the Store.
type Collector struct {
	Store *Store
	Kube  Kube
	Log   *slog.Logger
	// Retention returns the settings: off, days to keep, bytes per app.
	Retention func(ctx context.Context) (off bool, days int, maxBytes int64)
	Interval  time.Duration // pod discovery, default 15 s

	mu       sync.Mutex
	follow   map[string]context.CancelFunc // ns/pod → follower
	last     map[string]time.Time          // ns/pod → newest stored line (dedupe after reconnects)
	lastSave time.Time
}

func (c *Collector) statePath() string { return filepath.Join(c.Store.Dir, "state.json") }

func (c *Collector) loadState() {
	c.last = map[string]time.Time{}
	if b, err := os.ReadFile(c.statePath()); err == nil {
		_ = json.Unmarshal(b, &c.last)
	}
}

func (c *Collector) saveState() {
	c.mu.Lock()
	b, _ := json.Marshal(c.last)
	c.mu.Unlock()
	tmp := c.statePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, c.statePath())
	}
}

// Run collects until ctx ends.
func (c *Collector) Run(ctx context.Context) {
	c.follow = map[string]context.CancelFunc{}
	c.loadState()
	every := c.Interval
	if every <= 0 {
		every = 15 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	maint := time.NewTicker(10 * time.Minute)
	defer maint.Stop()
	c.maintain(ctx)
	for {
		c.reconcile(ctx)
		c.Store.Flush()
		if time.Since(c.lastSave) > 30*time.Second {
			c.saveState()
			c.lastSave = time.Now()
		}
		select {
		case <-ctx.Done():
			c.mu.Lock()
			for _, stop := range c.follow {
				stop()
			}
			c.mu.Unlock()
			c.saveState()
			c.Store.Close()
			return
		case <-maint.C:
			c.maintain(ctx)
		case <-t.C:
		}
	}
}

func (c *Collector) maintain(ctx context.Context) {
	off, days, maxBytes := c.retention(ctx)
	if off {
		days = 0
	}
	if err := c.Store.Maintain(max(days, 0), maxBytes); err != nil {
		c.Log.Debug("log history maintenance", "err", err)
	}
	// forget pods that are long gone
	c.mu.Lock()
	for k, t := range c.last {
		if _, live := c.follow[k]; !live && time.Since(t) > 48*time.Hour {
			delete(c.last, k)
		}
	}
	c.mu.Unlock()
}

func (c *Collector) retention(ctx context.Context) (bool, int, int64) {
	if c.Retention == nil {
		return false, 7, 512 << 20
	}
	return c.Retention(ctx)
}

// reconcile starts followers for running app pods and stops the others.
func (c *Collector) reconcile(ctx context.Context) {
	if off, _, _ := c.retention(ctx); off {
		c.mu.Lock()
		for k, stop := range c.follow {
			stop()
			delete(c.follow, k)
		}
		c.mu.Unlock()
		return
	}
	pods, err := c.Kube.List(ctx, "Pod", "", labelAppID)
	if err != nil {
		c.Log.Debug("log history: list pods", "err", err)
		return
	}
	seen := map[string]bool{}
	for _, p := range pods {
		ns, name := kube.Str(p, "metadata", "namespace"), kube.Str(p, "metadata", "name")
		app, err := strconv.ParseInt(kube.Str(p, "metadata", "labels", labelAppID), 10, 64)
		if err != nil || !running(p) {
			continue
		}
		key := ns + "/" + name
		seen[key] = true
		c.mu.Lock()
		_, ok := c.follow[key]
		if !ok {
			fctx, cancel := context.WithCancel(ctx)
			c.follow[key] = cancel
			go c.followPod(fctx, key, ns, name, app)
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	for k, stop := range c.follow {
		if !seen[k] {
			stop()
			delete(c.follow, k)
		}
	}
	c.mu.Unlock()
}

// running: the app container has started (logs can be read).
func running(p kube.Object) bool {
	for _, cs := range kube.Slice(p, "status", "containerStatuses") {
		if kube.Field(cs, "state", "running") != nil || kube.Field(cs, "state", "terminated") != nil {
			return true
		}
	}
	return false
}

// followPod streams one pod's log until it ends (the next reconcile starts it again if the pod still runs).
func (c *Collector) followPod(ctx context.Context, key, ns, pod string, app int64) {
	defer func() {
		c.mu.Lock()
		delete(c.follow, key) // let reconcile restart it (new container after a restart)
		c.mu.Unlock()
	}()
	c.mu.Lock()
	since := c.last[key]
	c.mu.Unlock()
	rc, err := c.Kube.Stream(ctx, observe.LogPath(ns, pod, true, false, 0, since))
	if err != nil {
		return
	}
	defer rc.Close()
	go func() { <-ctx.Done(); rc.Close() }()
	var bmu sync.Mutex
	var batch []Line
	flush := func() {
		bmu.Lock()
		defer bmu.Unlock()
		if len(batch) == 0 {
			return
		}
		if err := c.Store.Append(app, batch); err != nil {
			c.Log.Warn("log history: write", "err", err)
		}
		c.mu.Lock()
		c.last[key] = batch[len(batch)-1].Time
		c.mu.Unlock()
		batch = batch[:0]
	}
	done := make(chan struct{})
	defer close(done)
	go func() { // quiet apps: write what arrived within a second
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				flush()
			}
		}
	}()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := observe.ParseLine(pod, sc.Text())
		if l.Time.IsZero() {
			l.Time = time.Now().UTC()
		}
		if !since.IsZero() && !l.Time.After(since) {
			continue // already stored before a reconnect (sinceTime has second precision)
		}
		if len(l.Line) > 16<<10 {
			l.Line = l.Line[:16<<10]
		}
		bmu.Lock()
		batch = append(batch, Line{Pod: pod, Time: l.Time, Line: l.Line})
		full := len(batch) >= 500
		bmu.Unlock()
		if full {
			flush()
		}
	}
	flush()
}
