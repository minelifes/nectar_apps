package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/observe"
	"nectar_apps/internal/store"
	"nectar_apps/internal/wsock"
)

// ------------------------------------------------------------ usage history --

var metricRanges = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour,
}

func parseRange(r *http.Request) (string, time.Time, time.Time) {
	name := r.URL.Query().Get("range")
	d, ok := metricRanges[name]
	if !ok {
		name, d = "1h", time.Hour
	}
	to := time.Now().UTC()
	return name, to.Add(-d), to
}

func (s *Server) serverMetrics(w http.ResponseWriter, r *http.Request, _ *principal) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	name, from, to := parseRange(r)
	nodes, err := s.kube.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes API unavailable: "+err.Error())
		return
	}
	type nodeSeries struct {
		Name     string              `json:"name"`
		CPUAlloc int64               `json:"cpu_alloc_m"`
		MemAlloc int64               `json:"mem_alloc_b"`
		Points   []store.MetricPoint `json:"points"`
	}
	out := []nodeSeries{}
	for _, n := range nodes {
		pts, err := s.store.MetricHistory(r.Context(), "node:"+n.Name, from, to, 240)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		out = append(out, nodeSeries{Name: n.Name, CPUAlloc: n.Allocatable.CPUMillicores, MemAlloc: n.Allocatable.MemoryBytes, Points: pts})
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": name, "from": from, "to": to, "nodes": out})
}

func (s *Server) appUsage(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	name, from, to := parseRange(r)
	pts, err := s.store.MetricHistory(r.Context(), "app:"+strconv.FormatInt(app.ID, 10), from, to, 240)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	replicas := int64(app.Config.Replicas)
	writeJSON(w, http.StatusOK, map[string]any{
		"range": name, "from": from, "to": to, "points": pts,
		// requested resources of all replicas: the line the chart compares usage with
		"request_cpu_m": app.Config.Resources.CPUMillicores * replicas,
		"request_mem_b": app.Config.Resources.MemoryMB * replicas << 20,
		"replicas":      replicas,
	})
}

// ------------------------------------------------------------------- events --

func (s *Server) appEvents(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	evs, err := observe.AppEvents(r.Context(), s.kube, proj.Namespace, app.Slug, 100)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) serverEvents(w http.ResponseWriter, r *http.Request, _ *principal) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	evs, err := observe.WarningEvents(r.Context(), s.kube, 100)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evs)
}

// --------------------------------------------------------------------- logs --

func (s *Server) appPods(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	pods, err := observe.AppPods(r.Context(), s.kube, proj.Namespace, app.Slug)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pods)
}

