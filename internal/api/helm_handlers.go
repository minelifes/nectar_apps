package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/helm"
	"nectar_apps/internal/store"
)

// Helm charts in a project:
//
//	GET    /api/projects/{pid}/helm                    releases with their live status (app.view)
//	POST   /api/projects/{pid}/helm                    install {name, repo, chart, version, values, repo_username, repo_password} (helm.manage)
//	GET    /api/projects/{pid}/helm/{name}             detail: history, workloads, services, last output; values with helm.manage
//	PUT    /api/projects/{pid}/helm/{name}             upgrade with a new version / values / chart reference (helm.manage)
//	POST   /api/projects/{pid}/helm/{name}/rollback    {revision} (helm.manage)
//	DELETE /api/projects/{pid}/helm/{name}             uninstall (helm.manage)
//
// Operations run in the background (internal/helm: a Job in the project namespace, namespace-only
// access); the answer is 202 and the status tells when it is done.

type helmInput struct {
	Name         string  `json:"name"`
	Repo         *string `json:"repo"`
	Chart        *string `json:"chart"`
	Version      *string `json:"version"`
	Values       *string `json:"values"`
	RepoUsername *string `json:"repo_username"`
	RepoPassword *string `json:"repo_password"` // "" keeps the saved one; null too
}

type helmView struct {
	store.HelmRelease
	Revision     int    `json:"revision"`      // Helm's current revision (0 = none yet)
	HelmStatus   string `json:"helm_status"`   // Helm's status of that revision
	ChartVersion string `json:"chart_version"` // installed chart version
	AppVersion   string `json:"app_version"`
	HasPassword  bool   `json:"has_password"`
}

func (s *Server) helmView(ctx context.Context, h store.HelmRelease, ns string) helmView {
	v := helmView{HelmRelease: h, HasPassword: h.RepoPassword != ""}
	v.Output = "" // in the detail only
	if s.kube != nil {
		if hist, err := helm.History(ctx, s.kube, ns, h.Name); err == nil && len(hist) > 0 {
			v.Revision, v.HelmStatus, v.ChartVersion, v.AppVersion = hist[0].Revision, hist[0].Status, hist[0].ChartVersion, hist[0].AppVersion
		}
	}
	return v
}

