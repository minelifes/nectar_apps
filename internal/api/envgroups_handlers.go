package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

// Env groups: variables shared by several apps of a project (an app attaches groups by name; its own
// variables win). Environments: a project can be copied (staging ↔ production), and an app's live build
// promoted to the app of the same name in the other project.

var groupNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$`)

type groupVarView struct {
	Key    string  `json:"key"`
	Value  *string `json:"value,omitempty"`
	Secret bool    `json:"secret"`
}

type groupView struct {
	Name string         `json:"name"`
	Vars []groupVarView `json:"vars"`
	Apps []string       `json:"apps"`
}

// listEnvGroups: GET /api/projects/{pid}/env-groups — plain values with env.view, secrets only with reveal=1.
func (s *Server) listEnvGroups(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	gs, err := s.store.ListEnvGroups(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	canView := p.eff.Can("env.view", &proj.ID)
	reveal := canView && r.URL.Query().Get("reveal") == "1"
	out := []groupView{}
	for _, g := range gs {
		v := groupView{Name: g.Name, Apps: g.Apps, Vars: []groupVarView{}}
		for _, e := range g.Vars {
			gv := groupVarView{Key: e.Key, Secret: e.IsSecret}
			if canView && (!e.IsSecret || reveal) {
				if pt, err := s.envBox.Open(e.ValueEnc); err == nil {
					gv.Value = &pt
				}
			}
			v.Vars = append(v.Vars, gv)
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// putEnvGroup: PUT /api/projects/{pid}/env-groups/{name} [{key, value, secret}] — creates or replaces; an
// item without value keeps the current one.
func (s *Server) putEnvGroup(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	name := r.PathValue("name")
	if !groupNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "group name must be 1–30 lowercase letters, digits or dashes")
		return
	}
	var in []envItem
	if !decode(w, r, &in) {
		return
	}
	if len(in) > 200 {
		writeError(w, http.StatusBadRequest, "at most 200 variables")
		return
	}
	existing := map[string]store.EnvVar{}
	if g, err := s.store.GetEnvGroup(r.Context(), proj.ID, name); err == nil {
		for _, v := range g.Vars {
			existing[v.Key] = v
		}
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
			var err error
			if enc, err = s.envBox.Seal(*it.Value); err != nil {
				s.serverError(w, r, err)
				return
			}
		default:
			old, ok := existing[it.Key]
			if !ok {
				writeError(w, http.StatusBadRequest, it.Key+": a value is required")
				return
			}
			enc = old.ValueEnc
		}
		vars = append(vars, store.EnvVar{Key: it.Key, ValueEnc: enc, IsSecret: it.Secret})
	}
	g, err := s.store.SaveEnvGroup(r.Context(), proj.ID, name, vars)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	keys := make([]string, 0, len(vars))
	for _, v := range vars {
		keys = append(keys, v.Key)
	}
	s.audit(r, p, "env_group.save", proj.Slug+"/"+name, map[string]any{"keys": keys}) // never values
	writeJSON(w, http.StatusOK, map[string]any{"name": g.Name, "apps": g.Apps, "variables": len(vars)})
}

func (s *Server) deleteEnvGroup(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	name := r.PathValue("name")
	g, err := s.store.GetEnvGroup(r.Context(), proj.ID, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such group")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(g.Apps) > 0 {
		writeError(w, http.StatusConflict, "used by "+strings.Join(g.Apps, ", ")+" — detach it there first")
		return
	}
	if err := s.store.DeleteEnvGroup(r.Context(), proj.ID, name); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "env_group.delete", proj.Slug+"/"+name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// putAppEnvGroups: PUT /api/apps/{id}/env-groups ["shared", …] — the groups the app gets (in this order).
func (s *Server) putAppEnvGroups(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var names []string
	if !decode(w, r, &names) {
		return
	}
	gs, err := s.store.ListEnvGroups(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var clean []string
	for _, n := range names {
		if !slices.ContainsFunc(gs, func(g store.EnvGroup) bool { return g.Name == n }) {
			writeError(w, http.StatusBadRequest, "no env group "+n+" in this project")
			return
		}
		if !slices.Contains(clean, n) {
			clean = append(clean, n)
		}
	}
	if slices.Equal(clean, app.Config.EnvGroups) {
		writeJSON(w, http.StatusOK, app)
		return
	}
	app.Config.EnvGroups = clean
	app, err = s.store.SaveApp(r.Context(), app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.TouchEnv(r.Context(), app.ID) // not live until the next deploy
	s.audit(r, p, "app.env_groups", proj.Slug+"/"+app.Slug, map[string]any{"groups": clean})
	writeJSON(w, http.StatusOK, app)
}

// cloneApp copies an app (settings, variables, config files, env groups and database links of the same
// project) into a new app. Volumes are created empty; the copy is not deployed.
func (s *Server) cloneAppInto(r *http.Request, src store.App, srcProj, dst store.Project, slug, name string, addonMap map[int64]int64) (store.App, []string, error) {
	ctx := r.Context()
	var notes []string
	cfg := src.Config
	cfg.ResumeReplicas = 0
	if dst.ID != srcProj.ID && len(cfg.EnvGroups) > 0 {
		// groups are per project: keep those that exist in the target
		var kept []string
		for _, n := range cfg.EnvGroups {
			if _, err := s.store.GetEnvGroup(ctx, dst.ID, n); err == nil {
				kept = append(kept, n)
			} else {
				notes = append(notes, "env group "+n+" does not exist in "+dst.Slug)
			}
		}
		cfg.EnvGroups = kept
	}
	app, err := s.store.CreateApp(ctx, store.App{ProjectID: dst.ID, Slug: slug, Name: name, Kind: src.Kind, RuntimeID: src.RuntimeID,
		RuntimeVersion: src.RuntimeVersion, Config: cfg})
	if err != nil {
		return app, notes, err
	}
	if env, err := s.store.ListEnv(ctx, src.ID); err == nil && len(env) > 0 {
		if err := s.store.ReplaceEnv(ctx, app.ID, env); err != nil {
			return app, notes, err
		}
	}
	if files, err := s.store.ListFiles(ctx, src.ID); err == nil && len(files) > 0 {
		if err := s.store.ReplaceFiles(ctx, app.ID, files); err != nil {
			return app, notes, err
		}
	}
	if links, err := s.store.AddonLinks(ctx, 0, src.ID); err == nil {
		for _, l := range links {
			target := l.AddonID
			if addonMap != nil {
				var ok bool
				if target, ok = addonMap[l.AddonID]; !ok {
					continue
				}
			} else if dst.ID != srcProj.ID {
				notes = append(notes, "database links are not copied to another project")
				break
			}
			_ = s.store.LinkAddon(ctx, target, app.ID, l.Prefix)
		}
	}
	if src.Config.Image != "" && strings.Contains(src.Config.Image, ":pending") {
		notes = append(notes, "the app is built from Git: connect the repository on the copy too")
	}
	s.addDefaultDomain(ctx, app, dst)
	return app, notes, nil
}

// cloneApp: POST /api/apps/{id}/clone {slug, name, project_id} (app.create in the target project).
func (s *Server) cloneApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		ProjectID int64  `json:"project_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	dst := proj
	if in.ProjectID != 0 && in.ProjectID != proj.ID {
		var err error
		if dst, err = s.store.GetProject(r.Context(), in.ProjectID); err != nil {
			writeError(w, http.StatusNotFound, "target project not found")
			return
		}
	}
	if !p.eff.Can("app.create", &dst.ID) || !p.eff.Can("env.view", &proj.ID) {
		writeError(w, http.StatusForbidden, "copying an app needs app.create in the target project and env.view here")
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be 1–30 lowercase letters, digits or dashes")
		return
	}
	if taken, _ := s.store.SlugTaken(r.Context(), dst.ID, in.Slug); taken {
		writeError(w, http.StatusConflict, "an app or add-on with this slug already exists in "+dst.Slug)
		return
	}
	copied, notes, err := s.cloneAppInto(r, app, proj, dst, in.Slug, orStr(strings.TrimSpace(in.Name), app.Name+" (copy)"), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.clone", dst.Slug+"/"+copied.Slug, map[string]string{"from": proj.Slug + "/" + app.Slug})
	if notes == nil {
		notes = []string{}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"app": copied, "notes": notes})
}

