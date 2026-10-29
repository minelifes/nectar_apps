package api

import (
	"testing"

	"nectar_apps/internal/kube"
)

func TestScaleFilesVolumes(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	e.do(o, "POST", "/api/projects", map[string]any{"slug": "web", "name": "Web"})
	code, app, _ := e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": "img", "name": "Img", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80, "replicas": 1,
			"volumes": []any{map[string]any{"name": "data", "mount_path": "/data", "size_mb": 512}}}})
	if code != 201 {
		t.Fatalf("create: %d %v", code, app)
	}
	id := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/scale", map[string]any{"replicas": 2}); code != 409 {
		t.Fatalf("scale before deploy: %d", code)
	}

	// config files: validation, secret content hidden, kept when omitted
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+id+"/files", []any{map[string]any{"path": "etc/x", "content": "a"}}); code != 400 {
		t.Fatalf("relative path: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+id+"/files", []any{map[string]any{"path": "/data", "content": "a"}}); code != 400 {
		t.Fatalf("volume path: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+id+"/files", []any{
		map[string]any{"path": "/config/app.yml", "content": "a: 1\n"},
		map[string]any{"path": "/config/key.pem", "content": "SECRET", "secret": true},
	}); code != 204 {
		t.Fatalf("put files: %d", code)
	}
	_, _, files := e.do(o, "GET", "/api/apps/"+id+"/files", nil)
	if len(files) != 2 || files[1].(map[string]any)["content"] != nil || files[1].(map[string]any)["size"].(float64) != 6 {
		t.Fatalf("files: %v", files)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+id+"/files", []any{map[string]any{"path": "/config/key.pem", "secret": true}}); code != 204 {
		t.Fatalf("keep secret: %d", code)
	}
	if _, _, files = e.do(o, "GET", "/api/apps/"+id+"/files?reveal=1", nil); len(files) != 1 || files[0].(map[string]any)["content"] != "SECRET" {
		t.Fatalf("revealed: %v", files)
	}

	// deploy, then scale / stop / start / restart live
	if code, v, _ := e.do(o, "POST", "/api/apps/"+id+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d %v", code, v)
	}
	e.dep.Wait()
	if !e.kube.Has("PersistentVolumeClaim", "prj-web", "img-data") || !e.kube.Has("Secret", "prj-web", "img-userfiles") {
		t.Fatal("volume or files not created")
	}
	if code, v, _ := e.do(o, "POST", "/api/apps/"+id+"/scale", map[string]any{"replicas": 3}); code != 200 || v["config"].(map[string]any)["replicas"].(float64) != 3 {
		t.Fatalf("scale: %d %v", code, v)
	}
	if kube.Int(e.kube.Get("Deployment", "prj-web", "img"), "spec", "replicas") != 3 {
		t.Fatal("deployment not scaled")
	}
	if code, v, _ := e.do(o, "POST", "/api/apps/"+id+"/stop", nil); code != 200 || v["status"] != "stopped" {
		t.Fatalf("stop: %d %v", code, v)
	}
	if code, v, _ := e.do(o, "POST", "/api/apps/"+id+"/start", nil); code != 200 || v["config"].(map[string]any)["replicas"].(float64) != 3 {
		t.Fatalf("start: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/restart", nil); code != 204 {
		t.Fatalf("restart: %d", code)
	}

	// volumes: shrinking is refused, removing keeps the PVC until it is deleted
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"volumes": []any{map[string]any{"name": "data", "mount_path": "/data", "size_mb": 256}}}}); code != 400 {
		t.Fatalf("shrink: %d", code)
	}
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"volumes": []any{}}}); code != 200 {
		t.Fatalf("remove volume: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+id+"/volumes/data", nil); code != 409 {
		t.Fatalf("delete mounted volume: %d", code)
	}
	if code, _, vols := e.do(o, "GET", "/api/apps/"+id+"/volumes", nil); code != 200 || len(vols) != 1 || vols[0].(map[string]any)["mounted"] != true {
		t.Fatalf("volumes: %d %v", code, vols)
	}
	if code, v, _ := e.do(o, "GET", "/api/apps/"+id+"/status", nil); code != 200 || len(v["pending"].([]any)) != 1 {
		t.Fatalf("pending: %d %v", code, v)
	}
}
