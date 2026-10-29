package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
)

func jarBytes(major uint16) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("META-INF/MANIFEST.MF")
	w.Write([]byte("Main-Class: a.Main\n"))
	w, _ = zw.Create("a/Main.class")
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, 0xCAFEBABE)
	binary.BigEndian.PutUint16(b[6:], major)
	w.Write(b)
	zw.Close()
	return buf.Bytes()
}

func zipBytes(files ...string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, _ := zw.Create(f)
		w.Write([]byte("x"))
	}
	zw.Close()
	return buf.Bytes()
}

// raw sends a non-JSON request and returns status, headers and body.
func (e *testEnv) raw(c *http.Client, method, path string, body []byte, hdr ...string) (int, http.Header, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func (e *testEnv) owner() *http.Client {
	c := e.client()
	if code, _, _ := e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password"}); code != 201 {
		e.t.Fatalf("setup: %d", code)
	}
	return c
}

func TestProjectAppDeployFlow(t *testing.T) {
	e := newEnv(t)
	o := e.owner()

	// runtimes catalog
	if code, _, arr := e.do(o, "GET", "/api/runtimes", nil); code != 200 || len(arr) < 6 {
		t.Fatalf("runtimes: %d %d", code, len(arr))
	}

	// project → namespace in the cluster
	code, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	if code != 201 || proj["namespace"] != "prj-shop" || !e.kube.Has("Namespace", "", "prj-shop") {
		t.Fatalf("create project: %d %v", code, proj)
	}
	if code, _, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "Bad_Slug", "name": "x"}); code != 400 {
		t.Fatalf("bad slug: %d", code)
	}
	pid := itoa(proj["id"].(float64))

	// app with defaults from the runtime
	code, app, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "java"})
	if code != 201 {
		t.Fatalf("create app: %d %v", code, app)
	}
	cfg := app["config"].(map[string]any)
	if app["runtime_version"] != "21" || cfg["port"].(float64) != 8080 || cfg["resources"].(map[string]any)["memory_mb"].(float64) != 512 {
		t.Fatalf("defaults: %v", app)
	}
	aid := itoa(app["id"].(float64))
	for _, bad := range []map[string]any{
		{"slug": "x", "name": "X", "runtime": "static", "kind": "worker"},                         // unsupported kind
		{"slug": "y", "name": "Y", "runtime": "java", "runtime_version": "6"},                     // unknown version
		{"slug": "z", "name": "Z", "runtime": "image"},                                            // image missing
		{"slug": "w", "name": "W", "runtime": "java", "kind": "cron", "config": map[string]any{}}, // cron without schedule
		{"slug": "v", "name": "V", "runtime": "java", "config": map[string]any{"replicas": 99}},   // too many replicas
		{"slug": "u", "name": "U", "runtime": "java", "config": map[string]any{"expose": []any{map[string]any{"port": 1883, "protocol": "SCTP"}}}},
	} {
		if code, v, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", bad); code != 400 {
			t.Errorf("accepted invalid app %v: %d %v", bad, code, v)
		}
	}

	// env: secret values are masked, kept when omitted, revealed with permission
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+aid+"/env", []map[string]any{{"key": "DB_URL", "value": "postgres://x", "secret": true}, {"key": "MODE", "value": "prod"}}); code != 204 {
		t.Fatalf("put env: %d", code)
	}
	_, _, envs := e.do(o, "GET", "/api/apps/"+aid+"/env", nil)
	if len(envs) != 2 || envs[0].(map[string]any)["value"] != nil || envs[1].(map[string]any)["value"] != "prod" {
		t.Fatalf("env masked: %v", envs)
	}
	e.do(o, "PUT", "/api/apps/"+aid+"/env", []map[string]any{{"key": "DB_URL", "secret": true}, {"key": "MODE", "value": "staging"}})
	_, _, envs = e.do(o, "GET", "/api/apps/"+aid+"/env?reveal=1", nil)
	if envs[0].(map[string]any)["value"] != "postgres://x" || envs[1].(map[string]any)["value"] != "staging" {
		t.Fatalf("env keep/reveal: %v", envs)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+aid+"/env", []map[string]any{{"key": "1BAD", "value": "x"}}); code != 400 {
		t.Fatalf("bad env key: %d", code)
	}

	// raw upload: a Java 17 jar
	code, _, body := e.raw(o, "POST", "/api/apps/"+aid+"/artifacts?filename=app.jar", jarBytes(61))
	if code != 201 {
		t.Fatalf("upload: %d %s", code, body)
	}
	var art map[string]any
	json.Unmarshal(body, &art)
	det := art["detect"].(map[string]any)
	if det["runtime"] != "java" || det["version"] != "17" {
		t.Fatalf("detect: %v", det)
	}
	artID := art["id"].(float64)

	// deploy → release live in the (fake) cluster
	code, rel, _ := e.do(o, "POST", "/api/apps/"+aid+"/deploy", map[string]any{"artifact_id": artID})
	if code != 202 || rel["number"].(float64) != 1 {
		t.Fatalf("deploy: %d %v", code, rel)
	}
	e.dep.Wait()
	_, _, rels := e.do(o, "GET", "/api/apps/"+aid+"/releases", nil)
	if rels[0].(map[string]any)["status"] != "live" {
		t.Fatalf("release: %v", rels[0])
	}
	if !e.kube.Has("Deployment", "prj-shop", "api") {
		t.Fatal("deployment missing")
	}
	code, st, _ := e.do(o, "GET", "/api/apps/"+aid+"/status", nil)
	if code != 200 || st["status"] != "running" || st["live"].(map[string]any)["replicas"].(float64) != 1 {
		t.Fatalf("status: %d %v", code, st)
	}

	// the init container's URL downloads the artifact; a bad signature is rejected
	dep := e.kube.Get("Deployment", "prj-shop", "api")
	initEnv := kube.Slice(kube.Slice(dep, "spec", "template", "spec", "initContainers")[0], "env")
	var dl string
	for _, ev := range initEnv {
		if kube.Str(ev, "name") == "ARTIFACT_URL" {
			dl = kube.Str(ev, "value")
		}
	}
	u, _ := url.Parse(dl)
	code, _, got := e.raw(e.client(), "GET", u.Path+"?"+u.RawQuery, nil)
	if code != 200 || !bytes.Equal(got, jarBytes(61)) {
		t.Fatalf("artifact download: %d (%d bytes)", code, len(got))
	}
	if code, _, _ := e.raw(e.client(), "GET", u.Path+"?sig=deadbeef", nil); code != 403 {
		t.Fatalf("bad signature: %d", code)
	}

	// config change + redeploy, then roll back to #1: settings come back, today's scale stays
	e.do(o, "PATCH", "/api/apps/"+aid, map[string]any{"config": map[string]any{"replicas": 3, "args": "--new"}})
	e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil)
	e.dep.Wait()
	if kube.Int(e.kube.Get("Deployment", "prj-shop", "api"), "spec", "replicas") != 3 {
		t.Fatal("redeploy with 3 replicas not applied")
	}
	code, rb, _ := e.do(o, "POST", "/api/apps/"+aid+"/releases/1/rollback", nil)
	if code != 202 || rb["number"].(float64) != 3 {
		t.Fatalf("rollback: %d %v", code, rb)
	}
	e.dep.Wait()
	if kube.Int(e.kube.Get("Deployment", "prj-shop", "api"), "spec", "replicas") != 3 {
		t.Fatal("rollback must keep the current scale (3)")
	}
	if _, a, _ := e.do(o, "GET", "/api/apps/"+aid, nil); a["config"].(map[string]any)["args"] != nil || a["config"].(map[string]any)["replicas"].(float64) != 3 {
		t.Fatalf("rollback should restore release #1's settings and keep the scale: %v", a["config"])
	}

	// one-off command
	if code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/run", map[string]any{"command": []string{"sh", "-c", "echo hi"}}); code != 202 || !strings.HasPrefix(v["job"].(string), "api-run-") {
		t.Fatalf("run: %d %v", code, v)
	}

	// delete app → objects gone; delete project → namespace gone
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+aid, nil); code != 204 || e.kube.Has("Deployment", "prj-shop", "api") {
		t.Fatalf("delete app: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/projects/"+pid, nil); code != 204 || e.kube.Has("Namespace", "", "prj-shop") {
		t.Fatalf("delete project: %d", code)
	}
}

