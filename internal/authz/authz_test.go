package authz

import (
	"slices"
	"testing"
)

func p(i int64) *int64 { return &i }

func role(key string) []string {
	for _, r := range BuiltinRoles() {
		if r.Key == key {
			return r.Permissions
		}
	}
	panic(key)
}

func TestBuiltinRolesUseKnownPermissions(t *testing.T) {
	for _, r := range BuiltinRoles() {
		if u := Validate(r.Permissions); u != nil {
			t.Errorf("role %s has unknown permissions %v", r.Key, u)
		}
	}
	if slices.Contains(role("admin"), "role.manage") {
		t.Error("admin must not have role.manage")
	}
}

func TestOwnerCanEverything(t *testing.T) {
	e := Evaluate(true, nil)
	for _, k := range All() {
		if !e.Can(k, nil) {
			t.Fatalf("owner lacks %s", k)
		}
	}
}

func TestGlobalAndProjectBindings(t *testing.T) {
	// Viewer everywhere + Developer on project 7
	e := Evaluate(false, []Binding{
		{nil, role("viewer")},
		{p(7), role("developer")},
	})
	if !e.Can("app.view", p(99)) || !e.Can("app.view", nil) {
		t.Error("global viewer should see apps everywhere")
	}
	if !e.Can("app.deploy", p(7)) {
		t.Error("developer binding should allow deploy in project 7")
	}
	if e.Can("app.deploy", p(8)) || e.Can("app.deploy", nil) {
		t.Error("deploy must be limited to project 7")
	}
	// server.view is global-scope: a project binding must not grant it…
	e2 := Evaluate(false, []Binding{{p(7), role("developer")}})
	if e2.Can("server.view", nil) || e2.Can("server.view", p(7)) {
		t.Error("project binding must not grant a global permission")
	}
	// …but it does count for CanAnywhere for project perms
	if !e2.CanAnywhere("app.deploy") || e2.CanAnywhere("user.manage") {
		t.Error("CanAnywhere wrong")
	}
}

func TestCanGrantPreventsEscalation(t *testing.T) {
	// a user manager without role.manage / audit.view cannot grant Admin globally
	mgr := Evaluate(false, []Binding{{nil, []string{"user.manage", "project.view", "app.view"}}})
	if mgr.CanGrant(role("admin"), nil) {
		t.Error("escalation: granted admin without holding its permissions")
	}
	if !mgr.CanGrant([]string{"app.view"}, p(3)) {
		t.Error("should be able to grant what it holds")
	}
	// global-scope perms are ignored for project grants
	dev7 := Evaluate(false, []Binding{{p(7), role("developer")}})
	if !dev7.CanGrant(role("developer"), p(7)) {
		t.Error("project developer should be able to grant developer in its own project (server.view ignored)")
	}
	if dev7.CanGrant(role("developer"), p(8)) {
		t.Error("must not grant in another project")
	}
}

func TestListsAndUnknownPerms(t *testing.T) {
	e := Evaluate(false, []Binding{
		{nil, []string{"app.view", "removed.permission"}},
		{p(1), []string{"app.view", "app.deploy"}},
	})
	g, pr := e.Lists()
	if !slices.Equal(g, []string{"app.view"}) {
		t.Errorf("global = %v", g)
	}
	if !slices.Equal(pr[1], []string{"app.deploy"}) {
		t.Errorf("project 1 extras = %v", pr[1])
	}
	if u := Validate([]string{"app.view", "nope"}); !slices.Equal(u, []string{"nope"}) {
		t.Errorf("Validate = %v", u)
	}
}
