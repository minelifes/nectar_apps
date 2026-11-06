package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
	"nectar_apps/internal/wsock"
)

// Apps marked as databases: see internal/deploy/appdb.go.

// appDatabaseDetect: GET /api/apps/{id}/database/detect — a suggestion from the image and the variables.
func (s *Server) appDatabaseDetect(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	env, err := s.decryptEnv(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	keys := sortedKeys(env)
	writeJSON(w, http.StatusOK, map[string]any{"suggestion": deploy.DetectAppDatabase(app, env), "variables": keys})
}

func (s *Server) decryptEnv(r *http.Request, app store.App) (map[string]string, error) {
	vars, err := s.store.ListEnv(r.Context(), app.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		pt, err := s.envBox.Open(v.ValueEnc)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", v.Key, err)
		}
		out[v.Key] = pt
	}
	return out, nil
}

// putAppDatabase: PUT /api/apps/{id}/database — mark the app as a database (or change how).
func (s *Server) putAppDatabase(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in store.AppDatabase
	if !decode(w, r, &in) {
		return
	}
	if !slices.Contains(deploy.AppDatabaseEngines, in.Engine) {
		writeError(w, http.StatusBadRequest, "engine must be one of "+strings.Join(deploy.AppDatabaseEngines, ", "))
		return
	}
	if app.Kind != "web" && app.Kind != "worker" || app.Config.Port == 0 {
		writeError(w, http.StatusBadRequest, "only web and worker apps with a port can be databases")
		return
	}
	if in.Port < 0 || in.Port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be 1–65535 (or 0 for the engine's usual one)")
		return
	}
	in.User, in.Database = strings.TrimSpace(in.User), strings.TrimSpace(in.Database)
	if len(in.User) > 100 || len(in.Database) > 100 || strings.ContainsAny(in.User+in.Database, "\n\r\x00") {
		writeError(w, http.StatusBadRequest, "user and database must be at most 100 characters")
		return
	}
	env, err := s.decryptEnv(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, v := range []struct{ name, key string }{{"user", in.UserVar}, {"password", in.PasswordVar}, {"database", in.DatabaseVar}} {
		if v.key == "" {
			continue
		}
		if _, ok := env[v.key]; !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("the app has no variable %s (for the %s)", v.key, v.name))
			return
		}
	}
	if in.Engine != "redis" && in.PasswordVar == "" {
		writeError(w, http.StatusBadRequest, "choose the variable that holds the database password")
		return
	}
	if err := s.store.SetAppDatabase(r.Context(), app.ID, &in); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.database", proj.Slug+"/"+app.Slug, map[string]any{"engine": in.Engine})
	app, _ = s.store.GetApp(r.Context(), app.ID)
	writeJSON(w, http.StatusOK, app)
}

// deleteAppDatabase: DELETE /api/apps/{id}/database — an ordinary app again (links keep their variables
// until removed: their references then fail the next deploy, which says what to fix).
func (s *Server) deleteAppDatabase(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if err := s.store.SetAppDatabase(r.Context(), app.ID, nil); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.database", proj.Slug+"/"+app.Slug, map[string]any{"engine": nil})
	w.WriteHeader(http.StatusNoContent)
}

func needDatabase(w http.ResponseWriter, app store.App) bool {
	if app.Database == nil {
		writeError(w, http.StatusBadRequest, "the app is not marked as a database")
		return false
	}
	return true
}

// appDatabaseCredentials: GET /api/apps/{id}/database/credentials (audited), like an add-on's.
func (s *Server) appDatabaseCredentials(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !needDatabase(w, app) {
		return
	}
	env, err := s.decryptEnv(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	c, err := deploy.AppDatabaseCredentials(app.Database, env)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	port := deploy.AppDatabasePort(app)
	s.audit(r, p, "app.database.credentials", proj.Slug+"/"+app.Slug, nil)
	writeJSON(w, http.StatusOK, map[string]any{"host": app.Slug, "port": port, "user": c.User, "password": c.Password, "database": c.Database,
		"env": deploy.DatabaseEnv(app.Database.Engine, app.Slug, port, c, "")})
}

// appDatabaseTunnel: GET /api/apps/{id}/database/tunnel (WebSocket) — `nectar db connect` to a database app.
func (s *Server) appDatabaseTunnel(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !wsock.IsUpgrade(r) {
		writeError(w, http.StatusBadRequest, "this endpoint needs a WebSocket connection (use `nectar db connect`)")
		return
	}
	if r.Header.Get("Origin") != "" && !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin WebSocket refused")
		return
	}
	if !needDatabase(w, app) {
		return
	}
	if app.Config.Replicas == 0 {
		writeError(w, http.StatusConflict, "the app is stopped: start it first")
		return
	}
	addr := net.JoinHostPort(app.Slug+"."+proj.Namespace+".svc.cluster.local", strconv.Itoa(deploy.AppDatabasePort(app)))
	s.serveTunnel(w, r, p, addr, "app.database.tunnel", proj.Slug+"/"+app.Slug)
}

