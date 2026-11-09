package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/routing"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// Scale to zero: a web app with sleep_after_minutes sleeps (Deployment at 0 replicas, status "sleeping")
// after that long without requests through the gateway. Its routes carry web.wake: on the next request
// the gateway calls /internal/wake/{id}, holds the request until the app answers and passes it on.
// Requests from other apps straight to the service don't go through the gateway: they neither count as
// activity nor wake the app.

// signWake signs the wake URL of an app (not time-limited: the URL sits in the gateway's route).
func signWake(key []byte, appID int64) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("wake:" + strconv.FormatInt(appID, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) wakeKey() []byte {
	if s.artKey != nil {
		return s.artKey
	}
	return s.cfg.MasterKey
}

// wakeURL is the URL the gateway calls to wake an app (in-cluster panel address).
func (s *Server) wakeURL(appID int64) string {
	id := strconv.FormatInt(appID, 10)
	return strings.TrimRight(s.cfg.InternalURL, "/") + "/internal/wake/" + id + "?sig=" + signWake(s.wakeKey(), appID)
}

// internalWake is called by the gateway (signed URL, no session).
func (s *Server) internalWake(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !hmac.Equal([]byte(signWake(s.wakeKey(), id)), []byte(r.URL.Query().Get("sig"))) {
		writeError(w, http.StatusForbidden, "bad signature")
		return
	}
	app, err := s.store.GetApp(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such app")
		return
	}
	if app.Status == deploy.StatusSleeping {
		if err := s.wake(r.Context(), app, "gateway"); err != nil {
			s.log.Warn("wake", "app", app.Slug, "err", err)
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// wakeApp: "Wake now" in the UI.
func (s *Server) wakeApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	if app.Status != deploy.StatusSleeping {
		writeError(w, http.StatusConflict, "the app is not sleeping")
		return
	}
	if err := s.deployer.Wake(r.Context(), app, proj); err != nil {
		s.opErr(w, err)
		return
	}
	s.audit(r, p, "app.wake", proj.Slug+"/"+app.Slug, nil)
	app, _ = s.store.GetApp(r.Context(), app.ID)
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) wake(ctx context.Context, app store.App, by string) error {
	if s.deployer == nil {
		return errors.New("kubernetes is not configured")
	}
	proj, err := s.store.GetProject(ctx, app.ProjectID)
	if err != nil {
		return err
	}
	if err := s.deployer.Wake(ctx, app, proj); err != nil {
		return err
	}
	_ = s.store.Audit(ctx, nil, "system", "app.wake", proj.Slug+"/"+app.Slug, "", map[string]any{"by": by})
	return nil
}

// RunSleeper puts idle apps to sleep (every minute).
func (s *Server) RunSleeper(ctx context.Context) {
	awake := map[int64]time.Time{}
	for {
		s.sleepIdle(ctx, time.Now(), awake)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

// sleepIdle sleeps the apps without requests for their sleep_after_minutes. awake remembers since when
// the panel has seen each app running (the gateway's statistics start empty after its restart, and a
// woken app gets its full time again). Returns the apps put to sleep.
func (s *Server) sleepIdle(ctx context.Context, now time.Time, awake map[int64]time.Time) []string {
	if s.deployer == nil || s.gateway == nil || !s.syncer.CanWake(ctx) {
		clear(awake)
		return nil
	}
	apps, err := s.store.ListApps(ctx, 0)
	if err != nil {
		return nil
	}
	var slept []string
	seen := map[int64]bool{}
	for _, a := range apps {
		if a.Kind != runtimes.KindWeb || a.Config.SleepAfterMinutes <= 0 || a.Config.Replicas == 0 || a.Status != deploy.StatusRunning {
			continue
		}
		seen[a.ID] = true
		since, ok := awake[a.ID]
		if !ok {
			awake[a.ID] = now
			continue
		}
		idle := time.Duration(a.Config.SleepAfterMinutes) * time.Minute
		if now.Sub(since) < idle {
			continue
		}
		last, ok := s.lastRequest(ctx, a)
		if !ok || now.Sub(last) < idle {
			continue
		}
		proj, err := s.store.GetProject(ctx, a.ProjectID)
		if err != nil {
			continue
		}
		if err := s.deployer.Sleep(ctx, a, proj); err != nil {
			s.log.Warn("sleep", "app", a.Slug, "err", err)
			continue
		}
		delete(awake, a.ID)
		slept = append(slept, proj.Slug+"/"+a.Slug)
		_ = s.store.Audit(ctx, nil, "system", "app.sleep", proj.Slug+"/"+a.Slug, "", map[string]any{"idle_minutes": a.Config.SleepAfterMinutes})
	}
	for id := range awake {
		if !seen[id] {
			delete(awake, id)
		}
	}
	return slept
}

// lastRequest is the time of the newest request to any domain of the app that the gateway serves (zero
// time when none since the gateway started). ok=false: the app has no active domain (nothing could wake
// it) or the gateway didn't answer.
func (s *Server) lastRequest(ctx context.Context, a store.App) (time.Time, bool) {
	ds, err := s.store.ListDomains(ctx, a.ID)
	if err != nil {
		return time.Time{}, false
	}
	var last time.Time
	served := false
	done := map[string]bool{}
	for _, d := range ds {
		if d.Kind == store.DomainRedirect || d.Status != routing.StatusActive || done[d.Host] {
			continue
		}
		done[d.Host] = true
		served = true
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		sums, err := s.gateway.Metrics(cctx, 60, d.Host, false)
		cancel()
		if err != nil {
			return time.Time{}, false
		}
		for _, sm := range sums {
			if sm.Domain == d.Host && sm.LastSeen.After(last) {
				last = sm.LastSeen
			}
		}
	}
	return last, served
}
