package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/oci"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// Image update watcher: an image app follows a tag (postgres:17, ghcr.io/acme/api:stable). When the tag
// points to a newer image than the one the app's pods run, the app's image_updates setting decides:
// "notify" announces it (alert image_updated, a note on the app), "auto" deploys it. The new image is
// deployed pinned to its digest (tag@sha256:…), so every node runs exactly that image and the next check
// knows what runs. An automatic update never ships undeployed settings or variables: an app with pending
// changes is only announced.

// imageWatchView is GET /apps/{id}/image-updates.
type imageWatchView struct {
	Policy    string            `json:"policy"`           // "" | notify | auto
	Supported bool              `json:"supported"`        // false: not an image app, pinned, or built by Nectar
	Reason    string            `json:"reason,omitempty"` // why not supported
	Watch     *store.ImageWatch `json:"watch,omitempty"`  // the last check
}

// watchable says whether the app's image can be watched ("" = yes, else why not).
func (s *Server) watchable(app store.App) string {
	rt, ok := s.catalog.Get(app.RuntimeID)
	if !ok || rt.Artifact.Kind != runtimes.ArtifactNone {
		return "the app runs a runtime image managed by Nectar"
	}
	if s.registry.Enabled() && s.registry.Owns(app.Config.Image) {
		return "the image is built by Nectar: new images come from builds"
	}
	ref, err := oci.ParseRef(app.Config.Image)
	if err != nil {
		return err.Error()
	}
	if ref.Digest != "" {
		return "the image is pinned to a digest: there is nothing newer to follow"
	}
	return ""
}

func (s *Server) getImageWatch(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	v := imageWatchView{Policy: app.Config.ImageUpdates}
	v.Reason = s.watchable(app)
	v.Supported = v.Reason == ""
	if wt, err := s.store.GetImageWatch(r.Context(), app.ID); err == nil && wt.Image == app.Config.Image {
		v.Watch = &wt
	}
	writeJSON(w, http.StatusOK, v)
}

// checkImageNow checks the registry now. It records the result but does not act on it (no announcement,
// no deploy): that is the watcher's job, so viewing an app never deploys it.
func (s *Server) checkImageNow(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if why := s.watchable(app); why != "" {
		writeError(w, http.StatusConflict, why)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	wt, err := s.checkImage(ctx, app, proj, false)
	if err != nil && wt.Status == "" {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, imageWatchView{Policy: app.Config.ImageUpdates, Supported: true, Watch: &wt})
}

// deployNewerImage deploys the newer image now (with the app's current settings, pending ones included).
func (s *Server) deployNewerImage(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	wt, err := s.store.GetImageWatch(r.Context(), app.ID)
	if err != nil || wt.Image != app.Config.Image || wt.Status != store.WatchUpdate {
		writeError(w, http.StatusConflict, "no newer image is known for this app: check again")
		return
	}
	rel, err := s.deployer.Deploy(r.Context(), deploy.DeployRequest{App: app, By: &p.user.ID, Image: app.Config.Image + "@" + wt.LatestDigest})
	if err != nil {
		s.deployErr(w, err)
		return
	}
	wt.HandledDigest, wt.Message = wt.LatestDigest, fmt.Sprintf("deploying release #%d", rel.Number)
	_ = s.store.SaveImageWatch(r.Context(), wt)
	s.audit(r, p, "app.image_update", proj.Slug+"/"+app.Slug, map[string]any{"release": rel.Number, "digest": wt.LatestDigest})
	writeJSON(w, http.StatusAccepted, rel)
}

// registryClient reads registries with the saved logins.
func (s *Server) registryClient(ctx context.Context) *oci.Client {
	if s.oci != nil {
		return s.oci
	}
	var creds []registry.Credential
	if s.registryBox != nil {
		creds, _ = registry.LoadCredentials(ctx, s.store, s.registryBox)
	}
	return &oci.Client{Logins: func(host string) (oci.Login, bool) {
		for _, name := range oci.HostAliases(host) {
			for _, c := range creds {
				if strings.EqualFold(strings.TrimSuffix(c.Host, "/"), name) {
					return oci.Login{Username: c.Username, Password: c.Password}, true
				}
			}
		}
		return oci.Login{}, false
	}}
}

// runningDigest is the digest of the image the app's pods run ("" = not known: no pods yet).
func (s *Server) runningDigest(ctx context.Context, app store.App, proj store.Project) string {
	if s.deployer != nil {
		if d := s.deployer.RunningDigest(ctx, app, proj); d != "" {
			return d
		}
	}
	// not running (stopped, sleeping, a cron job between runs): the digest an update deployed
	if live, err := s.store.LiveRelease(ctx, app.ID); err == nil {
		if _, d, ok := strings.Cut(live.Image, "@"); ok && strings.HasPrefix(live.Image, app.Config.Image+"@") {
			return d
		}
	}
	return ""
}

// checkImage asks the registry what the app's tag points to and records it. With act, a newer image is
// announced or deployed as the app's setting says (once per digest).
func (s *Server) checkImage(ctx context.Context, app store.App, proj store.Project, act bool) (store.ImageWatch, error) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	now := time.Now().UTC()
	prev, prevErr := s.store.GetImageWatch(ctx, app.ID)
	if prevErr != nil || prev.Image != app.Config.Image {
		prev = store.ImageWatch{} // another tag: start over
	}
	wt := store.ImageWatch{AppID: app.ID, Image: app.Config.Image, CheckedAt: now, HandledDigest: prev.HandledDigest,
		LatestDigest: prev.LatestDigest, SeenAt: prev.SeenAt}
	ref, err := oci.ParseRef(app.Config.Image)
	if err != nil {
		return store.ImageWatch{}, err
	}
	wt.RunningDigest = s.runningDigest(ctx, app, proj)
	latest, err := s.registryClient(ctx).Digest(ctx, ref)
	if err != nil {
		wt.Status, wt.Message = store.WatchError, err.Error()
		_ = s.store.SaveImageWatch(ctx, wt)
		return wt, err
	}
	if latest != wt.LatestDigest {
		wt.LatestDigest, wt.SeenAt = latest, now
	}
	switch {
	case wt.RunningDigest == "":
		wt.Status, wt.Message = store.WatchUnknown, "not known which image the app runs: it has no running pods"
	case wt.RunningDigest == latest:
		wt.Status, wt.Message = store.WatchCurrent, ""
		wt.HandledDigest = latest
	default:
		wt.Status, wt.Message = store.WatchUpdate, ""
		if act && app.Config.ImageUpdates != "" && latest != wt.HandledDigest {
			wt.HandledDigest = latest
			wt.Message = s.actOnNewImage(ctx, app, proj, latest)
		}
	}
	return wt, s.store.SaveImageWatch(ctx, wt)
}

