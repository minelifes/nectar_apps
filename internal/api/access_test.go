package api

import (
	"net/http"
	"strings"
	"testing"
)

func bearer(tok string) []string { return []string{"Authorization", "Bearer " + tok} }

func roleIDs(t *testing.T, e *testEnv, c *http.Client) map[string]float64 {
	_, _, roles := e.do(c, "GET", "/api/roles", nil)
	out := map[string]float64{}
	for _, r := range roles {
		m := r.(map[string]any)
		if k, _ := m["builtin_key"].(string); k != "" {
			out[k] = m["id"].(float64)
		}
	}
	return out
}

func TestAPITokens(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	roles := roleIDs(t, e, o)
	_, p1, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "one", "name": "One"})
	_, p2, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "two", "name": "Two"})
	p1id, p2id := p1["id"].(float64), p2["id"].(float64)
	e.do(o, "POST", "/api/projects/"+itoa(p2id)+"/apps", map[string]any{"slug": "svc", "name": "Svc", "runtime": "java"})

	if code, _, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": ""}); code != 400 {
		t.Fatalf("empty name: %d", code)
	}
	// a Deployer token for project one only
	code, created, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "CI one", "role_id": roles["deployer"], "project_id": p1id, "expires_in_days": 90})
	tok, _ := created["token"].(string)
	if code != 201 || !strings.HasPrefix(tok, "nat_") || created["api_token"].(map[string]any)["expires_at"] == nil {
		t.Fatalf("create: %d %v", code, created)
	}
	anon := e.client()
	_, perms, _ := e.do(anon, "GET", "/api/me/permissions", nil, bearer(tok)...)
	if perms["owner"] == true || len(perms["global"].([]any)) != 0 {
		t.Fatalf("token permissions: %v", perms)
	}
	pp := perms["projects"].(map[string]any)[itoa(p1id)].([]any)
	if len(pp) != 4 { // project.view app.view app.deploy app.rollback
		t.Fatalf("project permissions: %v", pp)
	}
	if _, _, list := e.do(anon, "GET", "/api/projects", nil, bearer(tok)...); len(list) != 1 || list[0].(map[string]any)["slug"] != "one" {
		t.Fatalf("the token sees project one only: %v", list)
	}
	if code, _, _ := e.do(anon, "GET", "/api/projects/"+itoa(p2id)+"/apps", nil, bearer(tok)...); code != 404 {
		t.Fatalf("other project: %d", code)
	}
	if code, _, _ := e.do(anon, "GET", "/api/users", nil, bearer(tok)...); code != 403 {
		t.Fatalf("users with a deployer token: %d", code)
	}
	// tokens cannot make tokens or touch two-factor settings
	if code, _, _ := e.do(anon, "POST", "/api/me/tokens", map[string]any{"name": "x"}, bearer(tok)...); code != 403 {
		t.Fatalf("token creating a token: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/me/mfa/setup", nil, bearer(tok)...); code != 403 {
		t.Fatalf("token touching mfa: %d", code)
	}
	// a token without a role acts as the user (owner here)
	_, full, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "admin script"})
	if code, _, list := e.do(anon, "GET", "/api/projects", nil, bearer(full["token"].(string))...); code != 200 || len(list) != 2 {
		t.Fatalf("full token: %d %v", code, list)
	}
	// a project-only token without a role: project permissions there, nothing global
	_, scoped, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "two only", "project_id": p2id})
	if code, _, _ := e.do(anon, "GET", "/api/projects/"+itoa(p2id)+"/apps", nil, bearer(scoped["token"].(string))...); code != 200 {
		t.Fatalf("scoped token in its project: %d", code)
	}
	if code, _, _ := e.do(anon, "GET", "/api/server/nodes", nil, bearer(scoped["token"].(string))...); code != 403 {
		t.Fatalf("scoped token on the server page: %d", code)
	}

	// list, last use, revoke
	_, _, mine := e.do(o, "GET", "/api/me/tokens", nil)
	if len(mine) != 3 || mine[2].(map[string]any)["last_used_at"] == nil || mine[2].(map[string]any)["role_name"] != "Deployer" {
		t.Fatalf("list: %v", mine)
	}
	id := itoa(created["api_token"].(map[string]any)["id"].(float64))
	if code, _, _ := e.do(o, "DELETE", "/api/me/tokens/"+id, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _, _ := e.do(anon, "GET", "/api/me", nil, bearer(tok)...); code != 401 {
		t.Fatalf("revoked token still works: %d", code)
	}
	if code, _, all := e.do(o, "GET", "/api/tokens", nil); code != 200 || len(all) != 2 {
		t.Fatalf("all tokens: %d %v", code, all)
	}
	if code, _, _ := e.do(anon, "GET", "/api/me", nil, bearer("nat_bogus")...); code != 401 {
		t.Fatalf("bogus token: %d", code)
	}
}

