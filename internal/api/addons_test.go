package api

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
)

func TestDatabaseAddons(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	e.do(o, "POST", "/api/projects", map[string]any{"slug": "web", "name": "Web"})
	e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": "img", "name": "Img", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80, "replicas": 1}})

	if code, _, kinds := e.do(o, "GET", "/api/addon-kinds", nil); code != 200 || len(kinds) != 4 {
		t.Fatalf("catalog: %d %v", code, kinds)
	}
	if code, _, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "postgres", "slug": "img"}); code != 409 {
		t.Fatalf("slug of an app: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "mysql", "memory_mb": 128}); code != 400 {
		t.Fatalf("too little memory: %d", code)
	}
	code, db, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "postgres", "slug": "db", "size_mb": 1024})
	if code != 201 || db["version"] != "17" || db["host"] != "db" || db["port"].(float64) != 5432 || db["status"] != "running" {
		t.Fatalf("create: %d %v", code, db)
	}
	id := itoa(db["id"].(float64))
	ss := e.kube.Get("StatefulSet", "prj-web", "db")
	if ss == nil || kube.Str(kube.Slice(ss, "spec", "template", "spec", "containers")[0], "image") != "postgres:17-alpine" ||
		kube.Str(kube.Slice(ss, "spec", "volumeClaimTemplates")[0], "spec", "resources", "requests", "storage") != "1024Mi" {
		t.Fatalf("statefulset: %v", ss)
	}
	if !e.kube.Has("Service", "prj-web", "db") || !e.kube.Has("Secret", "prj-web", "db-credentials") {
		t.Fatal("service or secret missing")
	}
	if code, _, _ := e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": "db", "name": "X", "runtime": "image",
		"config": map[string]any{"image": "x:1", "port": 80}}); code != 409 {
		t.Fatalf("app named like an add-on: %d", code)
	}

	code, cred, _ := e.do(o, "GET", "/api/addons/"+id+"/credentials", nil)
	if code != 200 || cred["user"] != "app" || cred["database"] != "db" || len(cred["password"].(string)) < 20 {
		t.Fatalf("credentials: %d %v", code, cred)
	}
	url := cred["env"].(map[string]any)["DATABASE_URL"].(string)
	if !strings.HasPrefix(url, "postgres://app:") || !strings.HasSuffix(url, "@db:5432/db?sslmode=disable") {
		t.Fatalf("url %q", url)
	}

	// link with a prefix, deploy: the app gets the variables (its own variables would win)
	if code, _, _ := e.do(o, "POST", "/api/addons/"+id+"/links", map[string]any{"app_id": 1, "prefix": "bad"}); code != 400 {
		t.Fatalf("bad prefix: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/addons/"+id+"/links", map[string]any{"app_id": 1, "prefix": "MAIN_"}); code != 200 {
		t.Fatalf("link: %d", code)
	}
	if code, _, links := e.do(o, "GET", "/api/apps/1/addons", nil); code != 200 || len(links) != 1 ||
		!strings.Contains(strings.Join(toStrings(links[0].(map[string]any)["env"]), ","), "MAIN_DATABASE_URL") {
		t.Fatalf("app add-ons: %d %v", code, links)
	}
	e.do(o, "POST", "/api/apps/1/deploy", nil)
	e.dep.Wait()
	sec := e.kube.Get("Secret", "prj-web", "img-env")
	if v, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "MAIN_DATABASE_URL")); string(v) != url {
		t.Fatalf("linked env: %q", v)
	}

	// stop / start / delete
	if code, v, _ := e.do(o, "POST", "/api/addons/"+id+"/stop", nil); code != 200 || v["stopped"] != true ||
		kube.Int(e.kube.Get("StatefulSet", "prj-web", "db"), "spec", "replicas") != 0 {
		t.Fatalf("stop: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "POST", "/api/addons/"+id+"/start", nil); code != 200 || kube.Int(e.kube.Get("StatefulSet", "prj-web", "db"), "spec", "replicas") != 1 {
		t.Fatalf("start: %d", code)
	}
	if code, _, _ := e.do(o, "PATCH", "/api/addons/"+id, map[string]any{"size_mb": 512}); code != 400 {
		t.Fatalf("shrink: %d", code)
	}
	time.Sleep(1100 * time.Millisecond) // change times have second resolution
	if code, _, _ := e.do(o, "DELETE", "/api/addons/"+id, nil); code != 204 || e.kube.Has("StatefulSet", "prj-web", "db") {
		t.Fatalf("delete: %d", code)
	}
	if code, v, _ := e.do(o, "GET", "/api/apps/1/status", nil); code != 200 || !strings.Contains(strings.Join(toStrings(v["pending"]), ","), "environment") {
		t.Fatalf("pending after the linked add-on was deleted: %v", v)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