// projectAppDatabases: GET /api/projects/{pid}/app-databases — the project's database apps, in the shape
// of the add-on list (the CLI shows both).
func (s *Server) projectAppDatabases(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	apps, err := s.store.ListApps(r.Context(), proj.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := []map[string]any{}
	for _, a := range apps {
		if a.Database == nil {
			continue
		}
		k, _ := deploy.AddonKindByID(a.Database.Engine)
		out = append(out, map[string]any{"id": a.ID, "project_id": a.ProjectID, "slug": a.Slug, "name": a.Name, "kind": a.Database.Engine,
			"kind_name": k.Name, "version": "", "port": deploy.AppDatabasePort(a), "stopped": a.Config.Replicas == 0, "app": true})
	}
	writeJSON(w, http.StatusOK, out)
}

// dbLink is an app that gets a database app's connection variables.
type dbLink struct {
	AppID  int64    `json:"app_id"`
	Slug   string   `json:"slug"`
	Name   string   `json:"name"`
	Prefix string   `json:"prefix"`
	Keys   []string `json:"keys"`
}

// dbEnvKeys are the variables a link adds (without prefix).
func dbEnvKeys(engine string) []string {
	return sortedKeys(deploy.DatabaseEnv(engine, "h", 1, deploy.AddonCredentials{}, ""))
}

// appDatabaseLinks: GET /api/apps/{id}/database/links — the project's apps whose variables reference it.
func (s *Server) appDatabaseLinks(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	if !needDatabase(w, app) {
		return
	}
	links, err := s.dbLinks(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

func (s *Server) dbLinks(r *http.Request, db store.App) ([]dbLink, error) {
	apps, err := s.store.ListApps(r.Context(), db.ProjectID)
	if err != nil {
		return nil, err
	}
	keys := dbEnvKeys(db.Database.Engine)
	out := []dbLink{}
	for _, a := range apps {
		if a.ID == db.ID {
			continue
		}
		env, err := s.decryptEnv(r, a)
		if err != nil {
			return nil, err
		}
		var l *dbLink
		for _, k := range sortedKeys(env) {
			for _, key := range keys {
				if env[k] == "${"+db.Slug+"."+key+"}" && strings.HasSuffix(k, key) {
					if l == nil {
						l = &dbLink{AppID: a.ID, Slug: a.Slug, Name: a.Name, Prefix: strings.TrimSuffix(k, key)}
					}
					l.Keys = append(l.Keys, k)
				}
			}
		}
		if l != nil {
			out = append(out, *l)
		}
	}
	return out, nil
}

// linkAppDatabase: POST /api/apps/{id}/database/links {app_id, prefix} — the app gets the connection
// variables as references (${db.DATABASE_URL} …), resolved at each deploy of that app.
func (s *Server) linkAppDatabase(w http.ResponseWriter, r *http.Request, p *principal, db store.App, proj store.Project) {
	if !needDatabase(w, db) {
		return
	}
	var in struct {
		AppID  int64  `json:"app_id"`
		Prefix string `json:"prefix"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !envPrefixRe.MatchString(in.Prefix) {
		writeError(w, http.StatusBadRequest, "prefix must be uppercase letters, digits and _ ending with _ (e.g. AUTH_), or empty")
		return
	}
	target, err := s.store.GetApp(r.Context(), in.AppID)
	if err != nil || target.ProjectID != db.ProjectID || target.ID == db.ID {
		writeError(w, http.StatusBadRequest, "choose another app of the same project")
		return
	}
	if !p.eff.Can("env.edit", &target.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.edit")
		return
	}
	vars, err := s.store.ListEnv(r.Context(), target.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	byKey := map[string]int{}
	for i, v := range vars {
		byKey[v.Key] = i
	}
	var conflicts []string
	for _, key := range dbEnvKeys(db.Database.Engine) {
		name, ref := in.Prefix+key, "${"+db.Slug+"."+key+"}"
		enc, err := s.envBox.Seal(ref)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		ev := store.EnvVar{Key: name, ValueEnc: enc, IsSecret: strings.Contains(key, "PASSWORD") || key == "DATABASE_URL" || key == "REDIS_URL"}
		if i, ok := byKey[name]; ok {
			if cur, _ := s.envBox.Open(vars[i].ValueEnc); cur != ref {
				conflicts = append(conflicts, name)
				continue
			}
			vars[i] = ev
			continue
		}
		vars = append(vars, ev)
	}
	if len(conflicts) > 0 {
		writeError(w, http.StatusConflict, target.Slug+" already has "+strings.Join(conflicts, ", ")+": use a prefix, or remove them first")
		return
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
	if err := s.store.ReplaceEnv(r.Context(), target.ID, vars); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.database.link", proj.Slug+"/"+db.Slug, map[string]any{"app": target.Slug, "prefix": in.Prefix})
	links, _ := s.dbLinks(r, db)
	writeJSON(w, http.StatusOK, links)
}

// unlinkAppDatabase: DELETE /api/apps/{id}/database/links/{app} — removes the references to this database.
func (s *Server) unlinkAppDatabase(w http.ResponseWriter, r *http.Request, p *principal, db store.App, proj store.Project) {
	if !needDatabase(w, db) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("app"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad app id")
		return
	}
	target, err := s.store.GetApp(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && target.ProjectID != db.ProjectID) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !p.eff.Can("env.edit", &target.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.edit")
		return
	}
	vars, err := s.store.ListEnv(r.Context(), target.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	keep := vars[:0]
	for _, v := range vars {
		if pt, err := s.envBox.Open(v.ValueEnc); err == nil && strings.HasPrefix(pt, "${"+db.Slug+".") && strings.HasSuffix(pt, "}") &&
			strings.Count(pt, "${") == 1 {
			continue
		}
		keep = append(keep, v)
	}
	if err := s.store.ReplaceEnv(r.Context(), target.ID, keep); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "app.database.unlink", proj.Slug+"/"+db.Slug, map[string]any{"app": target.Slug})
	w.WriteHeader(http.StatusNoContent)
}
