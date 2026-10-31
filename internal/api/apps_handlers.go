package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/build"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/detect"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// --------------------------------------------------------- project scoping --

type projectHandler func(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project)
type appHandler func(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project)

// inProject loads {pid} and requires perm in that project.
func (s *Server) inProject(perm string, h projectHandler) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		id, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad project id")
			return
		}
		if !p.eff.Can(perm, &id) {
			// do not reveal whether the project exists
			if p.eff.Can("project.view", &id) {
				writeError(w, http.StatusForbidden, "missing permission: "+perm)
			} else {
				writeError(w, http.StatusNotFound, "project not found")
			}
			return
		}
		proj, err := s.store.GetProject(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		h(w, r, p, proj)
	})
}

// onApp loads app {id} and its project and requires perm in that project.
func (s *Server) onApp(perm string, h appHandler) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		id, ok := pathID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad app id")
			return
		}
		app, proj, ok := s.loadApp(w, r, p, id, perm)
		if ok {
			h(w, r, p, app, proj)
		}
	})
}

func (s *Server) loadApp(w http.ResponseWriter, r *http.Request, p *principal, id int64, perm string) (store.App, store.Project, bool) {
	app, err := s.store.GetApp(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return app, store.Project{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return app, store.Project{}, false
	}
	if !p.eff.Can(perm, &app.ProjectID) {
		if p.eff.Can("app.view", &app.ProjectID) {
			writeError(w, http.StatusForbidden, "missing permission: "+perm)
		} else {
			writeError(w, http.StatusNotFound, "app not found")
		}
		return app, store.Project{}, false
	}
	proj, err := s.store.GetProject(r.Context(), app.ProjectID)
	if err != nil {
		s.serverError(w, r, err)
		return app, proj, false
	}
	return app, proj, true
}

func (s *Server) needKube(w http.ResponseWriter) bool {
	if s.deployer == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return false
	}
	return true
}

// ---------------------------------------------------------------- runtimes --

func (s *Server) listRuntimes(w http.ResponseWriter, r *http.Request, _ *principal) {
	writeJSON(w, http.StatusOK, s.catalog.Runtimes)
}

