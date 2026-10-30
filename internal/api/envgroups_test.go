package api

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
)

func TestEnvGroupsCloneAndPromote(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, app, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"image": "ghcr.io/acme/api:1", "port": 8080}})
	aid := itoa(app["id"].(float64))

	// a shared group, attached to the app; the app's own variable wins
	if code, _, _ := e.do(o, "PUT", "/api/projects/"+pid+"/env-groups/shared", []map[string]any{
		{"key": "REGION", "value": "eu"}, {"key": "SENTRY_DSN", "value": "https://key@sentry", "secret": true}, {"key": "LOG", "value": "info"}}); code != 200 {
		t.Fatalf("save group: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+aid+"/env-groups", []string{"nope"}); code != 400 {
		t.Fatalf("unknown group: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+aid+"/env-groups", []string{"shared"}); code != 200 {
		t.Fatalf("attach: %d", code)
	}
	e.do(o, "PUT", "/api/apps/"+aid+"/env", []map[string]any{{"key": "LOG", "value": "debug"}, {"key": "WHERE", "value": "${shared.REGION}"}})
	if code, _, _ := e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d", code)
	}
	e.dep.Wait()
	sec := e.kube.Get("Secret", "prj-shop", "api-env")
	val := func(k string) string {
		b, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", k))
		return string(b)
	}
	if val("REGION") != "eu" || val("SENTRY_DSN") != "https://key@sentry" || val("LOG") != "debug" || val("WHERE") != "eu" {
		t.Fatalf("env secret: %v", sec["data"])
	}
	// listing hides secrets unless revealed; deleting a used group is refused
	_, _, groups := e.do(o, "GET", "/api/projects/"+pid+"/env-groups", nil)
	g := groups[0].(map[string]any)
	if g["apps"].([]any)[0] != "api" {
		t.Fatalf("group apps: %v", g)
	}
	for _, v := range g["vars"].([]any) {
		m := v.(map[string]any)
		if m["key"] == "SENTRY_DSN" && m["value"] != nil {
			t.Fatal("secret value listed without reveal")
		}
	}
	if code, _, _ := e.do(o, "DELETE", "/api/projects/"+pid+"/env-groups/shared", nil); code != 409 {
		t.Fatalf("delete used group: %d", code)
	}
	// changing the group makes the app pending again (times have second precision)
	time.Sleep(1100 * time.Millisecond)
	e.do(o, "PUT", "/api/projects/"+pid+"/env-groups/shared", []map[string]any{{"key": "REGION", "value": "us"}, {"key": "SENTRY_DSN", "secret": true}})
	if _, st, _ := e.do(o, "GET", "/api/apps/"+aid+"/status", nil); len(st["pending"].([]any)) == 0 {
		t.Fatalf("not pending after a group change: %v", st)
	}

	// clone the app in the same project
	code, c, _ := e.do(o, "POST", "/api/apps/"+aid+"/clone", map[string]any{"slug": "api2"})
	if code != 201 || c["app"].(map[string]any)["config"].(map[string]any)["env_groups"].([]any)[0] != "shared" {
		t.Fatalf("clone app: %d %v", code, c)
	}

	// staging copy of the project, then promote the live build back to it
	code, pc, _ := e.do(o, "POST", "/api/projects/"+pid+"/clone", map[string]any{"slug": "shop-staging", "environment": "staging", "source_environment": "production"})
	if code != 201 {
		t.Fatalf("clone project: %d %v", code, pc)
	}
	st := pc["project"].(map[string]any)
	if st["environment"] != "staging" || st["origin_id"].(float64) != proj["id"].(float64) || st["apps"].(float64) != 2 {
		t.Fatalf("staging project: %v", st)
	}
	spid := itoa(st["id"].(float64))
	_, _, sg := e.do(o, "GET", "/api/projects/"+spid+"/env-groups?reveal=1", nil)
	if len(sg) != 1 {
		t.Fatalf("groups not copied: %v", sg)
	}
	// promote shop/api (live: ghcr.io/acme/api:1) to the staging app
	code, pr, _ := e.do(o, "POST", "/api/apps/"+aid+"/promote", map[string]any{"project_id": st["id"]})
	if code != 202 || pr["build"] != "ghcr.io/acme/api:1" {
		t.Fatalf("promote: %d %v", code, pr)
	}
	e.dep.Wait()
	if d := e.kube.Get("Deployment", "prj-shop-staging", "api"); d == nil || !strings.Contains(kube.Str(kube.Slice(d, "spec", "template", "spec", "containers")[0], "image"), "ghcr.io/acme/api:1") {
		t.Fatalf("staging deployment: %v", d)
	}
	_, src, _ := e.do(o, "GET", "/api/projects/"+pid, nil)
	if src["environment"] != "production" {
		t.Fatalf("source environment: %v", src)
	}
}
