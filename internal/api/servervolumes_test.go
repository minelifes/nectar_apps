package api

import (
	"context"
	"testing"
)

// The server's volume list shows the size from the settings, also when the storage class could not grow
// the claim (local-path keeps the size it was created with).
func TestServerVolumesSettingSize(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "loop", "name": "Loop"})
	code0, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "mariadb", "name": "MariaDB",
		"runtime": "image", "kind": "worker", "config": map[string]any{"image": "mariadb:11",
			"volumes": []any{map[string]any{"name": "mariadb-data", "mount_path": "/var/lib/mysql", "size_mb": 64}}}})
	if code0 != 201 {
		t.Fatalf("create: %d %v", code0, app)
	}
	id := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+id+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d", code)
	}
	e.dep.Wait()
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{
		"volumes": []any{map[string]any{"name": "mariadb-data", "mount_path": "/var/lib/mysql", "size_mb": 256}}}}); code != 200 {
		t.Fatalf("resize: %d", code)
	}
	// local-path can't grow it: the claim keeps 64Mi
	if _, err := e.kube.Patch(context.Background(), "PersistentVolumeClaim", "prj-loop", "mariadb-mariadb-data",
		map[string]any{"spec": map[string]any{"resources": map[string]any{"requests": map[string]any{"storage": "64Mi"}}}}); err != nil {
		t.Fatal(err)
	}
	code, _, list := e.do(o, "GET", "/api/server/volumes", nil)
	if code != 200 || len(list) != 1 {
		t.Fatalf("volumes: %d %v", code, list)
	}
	if v := list[0].(map[string]any); v["volume"] != "mariadb-data" || v["requested_bytes"].(float64) != 256<<20 {
		t.Fatalf("size: %v", v)
	}
}