// ---------------------------------------------------------------- projects --

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$`)

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, p *principal) {
	all, err := s.store.ListProjects(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := []store.Project{}
	for _, pr := range all {
		if p.eff.Can("project.view", &pr.ID) {
			out = append(out, pr)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func validQuota(q *store.Quota) string {
	if q == nil {
		return ""
	}
	if q.CPUMillicores < 0 || q.MemoryMB < 0 || q.StorageGB < 0 || q.Pods < 0 || q.Apps < 0 {
		return "quota values must not be negative"
	}
	return ""
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Slug  string       `json:"slug"`
		Name  string       `json:"name"`
		Quota *store.Quota `json:"quota"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be 1–30 lowercase letters, digits or dashes (not starting or ending with a dash)")
		return
	}
	if n := len(strings.TrimSpace(in.Name)); n == 0 || n > 100 {
		writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
		return
	}
	if msg := validQuota(in.Quota); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	proj, err := s.store.CreateProject(r.Context(), in.Slug, in.Name, in.Quota)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a project with this slug already exists")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if s.deployer != nil {
		if err := s.deployer.EnsureProject(r.Context(), proj); err != nil {
			_ = s.store.DeleteProject(r.Context(), proj.ID)
			if strings.Contains(err.Error(), "is being terminated") {
				writeError(w, http.StatusConflict, "a deleted project with this slug is still being removed from the cluster; try again in a minute")
				return
			}
			s.log.Error("create namespace", "project", proj.Slug, "err", err)
			writeError(w, http.StatusBadGateway, "could not create the project namespace: "+err.Error())
			return
		}
	}
	s.audit(r, p, "project.create", proj.Slug, in)
	writeJSON(w, http.StatusCreated, proj)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	var raw map[string]json.RawMessage
	if !decode(w, r, &raw) {
		return
	}
	var name *string
	if v, ok := raw["name"]; ok {
		var n string
		if json.Unmarshal(v, &n) != nil || len(strings.TrimSpace(n)) == 0 || len(n) > 100 {
			writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
			return
		}
		name = &n
	}
	var quota *store.Quota
	_, setQuota := raw["quota"]
	if setQuota && string(raw["quota"]) != "null" {
		quota = &store.Quota{}
		if err := json.Unmarshal(raw["quota"], quota); err != nil {
			writeError(w, http.StatusBadRequest, "invalid quota")
			return
		}
		if msg := validQuota(quota); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	for k := range raw {
		if k != "name" && k != "quota" {
			writeError(w, http.StatusBadRequest, "unknown field "+k)
			return
		}
	}
	proj, err := s.store.UpdateProject(r.Context(), proj.ID, name, setQuota, quota)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if setQuota && s.deployer != nil {
		if err := s.deployer.EnsureProject(r.Context(), proj); err != nil {
			writeError(w, http.StatusBadGateway, "saved, but applying the quota failed: "+err.Error())
			return
		}
	}
	s.audit(r, p, "project.update", proj.Slug, map[string]any{"name": name, "quota": quota, "quota_changed": setQuota})
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	apps, err := s.store.ListApps(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var shas []string
	for _, a := range apps {
		arts, _ := s.store.ListArtifacts(r.Context(), a.ID, 1000)
		for _, art := range arts {
			shas = append(shas, art.SHA256)
		}
	}
	if s.deployer != nil {
		if err := s.deployer.DeleteProject(r.Context(), proj); err != nil {
			writeError(w, http.StatusBadGateway, "could not delete the namespace: "+err.Error())
			return
		}
	}
	if err := s.store.DeleteProject(r.Context(), proj.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.removeOrphanFiles(r, shas)
	s.resync(r)
	s.audit(r, p, "project.delete", proj.Slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeOrphanFiles(r *http.Request, shas []string) {
	for _, sha := range shas {
		if used, err := s.store.ArtifactSHAExists(r.Context(), sha); err == nil && !used {
			_ = s.artifacts.Remove(sha)
		}
	}
}

// -------------------------------------------------------------------- apps --

func (s *Server) listApps(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	apps, err := s.store.ListApps(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

// defaultConfig returns sensible defaults for a new app of the given runtime and kind.
func defaultConfig(rt runtimes.Runtime, kind string) store.AppConfig {
	c := store.AppConfig{Replicas: 1, Resources: store.Resources{CPUMillicores: 250, MemoryMB: 256}, Options: rt.OptionDefaults()}
	if rt.ID == "java" {
		c.Resources.MemoryMB = 512
	}
	if kind == runtimes.KindWeb {
		c.Port = rt.Port
		c.Health = store.HealthConfig{Type: rt.Health.Type, Path: rt.Health.Path}
	} else {
		c.Health = store.HealthConfig{Type: "none"}
	}
	if len(c.Options) == 0 {
		c.Options = nil
	}
	return c
}

var imageRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$`)

// validateConfig returns an error message for an invalid config, or "".
func validateConfig(rt runtimes.Runtime, kind string, c store.AppConfig) string {
	if rt.Artifact.Kind == runtimes.ArtifactNone && !imageRe.MatchString(c.Image) {
		return "image is required, e.g. ghcr.io/acme/app:1.2.3"
	}
	if kind == runtimes.KindWeb && (c.Port < 1 || c.Port > 65535) {
		return "port must be 1–65535"
	}
	if c.Replicas < 0 || c.Replicas > 20 {
		return "replicas must be 0–20"
	}
	if len(c.InternalPorts) > 10 {
		return "at most 10 internal ports"
	}
	for _, p := range c.InternalPorts {
		if p < 1 || p > 65535 {
			return "internal ports must be 1–65535"
		}
		if kind != runtimes.KindWeb && kind != runtimes.KindWorker {
			return "only web and worker apps can have internal ports"
		}
		if c.Port == 0 {
			return "internal ports need the app's main port"
		}
	}
	if c.Resources.CPUMillicores < 0 || c.Resources.CPUMillicores > 64000 || c.Resources.MemoryMB < 0 || c.Resources.MemoryMB > 262144 {
		return "resources out of range (CPU 0–64 cores, memory 0–256 GiB)"
	}
	if !slicesContains([]string{"tcp", "http", "none"}, c.Health.Type) {
		return "health.type must be tcp, http or none"
	}
	if c.Health.Type == "http" && c.Health.Path != "" && !strings.HasPrefix(c.Health.Path, "/") {
		return "health.path must start with /"
	}
	if kind == runtimes.KindCron && len(strings.Fields(c.Schedule)) != 5 && !strings.HasPrefix(c.Schedule, "@") {
		return "schedule must be a cron expression with 5 fields, e.g. \"*/15 * * * *\""
	}
	if len(c.Expose) > 10 {
		return "at most 10 exposed ports"
	}
	for _, e := range c.Expose {
		if e.Port < 1 || e.Port > 65535 {
			return "exposed port must be 1–65535"
		}
		if p := strings.ToUpper(e.Protocol); p != "" && p != "TCP" && p != "UDP" {
			return "exposed port protocol must be TCP or UDP"
		}
		if e.NodePort != 0 && (e.NodePort < 30000 || e.NodePort > 32767) {
			return "node_port must be 30000–32767 (or 0 to assign automatically)"
		}
	}
	if len(c.Expose) > 0 && kind != runtimes.KindWeb && kind != runtimes.KindWorker {
		return "only web and worker apps can expose ports"
	}
	if len(c.Command) > 50 || len(c.Args) > 4096 || len(c.ReleaseCommand) > 4096 {
		return "command, args or release command too long"
	}
	for k := range c.Options {
		if _, ok := rt.Options[k]; !ok {
			return "unknown option " + k
		}
	}
	return validateVolumes(c.Volumes, nil)
}

func slicesContains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	var in struct {
		Slug           string          `json:"slug"`
		Name           string          `json:"name"`
		Kind           string          `json:"kind"`
		Runtime        string          `json:"runtime"`
		RuntimeVersion string          `json:"runtime_version"`
		Config         json.RawMessage `json:"config"`
		// Source builds the image from a Git repository (image apps; needs the built-in registry).
		Source *sourceInput `json:"source"`
		// Build (with source) starts the first build right away and deploys it; false = later (the wizard).
		Build *bool `json:"build"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be 1–30 lowercase letters, digits or dashes")
		return
	}
	if n := len(strings.TrimSpace(in.Name)); n == 0 || n > 100 {
		writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
		return
	}
	rt, ok := s.catalog.Get(in.Runtime)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown runtime "+in.Runtime)
		return
	}
	if in.Kind == "" {
		in.Kind = runtimes.KindWeb
	}
	if !rt.Supports(in.Kind) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s does not support %s apps", rt.Name, in.Kind))
		return
	}
	if rt.Artifact.Kind != runtimes.ArtifactNone {
		v, ok := rt.Version(in.RuntimeVersion)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown version "+in.RuntimeVersion)
			return
		}
		in.RuntimeVersion = v.ID
	} else {
		in.RuntimeVersion = ""
	}
	cfg := defaultConfig(rt, in.Kind)
	if len(in.Config) > 0 && string(in.Config) != "null" {
		if err := json.Unmarshal(in.Config, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}
	}
	var src store.AppSource
	if in.Source != nil {
		if rt.Artifact.Kind != runtimes.ArtifactNone {
			writeError(w, http.StatusBadRequest, "only apps of the Docker image type can be built from Git")
			return
		}
		if !s.needBuilds(w) {
			return
		}
		if !p.eff.Can("app.deploy", &proj.ID) {
			writeError(w, http.StatusForbidden, "missing permission: app.deploy (the first build deploys)")
			return
		}
		src.AutoDeploy = true
		if msg, code := s.applySource(r.Context(), *in.Source, &src); code != 0 {
			writeError(w, code, msg)
			return
		}
		cfg.Image = s.registry.PullRef(proj.Namespace+"/"+in.Slug, pendingTag) // replaced by the first build
	}
	if msg := validateConfig(rt, in.Kind, cfg); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if taken, err := s.store.SlugTaken(r.Context(), proj.ID, in.Slug); err == nil && taken {
		writeError(w, http.StatusConflict, "an app or add-on with this slug already exists in the project")
		return
	}
	if proj.Quota != nil && proj.Quota.Apps > 0 && int64(proj.Apps) >= proj.Quota.Apps {
		writeError(w, http.StatusConflict, fmt.Sprintf("project quota allows %d apps", proj.Quota.Apps))
		return
	}
	app, err := s.store.CreateApp(r.Context(), store.App{ProjectID: proj.ID, Slug: in.Slug, Name: in.Name, Kind: in.Kind,
		RuntimeID: rt.ID, RuntimeVersion: in.RuntimeVersion, Config: cfg})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "an app with this slug already exists in the project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.addDefaultDomain(r.Context(), app, proj)
	s.audit(r, p, "app.create", proj.Slug+"/"+app.Slug, map[string]string{"runtime": rt.ID, "kind": in.Kind})
	if in.Source == nil {
		writeJSON(w, http.StatusCreated, app)
		return
	}
	src.AppID = app.ID
	if _, err := s.store.SaveAppSource(r.Context(), src); err != nil {
		s.serverError(w, r, err)
		return
	}
	out := map[string]any{"app": app}
	if in.Build != nil && !*in.Build {
		writeJSON(w, http.StatusCreated, out)
		return
	}
	b, err := s.builder.Start(r.Context(), build.Request{App: app, Project: proj, Trigger: "manual", Deploy: true, By: &p.user.ID})
	if err != nil {
		out["warning"] = "the first build did not start: " + err.Error()
	} else {
		out["build"] = b
	}
	writeJSON(w, http.StatusCreated, out)
}

// pendingTag marks the image of a Git app before its first build.
const pendingTag = "pending"

func (s *Server) getApp(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in struct {
		Name           *string         `json:"name"`
		RuntimeVersion *string         `json:"runtime_version"`
		Config         json.RawMessage `json:"config"`
	}
	if !decode(w, r, &in) {
		return
	}
	rt, _ := s.catalog.Get(app.RuntimeID)
	oldVolumes := slices.Clone(app.Config.Volumes)
	if in.Name != nil {
		if n := len(strings.TrimSpace(*in.Name)); n == 0 || n > 100 {
			writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
			return
		}
		app.Name = *in.Name
	}
	if in.RuntimeVersion != nil && rt.Artifact.Kind != runtimes.ArtifactNone {
		v, ok := rt.Version(*in.RuntimeVersion)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown version "+*in.RuntimeVersion)
			return
		}
		app.RuntimeVersion = v.ID
	}
	if len(in.Config) > 0 && string(in.Config) != "null" {
		if err := json.Unmarshal(in.Config, &app.Config); err != nil { // merges onto the current config
			writeError(w, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}
	}
	if msg := validateConfig(rt, app.Kind, app.Config); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	volumesChanged := !slices.Equal(oldVolumes, app.Config.Volumes)
	if volumesChanged {
		if msg := validateVolumes(app.Config.Volumes, oldVolumes); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		if !p.eff.Can("volume.manage", &app.ProjectID) {
			writeError(w, http.StatusForbidden, "missing permission: volume.manage")
			return
		}
	}
	app, err := s.store.SaveApp(r.Context(), app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var notes []string
	if volumesChanged {
		notes = s.resizeVolumes(r, app, proj, oldVolumes)
	}
	s.audit(r, p, "app.update", proj.Slug+"/"+app.Slug, nil)
	if len(notes) > 0 {
		w.Header().Set("X-Nectar-Notice", strings.Join(notes, "; "))
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if s.deployer != nil && s.deployer.Busy(app.ID) {
		writeError(w, http.StatusConflict, deploy.ErrBusy.Error())
		return
	}
	// its preview environments go with it
	if previews, err := s.store.ListPreviews(r.Context(), app.ID); err == nil {
		for _, pv := range previews {
			if pa, err := s.store.GetApp(r.Context(), pv.AppID); err == nil {
				if err := s.removeApp(r.Context(), pa, proj); err != nil {
					writeError(w, http.StatusBadGateway, "could not remove the preview "+pa.Slug+": "+err.Error())
					return
				}
			}
		}
	}
	if err := s.removeApp(r.Context(), app, proj); errors.Is(err, errCluster) {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.delete", proj.Slug+"/"+app.Slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

var errCluster = errors.New("could not remove the app from the cluster")

// removeApp deletes an app everywhere: cluster objects, database rows, uploads, logs, gateway routes and
// (for apps built from Git) its images in the built-in registry.
func (s *Server) removeApp(ctx context.Context, app store.App, proj store.Project) error {
	if s.deployer != nil {
		if s.deployer.Busy(app.ID) {
			return deploy.ErrBusy
		}
		if err := s.deployer.DeleteApp(ctx, app, proj); err != nil {
			return fmt.Errorf("%w: %v", errCluster, err)
		}
	}
	_, srcErr := s.store.GetAppSource(ctx, app.ID)
	arts, _ := s.store.ListArtifacts(ctx, app.ID, 1000)
	if err := s.store.DeleteApp(ctx, app.ID); err != nil {
		return err
	}
	for _, a := range arts {
		if used, err := s.store.ArtifactSHAExists(ctx, a.SHA256); err == nil && !used {
			_ = s.artifacts.Remove(a.SHA256)
		}
	}
	_ = s.store.DeletePlansFor(ctx, "app", app.ID)
	if s.logs != nil {
		_ = s.logs.DeleteApp(app.ID)
	}
	if srcErr == nil && s.builder != nil && s.builder.Registry.Enabled() {
		go func() { // build images of the app
			pctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := s.builder.Registry.Prune(pctx, build.Repo(proj, app), 0, nil); err != nil {
				s.log.Warn("registry prune", "app", app.Slug, "err", err)
			}
		}()
	}
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second) // its domains are gone (cascade) → remove the gateway routes
	defer cancel()
	if err := s.syncer.Sync(sctx); err != nil {
		s.log.Warn("route sync", "err", err)
	}
	return nil
}

func (s *Server) appStatus(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	live, err := s.deployer.Status(r.Context(), app, proj)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	pending, err := s.deployer.Pending(r.Context(), app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": app.Status, "message": app.StatusMessage, "live": live, "pending": pending})
}

// --------------------------------------------------------------------- env --

type envItem struct {
	Key    string  `json:"key"`
	Value  *string `json:"value,omitempty"`
	Secret bool    `json:"secret"`
}

func (s *Server) getEnv(w http.ResponseWriter, r *http.Request, p *principal, app store.App, _ store.Project) {
	vars, err := s.store.ListEnv(r.Context(), app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	reveal := r.URL.Query().Get("reveal") == "1"
	if reveal && !p.eff.Can("env.secrets.view", &app.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.secrets.view")
		return
	}
	out := make([]envItem, 0, len(vars))
	for _, v := range vars {
		it := envItem{Key: v.Key, Secret: v.IsSecret}
		if !v.IsSecret || reveal {
			pt, err := s.envBox.Open(v.ValueEnc)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			it.Value = &pt
		}
		out = append(out, it)
	}
	if reveal {
		s.audit(r, p, "env.reveal", strconv.FormatInt(app.ID, 10), nil)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) putEnv(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in []envItem
	if !decode(w, r, &in) {
		return
	}
	if len(in) > 200 {
		writeError(w, http.StatusBadRequest, "at most 200 variables")
		return
	}
	current, err := s.store.ListEnv(r.Context(), app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	existing := map[string]store.EnvVar{}
	for _, v := range current {
		existing[v.Key] = v
	}
	seen := map[string]bool{}
	var vars []store.EnvVar
	for _, it := range in {
		it.Key = strings.TrimSpace(it.Key)
		if !deploy.ValidEnvKey(it.Key) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid variable name %q", it.Key))
			return
		}
		if seen[it.Key] {
			writeError(w, http.StatusBadRequest, "duplicate variable "+it.Key)
			return
		}
		seen[it.Key] = true
		var enc string
		switch {
		case it.Value != nil:
			if len(*it.Value) > 64<<10 {
				writeError(w, http.StatusBadRequest, it.Key+": value too long (max 64 KiB)")
				return
			}
			if enc, err = s.envBox.Seal(*it.Value); err != nil {
				s.serverError(w, r, err)
				return
			}
		case existing[it.Key].ValueEnc != "":
			enc = existing[it.Key].ValueEnc // value omitted → keep (secrets are never sent back)
		default:
			writeError(w, http.StatusBadRequest, it.Key+": value is required")
			return
		}
		vars = append(vars, store.EnvVar{Key: it.Key, ValueEnc: enc, IsSecret: it.Secret})
	}
	if err := s.store.ReplaceEnv(r.Context(), app.ID, vars); err != nil {
		s.serverError(w, r, err)
		return
	}
	keys := make([]string, 0, len(vars))
	for _, v := range vars {
		keys = append(keys, v.Key)
	}
	s.audit(r, p, "env.update", proj.Slug+"/"+app.Slug, map[string]any{"keys": keys}) // never values
	w.WriteHeader(http.StatusNoContent)
}

// --------------------------------------------------------------- artifacts --

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	arts, err := s.store.ListArtifacts(r.Context(), app.ID, 50)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, arts)
}

func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return "artifact"
	}
	if len(name) > 200 {
		name = name[len(name)-200:]
	}
	return name
}

// uploadArtifact accepts the file as the raw request body (curl --data-binary @app.jar).
func (s *Server) uploadArtifact(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	rt, _ := s.catalog.Get(app.RuntimeID)
	if rt.Artifact.Kind == runtimes.ArtifactNone {
		writeError(w, http.StatusBadRequest, "this app runs a container image; it does not take uploads")
		return
	}
	if r.ContentLength > s.cfg.MaxArtifactBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is larger than the limit (%d MiB)", s.cfg.MaxArtifactBytes>>20))
		return
	}
	tmp, sha, size, err := s.artifacts.Receive(r.Body, s.cfg.MaxArtifactBytes)
	if errors.Is(err, artifacts.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is larger than the limit (%d MiB)", s.cfg.MaxArtifactBytes>>20))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload failed: "+err.Error())
		return
	}
	s.finishArtifact(w, r, p, app, proj, tmp, sha, size, cleanFilename(r.URL.Query().Get("filename")), http.StatusCreated)
}

// finishArtifact inspects, stores and records a received file.
func (s *Server) finishArtifact(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project, tmp, sha string, size int64, filename string, status int) {
	if size == 0 {
		os.Remove(tmp)
		writeError(w, http.StatusBadRequest, "the file is empty")
		return
	}
	res, err := detect.File(tmp, filename)
	if errors.Is(err, detect.ErrUnsafeArchive) {
		os.Remove(tmp)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		res.Warnings = append(res.Warnings, err.Error())
	}
	rt, _ := s.catalog.Get(app.RuntimeID)
	if res.Runtime != "" && res.Runtime != rt.ID {
		res.Warnings = append(res.Warnings, fmt.Sprintf("this looks like a %s artifact, but the app uses %s", res.Runtime, rt.Name))
	}
	if rt.Artifact.Kind == runtimes.ArtifactArchive && res.Archive == nil {
		os.Remove(tmp)
		writeError(w, http.StatusBadRequest, rt.Name+" apps need a .zip or .tar.gz archive")
		return
	}
	if err := s.artifacts.Commit(tmp, sha); err != nil {
		os.Remove(tmp)
		s.serverError(w, r, err)
		return
	}
	art, err := s.store.CreateArtifact(r.Context(), app.ID, sha, filename, size, res, &p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "artifact.upload", proj.Slug+"/"+app.Slug, map[string]any{"sha256": sha, "size": size, "filename": filename})
	w.Header().Set("Nectar-Artifact-Id", strconv.FormatInt(art.ID, 10))
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, art)
}

