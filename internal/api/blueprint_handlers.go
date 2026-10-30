package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"

	"nectar_apps/internal/blueprint"
	"nectar_apps/internal/store"
)

// Blueprints (nectar.yaml, one-click templates, docker-compose imports) are applied through the API's own
// endpoints, as the calling user: every step gets the same validation, permission checks and audit entries
// as if the user had clicked through the UI.

// applyStep is one line of the apply report.
type applyStep struct {
	Kind    string `json:"kind"`   // addon | app | env | volume | link | domain | deploy
	Name    string `json:"name"`   // slug, host, …
	Action  string `json:"action"` // created | updated | exists | linked | deployed | skipped | failed
	Message string `json:"message,omitempty"`
}

// internalCall runs an API request as the caller of r (same cookie / token) and returns the status and body.
func (s *Server) internalCall(r *http.Request, method, path string, body any) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, path, rd)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{"error":"internal request"}`)
	}
	req.RemoteAddr, req.Host = r.RemoteAddr, r.Host
	for _, h := range []string{"Cookie", "Authorization", "X-Forwarded-For"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.ipAllowlist(s.mux).ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func apiError(code int, body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return http.StatusText(code)
}

// applyBlueprint creates what is missing and updates what exists. It never deletes.
func (s *Server) applyBlueprint(r *http.Request, proj store.Project, b blueprint.Blueprint, deploy bool, noDeploy ...string) []applyStep {
	ctx := r.Context()
	var out []applyStep
	add := func(kind, name, action, msg string) { out = append(out, applyStep{kind, name, action, msg}) }
	pid := strconv.FormatInt(proj.ID, 10)

	addons := map[string]store.Addon{}
	if list, err := s.store.ListAddons(ctx, proj.ID); err == nil {
		for _, a := range list {
			addons[a.Slug] = a
		}
	}
	apps := map[string]store.App{}
	if list, err := s.store.ListApps(ctx, proj.ID); err == nil {
		for _, a := range list {
			apps[a.Slug] = a
		}
	}

	for _, g := range b.EnvGroups {
		current := map[string]store.EnvVar{}
		if eg, err := s.store.GetEnvGroup(ctx, proj.ID, g.Name); err == nil {
			for _, v := range eg.Vars {
				current[v.Key] = v
			}
		}
		var items []envItem
		has := map[string]bool{}
		for _, k := range blueprint.SortedKeys(g.Env) {
			v := g.Env[k]
			items, has[k] = append(items, envItem{Key: k, Value: &v}), true
		}
		for _, k := range blueprint.SortedKeys(g.Secrets) {
			v := g.Secrets[k]
			it := envItem{Key: k, Secret: true}
			if _, ok := current[k]; v != "" || !ok {
				it.Value = &v
			}
			items, has[k] = append(items, it), true
		}
		for k, v := range current {
			if !has[k] {
				items = append(items, envItem{Key: k, Secret: v.IsSecret})
			}
		}
		code, body := s.internalCall(r, "PUT", "/api/projects/"+pid+"/env-groups/"+g.Name, items)
		if code >= 300 {
			add("env", "group "+g.Name, "failed", apiError(code, body))
		} else {
			add("env", "group "+g.Name, "updated", strconv.Itoa(len(g.Env)+len(g.Secrets))+" variables")
		}
	}

	for _, a := range b.Addons {
		if _, ok := addons[a.Slug]; ok {
			add("addon", a.Slug, "exists", "kept as it is")
			continue
		}
		code, body := s.internalCall(r, "POST", "/api/projects/"+pid+"/addons", map[string]any{
			"kind": a.Kind, "slug": a.Slug, "name": a.Name, "version": a.Version, "size_mb": a.SizeMB, "memory_mb": a.MemoryMB})
		if code >= 300 {
			add("addon", a.Slug, "failed", apiError(code, body))
			continue
		}
		var created store.Addon
		_ = json.Unmarshal(body, &created)
		addons[a.Slug] = created
		add("addon", a.Slug, "created", a.Kind+" "+a.Version)
	}

	for _, a := range b.Apps {
		cfg := map[string]any{"image": a.Image, "port": a.Port, "command": a.Command, "args": a.Args, "schedule": a.Schedule,
			"release_command": a.ReleaseCommand, "internal_ports": a.InternalPorts}
		if a.Replicas != nil {
			cfg["replicas"] = *a.Replicas
		}
		if a.CPUMillicores > 0 || a.MemoryMB > 0 {
			res := map[string]any{}
			if a.CPUMillicores > 0 {
				res["cpu_millicores"] = a.CPUMillicores
			}
			if a.MemoryMB > 0 {
				res["memory_mb"] = a.MemoryMB
			}
			cfg["resources"] = res
		}
		if a.HealthPath != "" {
			cfg["health"] = map[string]any{"type": "http", "path": a.HealthPath}
		}
		if len(a.Volumes) > 0 {
			var vols []map[string]any
			for _, v := range a.Volumes {
				size := v.SizeMB
				if size == 0 {
					size = 1024
				}
				vols = append(vols, map[string]any{"name": v.Name, "mount_path": v.MountPath, "size_mb": size, "storage_class": v.StorageClass})
			}
			cfg["volumes"] = vols
		}
		if len(a.Expose) > 0 {
			var ports []map[string]any
			for _, p := range a.Expose {
				ports = append(ports, map[string]any{"port": p.Port, "protocol": strings.ToUpper(orStr(p.Protocol, "tcp"))})
			}
			cfg["expose"] = ports
		}
		for k, v := range cfg { // leave unset fields to the runtime defaults
			if v == nil || v == "" || v == 0 {
				delete(cfg, k)
			}
			if l, ok := v.([]string); ok && len(l) == 0 {
				delete(cfg, k)
			}
			if l, ok := v.([]int); ok && len(l) == 0 {
				delete(cfg, k)
			}
		}
		existing, exists := apps[a.Slug]
		if exists {
			code, body := s.internalCall(r, "PATCH", "/api/apps/"+strconv.FormatInt(existing.ID, 10), map[string]any{"name": a.Name, "config": cfg})
			if code >= 300 {
				add("app", a.Slug, "failed", apiError(code, body))
				continue
			}
			add("app", a.Slug, "updated", "")
		} else {
			code, body := s.internalCall(r, "POST", "/api/projects/"+pid+"/apps", map[string]any{
				"slug": a.Slug, "name": a.Name, "kind": a.Kind, "runtime": a.Runtime, "runtime_version": a.RuntimeVersion, "config": cfg})
			if code >= 300 {
				add("app", a.Slug, "failed", apiError(code, body))
				continue
			}
			_ = json.Unmarshal(body, &existing)
			apps[a.Slug] = existing
			add("app", a.Slug, "created", orStr(a.Image, a.Runtime))
		}
		appPath := "/api/apps/" + strconv.FormatInt(existing.ID, 10)

		// variables: the file's values win, secrets without a value keep theirs, other variables stay
		if len(a.Env)+len(a.Secrets) > 0 || len(a.EnvGroups) > 0 {
			current, _ := s.store.ListEnv(ctx, existing.ID)
			var items []envItem
			has := map[string]bool{}
			for _, k := range blueprint.SortedKeys(a.Env) {
				v := a.Env[k]
				items, has[k] = append(items, envItem{Key: k, Value: &v}), true
			}
			for _, k := range blueprint.SortedKeys(a.Secrets) {
				v := a.Secrets[k]
				it := envItem{Key: k, Secret: true}
				if v != "" || !slices.ContainsFunc(current, func(e store.EnvVar) bool { return e.Key == k }) {
					it.Value = &v
				}
				items, has[k] = append(items, it), true
			}
			for _, e := range current {
				if !has[e.Key] {
					items = append(items, envItem{Key: e.Key, Secret: e.IsSecret})
				}
			}
			code, body := s.internalCall(r, "PUT", appPath+"/env", items)
			if code >= 300 {
				add("env", a.Slug, "failed", apiError(code, body))
			} else {
				add("env", a.Slug, "updated", strconv.Itoa(len(a.Env)+len(a.Secrets))+" variables")
			}
			if len(a.EnvGroups) > 0 {
				code, body := s.internalCall(r, "PUT", appPath+"/env-groups", a.EnvGroups)
				if code >= 300 {
					add("env", a.Slug, "failed", "env groups: "+apiError(code, body))
				}
			}
		}

		// databases
		if len(a.Links) > 0 {
			linked, _ := s.store.AddonLinks(ctx, 0, existing.ID)
			for _, l := range a.Links {
				ad, ok := addons[l.Addon]
				if !ok {
					add("link", a.Slug+" → "+l.Addon, "failed", "no add-on "+l.Addon+" in the project")
					continue
				}
				if slices.ContainsFunc(linked, func(x store.AddonLink) bool { return x.AddonID == ad.ID }) {
					add("link", a.Slug+" → "+l.Addon, "exists", "")
					continue
				}
				code, body := s.internalCall(r, "POST", "/api/addons/"+strconv.FormatInt(ad.ID, 10)+"/links", map[string]any{"app_id": existing.ID, "prefix": l.Prefix})
				if code >= 300 {
					add("link", a.Slug+" → "+l.Addon, "failed", apiError(code, body))
				} else {
					add("link", a.Slug+" → "+l.Addon, "linked", "")
				}
			}
		}

		// domains (the default one under the base domain is added on creation)
		if len(a.Domains) > 0 {
			current, _ := s.store.ListDomains(ctx, existing.ID)
			for _, host := range a.Domains {
				host = store.NormalizeHost(host)
				if slices.ContainsFunc(current, func(d store.Domain) bool { return d.Host == host }) {
					add("domain", host, "exists", "")
					continue
				}
				code, body := s.internalCall(r, "POST", appPath+"/domains", map[string]any{"host": host})
				if code >= 300 {
					add("domain", host, "failed", apiError(code, body))
				} else {
					add("domain", host, "created", "")
				}
			}
		}

		if deploy {
			if a.Runtime != "image" {
				add("deploy", a.Slug, "skipped", "upload a build of the app to deploy it")
				continue
			}
			if slices.Contains(noDeploy, a.Slug) {
				add("deploy", a.Slug, "skipped", "set its image (or connect its Git repository) first")
				continue
			}
			code, body := s.internalCall(r, "POST", appPath+"/deploy", nil)
			if code >= 300 {
				add("deploy", a.Slug, "failed", apiError(code, body))
			} else {
				add("deploy", a.Slug, "deployed", "")
			}
		}
	}
	return out
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func failed(steps []applyStep) int {
	n := 0
	for _, s := range steps {
		if s.Action == "failed" {
			n++
		}
	}
	return n
}

// readBlueprint reads a request body: raw YAML/JSON, or {"blueprint": "<yaml>", "deploy": bool}.
func readBlueprintBody(w http.ResponseWriter, r *http.Request) (data []byte, deploy, dryRun bool, ok bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "the file is larger than 1 MB")
		return nil, false, false, false
	}
	deploy, dryRun = r.URL.Query().Get("deploy") != "0", r.URL.Query().Get("dry_run") == "1"
	var wrapped struct {
		Blueprint *string `json:"blueprint"`
		Compose   *string `json:"compose"`
		Deploy    *bool   `json:"deploy"`
		DryRun    bool    `json:"dry_run"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && (wrapped.Blueprint != nil || wrapped.Compose != nil) {
		if wrapped.Blueprint != nil {
			raw = []byte(*wrapped.Blueprint)
		} else {
			raw = []byte(*wrapped.Compose)
		}
		if wrapped.Deploy != nil {
			deploy = *wrapped.Deploy
		}
		dryRun = dryRun || wrapped.DryRun
	}
	return raw, deploy, dryRun, true
}

