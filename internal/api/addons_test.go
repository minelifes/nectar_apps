package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/wsock"
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
	// a slow first start (small CPU limit) is not restarted halfway: startup probe first, liveness after
	if c := kube.Slice(ss, "spec", "template", "spec", "containers")[0]; kube.Int(c, "startupProbe", "failureThreshold") != 60 ||
		kube.Int(c, "livenessProbe", "initialDelaySeconds") != 0 || kube.Str(c, "readinessProbe", "tcpSocket", "port") != "db" ||
		kube.Field(c, "readinessProbe", "exec") != nil {
		t.Fatalf("probes: %v", c)
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

// Databases get the app observe endpoints (usage, pods, logs, events, terminal) and can be shared with the
// whole project instead of linked per app.
func TestDatabaseObserveAndShare(t *testing.T) {
	e := newEnv(t)
	_, v, _ := e.do(e.client(), "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	tok := v["access_token"].(string)
	o := &http.Client{}
	auth := []string{"Authorization", "Bearer " + tok}
	e.do(o, "POST", "/api/projects", map[string]string{"slug": "shop", "name": "Shop"}, auth...)
	for _, slug := range []string{"api", "web"} {
		e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": slug, "name": slug, "runtime": "image",
			"config": map[string]any{"image": "nginx:1.29", "port": 80, "replicas": 1}}, auth...)
	}
	code, db, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "postgres", "slug": "db"}, auth...)
	if code != 201 {
		t.Fatalf("create: %d %v", code, db)
	}
	id := itoa(db["id"].(float64))

	// observe: same shapes as for apps
	if code, u, _ := e.do(o, "GET", "/api/addons/"+id+"/usage?range=1h", nil, auth...); code != 200 || u["request_mem_b"].(float64) <= 0 || u["replicas"].(float64) != 1 {
		t.Fatalf("usage: %d %v", code, u)
	}
	if code, _, pods := e.do(o, "GET", "/api/addons/"+id+"/pods", nil, auth...); code != 200 || len(pods) != 1 || pods[0].(map[string]any)["name"] != "db-0" {
		t.Fatalf("pods: %d %v", code, pods)
	}
	if code, _, lines := e.do(o, "GET", "/api/addons/"+id+"/logs?tail=10", nil, auth...); code != 200 || len(lines) != 1 ||
		!strings.Contains(lines[0].(map[string]any)["line"].(string), "ready to accept") {
		t.Fatalf("logs: %d %v", code, lines)
	}
	if code, _, _ := e.do(o, "GET", "/api/addons/"+id+"/events", nil, auth...); code != 200 {
		t.Fatalf("events: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/addons/"+id+"/restart", nil, auth...); code != 204 ||
		kube.Str(e.kube.Get("StatefulSet", "prj-shop", "db"), "spec", "template", "metadata", "annotations", "nectar.dev/restarted-at") == "" {
		t.Fatalf("restart: %d", code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := wsock.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/addons/"+id+"/exec/ws?access_token="+tok, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, b, _ := ws.ReadMessage()
	ws.Close()
	var m termMsg
	json.Unmarshal(b, &m)
	if m.T != "s" || m.Pod != "db-0" {
		t.Fatalf("terminal: %+v", m)
	}

	// shared with the project: every app gets the variables, no link needed
	if code, _, _ := e.do(o, "PUT", "/api/addons/"+id+"/shared", map[string]any{"shared": true, "prefix": "bad"}, auth...); code != 400 {
		t.Fatalf("bad prefix: %d", code)
	}
	if code, v, _ := e.do(o, "PUT", "/api/addons/"+id+"/shared", map[string]any{"shared": true}, auth...); code != 200 || v["shared"] != true {
		t.Fatalf("share: %d %v", code, v)
	}
	if code, _, list := e.do(o, "GET", "/api/apps/2/addons", nil, auth...); code != 200 || len(list) != 1 || list[0].(map[string]any)["shared"] != true ||
		!strings.Contains(strings.Join(toStrings(list[0].(map[string]any)["env"]), ","), "DATABASE_URL") {
		t.Fatalf("app sees the shared database: %d %v", code, list)
	}
	// a second shared postgres would set DATABASE_URL again: needs a prefix
	_, db2, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "postgres", "slug": "analytics"}, auth...)
	id2 := itoa(db2["id"].(float64))
	if code, v, _ := e.do(o, "PUT", "/api/addons/"+id2+"/shared", map[string]any{"shared": true}, auth...); code != 409 || !strings.Contains(v["error"].(string), "ANALYTICS_") {
		t.Fatalf("clash: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "PUT", "/api/addons/"+id2+"/shared", map[string]any{"shared": true, "prefix": "ANALYTICS_"}, auth...); code != 200 {
		t.Fatalf("share with prefix: %d", code)
	}
	// an own link wins over the shared variables (its prefix)
	e.do(o, "POST", "/api/addons/"+id+"/links", map[string]any{"app_id": 1, "prefix": "MAIN_"}, auth...)
	e.do(o, "POST", "/api/apps/1/deploy", nil, auth...)
	e.do(o, "POST", "/api/apps/2/deploy", nil, auth...)
	e.dep.Wait()
	has := func(app, key string) bool {
		_, ok := kube.Field(e.kube.Get("Secret", "prj-shop", app+"-env"), "data", key).(string)
		return ok
	}
	if !has("api", "MAIN_DATABASE_URL") || has("api", "DATABASE_URL") || !has("api", "ANALYTICS_DATABASE_URL") {
		t.Fatalf("api env: %v", e.kube.Get("Secret", "prj-shop", "api-env"))
	}
	if !has("web", "DATABASE_URL") || !has("web", "ANALYTICS_DATABASE_URL") {
		t.Fatalf("web env: %v", e.kube.Get("Secret", "prj-shop", "web-env"))
	}
	// stop sharing: the apps have a pending environment change
	time.Sleep(1100 * time.Millisecond) // change times have second resolution
	if code, _, _ := e.do(o, "PUT", "/api/addons/"+id2+"/shared", map[string]any{"shared": false}, auth...); code != 200 {
		t.Fatalf("unshare: %d", code)
	}
	if code, v, _ := e.do(o, "GET", "/api/apps/2/status", nil, auth...); code != 200 || !strings.Contains(strings.Join(toStrings(v["pending"]), ","), "environment") {
		t.Fatalf("pending after unshare: %v", v)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
