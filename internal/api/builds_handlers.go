package api

import (
	"context"
	"crypto/hmac"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/build"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// ------------------------------------------------------------ source --

type sourceInput struct {
	RepoURL     string  `json:"repo_url"`
	Branch      string  `json:"branch"`
	ContextDir  string  `json:"context_dir"`
	Dockerfile  string  `json:"dockerfile"`
	Token       *string `json:"token"` // nil = keep, "" = remove (public repository)
	AutoDeploy  *bool   `json:"auto_deploy"`
	BuildCPU    int64   `json:"build_cpu_millicores"`
	BuildMemory int64   `json:"build_memory_mb"`
}

type sourceView struct {
	store.AppSource
	HasToken      bool   `json:"has_token"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret,omitempty"` // only for app.edit
}

func (s *Server) needBuilds(w http.ResponseWriter) bool {
	if s.builder == nil || !s.builder.Registry.Enabled() {
		writeError(w, http.StatusServiceUnavailable, registry.ErrDisabled.Error())
		return false
	}
	return true
}

// applySource validates the input onto src (and checks that the repository and branch are readable).
func (s *Server) applySource(ctx context.Context, in sourceInput, src *store.AppSource) (string, int) {
	repo, err := build.NormalizeRepo(in.RepoURL)
	if err != nil {
		return err.Error(), http.StatusBadRequest
	}
	src.RepoURL = repo
	src.Branch = strings.TrimSpace(in.Branch)
	if src.Branch == "" {
		src.Branch = "main"
	}
	if !build.ValidBranch(src.Branch) {
		return "invalid branch name", http.StatusBadRequest
	}
	src.ContextDir = strings.Trim(strings.TrimSpace(in.ContextDir), "/")
	src.Dockerfile = strings.TrimSpace(in.Dockerfile)
	if src.Dockerfile == "" {
		src.Dockerfile = "Dockerfile"
	}
	if !build.ValidPath(src.ContextDir) || !build.ValidPath(src.Dockerfile) {
		return "context_dir and dockerfile must be relative paths inside the repository", http.StatusBadRequest
	}
	if in.AutoDeploy != nil {
		src.AutoDeploy = *in.AutoDeploy
	}
	if in.BuildCPU != 0 {
		src.BuildCPU = in.BuildCPU
	}
	if in.BuildMemory != 0 {
		src.BuildMemory = in.BuildMemory
	}
	if src.BuildCPU == 0 {
		src.BuildCPU = 2000
	}
	if src.BuildMemory == 0 {
		src.BuildMemory = 2048
	}
	if src.BuildCPU < 250 || src.BuildCPU > 16000 || src.BuildMemory < 512 || src.BuildMemory > 32768 {
		return "build resources: 250–16000 millicores and 512–32768 MB", http.StatusBadRequest
	}
	token, _ := s.builder.Box.Open(src.Token)
	if in.Token != nil {
		token = strings.TrimSpace(*in.Token)
		if token == "" {
			src.Token = ""
		} else if src.Token, err = s.builder.Box.Seal(token); err != nil {
			return err.Error(), http.StatusInternalServerError
		}
	}
	if src.WebhookSecret == "" {
		plain, _ := auth.NewToken("")
		if src.WebhookSecret, err = s.builder.Box.Seal(plain); err != nil {
			return err.Error(), http.StatusInternalServerError
		}
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := s.builder.Remote.Head(rctx, src.RepoURL, src.Branch, token); err != nil {
		return err.Error(), http.StatusBadRequest
	}
	return "", 0
}

func (s *Server) sourceView(r *http.Request, p *principal, app store.App, src store.AppSource) sourceView {
	v := sourceView{AppSource: src, HasToken: src.Token != "",
		WebhookURL: s.panelURL(r.Context(), r) + "/api/hooks/git/" + strconv.FormatInt(app.ID, 10)}
	if p.eff.Can("app.edit", &app.ProjectID) {
		v.WebhookSecret, _ = s.builder.Box.Open(src.WebhookSecret)
	}
	return v
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request, p *principal, app store.App, _ store.Project) {
	if s.builder == nil {
		writeError(w, http.StatusNotFound, "this app is not built from Git")
		return
	}
	src, err := s.store.GetAppSource(r.Context(), app.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "this app is not built from Git")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.sourceView(r, p, app, src))
}

func (s *Server) putSource(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needBuilds(w) {
		return
	}
	if rt, _ := s.catalog.Get(app.RuntimeID); rt.Artifact.Kind != runtimes.ArtifactNone {
		writeError(w, http.StatusBadRequest, "only apps of the Docker image type can be built from Git")
		return
	}
	var in sourceInput
	if !decode(w, r, &in) {
		return
	}
	src, err := s.store.GetAppSource(r.Context(), app.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, r, err)
		return
	}
	isNew := errors.Is(err, store.ErrNotFound)
	if isNew {
		src = store.AppSource{AppID: app.ID, AutoDeploy: true}
	}
	if msg, code := s.applySource(r.Context(), in, &src); code != 0 {
		writeError(w, code, msg)
		return
	}
	if src, err = s.store.SaveAppSource(r.Context(), src); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.source", proj.Slug+"/"+app.Slug, map[string]any{"repo": src.RepoURL, "branch": src.Branch, "auto_deploy": src.AutoDeploy})
	writeJSON(w, http.StatusOK, s.sourceView(r, p, app, src))
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if err := s.store.DeleteAppSource(r.Context(), app.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.source.delete", proj.Slug+"/"+app.Slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

// rotateWebhookSecret issues a new webhook secret (the old one stops working).
func (s *Server) rotateWebhookSecret(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needBuilds(w) {
		return
	}
	src, err := s.store.GetAppSource(r.Context(), app.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "this app is not built from Git")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	plain, _ := auth.NewToken("")
	if src.WebhookSecret, err = s.builder.Box.Seal(plain); err != nil {
		s.serverError(w, r, err)
		return
	}
	if src, err = s.store.SaveAppSource(r.Context(), src); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.source.webhook_secret", proj.Slug+"/"+app.Slug, nil)
	writeJSON(w, http.StatusOK, s.sourceView(r, p, app, src))
}

// ------------------------------------------------------------- builds --

func (s *Server) listBuilds(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.store.ListBuilds(r.Context(), app.ID, limit)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) buildFromPath(w http.ResponseWriter, r *http.Request, app store.App) (store.Build, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		writeError(w, http.StatusNotFound, "build not found")
		return store.Build{}, false
	}
	b, err := s.store.GetBuildByNumber(r.Context(), app.ID, n)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "build not found")
		return b, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return b, false
	}
	return b, true
}