func TestUploadsValidationAndTus(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "web", "name": "Web"})
	pid := itoa(proj["id"].(float64))
	_, site, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "site", "name": "Site", "runtime": "static"})
	sid := itoa(site["id"].(float64))
	_, img, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "img", "name": "Img", "runtime": "image", "config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	iid := itoa(img["id"].(float64))

	// static sites need an archive; zip-slip is rejected; image apps take no uploads; empty files rejected
	if code, _, b := e.raw(o, "POST", "/api/apps/"+sid+"/artifacts?filename=a.jar", jarBytes(61)); code != 201 && code != 400 {
		t.Fatalf("jar to static: %d %s", code, b)
	}
	if code, _, _ := e.raw(o, "POST", "/api/apps/"+sid+"/artifacts?filename=x.zip", zipBytes("../evil")); code != 400 {
		t.Fatalf("zip-slip: %d", code)
	}
	if code, _, _ := e.raw(o, "POST", "/api/apps/"+iid+"/artifacts", []byte("x")); code != 400 {
		t.Fatalf("upload to image app: %d", code)
	}
	if code, _, _ := e.raw(o, "POST", "/api/apps/"+sid+"/artifacts", nil); code != 400 {
		t.Fatalf("empty upload: %d", code)
	}
	if code, _, _ := e.raw(o, "POST", "/api/apps/"+sid+"/artifacts", bytes.Repeat([]byte{1}, 11<<20)); code != 413 {
		t.Fatalf("too large: %d", code)
	}

	// resumable upload in two chunks
	data := zipBytes("dist/index.html", "dist/app.js")
	code, h, _ := e.raw(o, "POST", "/api/apps/"+sid+"/uploads", nil,
		"Upload-Length", itoa(float64(len(data))), "Upload-Metadata", "filename "+base64.StdEncoding.EncodeToString([]byte("site.zip")), "Tus-Resumable", "1.0.0")
	if code != 201 || !strings.HasPrefix(h.Get("Location"), "/api/uploads/") {
		t.Fatalf("tus create: %d %v", code, h)
	}
	loc := h.Get("Location")
	half := len(data) / 2
	code, h, _ = e.raw(o, "PATCH", loc, data[:half], "Content-Type", "application/offset+octet-stream", "Upload-Offset", "0", "Tus-Resumable", "1.0.0")
	if code != 204 || h.Get("Upload-Offset") != itoa(float64(half)) {
		t.Fatalf("chunk 1: %d %v", code, h)
	}
	if code, h, _ := e.raw(o, "HEAD", loc, nil); code != 200 || h.Get("Upload-Offset") != itoa(float64(half)) {
		t.Fatalf("head: %d %v", code, h)
	}
	if code, _, _ := e.raw(o, "PATCH", loc, data[half:], "Content-Type", "application/offset+octet-stream", "Upload-Offset", "0"); code != 409 {
		t.Fatalf("wrong offset: %d", code)
	}
	code, h, _ = e.raw(o, "PATCH", loc, data[half:], "Content-Type", "application/offset+octet-stream", "Upload-Offset", itoa(float64(half)))
	if code != 204 || h.Get("Nectar-Artifact-Id") == "" {
		t.Fatalf("chunk 2: %d %v", code, h)
	}
	_, _, arts := e.do(o, "GET", "/api/apps/"+sid+"/artifacts", nil)
	last := arts[0].(map[string]any)
	if last["filename"] != "site.zip" || last["detect"].(map[string]any)["archive"].(map[string]any)["strip_prefix"] != "dist/" {
		t.Fatalf("tus artifact: %v", last)
	}
	if code, _, _ := e.raw(o, "HEAD", loc, nil); code != 404 {
		t.Fatalf("finished upload still exists: %d", code)
	}

	// deploy the image app: no artifact needed
	if code, v, _ := e.do(o, "POST", "/api/apps/"+iid+"/deploy", nil); code != 202 {
		t.Fatalf("image deploy: %d %v", code, v)
	}
	e.dep.Wait()
	c := kube.Slice(e.kube.Get("Deployment", "prj-web", "img"), "spec", "template", "spec", "containers")[0]
	if kube.Str(c, "image") != "traefik/whoami:v1.10" {
		t.Fatalf("image: %v", c["image"])
	}

	// capacity: 20 replicas × 1 CPU does not fit on the 4-CPU fake node
	e.do(o, "PATCH", "/api/apps/"+iid, map[string]any{"config": map[string]any{"replicas": 20, "resources": map[string]any{"cpu_millicores": 1000, "memory_mb": 128}}})
	code, v, _ := e.do(o, "POST", "/api/apps/"+iid+"/deploy", nil)
	if code != 409 || v["capacity"] != true {
		t.Fatalf("capacity: %d %v", code, v)
	}
	e.dep.Wait()
}