// appLogs returns recent lines (tail, max 10000) of all pods or one pod, oldest first. previous=1 reads the
// previous container instance (the crash before a restart). format=text downloads a plain text file.
func (s *Server) appLogs(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	q := r.URL.Query()
	tail, _ := strconv.Atoi(q.Get("tail"))
	pods, err := s.logPods(r.Context(), app, proj, q.Get("pod"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	lines, err := observe.TailLogs(r.Context(), s.kube, proj.Namespace, pods, tail, q.Get("previous") == "1")
	if err != nil {
		var se *kube.StatusError
		if errors.As(err, &se) && se.Code == http.StatusBadRequest {
			writeError(w, http.StatusBadRequest, se.Message) // e.g. "previous terminated container not found"
			return
		}
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	if q.Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.log"`, app.Slug, time.Now().UTC().Format("20060102-150405")))
		for _, l := range lines {
			fmt.Fprintf(w, "%s %s %s\n", l.Time.Format(time.RFC3339Nano), l.Pod, l.Line)
		}
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) logPods(ctx context.Context, app store.App, proj store.Project, only string) ([]string, error) {
	pods, err := observe.AppPods(ctx, s.kube, proj.Namespace, app.Slug)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range pods {
		if only == "" || p.Name == only {
			names = append(names, p.Name)
		}
	}
	return names, nil
}

// appLogsWS streams the logs of all pods of an app (or one pod) over a WebSocket. Messages are JSON:
//
//	{"type":"line","pod":"api-7c9…","t":"2026-10-28T00:47:35.12Z","line":"…"}
//	{"type":"pods","pods":[{name, phase, restarts, …}]}   (every 5 s; new pods are followed automatically)
//	{"type":"info","message":"…"}
func (s *Server) appLogsWS(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	if !wsock.IsUpgrade(r) {
		writeError(w, http.StatusBadRequest, "this endpoint needs a WebSocket connection")
		return
	}
	if o := r.Header.Get("Origin"); o != "" && !sameOrigin(r) && !(s.cfg.Dev && isLocalOrigin(o)) {
		writeError(w, http.StatusForbidden, "cross-origin WebSocket refused")
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 2000 {
		tail = 200
	}
	only := r.URL.Query().Get("pod")
	conn, err := wsock.Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	// the request context ends when this handler returns; the stream lives until the client goes away
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = conn.ReadLoop(nil)
		cancel()
	}()

	send := func(v any) bool {
		b, _ := json.Marshal(v)
		if err := conn.WriteText(b); err != nil {
			cancel()
			return false
		}
		return true
	}
	lines := make(chan observe.LogLine, 512)
	var mu sync.Mutex
	following := map[string]bool{}
	lastSeen := map[string]time.Time{}

	follow := func(pod string, tailLines int) {
		defer func() {
			mu.Lock()
			delete(following, pod)
			mu.Unlock()
		}()
		mu.Lock()
		since := lastSeen[pod]
		mu.Unlock()
		if !since.IsZero() {
			tailLines = 0 // resume after the last line we sent
			since = since.Add(time.Second)
		}
		rc, err := s.kube.Stream(ctx, observe.LogPath(proj.Namespace, pod, true, false, tailLines, since))
		if err != nil {
			return // container not started yet; the next pod listing tries again
		}
		defer rc.Close()
		_ = observe.ReadLines(ctx, rc, pod, func(l observe.LogLine) {
			mu.Lock()
			if l.Time.After(lastSeen[pod]) {
				lastSeen[pod] = l.Time
			}
			mu.Unlock()
			select {
			case lines <- l:
			case <-ctx.Done():
			}
		})
	}

	refresh := func(first bool) {
		pods, err := observe.AppPods(ctx, s.kube, proj.Namespace, app.Slug)
		if err != nil {
			send(map[string]any{"type": "info", "message": "cannot list pods: " + err.Error()})
			return
		}
		send(map[string]any{"type": "pods", "pods": pods})
		for _, p := range pods {
			if only != "" && p.Name != only {
				continue
			}
			mu.Lock()
			started := following[p.Name]
			if !started && len(following) < 20 {
				following[p.Name] = true
			} else {
				started = true
			}
			mu.Unlock()
			if !started {
				t := tail
				if !first {
					t = 50 // a pod that appeared later (new release, restart)
				}
				go follow(p.Name, t)
			}
		}
	}
	refresh(true)

	podTick := time.NewTicker(5 * time.Second)
	defer podTick.Stop()
	pingTick := time.NewTicker(25 * time.Second)
	defer pingTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case l := <-lines:
			if !send(map[string]any{"type": "line", "pod": l.Pod, "t": l.Time, "line": l.Line}) {
				return
			}
		case <-podTick.C:
			refresh(false)
		case <-pingTick.C:
			if conn.Ping() != nil {
				return
			}
		}
	}
}

// wsToken lets a WebSocket carry the bearer token as ?access_token= (browsers cannot set headers on
// WebSockets). Only for GET upgrade requests to /logs/ws.
func wsToken(r *http.Request) (string, bool) {
	if r.Method != http.MethodGet || !wsock.IsUpgrade(r) || !(strings.HasSuffix(r.URL.Path, "/logs/ws") || strings.HasSuffix(r.URL.Path, "/exec/ws")) {
		return "", false
	}
	t := r.URL.Query().Get("access_token")
	return t, t != ""
}