// getBuild returns a build with its output (live while it runs).
func (s *Server) getBuild(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	b, ok := s.buildFromPath(w, r, app)
	if !ok {
		return
	}
	logs := b.Log
	if s.builder != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		logs = s.builder.Logs(ctx, b)
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{"build": b, "log": logs})
}

func (s *Server) startBuild(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needBuilds(w) {
		return
	}
	in := struct {
		Deploy *bool `json:"deploy"`
	}{}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	trigger := "manual"
	if p.token != nil {
		trigger = "api"
	}
	b, err := s.builder.Start(r.Context(), build.Request{App: app, Project: proj, Trigger: trigger, Deploy: in.Deploy == nil || *in.Deploy, By: &p.user.ID})
	switch {
	case errors.Is(err, store.ErrBuildRunning):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil && b.ID == 0:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "app.build", proj.Slug+"/"+app.Slug, map[string]any{"build": b.Number, "commit": b.CommitSHA})
	writeJSON(w, http.StatusAccepted, b)
}

func (s *Server) cancelBuild(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	b, ok := s.buildFromPath(w, r, app)
	if !ok {
		return
	}
	if b.Status != store.BuildRunning || s.builder == nil {
		writeError(w, http.StatusConflict, "the build is not running")
		return
	}
	s.builder.Cancel(b.ID)
	s.audit(r, p, "app.build.cancel", proj.Slug+"/"+app.Slug, map[string]any{"build": b.Number})
	w.WriteHeader(http.StatusAccepted)
}

