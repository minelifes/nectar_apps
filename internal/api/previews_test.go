package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPreviewEnvironments(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	var mu sync.Mutex
	var statuses []map[string]string
	pkt := func(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }
	git := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/repos/acme/api/statuses/") { // Gitea commit status API
			if r.Header.Get("Authorization") != "token gt_write" {
				w.WriteHeader(401)
				return
			}
			var st map[string]string
			json.NewDecoder(r.Body).Decode(&st)
			st["sha"] = strings.TrimPrefix(r.URL.Path, "/api/v1/repos/acme/api/statuses/")
			mu.Lock()
			statuses = append(statuses, st)
			mu.Unlock()
			w.WriteHeader(201)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		fmt.Fprint(w, pkt("# service=git-upload-pack\n")+"0000"+pkt(headSHA+" refs/heads/main\x00caps\n")+"0000")
	}))
	defer git.Close()
	withBuilds(t, e)
	e.api.cfg.BaseDomain = "apps.example.com"
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	code, out, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"port": 8080, "replicas": 3}, "source": map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "main"}})
	if code != 201 {
		t.Fatalf("create: %d %v", code, out)
	}
	aid := itoa(out["app"].(map[string]any)["id"].(float64))
	e.api.builder.Wait()
	e.dep.Wait()
	e.do(o, "PUT", "/api/apps/"+aid+"/env", []map[string]any{{"key": "API_KEY", "value": "k1", "secret": true}})
	_, sv, _ := e.do(o, "PUT", "/api/apps/"+aid+"/source", map[string]any{"repo_url": git.URL + "/acme/api.git", "branch": "main",
		"token": "gt_write", "previews": true, "preview_ttl_days": 14})
	if sv["previews"] != true {
		t.Fatalf("previews setting: %v", sv)
	}
	secret := sv["webhook_secret"].(string)

	hook := func(action string, n int, sha string, fork bool, base string) (int, map[string]any) {
		headRepo := "acme/api"
		if fork {
			headRepo = "mallory/api"
		}
		body, _ := json.Marshal(map[string]any{"action": action, "number": n, "pull_request": map[string]any{
			"title": "Add cart", "html_url": "https://git.example.com/acme/api/pulls/" + itoa(float64(n)), "user": map[string]any{"login": "ann"},
			"head": map[string]any{"sha": sha, "ref": "cart", "repo": map[string]any{"full_name": headRepo}},
			"base": map[string]any{"ref": base, "repo": map[string]any{"full_name": "acme/api"}}}})
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/hooks/git/"+aid, bytes.NewReader(body))
		req.Header.Set("X-Gitea-Event", "pull_request")
		req.Header.Set("X-Gitea-Signature", hex.EncodeToString(m.Sum(nil)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		raw, _ := io.ReadAll(resp.Body)
		json.Unmarshal(raw, &v)
		e.api.builder.Wait()
		e.dep.Wait()
		return resp.StatusCode, v
	}
	sha1, sha2 := strings.Repeat("a", 40), strings.Repeat("b", 40)

	// a fork and another base branch get nothing
	if code, v := hook("opened", 7, sha1, true, "main"); code != 200 || !strings.Contains(v["note"].(string), "forks") {
		t.Fatalf("fork: %d %v", code, v)
	}
	if code, v := hook("opened", 7, sha1, false, "release"); code != 200 || !strings.Contains(v["note"].(string), "does not target") {
		t.Fatalf("other base: %d %v", code, v)
	}
	// opened → a preview app, built from the pull request's ref and deployed
	code, v := hook("opened", 7, sha1, false, "main")
	if code != 202 || v["preview"] != "api-pr7" || v["url"] != "https://api-pr7-shop.apps.example.com" {
		t.Fatalf("opened: %d %v", code, v)
	}
	_, pv, _ := e.do(o, "GET", "/api/apps/"+aid+"/previews", nil)
	list := pv["previews"].([]any)
	if len(list) != 1 {
		t.Fatalf("previews: %v", pv)
	}
	p0 := list[0].(map[string]any)
	pidApp := itoa(p0["app_id"].(float64))
	if p0["number"].(float64) != 7 || p0["head_ref"] != "refs/pull/7/head" || p0["author"] != "ann" {
		t.Fatalf("preview: %v", p0)
	}
	_, papp, _ := e.do(o, "GET", "/api/apps/"+pidApp, nil)
	if papp["config"].(map[string]any)["replicas"].(float64) != 1 || papp["config"].(map[string]any)["image"] != "registry.nectar.local/prj-shop/api-pr7:build-1" {
		t.Fatalf("preview app: %v", papp["config"])
	}
	if env, _ := e.api.store.ListEnv(t.Context(), int64(p0["app_id"].(float64))); len(env) != 1 || env[0].Key != "API_KEY" {
		t.Fatalf("variables not copied: %v", env)
	}
	_, b1, _ := e.do(o, "GET", "/api/apps/"+pidApp+"/builds/1", nil)
	if b := b1["build"].(map[string]any); b["status"] != "succeeded" || b["commit_sha"] != sha1 || b["branch"] != "refs/pull/7/head" || b["trigger"] != "pull-request" {
		t.Fatalf("preview build: %v", b1)
	}
	_, ofv, _ := e.do(o, "GET", "/api/apps/"+pidApp+"/previews", nil)
	if ofv["preview_of"].(map[string]any)["parent"] != "api" {
		t.Fatalf("preview_of: %v", ofv)
	}
	// new commits rebuild the same preview
	if code, v := hook("synchronized", 7, sha2, false, "main"); code != 202 || v["build"].(float64) != 2 {
		t.Fatalf("synchronize: %d %v", code, v)
	}
	mu.Lock()
	got := append([]map[string]string{}, statuses...)
	mu.Unlock()
	if len(got) < 4 || got[0]["state"] != "pending" || got[1]["state"] != "success" || got[1]["context"] != "nectar/preview" ||
		got[1]["target_url"] != "https://api-pr7-shop.apps.example.com" || got[3]["sha"] != sha2 {
		t.Fatalf("commit statuses: %v", got)
	}
	// closed → the preview app is gone
	if code, v := hook("closed", 7, sha2, false, "main"); code != 200 || v["deleted"] != true {
		t.Fatalf("closed: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "GET", "/api/apps/"+pidApp, nil); code != 404 {
		t.Fatalf("preview after close: %d", code)
	}
	// previews without activity expire; deleting the parent deletes its previews
	hook("opened", 8, sha1, false, "main")
	if n := e.api.expirePreviews(t.Context(), time.Now().Add(15*24*time.Hour)); n != 1 {
		t.Fatalf("expired: %d", n)
	}
	hook("opened", 9, sha1, false, "main")
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+aid, nil); code != 204 {
		t.Fatalf("delete parent: %d", code)
	}
	if left, _ := e.api.store.ListPreviews(t.Context(), 0); len(left) != 0 {
		t.Fatalf("previews left: %v", left)
	}
	if _, _, apps := e.do(o, "GET", "/api/projects/"+pid+"/apps", nil); len(apps) != 0 {
		t.Fatalf("apps left: %v", apps)
	}
}
