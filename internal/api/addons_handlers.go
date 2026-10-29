package api

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

type addonHandler func(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project)

// onAddon loads add-on {aid} and requires perm in its project.
func (s *Server) onAddon(perm string, h addonHandler) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		id, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad add-on id")
			return
		}
		a, err := s.store.GetAddon(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "add-on not found")
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if !p.eff.Can(perm, &a.ProjectID) {
			if p.eff.Can("app.view", &a.ProjectID) {
				writeError(w, http.StatusForbidden, "missing permission: "+perm)
			} else {
				writeError(w, http.StatusNotFound, "add-on not found")
			}
			return
		}
		proj, err := s.store.GetProject(r.Context(), a.ProjectID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		h(w, r, p, a, proj)
	})
}

// addonView is an add-on with its catalog info, live state and links.
type addonView struct {
	store.Addon
	KindName string            `json:"kind_name"`
	Port     int               `json:"port"`
	Host     string            `json:"host"` // reachable from apps of the same project
	Live     *deploy.AddonLive `json:"live,omitempty"`
	Links    []store.AddonLink `json:"links"`
	EnvNames []string          `json:"env_names"` // variables a link without prefix provides
}

func (s *Server) addonView(r *http.Request, a store.Addon, proj store.Project, live bool) addonView {
	k, _ := deploy.AddonKindByID(a.Kind)
	v := addonView{Addon: a, KindName: k.Name, Port: k.Port, Host: a.Slug, Links: []store.AddonLink{}}
	if links, err := s.store.AddonLinks(r.Context(), a.ID, 0); err == nil {
		v.Links = links
	}
	for name := range deploy.AddonEnv(a, deploy.AddonCredentials{}, "") {
		v.EnvNames = append(v.EnvNames, name)
	}
	slices.Sort(v.EnvNames)
	if live && s.deployer != nil {
		if l, err := s.deployer.AddonStatus(r.Context(), a, proj); err == nil {
			v.Live = &l
		}
	}
	return v
}

func (s *Server) addonCatalog(w http.ResponseWriter, _ *http.Request, _ *principal) {
	writeJSON(w, http.StatusOK, deploy.AddonKinds)
}

func (s *Server) listAddons(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	list, err := s.store.ListAddons(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := make([]addonView, 0, len(list))
	for _, a := range list {
		out = append(out, s.addonView(r, a, proj, true))
	}
	writeJSON(w, http.StatusOK, out)
}

func validAddonResources(cpu, mem, size int64, k deploy.AddonKind) string {
	switch {
	case cpu < 50 || cpu > 16000:
		return "CPU must be 50–16000 millicores"
	case mem < k.MinMemoryMB || mem > 65536:
		return "memory must be at least " + strconv.FormatInt(k.MinMemoryMB, 10) + " MiB for " + k.Name
	case size < 256 || size > 1<<20:
		return "storage must be 256 MiB – 1 TiB"
	}
	return ""
}

func (s *Server) createAddon(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	var in struct {
		Kind          string `json:"kind"`
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		Version       string `json:"version"`
		SizeMB        int64  `json:"size_mb"`
		CPUMillicores int64  `json:"cpu_millicores"`
		MemoryMB      int64  `json:"memory_mb"`
	}
	if !decode(w, r, &in) {
		return
	}
	k, ok := deploy.AddonKindByID(in.Kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "kind must be postgres, mysql, mariadb or redis")
		return
	}
	v, ok := k.Version(in.Version)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown "+k.Name+" version "+in.Version)
		return
	}
	if in.Slug == "" {
		in.Slug = k.ID
	}
	if !slugRe.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be 1–30 lowercase letters, digits or dashes")
		return
	}
	if in.Name = strings.TrimSpace(in.Name); in.Name == "" {
		in.Name = k.Name
	}
	if in.SizeMB == 0 {
		in.SizeMB = 2048
	}
	if in.CPUMillicores == 0 {
		in.CPUMillicores = 250
	}
	if in.MemoryMB == 0 {
		in.MemoryMB = max(256, k.MinMemoryMB)
	}
	if msg := validAddonResources(in.CPUMillicores, in.MemoryMB, in.SizeMB, k); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if taken, err := s.store.SlugTaken(r.Context(), proj.ID, in.Slug); err != nil {
		s.serverError(w, r, err)
		return
	} else if taken {
		writeError(w, http.StatusConflict, "an app or add-on named "+in.Slug+" already exists in the project")
		return
	}
	if !s.needKube(w) {
		return
	}
	sealed, err := s.deployer.SealCredentials(deploy.NewAddonCredentials(k.ID, in.Slug))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	a, err := s.store.CreateAddon(r.Context(), store.Addon{ProjectID: proj.ID, Slug: in.Slug, Name: in.Name, Kind: k.ID, Version: v.ID,
		SizeMB: in.SizeMB, CPUMillicores: in.CPUMillicores, MemoryMB: in.MemoryMB, Credentials: sealed})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "an add-on named "+in.Slug+" already exists in the project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.deployer.ApplyAddon(r.Context(), a); err != nil {
		_ = s.store.SetAddonStatus(r.Context(), a.ID, deploy.StatusFailed, err.Error())
		writeError(w, http.StatusBadGateway, "the add-on was saved but could not be started: "+err.Error())
		return
	}
	a, _ = s.store.GetAddon(r.Context(), a.ID)
	s.audit(r, p, "addon.create", proj.Slug+"/"+a.Slug, map[string]string{"kind": a.Kind, "version": a.Version})
	writeJSON(w, http.StatusCreated, s.addonView(r, a, proj, false))
}