// gitWebhook receives push events (GitHub, GitLab, Gitea/Forgejo, or any sender with ?token=<secret>).
func (s *Server) gitWebhook(w http.ResponseWriter, r *http.Request) {
	if s.builder == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("app"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "body")
		return
	}
	ctx := r.Context()
	src, err := s.store.GetAppSource(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found") // do not reveal which apps exist
		return
	}
	secret, err := s.builder.Box.Open(src.WebhookSecret)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	push, err := build.ParsePush(r.Header, body, secret)
	if push.Provider == "generic" && !hmac.Equal([]byte(r.URL.Query().Get("token")), []byte(secret)) {
		err = build.ErrSignature
	}
	if errors.Is(err, build.ErrSignature) {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch {
	case push.Ping:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "event ignored (only pushes start builds)"})
		return
	case push.Deleted || push.Branch == "" || push.Branch != src.Branch:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "not the branch " + src.Branch})
		return
	case !src.AutoDeploy:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "automatic builds are off for this app"})
		return
	}
	app, err := s.store.GetApp(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	proj, err := s.store.GetProject(ctx, app.ProjectID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	b, err := s.builder.Start(context.WithoutCancel(ctx), build.Request{App: app, Project: proj, Trigger: "webhook", Commit: push.Commit, Message: push.Message, Deploy: true})
	if errors.Is(err, store.ErrBuildRunning) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = s.store.Audit(ctx, nil, "webhook:"+push.Provider, "app.build", proj.Slug+"/"+app.Slug, clientIP(r), map[string]any{"build": b.Number, "commit": push.Commit})
	writeJSON(w, http.StatusAccepted, map[string]any{"build": b.Number, "commit": b.CommitSHA})
}

// ------------------------------------------------------ registry logins --

func (s *Server) getRegistries(w http.ResponseWriter, r *http.Request, _ *principal) {
	list, err := s.store.ListRegistryCredentials(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	builtin := map[string]any{"enabled": s.registry.Enabled()}
	if s.registry.Enabled() {
		builtin["host"] = s.registry.PullHost()
	}
	writeJSON(w, http.StatusOK, map[string]any{"logins": list, "builtin": builtin})
}

func (s *Server) putRegistry(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	host := strings.ToLower(strings.TrimSpace(in.Host))
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host = strings.TrimRight(host, "/")
	if host == "index.docker.io" || host == "registry-1.docker.io" {
		host = "docker.io"
	}
	if host == "" || strings.ContainsAny(host, " /@") || len(host) > 200 {
		writeError(w, http.StatusBadRequest, "host must be a registry name like ghcr.io, docker.io or registry.example.com:5000")
		return
	}
	if strings.TrimSpace(in.Username) == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password (or access token) are required")
		return
	}
	sealed, err := s.registryBox.Seal(in.Password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.PutRegistryCredential(r.Context(), host, strings.TrimSpace(in.Username), sealed); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "registry.login", host, map[string]string{"username": in.Username})
	s.refreshPullSecrets(r)
	s.getRegistries(w, r, p)
}

func (s *Server) deleteRegistry(w http.ResponseWriter, r *http.Request, p *principal) {
	id, err := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.store.DeleteRegistryCredential(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "registry.logout", strconv.FormatInt(id, 10), nil)
	s.refreshPullSecrets(r)
	w.WriteHeader(http.StatusNoContent)
}

// refreshPullSecrets updates the pull Secret of every project now (deploys do it as well).
func (s *Server) refreshPullSecrets(r *http.Request) {
	if s.deployer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Minute)
	go func() {
		defer cancel()
		projects, err := s.store.ListProjects(ctx)
		if err != nil {
			return
		}
		for _, p := range projects {
			if err := s.deployer.EnsureProject(ctx, p); err != nil {
				s.log.Warn("pull secret", "project", p.Slug, "err", err)
			}
		}
	}()
}
