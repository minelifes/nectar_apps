package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/oci"
)

// The watcher: a tag that points to a newer image is announced (notify) or deployed pinned to its digest
// (auto) — once per digest; an app with undeployed changes is only announced; "Deploy" does it by hand.
func TestImageWatch(t *testing.T) {
	e := newEnv(t)
	var mu sync.Mutex
	digest := "sha256:" + strings.Repeat("a", 64)
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/v2/acme/web/manifests/1" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest)
	}))
	defer reg.Close()
	setDigest := func(c string) {
		mu.Lock()
		digest = "sha256:" + strings.Repeat(c, 64)
		mu.Unlock()
	}
	e.api.oci = &oci.Client{Scheme: "http"}
	var events []string
	e.api.imageEvent = func(_ context.Context, subject, msg, _ string) { events = append(events, subject+": "+msg) }
	image := strings.TrimPrefix(reg.URL, "http://") + "/acme/web:1"
	e.kube.ImageDigests = map[string]string{image: "sha256:" + strings.Repeat("a", 64)}

	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, app, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": image, "port": 80}})
	id := itoa(app["id"].(float64))
	base := "/api/apps/" + id + "/image-updates"
	set := func(policy string) int {
		code, _, _ := e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"image_updates": policy}})
		return code
	}
	pending := func() []any {
		_, st, _ := e.do(o, "GET", "/api/apps/"+id+"/status", nil)
		p, _ := st["pending"].([]any)
		return p
	}
	if code := set("sometimes"); code != 400 {
		t.Fatalf("bad policy: %d", code)
	}
	e.do(o, "POST", "/api/apps/"+id+"/deploy", nil)
	e.dep.Wait()
	if code, v, _ := e.do(o, "GET", base, nil); code != 200 || v["supported"] != true || v["policy"] != "" || v["watch"] != nil {
		t.Fatalf("initial: %d %v", code, v)
	}

	// notify: the setting needs no deploy
	if code := set("notify"); code != 200 || len(pending()) != 0 {
		t.Fatalf("notify: %d pending %v", code, pending())
	}
	if code, v, _ := e.do(o, "POST", base+"/check", nil); code != 200 || v["watch"].(map[string]any)["status"] != "current" {
		t.Fatalf("check, current: %d %v", code, v)
	}
	setDigest("b")
	e.api.watchRound(context.Background())
	e.api.watchRound(context.Background())
	if len(events) != 1 || !strings.Contains(events[0], "newer image of "+image) {
		t.Fatalf("announced once: %v", events)
	}
	if _, v, _ := e.do(o, "GET", base, nil); v["watch"].(map[string]any)["status"] != "update" ||
		v["watch"].(map[string]any)["latest_digest"] != "sha256:"+strings.Repeat("b", 64) {
		t.Fatalf("update: %v", v)
	}
	// checking by hand records but does not act
	setDigest("c")
	e.do(o, "POST", base+"/check", nil)
	if len(events) != 1 {
		t.Fatalf("a manual check announced: %v", events)
	}

	// auto: deployed pinned to the digest; afterwards the app is current and has no pending changes
	set("auto")
	e.api.watchRound(context.Background())
	e.dep.Wait()
	_, _, list := e.do(o, "GET", "/api/apps/"+id+"/releases", nil)
	if img := list[0].(map[string]any)["image"]; img != image+"@sha256:"+strings.Repeat("c", 64) {
		t.Fatalf("deployed image: %v", img)
	}
	if len(events) != 2 || !strings.Contains(events[1], "was updated") || len(pending()) != 0 {
		t.Fatalf("auto: %v pending %v", events, pending())
	}
	e.do(o, "POST", base+"/check", nil)
	if _, v, _ := e.do(o, "GET", base, nil); v["watch"].(map[string]any)["status"] != "current" {
		t.Fatalf("after the update: %v", v)
	}

	// auto, but undeployed changes: only announced (pending changes are compared by the second)
	time.Sleep(1100 * time.Millisecond)
	e.do(o, "PUT", "/api/apps/"+id+"/env", []map[string]any{{"key": "MODE", "value": "new"}})
	setDigest("d")
	e.api.watchRound(context.Background())
	e.dep.Wait()
	if _, _, list := e.do(o, "GET", "/api/apps/"+id+"/releases", nil); len(list) != 2 {
		t.Fatalf("deployed despite pending changes: %d releases", len(list))
	}
	if len(events) != 3 || !strings.Contains(events[2], "undeployed changes (environment)") {
		t.Fatalf("pending: %v", events)
	}
	// by hand: deployed (with the pending changes)
	if code, rel, _ := e.do(o, "POST", base+"/deploy", nil); code != 202 || rel["image"] != image+"@sha256:"+strings.Repeat("d", 64) {
		t.Fatalf("deploy by hand: %d %v", code, rel)
	}
	e.dep.Wait()
	if _, _, list := e.do(o, "GET", "/api/audit?limit=50", nil); !strings.Contains(stringsOf(list), "app.image_update") {
		t.Fatal("no audit entry")
	}

	// apps that can't be watched say why
	_, pinned, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "pinned", "name": "Pinned", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami@sha256:" + strings.Repeat("e", 64), "port": 80}})
	if _, v, _ := e.do(o, "GET", "/api/apps/"+itoa(pinned["id"].(float64))+"/image-updates", nil); v["supported"] != false ||
		!strings.Contains(v["reason"].(string), "pinned") {
		t.Fatalf("pinned: %v", v)
	}
	if code, _, _ := e.do(o, "POST", "/api/apps/"+itoa(pinned["id"].(float64))+"/image-updates/check", nil); code != 409 {
		t.Fatalf("check pinned: %d", code)
	}
}