func TestProjectPermissions(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	_, p1, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "one", "name": "One"})
	_, p2, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "two", "name": "Two"})
	p1id, p2id := p1["id"].(float64), p2["id"].(float64)
	_, a2, _ := e.do(o, "POST", "/api/projects/"+itoa(p2id)+"/apps", map[string]any{"slug": "svc", "name": "Svc", "runtime": "java"})

	// a viewer of project one only
	_, u, _ := e.do(o, "POST", "/api/users", map[string]string{"email": "v@x.io", "name": "V", "password": "viewer-pass1"})
	_, _, roles := e.do(o, "GET", "/api/roles", nil)
	var viewerID float64
	for _, r := range roles {
		if r.(map[string]any)["builtin_key"] == "viewer" {
			viewerID = r.(map[string]any)["id"].(float64)
		}
	}
	e.do(o, "PUT", "/api/users/"+itoa(u["id"].(float64))+"/bindings", []map[string]any{{"role_id": viewerID, "project_id": p1id}})
	v := e.client()
	e.do(v, "POST", "/api/auth/login", map[string]string{"email": "v@x.io", "password": "viewer-pass1"})

	if _, _, list := e.do(v, "GET", "/api/projects", nil); len(list) != 1 || list[0].(map[string]any)["slug"] != "one" {
		t.Fatalf("viewer project list: %v", list)
	}
	if code, _, _ := e.do(v, "GET", "/api/projects/"+itoa(p2id), nil); code != 404 {
		t.Fatalf("other project visible: %d", code)
	}
	if code, _, _ := e.do(v, "GET", "/api/apps/"+itoa(a2["id"].(float64)), nil); code != 404 {
		t.Fatalf("other project's app visible: %d", code)
	}
	if code, _, _ := e.do(v, "POST", "/api/projects/"+itoa(p1id)+"/apps", map[string]any{"slug": "x", "name": "X", "runtime": "java"}); code != 403 {
		t.Fatalf("viewer created an app: %d", code)
	}
	if code, _, _ := e.do(v, "POST", "/api/projects", map[string]any{"slug": "three", "name": "Three"}); code != 403 {
		t.Fatalf("viewer created a project: %d", code)
	}
	if code, _, _ := e.do(v, "GET", "/api/apps/1/env?reveal=1", nil); code != 404 && code != 403 {
		t.Fatalf("reveal: %d", code)
	}
	_ = deploy.StatusRunning
}