// internalArtifact serves artifact files to app pods (init container) via signed URLs.
func (s *Server) internalArtifact(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha")
	if !artifacts.ValidSHA(sha) || !s.verifyArtifact(sha, r.URL.Query().Get("sig")) {
		writeError(w, http.StatusForbidden, "invalid signature")
		return
	}
	if used, err := s.store.ArtifactSHAExists(r.Context(), sha); err != nil || !used || !s.artifacts.Exists(sha) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	f, err := os.Open(s.artifacts.Path(sha))
	if err != nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, sha, st.ModTime(), f)
}

// ------------------------------------------------------- resumable (tus) --

const (
	tusVersion = "1.0.0"
	uploadTTL  = 24 * time.Hour // unfinished uploads are discarded after a day
)

func (s *Server) tusOptions(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Tus-Resumable", tusVersion)
	h.Set("Tus-Version", tusVersion)
	h.Set("Tus-Extension", "creation,termination")
	h.Set("Tus-Max-Size", strconv.FormatInt(s.cfg.MaxArtifactBytes, 10))
	w.WriteHeader(http.StatusNoContent)
}

func tusFilename(meta string) string {
	for _, pair := range strings.Split(meta, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(pair), " ")
		if k == "filename" {
			if b, err := base64.StdEncoding.DecodeString(v); err == nil {
				return string(b)
			}
		}
	}
	return ""
}

