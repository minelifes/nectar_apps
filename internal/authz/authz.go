// Package authz defines the permission catalog, the built-in roles and the evaluation of
// effective permissions.
//
// Model:
//   - Users marked as Owner have every permission.
//   - Other users get permissions through role bindings. A binding is global (project = nil)
//     or scoped to one project.
//   - Global bindings grant all of the role's permissions everywhere.
//   - Project bindings grant only project-scoped permissions, and only in that project.
//   - Effective rights are the union of all matching bindings.
package authz

import (
	"slices"
	"sort"
)

// Scope tells where a permission can be granted.
type Scope string

const (
	// ScopeProject permissions can be granted per project (or globally).
	ScopeProject Scope = "project"
	// ScopeGlobal permissions are only meaningful globally (ignored in project bindings).
	ScopeGlobal Scope = "global"
)

// Def describes a permission.
type Def struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Scope       Scope  `json:"scope"`
}

// Group is a set of permissions shown together in the role editor.
type Group struct {
	Group       string `json:"group"`
	Permissions []Def  `json:"permissions"`
}

// Catalog is the fixed list of permissions. Adding a permission here makes it available in the
// role editor; built-in roles are updated on startup.
var Catalog = []Group{
	{"Projects", []Def{
		{"project.view", "See projects and their apps", ScopeProject},
		{"project.create", "Create projects", ScopeGlobal},
		{"project.edit", "Rename projects, change quotas", ScopeProject},
		{"project.delete", "Delete projects", ScopeProject},
		{"project.members", "Manage project members", ScopeProject},
	}},
	{"Apps", []Def{
		{"app.view", "See apps, their status and configuration", ScopeProject},
		{"app.create", "Create apps", ScopeProject},
		{"app.edit", "Change app configuration and resources", ScopeProject},
		{"app.deploy", "Deploy new versions", ScopeProject},
		{"app.rollback", "Roll back to a previous release", ScopeProject},
		{"app.restart", "Restart, stop, start and scale apps", ScopeProject},
		{"app.delete", "Delete apps", ScopeProject},
	}},
	{"Runtime access", []Def{
		{"app.logs", "Read app logs", ScopeProject},
		{"app.exec", "Open a terminal inside app containers", ScopeProject},
		{"app.metrics", "See app metrics", ScopeProject},
	}},
	{"Configuration", []Def{
		{"env.view", "See plain environment variables", ScopeProject},
		{"env.secrets.view", "Reveal secret values", ScopeProject},
		{"env.edit", "Change environment variables and config files", ScopeProject},
		{"volume.manage", "Create, resize and delete volumes", ScopeProject},
	}},
	{"Domains", []Def{
		{"domain.view", "See domains and certificates", ScopeProject},
		{"domain.manage", "Add, change and remove domains", ScopeProject},
		{"domain.tls", "Issue and manage TLS certificates", ScopeProject},
	}},
	{"Server", []Def{
		{"server.view", "See server resources, nodes and pods", ScopeGlobal},
		{"server.manage", "Manage runtime catalog, pre-pulled images, updates", ScopeGlobal},
	}},
	{"Backups", []Def{
		{"backup.manage", "Configure backup targets and plans, run backups", ScopeGlobal},
		{"backup.restore", "Restore backups (overwrites current data)", ScopeGlobal},
	}},
	{"Administration", []Def{
		{"user.manage", "Manage users and their roles", ScopeGlobal},
		{"role.manage", "Create and edit custom roles", ScopeGlobal},
		{"token.manage", "Manage other users' API tokens", ScopeGlobal},
		{"settings.manage", "Change base domain, gateway and other settings", ScopeGlobal},
		{"audit.view", "Read the audit log", ScopeGlobal},
	}},
}

var defs = func() map[string]Def {
	m := map[string]Def{}
	for _, g := range Catalog {
		for _, d := range g.Permissions {
			m[d.Key] = d
		}
	}
	return m
}()

// Lookup returns the definition of a permission.
func Lookup(key string) (Def, bool) { d, ok := defs[key]; return d, ok }