// actOnNewImage announces or deploys a newer image; returns the note for the app.
func (s *Server) actOnNewImage(ctx context.Context, app store.App, proj store.Project, digest string) string {
	path := "/apps/" + strconv.FormatInt(app.ID, 10)
	announce := func(msg string) {
		if s.imageEvent != nil {
			s.imageEvent(context.WithoutCancel(ctx), app.Name, msg, path)
		}
	}
	short := strings.TrimPrefix(digest, "sha256:")[:12]
	if app.Config.ImageUpdates != "auto" {
		announce(fmt.Sprintf("A newer image of %s is available (%s). Deploy it from the app's Settings.", app.Config.Image, short))
		return "announced"
	}
	if s.deployer == nil {
		return "not deployed: no cluster"
	}
	if pending, err := s.deployer.Pending(ctx, app); err == nil && len(pending) > 0 {
		msg := fmt.Sprintf("A newer image of %s is available (%s), but it was not deployed automatically: the app has undeployed changes (%s). Deploy them, or deploy the image from the app's Settings.",
			app.Config.Image, short, strings.Join(pending, ", "))
		announce(msg)
		return "not deployed automatically: the app has undeployed changes (" + strings.Join(pending, ", ") + ")"
	}
	rel, err := s.deployer.Deploy(ctx, deploy.DeployRequest{App: app, Image: app.Config.Image + "@" + digest})
	if err != nil {
		announce(fmt.Sprintf("A newer image of %s is available (%s), but deploying it failed: %v", app.Config.Image, short, err))
		return "deploying it failed: " + err.Error()
	}
	_ = s.store.Audit(ctx, nil, "system", "app.image_update", proj.Slug+"/"+app.Slug, "", map[string]any{"release": rel.Number, "digest": digest})
	announce(fmt.Sprintf("%s was updated to the newer image of %s (%s): release #%d.", app.Name, app.Config.Image, short, rel.Number))
	return fmt.Sprintf("deploying release #%d", rel.Number)
}

// RunImageWatch checks the apps with image updates on every `every` (0 = off).
func (s *Server) RunImageWatch(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	wait := 2 * time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = every
		s.watchRound(ctx)
	}
}

// watchRound checks every app whose image updates are on.
func (s *Server) watchRound(ctx context.Context) {
	apps, err := s.store.ListApps(ctx, 0)
	if err != nil {
		return
	}
	projects := map[int64]store.Project{}
	for _, a := range apps {
		if a.Config.ImageUpdates == "" || s.watchable(a) != "" {
			continue
		}
		if _, err := s.store.LiveRelease(ctx, a.ID); err != nil {
			continue // never deployed
		}
		p, ok := projects[a.ProjectID]
		if !ok {
			if p, err = s.store.GetProject(ctx, a.ProjectID); err != nil {
				continue
			}
			projects[a.ProjectID] = p
		}
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		_, err := s.checkImage(cctx, a, p, true)
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			s.log.Debug("image watch", "app", a.Slug, "err", err)
		}
	}
}
