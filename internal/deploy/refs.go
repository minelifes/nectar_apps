package deploy

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"nectar_apps/internal/store"
)

// References in env values: "${db.PGHOST}" is the variable PGHOST that the project's add-on "db" gives
// linked apps, "${api.host}" / "${api.port}" / "${api.url}" the in-cluster address of the project's app
// "api" and "${api.KEY}" one of its own variables (as set on the app, not resolved again), "${shared.KEY}"
// a variable of the project's env group "shared". "$${…}" is a literal "${…}".
// References are resolved at every deploy, so a rotated password or a moved port is picked up.

var refRe = regexp.MustCompile(`\$?\$\{([a-z0-9]([a-z0-9-]*[a-z0-9])?)\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// HasRefs reports whether a value contains references.
func HasRefs(v string) bool { return strings.Contains(v, "${") && refRe.MatchString(v) }

// refSource resolves the targets of references in one project (loaded lazily).
type refSource struct {
	d       *Deployer
	project store.Project
	apps    map[string]store.App
	addons  map[string]map[string]string
	groups  map[string]map[string]string
	appEnv  map[string]map[string]string // decrypted on first use
}

func (r *refSource) load(ctx context.Context) error {
	if r.apps != nil {
		return nil
	}
	r.apps, r.addons, r.groups = map[string]store.App{}, map[string]map[string]string{}, map[string]map[string]string{}
	apps, err := r.d.store.ListApps(ctx, r.project.ID)
	if err != nil {
		return err
	}
	for _, a := range apps {
		r.apps[a.Slug] = a
	}
	addons, err := r.d.store.ListAddons(ctx, r.project.ID)
	if err != nil {
		return err
	}
	for _, a := range addons {
		c, err := r.d.AddonCredentials(a)
		if err != nil {
			return fmt.Errorf("add-on %s: %w", a.Slug, err)
		}
		r.addons[a.Slug] = AddonEnv(a, c, "")
	}
	if r.groups, err = r.d.EnvGroups(ctx, r.project.ID); err != nil {
		return err
	}
	return nil
}

func (r *refSource) value(name, field string) (string, bool) {
	if env, ok := r.addons[name]; ok {
		v, ok := env[field]
		return v, ok
	}
	if g, ok := r.groups[name]; ok {
		v, ok := g[field]
		return v, ok
	}
	if a, ok := r.apps[name]; ok {
		host := a.Slug + "." + r.project.Namespace + ".svc.cluster.local"
		port := strconv.Itoa(a.Config.Port)
		switch field {
		case "host":
			return host, true
		case "port":
			return port, a.Config.Port > 0
		case "url":
			return "http://" + host + ":" + port, a.Config.Port > 0
		}
		if r.appEnv == nil {
			r.appEnv = map[string]map[string]string{}
		}
		env, ok := r.appEnv[name]
		if !ok {
			env, _ = r.d.DecryptEnv(context.Background(), a.ID)
			r.appEnv[name] = env
		}
		v, ok := env[field]
		return v, ok
	}
	return "", false
}

// ResolveRefs replaces the references in env (in place). Unknown references are an error, so a typo does
// not deploy an app with a broken setting.
func (d *Deployer) ResolveRefs(ctx context.Context, p store.Project, env map[string]string) error {
	src := &refSource{d: d, project: p}
	var missing []string
	for k, v := range env {
		if !strings.Contains(v, "${") {
			continue
		}
		if err := src.load(ctx); err != nil {
			return err
		}
		env[k] = refRe.ReplaceAllStringFunc(v, func(m string) string {
			if strings.HasPrefix(m, "$$") {
				return m[1:] // escaped
			}
			sub := refRe.FindStringSubmatch(m)
			if val, ok := src.value(sub[1], sub[3]); ok {
				return val
			}
			missing = append(missing, k+": "+m)
			return m
		})
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("unknown references in the environment (%s) — add-ons, apps and env groups of the same project can be referenced", strings.Join(missing, "; "))
	}
	return nil
}

// EnvGroups returns the decrypted env groups of a project (group → key → value).
func (d *Deployer) EnvGroups(ctx context.Context, projectID int64) (map[string]map[string]string, error) {
	gs, err := d.store.ListEnvGroups(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]string, len(gs))
	for _, g := range gs {
		m := make(map[string]string, len(g.Vars))
		for _, v := range g.Vars {
			pt, err := d.envBox.Open(v.ValueEnc)
			if err != nil {
				return nil, fmt.Errorf("env group %s, %s: %w", g.Name, v.Key, err)
			}
			m[v.Key] = pt
		}
		out[g.Name] = m
	}
	return out, nil
}