func (s *Server) getAddon(w http.ResponseWriter, r *http.Request, _ *principal, a store.Addon, proj store.Project) {
	writeJSON(w, http.StatusOK, s.addonView(r, a, proj, true))
}

// updateAddon changes name, resources and the volume size (grow only; applied to the PVC when the storage
// class supports it).
func (s *Server) updateAddon(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	var in struct {
		Name          *string `json:"name"`
		SizeMB        *int64  `json:"size_mb"`
		CPUMillicores *int64  `json:"cpu_millicores"`
		MemoryMB      *int64  `json:"memory_mb"`
	}
	if !decode(w, r, &in) {
		return
	}
	k, _ := deploy.AddonKindByID(a.Kind)
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		a.Name = strings.TrimSpace(*in.Name)
	}
	if in.CPUMillicores != nil {
		a.CPUMillicores = *in.CPUMillicores
	}
	if in.MemoryMB != nil {
		a.MemoryMB = *in.MemoryMB
	}
	grow := false
	if in.SizeMB != nil {
		if *in.SizeMB < a.SizeMB {
			writeError(w, http.StatusBadRequest, "storage can only grow")
			return
		}
		grow = *in.SizeMB > a.SizeMB
		a.SizeMB = *in.SizeMB
	}
	if msg := validAddonResources(a.CPUMillicores, a.MemoryMB, a.SizeMB, k); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.needKube(w) {
		return
	}
	a, err := s.store.SaveAddon(r.Context(), a)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.deployer.ApplyAddon(r.Context(), a); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if grow {
		if ok, _ := s.deployer.ResizeClaim(r.Context(), proj.Namespace, "data-"+a.Slug+"-0", a.SizeMB); !ok {
			w.Header().Set("X-Nectar-Notice", "the new size is recorded; this storage class cannot grow existing volumes")
		}
	}
	s.audit(r, p, "addon.update", proj.Slug+"/"+a.Slug, nil)
	writeJSON(w, http.StatusOK, s.addonView(r, a, proj, false))
}