// cloneProject: POST /api/projects/{pid}/clone {slug, name, environment, source_environment} — a new
// project with the same env groups, databases (new and empty, new passwords) and apps (settings,
// variables, config files, links); nothing is deployed. environment labels the copy (e.g. "staging").
func (s *Server) cloneProject(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	var in struct {
		Slug              string `json:"slug"`
		Name              string `json:"name"`
		Environment       string `json:"environment"`
		SourceEnvironment string `json:"source_environment"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !p.eff.Can("project.create", nil) || !p.eff.Can("env.view", &proj.ID) {
		writeError(w, http.StatusForbidden, "copying a project needs project.create and env.view")
		return
	}
	for _, e := range []string{in.Environment, in.SourceEnvironment} {
		if e != "" && !groupNameRe.MatchString(e) {
			writeError(w, http.StatusBadRequest, "environment must be a short name like staging or production")
			return
		}
	}
	code, body := s.internalCall(r, "POST", "/api/projects", map[string]any{"slug": in.Slug, "name": orStr(in.Name, proj.Name+" ("+orStr(in.Environment, "copy")+")"), "quota": proj.Quota})
	if code >= 300 {
		writeError(w, code, apiError(code, body))
		return
	}
	var dst store.Project
	_ = json.Unmarshal(body, &dst)
	ctx := r.Context()
	origin := proj.ID
	_ = s.store.SetProjectEnvironment(ctx, dst.ID, in.Environment, &origin)
	if in.SourceEnvironment != "" {
		_ = s.store.SetProjectEnvironment(ctx, proj.ID, in.SourceEnvironment, proj.OriginID)
	}
	var notes []string
	if gs, err := s.store.ListEnvGroups(ctx, proj.ID); err == nil {
		for _, g := range gs {
			if _, err := s.store.SaveEnvGroup(ctx, dst.ID, g.Name, g.Vars); err != nil {
				notes = append(notes, "env group "+g.Name+": "+err.Error())
			}
		}
	}
	addonMap := map[int64]int64{}
	if addons, err := s.store.ListAddons(ctx, proj.ID); err == nil {
		for _, a := range addons {
			code, body := s.internalCall(r, "POST", "/api/projects/"+strconv.FormatInt(dst.ID, 10)+"/addons", map[string]any{
				"kind": a.Kind, "slug": a.Slug, "name": a.Name, "version": a.Version, "size_mb": a.SizeMB, "memory_mb": a.MemoryMB, "cpu_millicores": a.CPUMillicores})
			if code >= 300 {
				notes = append(notes, "database "+a.Slug+": "+apiError(code, body))
				continue
			}
			var created store.Addon
			_ = json.Unmarshal(body, &created)
			addonMap[a.ID] = created.ID
		}
	}
	apps, _ := s.store.ListApps(ctx, proj.ID)
	for _, a := range apps {
		if _, n, err := s.cloneAppInto(r, a, proj, dst, a.Slug, a.Name, addonMap); err != nil {
			notes = append(notes, "app "+a.Slug+": "+err.Error())
		} else {
			notes = append(notes, n...)
		}
	}
	dst, _ = s.store.GetProject(ctx, dst.ID)
	s.audit(r, p, "project.clone", dst.Slug, map[string]any{"from": proj.Slug, "environment": in.Environment, "apps": len(apps)})
	if notes == nil {
		notes = []string{}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": dst, "notes": notes})
}

// promoteApp: POST /api/apps/{id}/promote {project_id} — deploys this app's live build (image or uploaded
// artifact) to the app of the same name in the other project (e.g. staging → production). Its settings and
// variables stay as they are there.
func (s *Server) promoteApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in struct {
		ProjectID int64 `json:"project_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	dstProj, err := s.store.GetProject(ctx, in.ProjectID)
	if err != nil || dstProj.ID == proj.ID {
		writeError(w, http.StatusNotFound, "target project not found")
		return
	}
	if !p.eff.Can("app.deploy", &dstProj.ID) {
		writeError(w, http.StatusForbidden, "missing permission: app.deploy in "+dstProj.Slug)
		return
	}
	var target store.App
	if apps, err := s.store.ListApps(ctx, dstProj.ID); err == nil {
		for _, a := range apps {
			if a.Slug == app.Slug {
				target = a
			}
		}
	}
	if target.ID == 0 {
		writeError(w, http.StatusNotFound, dstProj.Slug+" has no app "+app.Slug)
		return
	}
	if target.RuntimeID != app.RuntimeID {
		writeError(w, http.StatusConflict, "the apps have different runtimes")
		return
	}
	live, err := s.store.LiveRelease(ctx, app.ID)
	if err != nil {
		writeError(w, http.StatusConflict, app.Slug+" has no live release to promote")
		return
	}
	req := deploy.DeployRequest{App: target, By: &p.user.ID}
	what := ""
	switch {
	case live.ArtifactID != nil:
		art, err := s.store.GetArtifact(ctx, *live.ArtifactID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		copied, err := s.store.CreateArtifact(ctx, target.ID, art.SHA256, art.Filename, art.Size, art.Detect, &p.user.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		req.ArtifactID, what = &copied.ID, art.Filename
	default:
		img := live.Image
		if img == "" {
			img = app.Config.Image
		}
		if target.Config.Image != img {
			target.Config.Image = img
			if target, err = s.store.SaveApp(ctx, target); err != nil {
				s.serverError(w, r, err)
				return
			}
			req.App = target
		}
		what = img
	}
	rel, err := s.deployer.Deploy(ctx, req)
	if err != nil {
		s.deployErr(w, err)
		return
	}
	s.audit(r, p, "app.promote", dstProj.Slug+"/"+target.Slug, map[string]any{"from": proj.Slug + "/" + app.Slug, "release": rel.Number, "build": what})
	writeJSON(w, http.StatusAccepted, map[string]any{"release": rel, "app_id": target.ID, "build": what})
}
