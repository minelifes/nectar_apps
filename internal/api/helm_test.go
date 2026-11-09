package api

import (
	"testing"
	"time"

	"nectar_apps/internal/helm"
	"nectar_apps/internal/kubefake"
)

// Helm charts in a project: install (background Job), values kept sealed and shown only with
// helm.manage, upgrade, rollback needs a known revision, a failed install is reported, uninstall.
func TestHelmReleases(t *testing.T) {
	e := newEnv(t)
	fk := kubefake.New()
	e.api.helmRunner = &helm.Runner{Kube: fk, Poll: time.Millisecond}
	o := e.owner()
	roles := roleIDs(t, e, o)
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	base := "/api/projects/" + pid + "/helm"

	for _, bad := range []map[string]any{
		{"name": "Web", "repo": "https://charts.bitnami.com/bitnami", "chart": "nginx"},
		{"name": "web", "repo": "ftp://x", "chart": "nginx"},
		{"name": "web", "repo": "https://charts.bitnami.com/bitnami", "chart": "nginx", "values": "- a\n- b\n"},
	} {
		if code, _, _ := e.do(o, "POST", base, bad); code != 400 {
			t.Fatalf("%v: %d", bad, code)
		}
	}
	code, rel, _ := e.do(o, "POST", base, map[string]any{"name": "web", "repo": "https://charts.bitnami.com/bitnami/", "chart": "nginx",
		"version": "18.2.0", "values": "replicaCount: 2\n", "repo_username": "me", "repo_password": "secret"})
	if code != 202 || rel["status"] != "installing" || rel["repo"] != "https://charts.bitnami.com/bitnami" || rel["has_password"] != true {
		t.Fatalf("install: %d %v", code, rel)
	}
	if code, _, _ := e.do(o, "POST", base, map[string]any{"name": "web", "repo": "https://x.example.com", "chart": "nginx"}); code != 409 {
		t.Fatalf("same name: %d", code)
	}
	e.api.helmWG.Wait()
	_, d, _ := e.do(o, "GET", base+"/web", nil)
	if d["release"].(map[string]any)["status"] != "deployed" || d["values"] != "replicaCount: 2\n" {
		t.Fatalf("detail: %v", d)
	}
	if _, ok := d["release"].(map[string]any)["repo_password"]; ok {
		t.Fatal("password returned")
	}

	// a viewer sees the release, not its values, and can't change it
	_, tok, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "view", "role_id": roles["viewer"], "project_id": proj["id"]})
	vt := bearer(tok["token"].(string))
	anon := e.client()
	if _, _, list := e.do(anon, "GET", base, nil, vt...); len(list) != 1 {
		t.Fatalf("viewer list: %v", list)
	}
	if _, vd, _ := e.do(anon, "GET", base+"/web", nil, vt...); vd["values"] != nil {
		t.Fatalf("viewer sees values: %v", vd["values"])
	}
	if code, _, _ := e.do(anon, "PUT", base+"/web", map[string]any{"version": "19.0.0"}, vt...); code != 403 {
		t.Fatalf("viewer upgrade: %d", code)
	}

	// upgrade keeps the password when none is sent
	if code, up, _ := e.do(o, "PUT", base+"/web", map[string]any{"version": "19.0.0", "values": "replicaCount: 3\n"}); code != 202 || up["version"] != "19.0.0" {
		t.Fatalf("upgrade: %d %v", code, up)
	}
	e.api.helmWG.Wait()
	_, d, _ = e.do(o, "GET", base+"/web", nil)
	if r := d["release"].(map[string]any); r["status"] != "deployed" || r["has_password"] != true || d["values"] != "replicaCount: 3\n" {
		t.Fatalf("after upgrade: %v", d)
	}
	if code, _, _ := e.do(o, "POST", base+"/web/rollback", map[string]any{"revision": 7}); code != 400 {
		t.Fatalf("rollback to an unknown revision: %d", code)
	}

	// a failing job: failed with helm's message
	fk.JobOK = false
	e.do(o, "PUT", base+"/web", map[string]any{"version": "20.0.0"})
	e.api.helmWG.Wait()
	_, d, _ = e.do(o, "GET", base+"/web", nil)
	if r := d["release"].(map[string]any); r["status"] != "failed" || r["message"] == "" {
		t.Fatalf("failed upgrade: %v", r)
	}

	// uninstall: a failed one keeps the release, a successful one removes it
	if code, _, _ := e.do(o, "DELETE", base+"/web", nil); code != 202 {
		t.Fatalf("delete: %d", code)
	}
	e.api.helmWG.Wait()
	if _, _, list := e.do(o, "GET", base, nil); len(list) != 1 || list[0].(map[string]any)["status"] != "failed" {
		t.Fatalf("after a failed uninstall: %v", list)
	}
	fk.JobOK = true
	e.do(o, "DELETE", base+"/web", nil)
	e.api.helmWG.Wait()
	if _, _, list := e.do(o, "GET", base, nil); len(list) != 0 {
		t.Fatalf("after delete: %v", list)
	}
}