func TestProjectMembers(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	roles := roleIDs(t, e, o)
	_, p1, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "one", "name": "One"})
	pid := itoa(p1["id"].(float64))
	_, lead, _ := e.do(o, "POST", "/api/users", map[string]string{"email": "lead@x.io", "name": "Lead", "password": "lead-password1"})
	_, dev, _ := e.do(o, "POST", "/api/users", map[string]string{"email": "dev@x.io", "name": "Dev", "password": "dev-password1"})
	leadID, devID := itoa(lead["id"].(float64)), itoa(dev["id"].(float64))

	// a custom role "Lead" with project.members + developer rights, bound to project one
	code, lr, _ := e.do(o, "POST", "/api/roles", map[string]any{"name": "Lead", "permissions": []string{"project.view", "project.members", "app.view", "app.deploy"}})
	if code != 201 {
		t.Fatalf("role: %d %v", code, lr)
	}
	if code, _, _ := e.do(o, "PUT", "/api/projects/"+pid+"/members/"+leadID, map[string]any{"role_ids": []float64{lr["id"].(float64)}}); code != 204 {
		t.Fatalf("add lead: %d", code)
	}
	lc := e.client()
	e.do(lc, "POST", "/api/auth/login", map[string]string{"email": "lead@x.io", "password": "lead-password1"})
	if code, _, list := e.do(lc, "GET", "/api/users/directory", nil); code != 200 || len(list) != 3 {
		t.Fatalf("directory: %d %v", code, list)
	}
	// the lead can add the viewer role (it holds those permissions in the project?) — viewer has app.logs, which the lead lacks
	if code, _, _ := e.do(lc, "PUT", "/api/projects/"+pid+"/members/"+devID, map[string]any{"role_ids": []float64{roles["viewer"]}}); code != 403 {
		t.Fatalf("granting more than you hold: %d", code)
	}
	if code, _, _ := e.do(lc, "PUT", "/api/projects/"+pid+"/members/"+devID, map[string]any{"role_ids": []float64{lr["id"].(float64)}}); code != 204 {
		t.Fatalf("lead adds dev: %d", code)
	}
	// global bindings are kept when project roles change
	e.do(o, "PUT", "/api/users/"+devID+"/bindings", []map[string]any{{"role_id": roles["viewer"]}, {"role_id": lr["id"], "project_id": p1["id"]}})
	e.do(lc, "DELETE", "/api/projects/"+pid+"/members/"+devID, nil)
	_, _, bs := e.do(o, "GET", "/api/users/"+devID+"/bindings", nil)
	if len(bs) != 1 || bs[0].(map[string]any)["project_id"] != nil {
		t.Fatalf("bindings after removal: %v", bs)
	}
	_, _, members := e.do(lc, "GET", "/api/projects/"+pid+"/members", nil)
	if len(members) != 3 || members[0].(map[string]any)["is_owner"] != true {
		t.Fatalf("members: %v", members)
	}
	// someone without project.members
	dc := e.client()
	e.do(dc, "POST", "/api/auth/login", map[string]string{"email": "dev@x.io", "password": "dev-password1"})
	if code, _, _ := e.do(dc, "PUT", "/api/projects/"+pid+"/members/"+leadID, map[string]any{"role_ids": []float64{}}); code != 403 {
		t.Fatalf("viewer edits members: %d", code)
	}
	if code, _, _ := e.do(dc, "GET", "/api/users/directory", nil); code != 403 {
		t.Fatalf("viewer directory: %d", code)
	}
}

func TestGatewayBlockAndRateLimit(t *testing.T) {
	e, fg := newEnvGW(t)
	o := e.owner()
	if code, _, _ := e.do(o, "POST", "/api/gateway/blocked", map[string]any{"ip": "10.0.0.7"}); code != 400 {
		t.Fatalf("private ip: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/gateway/blocked", map[string]any{"ip": "203.0.113.9", "ttl_seconds": 3600}); code != 204 {
		t.Fatalf("block: %d", code)
	}
	if len(fg.block) != 1 || fg.block[0]["ip"] != "203.0.113.9" || !strings.Contains(fg.block[0]["reason"].(string), "o@x.io") {
		t.Fatalf("gateway got %v", fg.block)
	}
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": "nginx:1", "port": 80}})
	aid := itoa(app["id"].(float64))
	e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil)
	e.dep.Wait()
	if code, _, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "r.example.com", "rate_limit": -1}); code != 400 {
		t.Fatalf("negative rate limit: %d", code)
	}
	if code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "r.example.com", "rate_limit": 2}); code != 400 || !strings.Contains(v["error"].(string), "24 hours") {
		t.Fatalf("rate limit below the minimum: %d %v", code, v)
	}
	// a gateway that answers 429 over the limit accepts low limits
	fg.mu.Lock()
	fg.features = []string{"rate-limit-429"}
	fg.mu.Unlock()
	if code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "low.example.com", "rate_limit": 2}); code != 201 {
		t.Fatalf("low rate limit with rate-limit-429: %d %v", code, v)
	}
	fg.mu.Lock()
	fg.features = nil
	fg.mu.Unlock()
	code, d, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "r.example.com", "rate_limit": 50})
	if code != 201 || d["rate_limit"].(float64) != 50 {
		t.Fatalf("domain: %d %v", code, d)
	}
	if rt := fg.byDomain("r.example.com"); rt == nil || rt["rate_limit"].(float64) != 50 {
		t.Fatalf("route %v", rt)
	}
	e.do(o, "PATCH", "/api/apps/"+aid+"/domains/"+itoa(d["id"].(float64)), map[string]any{"rate_limit": 0})
	if rt := fg.byDomain("r.example.com"); rt["rate_limit"].(float64) != 0 {
		t.Fatalf("route after update %v", rt)
	}
	if code, _, list := e.do(o, "GET", "/api/gateway/routes", nil); code != 200 || len(list) < 2 {
		t.Fatalf("routes relay: %d %v", code, list)
	}
}
