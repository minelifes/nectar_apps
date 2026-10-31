package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/alerts"
	"nectar_apps/internal/auth"
	"nectar_apps/internal/build"
	"nectar_apps/internal/store"
)

// Preview environments: with previews on (app → Builds → Source), every pull request into the app's
// branch gets its own app in the same project — a copy of the app (settings, variables, config files, env
// groups; database links only when allowed) built from the pull request's head and reachable at its own
// default domain. New commits rebuild it; closing or merging the pull request deletes it. The Git host
// shows the result as the commit status "nectar/preview" (when the access token may write statuses).

// maxPreviews limits the previews of one app (each one is a running copy).
const maxPreviews = 10

// previewSlug is the slug of a preview app: <parent>-pr<n>, shortened to the 30 characters a slug may have.
func previewSlug(parent string, n int) string {
	suffix := "-pr" + strconv.Itoa(n)
	if len(parent)+len(suffix) > 30 {
		parent = strings.TrimRight(parent[:30-len(suffix)], "-")
	}
	return parent + suffix
}

// handlePullRequest acts on a pull request event of a parent app. It answers the webhook.
func (s *Server) handlePullRequest(w http.ResponseWriter, r *http.Request, parent store.App, src store.AppSource, ev build.Push) {
	ctx := r.Context()
	pr := ev.PR
	note := func(msg string) { writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": msg}) }
	switch {
	case !src.Previews:
		note("preview environments are off for this app")
		return
	case pr.Action == "":
		note("pull request event ignored (only opened, new commits and closed matter)")
		return
	case pr.BaseBranch != "" && pr.BaseBranch != src.Branch:
		note("the pull request does not target " + src.Branch)
		return
	case pr.Fork && !src.PreviewForks && pr.Action != build.PRClose:
		note("pull requests from forks get no preview (allow them in the app's source settings)")
		return
	}
	proj, err := s.store.GetProject(ctx, parent.ProjectID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	existing, err := s.store.FindPreview(ctx, parent.ID, pr.Number)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, r, err)
		return
	}
	found := err == nil
	actor := "webhook:" + ev.Provider
	if pr.Action == build.PRClose {
		if !found {
			note("no preview for this pull request")
			return
		}
		app, err := s.store.GetApp(ctx, existing.AppID)
		if err == nil {
			err = s.removeApp(context.WithoutCancel(ctx), app, proj)
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		_ = s.store.Audit(ctx, nil, actor, "preview.delete", proj.Slug+"/"+previewSlug(parent.Slug, pr.Number), clientIP(r), map[string]any{"pull_request": pr.Number})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
		return
	}
	if len(pr.HeadSHA) != 40 {
		writeError(w, http.StatusBadRequest, "webhook: no head commit in the pull request event")
		return
	}
	var app store.App
	if found {
		if app, err = s.store.GetApp(ctx, existing.AppID); err != nil {
			s.serverError(w, r, err)
			return
		}
	} else {
		if list, _ := s.store.ListPreviews(ctx, parent.ID); len(list) >= maxPreviews {
			writeJSON(w, http.StatusConflict, map[string]any{"error": fmt.Sprintf("%s already has %d previews: close a pull request or delete a preview", parent.Slug, maxPreviews)})
			return
		}
		if app, err = s.createPreview(r, parent, proj, src, pr); err != nil {
			writeError(w, http.StatusBadGateway, "could not create the preview: "+err.Error())
			return
		}
		_ = s.store.Audit(ctx, nil, actor, "preview.create", proj.Slug+"/"+app.Slug, clientIP(r), map[string]any{"pull_request": pr.Number, "title": pr.Title})
	}
	if err := s.store.SavePreview(ctx, store.Preview{AppID: app.ID, ParentID: parent.ID, Number: pr.Number, Title: pr.Title, URL: pr.URL,
		Author: pr.Author, HeadRef: pr.HeadRef, CommitSHA: pr.HeadSHA, Provider: ev.Provider}); err != nil {
		s.serverError(w, r, err)
		return
	}
	b, err := s.builder.Start(context.WithoutCancel(ctx), build.Request{App: app, Project: proj, Trigger: "pull-request", Commit: pr.HeadSHA,
		Message: fmt.Sprintf("#%d %s", pr.Number, pr.Title), Deploy: true})
	if errors.Is(err, store.ErrBuildRunning) {
		// the running build finishes first; previewBuildFinished then builds the newest commit
		writeJSON(w, http.StatusAccepted, map[string]any{"preview": app.Slug, "queued": pr.HeadSHA})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.reportPreview(context.WithoutCancel(ctx), app, b, build.StatePending, "Building the preview")
	writeJSON(w, http.StatusAccepted, map[string]any{"preview": app.Slug, "build": b.Number, "url": s.previewURL(ctx, app)})
}

// createPreview copies the parent app into a new preview app and points its source at the pull request.
func (s *Server) createPreview(r *http.Request, parent store.App, proj store.Project, src store.AppSource, pr *build.PullRequest) (store.App, error) {
	ctx := r.Context()
	slug := previewSlug(parent.Slug, pr.Number)
	if taken, _ := s.store.SlugTaken(ctx, proj.ID, slug); taken {
		return store.App{}, fmt.Errorf("%s is already used in %s", slug, proj.Slug)
	}
	name := fmt.Sprintf("%s · PR #%d", parent.Name, pr.Number)
	app, _, err := s.cloneAppInto(r, parent, proj, proj, slug, name, nil)
	if err != nil {
		return app, err
	}
	fail := func(err error) (store.App, error) {
		_ = s.removeApp(context.WithoutCancel(ctx), app, proj)
		return store.App{}, err
	}
	if !src.PreviewDatabases {
		// previews must not write into the parent's databases unless the app owner allows it
		if links, err := s.store.AddonLinks(ctx, 0, app.ID); err == nil {
			for _, l := range links {
				_ = s.store.UnlinkAddon(ctx, l.AddonID, app.ID)
			}
		}
	}
	if app.Config.Replicas > 1 {
		app.Config.Replicas = 1
		if app, err = s.store.SaveApp(ctx, app); err != nil {
			return fail(err)
		}
	}
	plain, _ := auth.NewToken("")
	secret, err := s.builder.Box.Seal(plain) // previews get no webhook of their own; the parent's events drive them
	if err != nil {
		return fail(err)
	}
	ps := src
	ps.AppID, ps.Branch, ps.WebhookSecret = app.ID, pr.HeadRef, secret
	ps.AutoDeploy, ps.Previews, ps.PreviewDatabases, ps.PreviewForks = false, false, false, false
	if _, err := s.store.SaveAppSource(ctx, ps); err != nil {
		return fail(err)
	}
	return app, nil
}

// previewURL is the preview's default domain ("" when the panel has no base domain).
func (s *Server) previewURL(ctx context.Context, app store.App) string {
	ds, err := s.store.ListDomains(ctx, app.ID)
	if err != nil {
		return ""
	}
	for _, d := range ds {
		if d.IsDefault {
			return "https://" + d.Host
		}
	}
	if len(ds) > 0 {
		return "https://" + ds[0].Host
	}
	return ""
}

// reportPreview sets the commit status of a preview build on the Git host (best effort).
func (s *Server) reportPreview(ctx context.Context, app store.App, b store.Build, state, desc string) {
	pv, err := s.store.GetPreview(ctx, app.ID)
	if err != nil {
		return
	}
	src, err := s.store.GetAppSource(ctx, app.ID)
	if err != nil || src.Token == "" {
		return
	}
	token, err := s.builder.Box.Open(src.Token)
	if err != nil {
		return
	}
	// success links the preview itself; pending and failure the app page in the panel (build log)
	target := s.previewURL(ctx, app)
	if set, err := alerts.LoadSettings(ctx, s.store); state != build.StateSuccess && err == nil && set.PanelURL != "" {
		target = strings.TrimRight(set.PanelURL, "/") + "/apps/" + strconv.FormatInt(app.ID, 10)
	}
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := build.ReportStatus(sctx, nil, build.CommitStatus{Provider: pv.Provider, RepoURL: src.RepoURL, Token: token, SHA: b.CommitSHA,
		State: state, TargetURL: target, Description: desc}); err != nil {
		s.log.Warn("preview commit status", "app", app.Slug, "err", err)
	}
}

// previewBuildFinished (Builder.OnFinish) reports the outcome of a preview build and, when the pull request
// got new commits meanwhile, builds the newest one.
func (s *Server) previewBuildFinished(ctx context.Context, b store.Build) {
	pv, err := s.store.GetPreview(ctx, b.AppID)
	if err != nil {
		return // not a preview
	}
	app, err := s.store.GetApp(ctx, b.AppID)
	if err != nil {
		return
	}
	switch b.Status {
	case store.BuildSucceeded:
		s.reportPreview(ctx, app, b, build.StateSuccess, "Preview deployed")
	case store.BuildFailed:
		s.reportPreview(ctx, app, b, build.StateFailure, "Build failed: "+b.Message)
	}
	if pv.CommitSHA == "" || pv.CommitSHA == b.CommitSHA || b.Status == store.BuildCanceled {
		return
	}
	proj, err := s.store.GetProject(ctx, app.ProjectID)
	if err != nil {
		return
	}
	nb, err := s.builder.Start(ctx, build.Request{App: app, Project: proj, Trigger: "pull-request", Commit: pv.CommitSHA,
		Message: fmt.Sprintf("#%d %s", pv.Number, pv.Title), Deploy: true})
	if err != nil {
		s.log.Warn("preview build", "app", app.Slug, "err", err)
		return
	}
	s.reportPreview(ctx, app, nb, build.StatePending, "Building the preview")
}

type previewView struct {
	store.Preview
	Slug     string `json:"slug"`
	Status   string `json:"status"`
	Live     string `json:"live_url,omitempty"`
	ParentOf string `json:"parent,omitempty"`
}

// listPreviews: GET /api/apps/{id}/previews — the app's previews, or (for a preview) what it previews.
func (s *Server) listPreviews(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	ctx := r.Context()
	out := map[string]any{"previews": []previewView{}}
	if pv, err := s.store.GetPreview(ctx, app.ID); err == nil {
		v := previewView{Preview: pv, Slug: app.Slug, Status: app.Status, Live: s.previewURL(ctx, app)}
		if parent, err := s.store.GetApp(ctx, pv.ParentID); err == nil {
			v.ParentOf = parent.Slug
		}
		out["preview_of"] = v
	}
	list, err := s.store.ListPreviews(ctx, app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	views := []previewView{}
	for _, pv := range list {
		pa, err := s.store.GetApp(ctx, pv.AppID)
		if err != nil {
			continue
		}
		views = append(views, previewView{Preview: pv, Slug: pa.Slug, Status: pa.Status, Live: s.previewURL(ctx, pa)})
	}
	out["previews"] = views
	writeJSON(w, http.StatusOK, out)
}

// RunPreviews deletes previews without activity for longer than their app allows (hourly).
func (s *Server) RunPreviews(ctx context.Context) {
	for {
		s.expirePreviews(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func (s *Server) expirePreviews(ctx context.Context, now time.Time) int {
	list, err := s.store.ListPreviews(ctx, 0)
	if err != nil {
		return 0
	}
	n := 0
	for _, pv := range list {
		src, err := s.store.GetAppSource(ctx, pv.ParentID)
		if err != nil || src.PreviewTTLDays <= 0 || now.Sub(pv.UpdatedAt) < time.Duration(src.PreviewTTLDays)*24*time.Hour {
			continue
		}
		app, err := s.store.GetApp(ctx, pv.AppID)
		if err != nil {
			continue
		}
		proj, err := s.store.GetProject(ctx, app.ProjectID)
		if err != nil {
			continue
		}
		if err := s.removeApp(ctx, app, proj); err != nil {
			s.log.Warn("expire preview", "app", app.Slug, "err", err)
			continue
		}
		_ = s.store.Audit(ctx, nil, "system", "preview.expire", proj.Slug+"/"+app.Slug, "", map[string]any{"pull_request": pv.Number, "days": src.PreviewTTLDays})
		n++
	}
	return n
}