func (s *Server) listHelm(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	list, err := s.store.ListHelmReleases(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := make([]helmView, 0, len(list))
	for _, h := range list {
		out = append(out, s.helmView(r.Context(), h, proj.Namespace))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) helmRelease(w http.ResponseWriter, r *http.Request, proj store.Project) (store.HelmRelease, bool) {
	h, err := s.store.GetHelmReleaseByName(r.Context(), proj.ID, r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chart not found")
		return h, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return h, false
	}
	return h, true
}

func (s *Server) getHelm(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	h, ok := s.helmRelease(w, r, proj)
	if !ok {
		return
	}
	ctx := r.Context()
	out := map[string]any{"release": s.helmView(ctx, h, proj.Namespace), "output": h.Output}
	if p.eff.Can("helm.manage", &proj.ID) {
		vals, _ := s.envBox.Open(h.ValuesEnc)
		out["values"] = vals
	}
	if s.kube != nil {
		hist, _ := helm.History(ctx, s.kube, proj.Namespace, h.Name)
		if hist == nil {
			hist = []helm.Revision{}
		}
		out["history"] = hist
		out["workloads"] = helm.Workloads(ctx, s.kube, proj.Namespace, h.Name)
		out["services"] = helm.Services(ctx, s.kube, proj.Namespace, h.Name)
	}
	writeJSON(w, http.StatusOK, out)
}

// applyHelmInput fills h from in; returns an error message or "".
func (s *Server) applyHelmInput(in helmInput, h *store.HelmRelease, values *string) string {
	if in.Repo != nil {
		h.Repo = strings.TrimRight(strings.TrimSpace(*in.Repo), "/")
	}
	if in.Chart != nil {
		h.Chart = strings.TrimSpace(*in.Chart)
	}
	if in.Version != nil {
		h.Version = strings.TrimSpace(*in.Version)
	}
	if in.Values != nil {
		*values = *in.Values
	}
	if in.RepoUsername != nil {
		h.RepoUsername = strings.TrimSpace(*in.RepoUsername)
		if h.RepoUsername == "" {
			h.RepoPassword = ""
		}
	}
	if in.RepoPassword != nil && *in.RepoPassword != "" {
		enc, err := s.envBox.Seal(*in.RepoPassword)
		if err != nil {
			return err.Error()
		}
		h.RepoPassword = enc
	}
	if strings.ContainsAny(h.RepoUsername, "\n\r") {
		return "repository user: one line"
	}
	op := helm.Op{Kind: "install", Name: h.Name, Repo: h.Repo, Chart: h.Chart, Version: h.Version, Values: *values}
	if err := op.Check(); err != nil {
		return err.Error()
	}
	if err := checkYAML(*values); err != "" {
		return err
	}
	enc, err := s.envBox.Seal(*values)
	if err != nil {
		return err.Error()
	}
	h.ValuesEnc = enc
	return ""
}

func (s *Server) createHelm(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in helmInput
	if !decode(w, r, &in) {
		return
	}
	h := store.HelmRelease{ProjectID: proj.ID, Name: strings.TrimSpace(in.Name)}
	values := ""
	if msg := s.applyHelmInput(in, &h, &values); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	// a release of that name that Helm already has (installed outside the panel) would be taken over
	if hist, _ := helm.History(r.Context(), s.kube, proj.Namespace, h.Name); len(hist) > 0 {
		writeError(w, http.StatusConflict, "Helm already has a release "+h.Name+" in this project: choose another name")
		return
	}
	if err := s.deployer.EnsureProject(r.Context(), proj); err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	h, err := s.store.CreateHelmRelease(r.Context(), h, &p.user.ID)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a chart named "+in.Name+" is already installed in this project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "helm.install", proj.Slug+"/"+h.Name, map[string]any{"repo": h.Repo, "chart": h.Chart, "version": h.Version})
	s.runHelm(proj, h, helm.Op{Kind: "install"}, values)
	writeJSON(w, http.StatusAccepted, s.helmView(r.Context(), h, proj.Namespace))
}

func (s *Server) upgradeHelm(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	h, ok := s.helmRelease(w, r, proj)
	if !ok {
		return
	}
	var in helmInput
	if !decode(w, r, &in) {
		return
	}
	values, _ := s.envBox.Open(h.ValuesEnc)
	if msg := s.applyHelmInput(in, &h, &values); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if started, err := s.store.StartHelmOp(r.Context(), h.ID, store.HelmUpgrading); err != nil || !started {
		writeError(w, http.StatusConflict, "another operation of this chart is running")
		return
	}
	if err := s.store.UpdateHelmChart(r.Context(), h); err != nil {
		_ = s.store.FinishHelmOp(r.Context(), h.ID, store.HelmFailed, err.Error(), "")
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "helm.upgrade", proj.Slug+"/"+h.Name, map[string]any{"repo": h.Repo, "chart": h.Chart, "version": h.Version})
	h.Status = store.HelmUpgrading
	s.runHelm(proj, h, helm.Op{Kind: "install"}, values)
	writeJSON(w, http.StatusAccepted, s.helmView(r.Context(), h, proj.Namespace))
}

func (s *Server) rollbackHelm(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	h, ok := s.helmRelease(w, r, proj)
	if !ok {
		return
	}
	var in struct {
		Revision int `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	hist, _ := helm.History(r.Context(), s.kube, proj.Namespace, h.Name)
	found := false
	for _, rev := range hist {
		found = found || rev.Revision == in.Revision
	}
	if !found {
		writeError(w, http.StatusBadRequest, "no revision "+strconv.Itoa(in.Revision)+" in the history")
		return
	}
	if started, err := s.store.StartHelmOp(r.Context(), h.ID, store.HelmRollingBack); err != nil || !started {
		writeError(w, http.StatusConflict, "another operation of this chart is running")
		return
	}
	s.audit(r, p, "helm.rollback", proj.Slug+"/"+h.Name, map[string]any{"revision": in.Revision})
	h.Status = store.HelmRollingBack
	s.runHelm(proj, h, helm.Op{Kind: "rollback", Revision: in.Revision}, "")
	writeJSON(w, http.StatusAccepted, s.helmView(r.Context(), h, proj.Namespace))
}

func (s *Server) deleteHelm(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	h, ok := s.helmRelease(w, r, proj)
	if !ok {
		return
	}
	if store.HelmBusy(h.Status) {
		writeError(w, http.StatusConflict, "another operation of this chart is running")
		return
	}
	if hist, err := helm.History(r.Context(), s.kube, proj.Namespace, h.Name); err == nil && len(hist) == 0 {
		// Helm never got it (a first install that failed): nothing to uninstall
		if err := s.store.DeleteHelmRelease(r.Context(), h.ID); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.audit(r, p, "helm.uninstall", proj.Slug+"/"+h.Name, nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if started, err := s.store.StartHelmOp(r.Context(), h.ID, store.HelmUninstalling); err != nil || !started {
		writeError(w, http.StatusConflict, "another operation of this chart is running")
		return
	}
	s.audit(r, p, "helm.uninstall", proj.Slug+"/"+h.Name, nil)
	h.Status = store.HelmUninstalling
	s.runHelm(proj, h, helm.Op{Kind: "uninstall"}, "")
	writeJSON(w, http.StatusAccepted, s.helmView(r.Context(), h, proj.Namespace))
}

// runHelm runs an operation in the background and records its result. A release Helm never got (a
// failed first install) or an uninstalled one is removed from the panel.
func (s *Server) runHelm(proj store.Project, h store.HelmRelease, op helm.Op, values string) {
	op.Name, op.Repo, op.Chart, op.Version, op.Values, op.Username = h.Name, h.Repo, h.Chart, h.Version, values, h.RepoUsername
	if h.RepoPassword != "" {
		op.Password, _ = s.envBox.Open(h.RepoPassword)
	}
	runner := s.helmRunner
	if runner == nil {
		runner = &helm.Runner{Kube: s.kube}
	}
	s.helmWG.Add(1)
	go func() {
		defer s.helmWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		out, err := runner.Run(ctx, proj.Namespace, op)
		switch {
		case err == nil && op.Kind == "uninstall":
			_ = s.store.DeleteHelmRelease(ctx, h.ID)
		case err == nil:
			_ = s.store.FinishHelmOp(ctx, h.ID, store.HelmDeployed, "", out)
		default:
			_ = s.store.FinishHelmOp(ctx, h.ID, store.HelmFailed, err.Error(), out)
			s.log.Warn("helm operation failed", "project", proj.Slug, "release", h.Name, "op", op.Kind, "err", err)
		}
	}()
}

// checkYAML: values must be a YAML mapping (or empty). Only the shape is checked; Helm reads the rest.
func checkYAML(v string) string {
	t := strings.TrimSpace(v)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "{") {
		return ""
	}
	for _, line := range strings.Split(t, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || l == "---" {
			continue
		}
		if strings.HasPrefix(l, "- ") && !strings.HasPrefix(line, " ") {
			return "values must be a YAML mapping (key: value), not a list"
		}
		if !strings.Contains(l, ":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			return "values must be YAML key: value lines"
		}
		break
	}
	return ""
}