func (s *Server) tusCreate(w http.ResponseWriter, r *http.Request, p *principal, app store.App, _ store.Project) {
	w.Header().Set("Tus-Resumable", tusVersion)
	rt, _ := s.catalog.Get(app.RuntimeID)
	if rt.Artifact.Kind == runtimes.ArtifactNone {
		writeError(w, http.StatusBadRequest, "this app runs a container image; it does not take uploads")
		return
	}
	size, err := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
	if err != nil || size <= 0 {
		writeError(w, http.StatusBadRequest, "Upload-Length header is required")
		return
	}
	if size > s.cfg.MaxArtifactBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is larger than the limit (%d MiB)", s.cfg.MaxArtifactBytes>>20))
		return
	}
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	if err := s.artifacts.StartUpload(id); err != nil {
		s.serverError(w, r, err)
		return
	}
	u := store.Upload{ID: id, AppID: app.ID, Filename: cleanFilename(tusFilename(r.Header.Get("Upload-Metadata"))), Size: size, CreatedBy: &p.user.ID}
	if err := s.store.CreateUpload(r.Context(), u, uploadTTL); err != nil {
		_ = s.artifacts.RemoveUpload(id)
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/uploads/"+id)
	w.WriteHeader(http.StatusCreated)
}

// tusUpload loads {uid} and checks that the caller may deploy the app it belongs to.
func (s *Server) tusUpload(w http.ResponseWriter, r *http.Request, p *principal) (store.Upload, store.App, store.Project, bool) {
	w.Header().Set("Tus-Resumable", tusVersion)
	w.Header().Set("Cache-Control", "no-store")
	u, err := s.store.GetUpload(r.Context(), r.PathValue("uid"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "upload not found or expired")
		return u, store.App{}, store.Project{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return u, store.App{}, store.Project{}, false
	}
	app, proj, ok := s.loadApp(w, r, p, u.AppID, "app.deploy")
	return u, app, proj, ok
}

func (s *Server) tusHead(w http.ResponseWriter, r *http.Request, p *principal) {
	u, _, _, ok := s.tusUpload(w, r, p)
	if !ok {
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(u.Offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(u.Size, 10))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) tusPatch(w http.ResponseWriter, r *http.Request, p *principal) {
	u, app, proj, ok := s.tusUpload(w, r, p)
	if !ok {
		return
	}
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/offset+octet-stream")
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset != u.Offset {
		w.Header().Set("Upload-Offset", strconv.FormatInt(u.Offset, 10))
		writeError(w, http.StatusConflict, "Upload-Offset does not match the server offset")
		return
	}
	n, err := s.artifacts.AppendUpload(u.ID, offset, r.Body, u.Size-offset)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.SetUploadOffset(r.Context(), u.ID, offset, offset+n); err != nil {
		writeError(w, http.StatusConflict, "concurrent upload request")
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(offset+n, 10))
	if offset+n < u.Size {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// complete: hash, inspect, store
	tmp, sha, err := s.artifacts.FinishUpload(u.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.DeleteUpload(r.Context(), u.ID)
	s.finishArtifact(w, r, p, app, proj, tmp, sha, u.Size, u.Filename, http.StatusNoContent)
}

func (s *Server) tusDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	u, _, _, ok := s.tusUpload(w, r, p)
	if !ok {
		return
	}
	_ = s.artifacts.RemoveUpload(u.ID)
	_ = s.store.DeleteUpload(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// ----------------------------------------------------------------- deploys --

func (s *Server) deployErr(w http.ResponseWriter, err error) {
	var ce *deploy.CapacityError
	switch {
	case errors.As(err, &ce):
		writeJSON(w, http.StatusConflict, map[string]any{"error": ce.Error(), "capacity": true, "hint": "free resources, lower the app's resources, or deploy with force=true"})
	case errors.Is(err, deploy.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) deployApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in struct {
		ArtifactID *int64 `json:"artifact_id"`
		Force      bool   `json:"force"`
		// Image switches the image of an image app before deploying (CI: deploy the tag just built).
		Image *string `json:"image"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	if in.Image == nil && strings.HasSuffix(app.Config.Image, ":"+pendingTag) && s.registry.Enabled() && s.registry.Owns(app.Config.Image) {
		writeError(w, http.StatusConflict, "this app has no image yet: start a build first")
		return
	}
	if in.Image != nil {
		img := strings.TrimSpace(*in.Image)
		if rt, _ := s.catalog.Get(app.RuntimeID); rt.Artifact.Kind != runtimes.ArtifactNone {
			writeError(w, http.StatusBadRequest, "image is only for apps of the Docker image type; upload an artifact instead")
			return
		}
		if img == "" || strings.ContainsAny(img, " \t\n") || len(img) > 500 {
			writeError(w, http.StatusBadRequest, "image must be an image reference like ghcr.io/acme/api:1.4.2")
			return
		}
		if img != app.Config.Image {
			app.Config.Image = img
			var err error
			if app, err = s.store.SaveApp(r.Context(), app); err != nil {
				s.serverError(w, r, err)
				return
			}
		}
	}
	rel, err := s.deployer.Deploy(r.Context(), deploy.DeployRequest{App: app, ArtifactID: in.ArtifactID, By: &p.user.ID, Force: in.Force})
	if err != nil {
		s.deployErr(w, err)
		return
	}
	s.audit(r, p, "app.deploy", proj.Slug+"/"+app.Slug, map[string]any{"release": rel.Number, "artifact_id": rel.ArtifactID, "force": in.Force, "image": in.Image})
	writeJSON(w, http.StatusAccepted, rel)
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	rels, err := s.store.ListReleases(r.Context(), app.ID, 50)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad release number")
		return
	}
	target, err := s.store.GetReleaseByNumber(r.Context(), app.ID, n)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	cfg := rollbackConfig(app.Config, target.Config)
	rel, err := s.deployer.Deploy(r.Context(), deploy.DeployRequest{App: app, ArtifactID: target.ArtifactID, By: &p.user.ID,
		Config: &cfg, Image: target.Image, Force: true})
	if err != nil {
		s.deployErr(w, err)
		return
	}
	app.Config = cfg // future deploys continue from the rolled-back config
	_, _ = s.store.SaveApp(r.Context(), app)
	s.audit(r, p, "app.rollback", proj.Slug+"/"+app.Slug, map[string]int{"to": n, "release": rel.Number})
	writeJSON(w, http.StatusAccepted, rel)
}

// rollbackConfig is the target release's config with today's scale and volumes: a rollback restores
// code and settings, it must not shrink a scaled-up app or unmount a volume added since that release.
func rollbackConfig(current, target store.AppConfig) store.AppConfig {
	cfg := target
	cfg.Replicas, cfg.ResumeReplicas = current.Replicas, current.ResumeReplicas
	cfg.Volumes = current.Volumes
	return cfg
}

func (s *Server) runTask(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in struct {
		Command []string `json:"command"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	if app.Kind != runtimes.KindTask && len(in.Command) == 0 {
		writeError(w, http.StatusBadRequest, "command is required for non-task apps")
		return
	}
	if len(in.Command) > 50 {
		writeError(w, http.StatusBadRequest, "command too long")
		return
	}
	job, err := s.deployer.RunTask(r.Context(), app, in.Command)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, p, "app.run", proj.Slug+"/"+app.Slug, map[string]any{"job": job, "command": in.Command})
	writeJSON(w, http.StatusAccepted, map[string]string{"job": job})
}

// verifyArtifact accepts URLs signed with the artifact key and, from before it existed, the master key.
func (s *Server) verifyArtifact(sha, sig string) bool {
	if s.artKey != nil && deploy.VerifyArtifact(s.artKey, sha, sig) {
		return true
	}
	return deploy.VerifyArtifact(s.cfg.MasterKey, sha, sig)
}
