package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/registry"
	"nectar_apps/internal/scan"
	"nectar_apps/internal/store"
)

// Image scanning: the image of an app's running release is checked for known vulnerabilities with Trivy
// (internal/scan) — on request, and every NECTAR_IMAGE_SCAN_HOURS when set. The panel keeps the last
// result per app: counts by severity, how many have a fix, and the worst findings.

// scanView is an app's scan state ("none" when it was never scanned).
type scanView struct {
	store.ImageScan
	// Stale: the app now runs another image than the scanned one.
	Stale bool `json:"stale"`
	// Current is the image of the running release ("" = not deployed).
	Current string `json:"current_image"`
}

func (s *Server) getScan(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	v := scanView{}
	if live, err := s.store.LiveRelease(r.Context(), app.ID); err == nil {
		v.Current = live.Image
	}
	sc, err := s.store.GetImageScan(r.Context(), app.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		v.ImageScan = store.ImageScan{AppID: app.ID, Status: "none", Findings: json.RawMessage("[]")}
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	default:
		v.ImageScan = sc
		v.Stale = v.Current != "" && sc.Image != v.Current
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) startScan(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	live, err := s.store.LiveRelease(r.Context(), app.ID)
	if err != nil || live.Image == "" {
		writeError(w, http.StatusConflict, "deploy the app first: there is no image to scan")
		return
	}
	if err := s.scanImage(r.Context(), app, live.Image); errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a scan of this app is already running")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, p, "app.scan", proj.Slug+"/"+app.Slug, map[string]any{"image": live.Image})
	writeJSON(w, http.StatusAccepted, scanView{Current: live.Image, ImageScan: store.ImageScan{AppID: app.ID, Image: live.Image,
		Status: store.ScanRunning, Findings: json.RawMessage("[]"), StartedAt: time.Now().UTC()}})
}

// scanRequest turns the image a pod runs into what the scanner pod can reach: an image of the built-in
// registry is read over its in-cluster address (plain HTTP, the build login); other registries get the
// saved logins.
func (s *Server) scanRequest(ctx context.Context, image string) scan.Request {
	req := scan.Request{Image: image}
	var creds []registry.Credential
	if s.registry.Enabled() && s.registry.Owns(image) {
		req.Image = s.registry.PushHost() + strings.TrimPrefix(image, s.registry.PullHost())
		req.PlainHTTP = true
		creds = append(creds, s.registry.BuildCredential())
	}
	if s.registryBox != nil {
		if extra, err := registry.LoadCredentials(ctx, s.store, s.registryBox); err == nil {
			creds = append(creds, extra...)
		}
	}
	if len(creds) > 0 {
		req.DockerConfig = registry.DockerConfig(creds)
	}
	return req
}

// scanImage starts a scan in the background (one at a time on the server: a scan downloads the
// vulnerability database and the image). store.ErrConflict when the app's scan is already running.
func (s *Server) scanImage(ctx context.Context, app store.App, image string) error {
	prev, prevErr := s.store.GetImageScan(ctx, app.ID)
	if err := s.store.StartImageScan(ctx, app.ID, image); err != nil {
		return err
	}
	sc := s.scanner
	if sc == nil {
		sc = &scan.Scanner{Kube: s.kube, Image: s.cfg.TrivyImage}
	}
	req := s.scanRequest(ctx, image)
	s.scanWG.Add(1)
	go func() {
		defer s.scanWG.Done()
		s.scanMu.Lock()
		defer s.scanMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		out := store.ImageScan{AppID: app.ID, Status: store.ScanDone}
		res, err := sc.Scan(ctx, "app-"+strconv.FormatInt(app.ID, 10), req)
		if err != nil {
			out.Status, out.Message = store.ScanFailed, err.Error()
			s.log.Warn("image scan", "app", app.Slug, "image", image, "err", err)
		} else {
			out.Total, out.Critical, out.High, out.Medium, out.Low, out.Unknown, out.Fixable = res.Total, res.Critical, res.High, res.Medium, res.Low, res.Unknown, res.Fixable
			out.Findings, _ = json.Marshal(res.Findings)
			// announce critical findings that are new: another image, or more of them than last time
			known := prevErr == nil && prev.Status == store.ScanDone && prev.Image == image && prev.Critical >= res.Critical
			if res.Critical > 0 && !known && s.scanEvent != nil {
				s.scanEvent(context.WithoutCancel(ctx), app.Name, fmt.Sprintf("The image of %s has %d critical vulnerabilities (%d of all %d findings are fixed in a newer version).",
					app.Name, res.Critical, res.Fixable, res.Total), "/apps/"+strconv.FormatInt(app.ID, 10))
			}
		}
		if err := s.store.FinishImageScan(context.WithoutCancel(ctx), out); err != nil {
			s.log.Warn("image scan: saving the result", "app", app.Slug, "err", err)
		}
	}()
	return nil
}

// RunImageScans re-scans deployed apps whose last scan is older than every (or of another image), one
// per round, so new vulnerabilities in an unchanged image show up. every <= 0: off.
func (s *Server) RunImageScans(ctx context.Context, every time.Duration) {
	if every <= 0 || s.kube == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
		if app, image, ok := s.nextScan(ctx, time.Now(), every); ok {
			if err := s.scanImage(ctx, app, image); err != nil && !errors.Is(err, store.ErrConflict) {
				s.log.Warn("image scan", "app", app.Slug, "err", err)
			}
		}
	}
}

// nextScan picks the deployed app whose scan is the most overdue.
func (s *Server) nextScan(ctx context.Context, now time.Time, every time.Duration) (store.App, string, bool) {
	apps, err := s.store.ListApps(ctx, 0)
	if err != nil {
		return store.App{}, "", false
	}
	scans, err := s.store.ListImageScans(ctx)
	if err != nil {
		return store.App{}, "", false
	}
	last := map[int64]store.ImageScan{}
	for _, sc := range scans {
		last[sc.AppID] = sc
	}
	var pick store.App
	var pickImage string
	var pickAt time.Time
	found := false
	for _, a := range apps {
		live, err := s.store.LiveRelease(ctx, a.ID)
		if err != nil || live.Image == "" {
			continue
		}
		sc, scanned := last[a.ID]
		if scanned && sc.Status == store.ScanRunning {
			continue
		}
		at := time.Time{} // never scanned: first
		if scanned {
			at = sc.StartedAt
			// a failed scan (e.g. no access to the registry) is retried at the same pace, not every round
			if sc.Image == live.Image && now.Sub(at) < every {
				continue
			}
		}
		if !found || at.Before(pickAt) {
			pick, pickImage, pickAt, found = a, live.Image, at, true
		}
	}
	return pick, pickImage, found
}

// scanSummary is an app's last scan without the findings (badges in the project's app list).
type scanSummary struct {
	AppID    int64  `json:"app_id"`
	Status   string `json:"status"`
	Total    int    `json:"total"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Fixable  int    `json:"fixable"`
}

func (s *Server) projectScans(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	apps, err := s.store.ListApps(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	scans, err := s.store.ListImageScans(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	mine := map[int64]bool{}
	for _, a := range apps {
		mine[a.ID] = true
	}
	out := []scanSummary{}
	for _, sc := range scans {
		if mine[sc.AppID] {
			out = append(out, scanSummary{AppID: sc.AppID, Status: sc.Status, Total: sc.Total, Critical: sc.Critical, High: sc.High, Fixable: sc.Fixable})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
