package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/build"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/secretbox"
)

const headSHA = "3333333333333333333333333333333333333333"

func fakeGit(t *testing.T) *httptest.Server {
	pkt := func(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		fmt.Fprint(w, pkt("# service=git-upload-pack\n")+"0000"+pkt(headSHA+" refs/heads/main\x00caps\n")+"0000")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withBuilds(t *testing.T, e *testEnv) {
	box, _ := secretbox.New(bytes.Repeat([]byte{5}, 32), "builds")
	reg := &registry.Registry{Kube: e.kube, Namespace: "nectar-system", Password: "pw", APIBase: "http://127.0.0.1:1"}
	e.api.registry = reg
	e.api.builder = &build.Builder{Store: e.api.store, Kube: e.kube, Registry: reg, Deployer: e.dep, Box: box, Poll: 5 * time.Millisecond}
}

func TestGitBuildsAPI(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	git := fakeGit(t)
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	src := map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "main"}
	// without the registry Git apps are refused
	if code, _, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"port": 8080}, "source": src}); code != 503 {
		t.Fatalf("no registry: %d", code)
	}
	withBuilds(t, e)
	if code, _, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "j", "name": "J", "runtime": "java", "source": src}); code != 400 {
		t.Fatalf("git source on a java app: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"port": 8080}, "source": map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "dev"}}); code != 400 {
		t.Fatalf("unknown branch: %d", code)
	}
	code, out, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"port": 8080}, "source": src})
	if code != 201 || out["build"] == nil {
		t.Fatalf("create git app: %d %v", code, out)
	}
	app := out["app"].(map[string]any)
	aid := itoa(app["id"].(float64))
	if !strings.HasSuffix(app["config"].(map[string]any)["image"].(string), "prj-shop/api:pending") {
		t.Fatalf("placeholder image %v", app["config"])
	}
	e.api.builder.Wait()
	e.dep.Wait()
	// the first build pushed build-1 and deployed it
	_, b1, _ := e.do(o, "GET", "/api/apps/"+aid+"/builds/1", nil)
	if b := b1["build"].(map[string]any); b["status"] != "succeeded" || b["commit_sha"] != headSHA || b["release"] == nil {
		t.Fatalf("build 1: %v", b1)
	}
	_, a, _ := e.do(o, "GET", "/api/apps/"+aid, nil)
	if a["config"].(map[string]any)["image"] != "registry.nectar.local/prj-shop/api:build-1" {
		t.Fatalf("image after build %v", a["config"])
	}
	// source view: webhook URL + secret for app.edit, never the token
	code, sv, _ := e.do(o, "GET", "/api/apps/"+aid+"/source", nil)
	secret, _ := sv["webhook_secret"].(string)
	if code != 200 || secret == "" || !strings.HasSuffix(sv["webhook_url"].(string), "/api/hooks/git/"+aid) || sv["has_token"] != false {
		t.Fatalf("source: %d %v", code, sv)
	}
	if code, sv, _ = e.do(o, "PUT", "/api/apps/"+aid+"/source", map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "main", "token": "ghp_x", "context_dir": "../x"}); code != 400 {
		t.Fatalf("bad context: %d %v", code, sv)
	}
	if code, sv, _ = e.do(o, "PUT", "/api/apps/"+aid+"/source", map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "main", "token": "ghp_x", "auto_deploy": true}); code != 200 || sv["has_token"] != true {
		t.Fatalf("set token: %d %v", code, sv)
	}

	// webhook: GitHub signature
	body := []byte(`{"ref":"refs/heads/main","after":"` + strings.Repeat("4", 40) + `","head_commit":{"message":"feat: hook"}}`)
	hook := func(sig string, b []byte) int {
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/hooks/git/"+aid, bytes.NewReader(b))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := hook("00", body); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	if code := hook(hex.EncodeToString(m.Sum(nil)), body); code != 202 {
		t.Fatalf("webhook: %d", code)
	}
	e.api.builder.Wait()
	e.dep.Wait()
	_, b2, _ := e.do(o, "GET", "/api/apps/"+aid+"/builds/2", nil)
	if b := b2["build"].(map[string]any); b["trigger"] != "webhook" || b["commit_message"] != "feat: hook" || b["commit_sha"] != strings.Repeat("4", 40) {
		t.Fatalf("build 2: %v", b2)
	}
	// another branch is ignored
	other := []byte(`{"ref":"refs/heads/feature","after":"` + strings.Repeat("5", 40) + `"}`)
	m = hmac.New(sha256.New, []byte(secret))
	m.Write(other)
	if code := hook(hex.EncodeToString(m.Sum(nil)), other); code != 200 {
		t.Fatalf("other branch: %d", code)
	}
	if _, _, list := e.do(o, "GET", "/api/apps/"+aid+"/builds", nil); len(list) != 2 {
		t.Fatalf("builds: %v", list)
	}
	// manual build without deploy
	if code, b, _ := e.do(o, "POST", "/api/apps/"+aid+"/builds", map[string]any{"deploy": false}); code != 202 || b["deploy"] != false {
		t.Fatalf("manual build: %d %v", code, b)
	}
	e.api.builder.Wait()
	// rotating the secret invalidates the old one
	_, sv, _ = e.do(o, "POST", "/api/apps/"+aid+"/source/webhook-secret", nil)
	if sv["webhook_secret"] == secret {
		t.Fatal("secret not rotated")
	}
	// a Git app without a build cannot be deployed directly
	e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image", "config": map[string]any{"port": 80}, "source": src})
	e.api.builder.Wait()
}

