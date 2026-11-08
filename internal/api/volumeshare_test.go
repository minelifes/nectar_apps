package api

import (
	"strings"
	"testing"
)

// Apps of a project share a volume: the claim, size and class come from the owner; the owner can't drop,
// delete or lose it (app delete) while another app mounts it; its size changes follow.
func TestSharedVolumes(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	mk := func(slug string, vols []any) (int, map[string]any) {
		code, a, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": slug, "name": slug, "runtime": "image",
			"config": map[string]any{"image": "nginx:1", "port": 80, "volumes": vols}})
		return code, a
	}
	vol := func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	code, web := mk("web", []any{vol("name", "uploads", "mount_path", "/srv/uploads", "size_mb", 512, "storage_class", "local-path")})
	if code != 201 {
		t.Fatalf("owner: %d %v", code, web)
	}
	webID := itoa(web["id"].(float64))

	for _, bad := range []map[string]any{
		vol("name", "u", "mount_path", "/u", "from", "nope/uploads"),
		vol("name", "u", "mount_path", "/u", "from", "web/missing"),
		vol("name", "u", "mount_path", "/u", "from", "web"),
		vol("name", "u", "mount_path", "/u", "size_mb", 512, "read_only", true),
	} {
		if code, a := mk("bad", []any{bad}); code != 400 {
			t.Fatalf("%v: %d %v", bad, code, a)
		}
	}

	code, worker := mk("worker", []any{vol("name", "media", "mount_path", "/media", "from", "web/uploads", "read_only", true)})
	if code != 201 {
		t.Fatalf("consumer: %d %v", code, worker)
	}
	wv := worker["config"].(map[string]any)["volumes"].([]any)[0].(map[string]any)
	if wv["claim"] != "web-uploads" || wv["size_mb"].(float64) != 512 || wv["storage_class"] != "local-path" || wv["read_only"] != true {
		t.Fatalf("resolved volume: %v", wv)
	}
	workerID := itoa(worker["id"].(float64))

	// no chains, no mounting itself
	if code, _ := mk("third", []any{vol("name", "m", "mount_path", "/m", "from", "worker/media")}); code != 400 {
		t.Fatalf("chain: %d", code)
	}
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID, map[string]any{"config": map[string]any{"volumes": []any{
		vol("name", "uploads", "mount_path", "/srv/uploads", "size_mb", 512, "storage_class", "local-path"),
		vol("name", "again", "mount_path", "/again", "from", "web/uploads")}}}); code != 400 {
		t.Fatalf("own volume twice: %d", code)
	}
	// an own volume can't turn into a shared one under the same name
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID, map[string]any{"config": map[string]any{"volumes": []any{
		vol("name", "uploads", "mount_path", "/srv/uploads", "from", "worker/media")}}}); code != 400 {
		t.Fatalf("own → shared: %d", code)
	}

	// the owner grows it: the consumer follows
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID, map[string]any{"config": map[string]any{"volumes": []any{
		vol("name", "uploads", "mount_path", "/srv/uploads", "size_mb", 2048, "storage_class", "local-path")}}}); code != 200 {
		t.Fatalf("grow: %d", code)
	}
	if _, w2, _ := e.do(o, "GET", "/api/apps/"+workerID, nil); w2["config"].(map[string]any)["volumes"].([]any)[0].(map[string]any)["size_mb"].(float64) != 2048 {
		t.Fatalf("consumer size: %v", w2["config"])
	}

	// the owner can't drop it, delete it or be deleted while it is mounted elsewhere
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID, map[string]any{"config": map[string]any{"volumes": []any{}}}); code != 409 {
		t.Fatalf("drop: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+webID+"/volumes/uploads", nil); code != 409 {
		t.Fatalf("delete volume: %d", code)
	}
	code, body, _ := e.do(o, "DELETE", "/api/apps/"+webID, nil)
	if code != 409 || !strings.Contains(body["error"].(string), "worker") {
		t.Fatalf("delete owner: %d %v", code, body)
	}

	// the consumer lets go: now it can
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+workerID, map[string]any{"config": map[string]any{"volumes": []any{}}}); code != 200 {
		t.Fatalf("consumer drops it: %d", code)
	}
	if code, _, _ := e.do(o, "PATCH", "/api/apps/"+webID, map[string]any{"config": map[string]any{"volumes": []any{}}}); code != 200 {
		t.Fatalf("drop after: %d", code)
	}
}
