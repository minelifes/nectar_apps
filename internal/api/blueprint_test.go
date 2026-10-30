package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"nectar_apps/internal/blueprint"
)

func stepsOf(v map[string]any) map[string]string {
	out := map[string]string{}
	if _, ok := v["steps"].([]any); !ok {
		return map[string]string{"error": fmt.Sprint(v)}
	}
	for _, s := range v["steps"].([]any) {
		m := s.(map[string]any)
		out[fmt.Sprint(m["kind"])+":"+fmt.Sprint(m["name"])] = fmt.Sprint(m["action"]) + " " + fmt.Sprint(m["message"])
	}
	return out
}

func TestTemplatesApplyExportCompose(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))

	code, _, list := e.do(o, "GET", "/api/templates", nil)
	if code != 200 || len(list) < 6 {
		t.Fatalf("templates: %d %d", code, len(list))
	}

	// one-click Grafana
	code, v, _ := e.do(o, "POST", "/api/projects/"+pid+"/templates/grafana", map[string]any{"name": "graf", "inputs": map[string]string{"admin_user": "boss"}})
	st := stepsOf(v)
	if code != 200 || v["failed"].(float64) != 0 || !strings.HasPrefix(st["app:graf"], "created") || !strings.HasPrefix(st["deploy:graf"], "deployed") || v["app_id"].(float64) == 0 {
		t.Fatalf("install grafana: %d %v", code, v)
	}
	e.dep.Wait()
	aid := itoa(v["app_id"].(float64))
	_, _, env := e.do(o, "GET", "/api/apps/"+aid+"/env", nil)
	found := map[string]bool{}
	for _, x := range env {
		m := x.(map[string]any)
		found[m["key"].(string)] = m["secret"] == true
	}
	if !found["GF_SECURITY_ADMIN_PASSWORD"] || found["GF_SECURITY_ADMIN_USER"] {
		t.Fatalf("grafana env: %v", env)
	}
	// the same name again: refused before anything is created
	if code, _, _ := e.do(o, "POST", "/api/projects/"+pid+"/templates/grafana", map[string]any{"name": "graf"}); code != 409 {
		t.Fatalf("duplicate install: %d", code)
	}

	// WordPress: a database add-on and references to it, resolved at deploy
	code, v, _ = e.do(o, "POST", "/api/projects/"+pid+"/templates/wordpress", map[string]any{"name": "blog"})
	st = stepsOf(v)
	if code != 200 || v["failed"].(float64) != 0 || !strings.HasPrefix(st["addon:blog-db"], "created") || !strings.HasPrefix(st["deploy:blog"], "deployed") {
		t.Fatalf("install wordpress: %d %v", code, v)
	}
	e.dep.Wait()
	sec := e.kube.Get("Secret", "prj-shop", "blog-env")
	if sec == nil {
		t.Fatal("no env secret for blog")
	}

	// export → nectar.yaml without secret values
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/projects/"+pid+"/export", nil)
	resp, err := o.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	y, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	b, err := blueprint.Parse(y)
	if err != nil || len(b.Apps) != 2 || len(b.Addons) != 1 {
		t.Fatalf("export: %v\n%s", err, y)
	}
	if !strings.Contains(string(y), "GF_SECURITY_ADMIN_PASSWORD: \"\"") {
		t.Fatalf("secret values must not be exported:\n%s", y)
	}

	// apply the file with a change: the app is updated, the secret keeps its value
	changed := strings.Replace(string(y), "GF_SECURITY_ADMIN_USER: boss", "GF_SECURITY_ADMIN_USER: chief", 1)
	req, _ = http.NewRequest("POST", e.srv.URL+"/api/projects/"+pid+"/apply?deploy=0", bytes.NewReader([]byte(changed)))
	resp, err = o.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("apply: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	_, _, env = e.do(o, "GET", "/api/apps/"+aid+"/env?reveal=1", nil)
	vals := map[string]string{}
	for _, x := range env {
		m := x.(map[string]any)
		vals[m["key"].(string)], _ = m["value"].(string)
	}
	if vals["GF_SECURITY_ADMIN_USER"] != "chief" || len(vals["GF_SECURITY_ADMIN_PASSWORD"]) != 24 {
		t.Fatalf("after apply: %v", vals)
	}

	// docker-compose: preview, then import
	compose := `services:
  api:
    image: ghcr.io/acme/api:2
    ports: ["8000:8000"]
    environment: { CACHE: redis://cache:6379 }
  cache:
    image: redis:7
`
	code, v, _ = e.do(o, "POST", "/api/projects/"+pid+"/import/compose", map[string]any{"compose": compose, "dry_run": true})
	if code != 200 || !strings.Contains(v["yaml"].(string), "slug: cache") {
		t.Fatalf("compose preview: %d %v", code, v)
	}
	code, v, _ = e.do(o, "POST", "/api/projects/"+pid+"/import/compose", map[string]any{"compose": compose})
	st = stepsOf(v)
	if code != 200 || v["failed"].(float64) != 0 || !strings.HasPrefix(st["app:cache"], "created") || !strings.HasPrefix(st["deploy:api"], "deployed") {
		t.Fatalf("compose import: %d %v", code, v)
	}
	e.dep.Wait()
	if svc := e.kube.Get("Service", "prj-shop", "cache"); svc == nil {
		t.Fatal("the redis worker has no internal Service")
	}

	// a viewer cannot apply anything
	if code, _, _ := e.do(e.client(), "POST", "/api/projects/"+pid+"/apply", map[string]any{"blueprint": "apps: []"}); code != 401 {
		t.Fatalf("anonymous apply: %d", code)
	}
}
