package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/scan"
)

type scanKube struct {
	*kubefake.Fake
	logs string
}

func (k *scanKube) PodLogs(context.Context, string, string, string, int, bool) (string, error) {
	return k.logs, nil
}

func (k *scanKube) List(ctx context.Context, kind, ns, sel string) ([]kube.Object, error) {
	if kind == "Pod" {
		return []kube.Object{{"metadata": map[string]any{"name": "scan-pod", "namespace": ns}}}, nil
	}
	return k.Fake.List(ctx, kind, ns, sel)
}

// Scanning an app's image: needs a deploy, runs in the background, keeps counts and findings, reports a
// failure, marks the result stale after another image is deployed, and the re-scan loop picks it up.
func TestImageScan(t *testing.T) {
	e := newEnv(t)
	sk := &scanKube{Fake: kubefake.New(), logs: "NVSUM\t3\t1\t1\t1\t0\t0\t2\n" +
		"NV\tCRITICAL\tCVE-2026-0001\t\"openssl\"\t\"3.0.1\"\t\"3.0.9\"\t\"web (debian 12)\"\t\"openssl: rce\"\n" +
		"NV\tHIGH\tCVE-2026-0002\t\"zlib\"\t\"1.2.11\"\t\"\"\t\"web (debian 12)\"\t\"zlib: overflow\"\nNVDONE\n"}
	e.api.scanner = &scan.Scanner{Kube: sk, Poll: time.Millisecond}
	var events []string
	e.api.scanEvent = func(_ context.Context, subject, message, path string) {
		events = append(events, subject+"|"+path+"|"+message)
	}
	o := e.owner()
	roles := roleIDs(t, e, o)
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web",
		"runtime": "image", "config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	id := itoa(app["id"].(float64))
	base := "/api/apps/" + id + "/scan"

	if code, sc, _ := e.do(o, "GET", base, nil); code != 200 || sc["status"] != "none" || sc["current_image"] != "" {
		t.Fatalf("never scanned: %d %v", code, sc)
	}
	if code, _, _ := e.do(o, "POST", base, nil); code != 409 {
		t.Fatalf("scan before a deploy: %d", code)
	}
	e.do(o, "POST", "/api/apps/"+id+"/deploy", nil)
	e.dep.Wait()
	code, sc, _ := e.do(o, "POST", base, nil)
	if code != 202 || sc["status"] != "running" || sc["image"] != "traefik/whoami:v1.10" {
		t.Fatalf("start: %d %v", code, sc)
	}
	e.api.scanWG.Wait()
	_, sc, _ = e.do(o, "GET", base, nil)
	if sc["status"] != "done" || sc["total"].(float64) != 3 || sc["critical"].(float64) != 1 || sc["fixable"].(float64) != 2 || sc["stale"] != false ||
		len(sc["findings"].([]any)) != 2 || sc["findings"].([]any)[0].(map[string]any)["id"] != "CVE-2026-0001" {
		t.Fatalf("result: %v", sc)
	}

	// the critical finding is announced once: scanning the same image again with the same result is not news
	if len(events) != 1 || !strings.HasPrefix(events[0], "Web|/apps/"+id+"|") || !strings.Contains(events[0], "1 critical") {
		t.Fatalf("events: %v", events)
	}
	e.do(o, "POST", base, nil)
	e.api.scanWG.Wait()
	if len(events) != 1 {
		t.Fatalf("announced again: %v", events)
	}
	// the project's list of scans, for badges
	if code, _, list := e.do(o, "GET", "/api/projects/"+itoa(proj["id"].(float64))+"/scans", nil); code != 200 || len(list) != 1 ||
		list[0].(map[string]any)["critical"].(float64) != 1 || list[0].(map[string]any)["findings"] != nil {
		t.Fatalf("project scans: %d %v", code, list)
	}

	// a viewer sees the result and can't start a scan
	_, tok, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "view", "role_id": roles["viewer"], "project_id": proj["id"]})
	vt := bearer(tok["token"].(string))
	if code, _, _ := e.do(e.client(), "GET", base, nil, vt...); code != 200 {
		t.Fatalf("viewer get: %d", code)
	}
	if code, _, _ := e.do(e.client(), "POST", base, nil, vt...); code != 403 {
		t.Fatalf("viewer scan: %d", code)
	}

	// nothing is due right after a scan; a day later it is
	now := time.Now()
	if _, _, ok := e.api.nextScan(context.Background(), now, 24*time.Hour); ok {
		t.Fatal("re-scan due at once")
	}
	if a, image, ok := e.api.nextScan(context.Background(), now.Add(25*time.Hour), 24*time.Hour); !ok || a.Slug != "web" || image != "traefik/whoami:v1.10" {
		t.Fatalf("re-scan after a day: %v %s %v", a.Slug, image, ok)
	}

	// another image is deployed: the old result is stale and due
	e.do(o, "PATCH", "/api/apps/"+id, map[string]any{"config": map[string]any{"image": "traefik/whoami:v1.11"}})
	e.do(o, "POST", "/api/apps/"+id+"/deploy", nil)
	e.dep.Wait()
	if _, sc, _ := e.do(o, "GET", base, nil); sc["stale"] != true || sc["current_image"] != "traefik/whoami:v1.11" {
		t.Fatalf("stale: %v", sc)
	}
	if _, image, ok := e.api.nextScan(context.Background(), now, 24*time.Hour); !ok || image != "traefik/whoami:v1.11" {
		t.Fatalf("new image not due: %s %v", image, ok)
	}

	// a failed scan keeps trivy's message
	sk.JobOK = false
	sk.logs = "FATAL\timage scan error: unable to find the specified image\n"
	e.do(o, "POST", base, nil)
	e.api.scanWG.Wait()
	if _, sc, _ := e.do(o, "GET", base, nil); sc["status"] != "failed" || !strings.Contains(sc["message"].(string), "unable to find") || sc["total"].(float64) != 0 {
		t.Fatalf("failed: %v", sc)
	}
	if _, _, list := e.do(o, "GET", "/api/audit?limit=50", nil); !strings.Contains(stringsOf(list), "app.scan") {
		t.Fatal("no audit entry")
	}
}

// Images of the built-in registry are read over its in-cluster address with the build login.
func TestScanRequestBuiltinRegistry(t *testing.T) {
	e := newEnv(t)
	if req := e.api.scanRequest(context.Background(), "ghcr.io/acme/web:1"); req.Image != "ghcr.io/acme/web:1" || req.PlainHTTP || req.DockerConfig != nil {
		t.Fatalf("public image: %+v", req)
	}
	e.api.registry = &registry.Registry{Namespace: "nectar-system", Password: "pw"}
	req := e.api.scanRequest(context.Background(), e.api.registry.PullRef("prj-shop/web", "12"))
	if req.Image != "nectar-registry.nectar-system.svc:5000/prj-shop/web:12" || !req.PlainHTTP ||
		!strings.Contains(string(req.DockerConfig), "nectar-registry.nectar-system.svc:5000") {
		t.Fatalf("built-in image: %+v %s", req, req.DockerConfig)
	}
}