func (s *Server) setAddonStopped(stopped bool) addonHandler {
	return func(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
		if !s.needKube(w) {
			return
		}
		a.Stopped = stopped
		a, err := s.store.SaveAddon(r.Context(), a)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if err := s.deployer.ApplyAddon(r.Context(), a); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		action := "addon.start"
		if stopped {
			action = "addon.stop"
		}
		s.audit(r, p, action, proj.Slug+"/"+a.Slug, nil)
		a, _ = s.store.GetAddon(r.Context(), a.ID)
		writeJSON(w, http.StatusOK, s.addonView(r, a, proj, false))
	}
}

func (s *Server) deleteAddon(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	if s.deployer != nil {
		if err := s.deployer.DeleteAddon(r.Context(), a, proj); err != nil {
			writeError(w, http.StatusBadGateway, "could not remove the add-on from the cluster: "+err.Error())
			return
		}
	}
	if err := s.store.DeleteAddon(r.Context(), a.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.DeletePlansFor(r.Context(), "addon", a.ID)
	s.audit(r, p, "addon.delete", proj.Slug+"/"+a.Slug, map[string]string{"kind": a.Kind})
	w.WriteHeader(http.StatusNoContent)
}

// addonCredentials reveals the connection settings (audited).
func (s *Server) addonCredentials(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	if s.deployer == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	c, err := s.deployer.AddonCredentials(a)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	k, _ := deploy.AddonKindByID(a.Kind)
	s.audit(r, p, "addon.credentials", proj.Slug+"/"+a.Slug, nil)
	writeJSON(w, http.StatusOK, map[string]any{"host": a.Slug, "port": k.Port, "user": c.User, "password": c.Password, "database": c.Database,
		"env": deploy.AddonEnv(a, c, "")})
}

var envPrefixRe = regexp.MustCompile(`^([A-Z][A-Z0-9_]{0,30}_)?$`)

func (s *Server) linkAddon(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	var in struct {
		AppID  int64  `json:"app_id"`
		Prefix string `json:"prefix"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Prefix = strings.ToUpper(strings.TrimSpace(in.Prefix))
	if !envPrefixRe.MatchString(in.Prefix) {
		writeError(w, http.StatusBadRequest, "prefix must be uppercase letters, digits and _, ending with _ (e.g. CACHE_)")
		return
	}
	app, err := s.store.GetApp(r.Context(), in.AppID)
	if err != nil || app.ProjectID != a.ProjectID {
		writeError(w, http.StatusBadRequest, "the app must be in the same project")
		return
	}
	if !p.eff.Can("env.edit", &app.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.edit")
		return
	}
	if err := s.store.LinkAddon(r.Context(), a.ID, app.ID, in.Prefix); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "addon.link", proj.Slug+"/"+a.Slug, map[string]any{"app": app.Slug, "prefix": in.Prefix})
	writeJSON(w, http.StatusOK, s.addonView(r, a, proj, false))
}

func (s *Server) unlinkAddon(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	appID, err := strconv.ParseInt(r.PathValue("app"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad app id")
		return
	}
	if !p.eff.Can("env.edit", &a.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.edit")
		return
	}
	if err := s.store.UnlinkAddon(r.Context(), a.ID, appID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not linked")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "addon.unlink", proj.Slug+"/"+a.Slug, map[string]any{"app_id": appID})
	w.WriteHeader(http.StatusNoContent)
}

// appAddons lists the add-ons linked to an app with the variable names they provide.
func (s *Server) appAddons(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	links, err := s.store.AddonLinks(r.Context(), 0, app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	type item struct {
		Addon  addonView `json:"addon"`
		Prefix string    `json:"prefix"`
		Env    []string  `json:"env"`
	}
	out := []item{}
	for _, l := range links {
		a, err := s.store.GetAddon(r.Context(), l.AddonID)
		if err != nil {
			continue
		}
		var names []string
		for k := range deploy.AddonEnv(a, deploy.AddonCredentials{}, l.Prefix) {
			names = append(names, k)
		}
		slices.Sort(names)
		out = append(out, item{Addon: s.addonView(r, a, proj, false), Prefix: l.Prefix, Env: names})
	}
	writeJSON(w, http.StatusOK, out)
}
