package api

import (
	"testing"
	"time"

	"nectar_apps/internal/logstore"
)

func TestLogHistory(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": "nginx:1", "port": 80}})
	aid := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "GET", "/api/apps/"+aid+"/logs/history", nil); code != 404 {
		t.Fatalf("without a store: %d", code)
	}
	ls, err := logstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.api.logs = ls
	now := time.Now().UTC()
	ls.Append(int64(app["id"].(float64)), []logstore.Line{
		{Pod: "web-1", Time: now.Add(-2 * time.Second), Line: "GET / 200"},
		{Pod: "web-1", Time: now.Add(-time.Second), Line: "panic: boom"},
	})
	code, v, _ := e.do(o, "GET", "/api/apps/"+aid+"/logs/history?q=PANIC", nil)
	if code != 200 || len(v["lines"].([]any)) != 1 || v["lines"].([]any)[0].(map[string]any)["line"] != "panic: boom" {
		t.Fatalf("search: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "GET", "/api/apps/"+aid+"/logs/history?q=(&regex=1", nil); code != 400 {
		t.Fatalf("bad regex: %d", code)
	}
	if code, _, _ := e.do(o, "GET", "/api/apps/"+aid+"/logs/history?from=yesterday", nil); code != 400 {
		t.Fatalf("bad time: %d", code)
	}
	if code, v, _ := e.do(o, "GET", "/api/apps/"+aid+"/logs/history?limit=1", nil); code != 200 || v["more"] != true || v["next"] == nil {
		t.Fatalf("paging: %d %v", code, v)
	}
}