// All returns every permission key, sorted.
func All() []string {
	out := make([]string, 0, len(defs))
	for k := range defs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Builtin role definitions. They are created/updated in the database on startup and are read-only.
// Owner is not a role: it is a flag on the user that grants everything.
type BuiltinRole struct {
	Key         string
	Name        string
	Description string
	Permissions []string
}

// BuiltinRoles returns the built-in roles with their current permissions.
func BuiltinRoles() []BuiltinRole {
	admin := slices.DeleteFunc(All(), func(k string) bool { return k == "role.manage" })
	return []BuiltinRole{
		{"admin", "Admin", "Everything except editing roles", admin},
		{"developer", "Developer", "Build, deploy and operate apps", []string{
			"project.view", "app.view", "app.create", "app.edit", "app.deploy", "app.rollback", "app.restart",
			"app.logs", "app.exec", "app.metrics", "env.view", "env.edit", "volume.manage",
			"domain.view", "domain.manage", "domain.tls", "server.view",
		}},
		{"deployer", "Deployer", "Deploy and roll back only — for CI tokens", []string{
			"project.view", "app.view", "app.deploy", "app.rollback",
		}},
		{"viewer", "Viewer", "Read-only access", []string{
			"project.view", "app.view", "app.logs", "app.metrics", "env.view", "domain.view", "server.view",
		}},
	}
}

// Validate returns the unknown keys in perms (nil if all are valid).
func Validate(perms []string) (unknown []string) {
	for _, p := range perms {
		if _, ok := defs[p]; !ok {
			unknown = append(unknown, p)
		}
	}
	return unknown
}

// Binding is the input to Evaluate: a role's permissions bound globally or to a project.
type Binding struct {
	ProjectID   *int64
	Permissions []string
}

// Effective holds the evaluated permissions of a principal.
type Effective struct {
	Owner    bool
	Global   map[string]bool
	Projects map[int64]map[string]bool
}

// Evaluate computes effective permissions.
func Evaluate(owner bool, bindings []Binding) Effective {
	e := Effective{Owner: owner, Global: map[string]bool{}, Projects: map[int64]map[string]bool{}}
	for _, b := range bindings {
		for _, p := range b.Permissions {
			d, ok := defs[p]
			if !ok {
				continue // permission removed from the catalog
			}
			if b.ProjectID == nil {
				e.Global[p] = true
				continue
			}
			if d.Scope != ScopeProject {
				continue
			}
			m := e.Projects[*b.ProjectID]
			if m == nil {
				m = map[string]bool{}
				e.Projects[*b.ProjectID] = m
			}
			m[p] = true
		}
	}
	return e
}

// Can reports whether the principal has perm, globally (project == nil) or in the given project.
func (e Effective) Can(perm string, project *int64) bool {
	if e.Owner || e.Global[perm] {
		return true
	}
	return project != nil && e.Projects[*project][perm]
}

// CanAnywhere reports whether perm is granted globally or in at least one project.
func (e Effective) CanAnywhere(perm string) bool {
	if e.Can(perm, nil) {
		return true
	}
	for _, m := range e.Projects {
		if m[perm] {
			return true
		}
	}
	return false
}

// CanGrant reports whether the principal may hand out a binding of perms in the given scope:
// you can only grant what you hold yourself (prevents privilege escalation via user.manage).
func (e Effective) CanGrant(perms []string, project *int64) bool {
	for _, p := range perms {
		d, ok := defs[p]
		if !ok {
			continue
		}
		if project != nil && d.Scope != ScopeProject {
			continue // not granted by a project binding anyway
		}
		if !e.Can(p, project) {
			return false
		}
	}
	return true
}

// Lists returns the effective permissions as sorted slices (for the API).
func (e Effective) Lists() (global []string, projects map[int64][]string) {
	if e.Owner {
		return All(), map[int64][]string{}
	}
	global = keys(e.Global)
	projects = map[int64][]string{}
	for id, m := range e.Projects {
		var extra []string
		for k := range m {
			if !e.Global[k] {
				extra = append(extra, k)
			}
		}
		if len(extra) > 0 {
			sort.Strings(extra)
			projects[id] = extra
		}
	}
	return global, projects
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Intersect returns what both a and b allow (an API token limited by its role and project).
func Intersect(a, b Effective) Effective {
	out := Effective{Owner: a.Owner && b.Owner, Global: map[string]bool{}, Projects: map[int64]map[string]bool{}}
	if out.Owner {
		return out
	}
	for _, k := range All() {
		if a.Can(k, nil) && b.Can(k, nil) {
			out.Global[k] = true
		}
	}
	ids := map[int64]bool{}
	for id := range a.Projects {
		ids[id] = true
	}
	for id := range b.Projects {
		ids[id] = true
	}
	for id := range ids {
		for _, k := range All() {
			if out.Global[k] {
				continue
			}
			if a.Can(k, &id) && b.Can(k, &id) {
				m := out.Projects[id]
				if m == nil {
					m = map[string]bool{}
					out.Projects[id] = m
				}
				m[k] = true
			}
		}
	}
	return out
}

// Restrict limits e to one project: global project-scoped permissions apply only there, global-only
// permissions (users, settings, server) are dropped.
func Restrict(e Effective, project int64) Effective {
	out := Effective{Global: map[string]bool{}, Projects: map[int64]map[string]bool{}}
	m := map[string]bool{}
	for _, k := range All() {
		if d := defs[k]; d.Scope == ScopeProject && e.Can(k, &project) {
			m[k] = true
		}
	}
	if len(m) > 0 {
		out.Projects[project] = m
	}
	return out
}