// applyProject: POST /api/projects/{pid}/apply — a nectar.yaml (YAML or JSON body). ?deploy=0 only saves,
// ?dry_run=1 only checks the file.
func (s *Server) applyProject(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	raw, deploy, dryRun, ok := readBlueprintBody(w, r)
	if !ok {
		return
	}
	b, err := blueprint.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{"blueprint": b})
		return
	}
	steps := s.applyBlueprint(r, proj, b, deploy)
	s.audit(r, p, "project.apply", proj.Slug, map[string]int{"apps": len(b.Apps), "addons": len(b.Addons), "failed": failed(steps)})
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "failed": failed(steps)})
}

// listTemplates: GET /api/templates — the one-click apps.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, _ *principal) {
	ts, err := blueprint.Templates()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// installTemplate: POST /api/projects/{pid}/templates/{tid} {name, inputs, deploy} — installs a one-click app.
func (s *Server) installTemplate(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	t, ok := blueprint.TemplateByID(r.PathValue("tid"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such template")
		return
	}
	var in struct {
		Name   string            `json:"name"`
		Inputs map[string]string `json:"inputs"`
		Deploy *bool             `json:"deploy"`
	}
	if !decode(w, r, &in) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(orStr(in.Name, t.ID)))
	if !slugRe.MatchString(name) || len(name) > 26 { // room for "-db"
		writeError(w, http.StatusBadRequest, "name must be 1–26 lowercase letters, digits or dashes")
		return
	}
	b, err := t.Render(name, in.Inputs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, a := range b.Apps {
		if taken, _ := s.store.SlugTaken(r.Context(), proj.ID, a.Slug); taken {
			writeError(w, http.StatusConflict, a.Slug+" already exists in this project — choose another name")
			return
		}
	}
	for _, a := range b.Addons {
		if taken, _ := s.store.SlugTaken(r.Context(), proj.ID, a.Slug); taken {
			writeError(w, http.StatusConflict, a.Slug+" already exists in this project — choose another name")
			return
		}
	}
	deploy := in.Deploy == nil || *in.Deploy
	steps := s.applyBlueprint(r, proj, b, deploy)
	s.audit(r, p, "template.install", proj.Slug+"/"+name, map[string]any{"template": t.ID, "failed": failed(steps)})
	var appID int64
	if a, err := s.store.ListApps(r.Context(), proj.ID); err == nil {
		for _, x := range a {
			if x.Slug == name {
				appID = x.ID
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "failed": failed(steps), "app_id": appID, "notes": strings.ReplaceAll(t.Notes, "[[name]]", name)})
}

// exportProject: GET /api/projects/{pid}/export — the project as nectar.yaml. Secret values are left out
// (applying the file keeps the current ones); default domains are left out (they are recreated).
func (s *Server) exportProject(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	ctx := r.Context()
	b := blueprint.Blueprint{Version: blueprint.Version, Project: proj.Slug}
	addons, err := s.store.ListAddons(ctx, proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if gs, err := s.store.ListEnvGroups(ctx, proj.ID); err == nil && (p.eff.Can("env.view", &proj.ID) || p.eff.Can("env.edit", &proj.ID)) {
		for _, g := range gs {
			bg := blueprint.EnvGroup{Name: g.Name}
			for _, e := range g.Vars {
				if e.IsSecret {
					if bg.Secrets == nil {
						bg.Secrets = map[string]string{}
					}
					bg.Secrets[e.Key] = ""
				} else if v, err := s.envBox.Open(e.ValueEnc); err == nil {
					if bg.Env == nil {
						bg.Env = map[string]string{}
					}
					bg.Env[e.Key] = v
				}
			}
			b.EnvGroups = append(b.EnvGroups, bg)
		}
	}
	slugOf := map[int64]string{}
	for _, a := range addons {
		slugOf[a.ID] = a.Slug
		b.Addons = append(b.Addons, blueprint.Addon{Slug: a.Slug, Name: a.Name, Kind: a.Kind, Version: a.Version, SizeMB: a.SizeMB, MemoryMB: a.MemoryMB})
	}
	apps, err := s.store.ListApps(ctx, proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, a := range apps {
		c := a.Config
		replicas := c.Replicas
		ba := blueprint.App{Slug: a.Slug, Name: a.Name, Kind: a.Kind, Runtime: a.RuntimeID, RuntimeVersion: a.RuntimeVersion, Image: c.Image,
			Port: c.Port, InternalPorts: c.InternalPorts, Command: c.Command, Args: c.Args, Replicas: &replicas,
			CPUMillicores: c.Resources.CPUMillicores, MemoryMB: c.Resources.MemoryMB, Schedule: c.Schedule, ReleaseCommand: c.ReleaseCommand,
			EnvGroups: c.EnvGroups}
		if c.Health.Type == "http" {
			ba.HealthPath = c.Health.Path
		}
		for _, v := range c.Volumes {
			ba.Volumes = append(ba.Volumes, blueprint.Volume{Name: v.Name, MountPath: v.MountPath, SizeMB: v.SizeMB, StorageClass: v.StorageClass})
		}
		for _, e := range c.Expose {
			ba.Expose = append(ba.Expose, blueprint.Port{Port: e.Port, Protocol: strings.ToLower(e.Protocol)})
		}
		if p.eff.Can("env.view", &proj.ID) || p.eff.Can("env.edit", &proj.ID) {
			if env, err := s.store.ListEnv(ctx, a.ID); err == nil {
				for _, e := range env {
					if e.IsSecret {
						if ba.Secrets == nil {
							ba.Secrets = map[string]string{}
						}
						ba.Secrets[e.Key] = "" // never exported
						continue
					}
					if v, err := s.envBox.Open(e.ValueEnc); err == nil {
						if ba.Env == nil {
							ba.Env = map[string]string{}
						}
						ba.Env[e.Key] = v
					}
				}
			}
		}
		if links, err := s.store.AddonLinks(ctx, 0, a.ID); err == nil {
			for _, l := range links {
				ba.Links = append(ba.Links, blueprint.Link{Addon: slugOf[l.AddonID], Prefix: l.Prefix})
			}
		}
		if doms, err := s.store.ListDomains(ctx, a.ID); err == nil {
			for _, d := range doms {
				if !d.IsDefault && d.Kind == store.DomainApp && d.PathPrefix == "/" {
					ba.Domains = append(ba.Domains, d.Host)
				}
			}
		}
		b.Apps = append(b.Apps, ba)
	}
	out, err := b.Marshal()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "project.export", proj.Slug, nil)
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nectar.yaml"`)
	_, _ = w.Write([]byte("# nectar.yaml — project " + proj.Slug + ". Secret values are not exported: applying this file keeps\n# the current ones (or set them here). Apply: nectar apply -f nectar.yaml --project " + proj.Slug + "\n"))
	_, _ = w.Write(out)
}

// importCompose: POST /api/projects/{pid}/import/compose — a docker-compose.yml (raw or {compose, deploy,
// dry_run}). dry_run returns the converted nectar.yaml and the notes without changing anything.
func (s *Server) importCompose(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	raw, deploy, dryRun, ok := readBlueprintBody(w, r)
	if !ok {
		return
	}
	b, notes, err := blueprint.FromCompose(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if notes == nil {
		notes = []string{}
	}
	if dryRun {
		y, _ := b.Marshal()
		writeJSON(w, http.StatusOK, map[string]any{"blueprint": b, "yaml": string(y), "notes": notes})
		return
	}
	for _, a := range b.Apps {
		if taken, _ := s.store.SlugTaken(r.Context(), proj.ID, a.Slug); taken {
			writeError(w, http.StatusConflict, a.Slug+" already exists in this project — import into another project or rename the service")
			return
		}
	}
	// services built from source get a placeholder image: create them, deploy the others
	var build []string
	for i, a := range b.Apps {
		if strings.HasPrefix(a.Image, blueprint.PlaceholderRegistry) {
			zero := 0
			b.Apps[i].Replicas = &zero
			build = append(build, a.Slug)
		}
	}
	steps := s.applyBlueprint(r, proj, b, deploy, build...)
	s.audit(r, p, "project.import_compose", proj.Slug, map[string]int{"apps": len(b.Apps), "failed": failed(steps)})
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "failed": failed(steps), "notes": notes})
}