func TestRegistryLogins(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	box, _ := secretbox.New(bytes.Repeat([]byte{6}, 32), "registries")
	e.api.registryBox = box
	if code, _, _ := e.do(o, "PUT", "/api/settings/registries", map[string]any{"host": "ghcr.io", "username": "me"}); code != 400 {
		t.Fatalf("no password: %d", code)
	}
	code, v, _ := e.do(o, "PUT", "/api/settings/registries", map[string]any{"host": "https://GHCR.io/", "username": "me", "password": "ghp_123"})
	logins := v["logins"].([]any)
	if code != 200 || len(logins) != 1 || logins[0].(map[string]any)["host"] != "ghcr.io" || logins[0].(map[string]any)["password"] != nil {
		t.Fatalf("put: %d %v", code, v)
	}
	if v["builtin"].(map[string]any)["enabled"] != false {
		t.Error("builtin disabled")
	}
	// the pull secret of a project carries the login
	pull, err := registry.PullSecret(context.Background(), e.api.store, box)
	if err != nil || !strings.Contains(string(pull), `"ghcr.io"`) {
		t.Fatalf("pull secret %s %v", pull, err)
	}
	e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	id := itoa(logins[0].(map[string]any)["id"].(float64))
	if code, _, _ := e.do(o, "DELETE", "/api/settings/registries/"+id, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if pull, _ := registry.PullSecret(context.Background(), e.api.store, box); pull != nil {
		t.Fatal("pull secret after delete")
	}
}

func TestNodeActions(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	var evicted []string
	unsched := map[string]bool{}
	k8s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/namespaces/nectar-system/secrets/nectar-join":
			fmt.Fprint(w, `{"data":{"url":"aHR0cHM6Ly8xMC4wLjAuNTo2NDQz","token":"SzEwMDo6c2VydmVyOmFiYwo="}}`)
		case r.URL.Path == "/api/v1/nodes":
			fmt.Fprintf(w, `{"items":[
				{"metadata":{"name":"server","labels":{"node-role.kubernetes.io/control-plane":"true"}},"spec":{"unschedulable":%v},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
				{"metadata":{"name":"worker"},"spec":{"unschedulable":%v},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
				{"metadata":{"name":"edge","labels":{"nectar.dev/edge":"true"}},"spec":{"unschedulable":true},"status":{"conditions":[{"type":"Ready","status":"True"}]}}]}`, unsched["server"], unsched["worker"])
		case strings.HasPrefix(r.URL.Path, "/api/v1/nodes/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
			switch r.Method {
			case "PATCH":
				b, _ := io.ReadAll(r.Body)
				unsched[name] = strings.Contains(string(b), "true")
				fmt.Fprint(w, `{}`)
			case "DELETE":
				fmt.Fprint(w, `{}`)
			default:
				labels := `{"kubernetes.io/hostname":"` + name + `"}`
				if name == "server" {
					labels = `{"node-role.kubernetes.io/control-plane":"true"}`
				}
				if name == "edge" {
					labels = `{"nectar.dev/edge":"true"}`
				}
				fmt.Fprintf(w, `{"metadata":{"name":%q,"labels":%s},"spec":{"unschedulable":%v}}`, name, labels, unsched[name])
			}
		case r.URL.Path == "/api/v1/pods":
			fmt.Fprint(w, `{"items":[
				{"metadata":{"name":"api-1","namespace":"prj-shop"},"status":{"phase":"Running"}},
				{"metadata":{"name":"svclb","namespace":"kube-system","ownerReferences":[{"kind":"DaemonSet"}]},"status":{"phase":"Running"}},
				{"metadata":{"name":"db-0","namespace":"prj-shop"},"status":{"phase":"Running"}},
				{"metadata":{"name":"done","namespace":"prj-shop"},"status":{"phase":"Succeeded"}}]}`)
		case strings.HasSuffix(r.URL.Path, "/eviction"):
			if strings.Contains(r.URL.Path, "db-0") {
				w.WriteHeader(429)
				fmt.Fprint(w, `{"message":"Cannot evict pod as it would violate the pod's disruption budget."}`)
				return
			}
			evicted = append(evicted, r.URL.Path)
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer k8s.Close()
	kc, _ := kube.New(k8s.URL)
	e.api.kube = kc
	e.api.cfg.SystemNamespace = "nectar-system"
	e.api.cfg.RegistryPassword = "regpw"

	code, j, _ := e.do(o, "GET", "/api/server/join", nil)
	if code != 200 || !strings.Contains(j["command"].(string), "--join https://10.0.0.5:6443 --token K100::server:abc --registry-password regpw") {
		t.Fatalf("join: %d %v", code, j)
	}
	if code, _, _ := e.do(o, "POST", "/api/server/nodes/worker/cordon", nil); code != 200 || !unsched["worker"] {
		t.Fatalf("cordon: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/server/nodes/server", nil); code != 409 {
		t.Fatalf("delete control plane: %d", code)
	}
	// draining is refused when it would take things offline, unless forced
	if code, v, _ := e.do(o, "POST", "/api/server/nodes/edge/drain", nil); code != 409 || !strings.Contains(v["error"].(string), "edge node") {
		t.Fatalf("drain edge: %d %v", code, v)
	}
	unsched["server"] = true
	if code, v, _ := e.do(o, "POST", "/api/server/nodes/worker/drain", nil); code != 409 || !strings.Contains(v["error"].(string), "no other ready node") {
		t.Fatalf("drain the last schedulable node: %d %v", code, v)
	}
	evicted = nil
	if code, _, _ := e.do(o, "POST", "/api/server/nodes/worker/drain?force=true", nil); code != 200 || len(evicted) != 1 {
		t.Fatalf("forced drain: %d %v", code, evicted)
	}
	unsched["server"], unsched["worker"] = false, true
	code, d, _ := e.do(o, "POST", "/api/server/nodes/worker/drain", nil)
	if code != 200 || len(d["evicted"].([]any)) != 1 || len(d["failed"].([]any)) != 1 || d["skipped"].(float64) != 2 {
		t.Fatalf("drain: %d %v", code, d)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/server/nodes/worker", nil); code != 200 {
		t.Fatalf("delete worker: %d", code)
	}
	e.do(o, "POST", "/api/server/nodes/worker/uncordon", nil)
	if unsched["worker"] {
		t.Fatal("uncordon")
	}
	if code, _, _ := e.do(o, "POST", "/api/server/nodes/worker/explode", nil); code != 404 {
		t.Fatalf("unknown action: %d", code)
	}
}
