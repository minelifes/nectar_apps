package build

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"

	_ "modernc.org/sqlite"
)

func pkt(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

const sha1 = "1111111111111111111111111111111111111111"
const sha2 = "2222222222222222222222222222222222222222"

// gitServer answers info/refs like GitHub; /private needs the token "tok".
func gitServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/info/refs") || r.URL.Query().Get("service") != "git-upload-pack" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "private") {
			if _, pw, ok := r.BasicAuth(); !ok || pw != "tok" {
				w.WriteHeader(401)
				return
			}
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		fmt.Fprint(w, pkt("# service=git-upload-pack\n")+"0000"+
			pkt(sha1+" HEAD\x00multi_ack side-band-64k symref=HEAD:refs/heads/main\n")+
			pkt(sha1+" refs/heads/main\n")+pkt(sha2+" refs/heads/dev\n")+pkt(sha2+" refs/tags/v1\n")+"0000")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRemoteHead(t *testing.T) {
	srv := gitServer(t)
	ctx := context.Background()
	rm := Remote{}
	if sha, err := rm.Head(ctx, srv.URL+"/acme/api.git", "main", ""); err != nil || sha != sha1 {
		t.Fatalf("main: %s %v", sha, err)
	}
	if sha, _ := rm.Head(ctx, srv.URL+"/acme/api.git", "dev", ""); sha != sha2 {
		t.Fatalf("dev: %s", sha)
	}
	if _, err := rm.Head(ctx, srv.URL+"/acme/api.git", "nope", ""); err == nil || !strings.Contains(err.Error(), "branch not found") {
		t.Fatalf("missing branch: %v", err)
	}
	if _, err := rm.Head(ctx, srv.URL+"/acme/private.git", "main", ""); err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("private without token: %v", err)
	}
	if sha, err := rm.Head(ctx, srv.URL+"/acme/private.git", "main", "tok"); err != nil || sha != sha1 {
		t.Fatalf("private with token: %s %v", sha, err)
	}
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer html.Close()
	if _, err := rm.Head(ctx, html.URL+"/acme/api", "main", ""); err == nil || !strings.Contains(err.Error(), "Git server") {
		t.Fatalf("not a git server: %v", err)
	}
}

func TestValidation(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/acme/api.git":  "https://github.com/acme/api.git",
		"https://github.com/acme/api/":     "https://github.com/acme/api",
		"git@github.com:acme/api.git":      "https://github.com/acme/api.git",
		"http://gitea.lan:3000/me/app.git": "http://gitea.lan:3000/me/app.git",
	} {
		if got, err := NormalizeRepo(in); err != nil || got != want {
			t.Errorf("%s → %s %v", in, got, err)
		}
	}
	for _, bad := range []string{"ftp://x/y", "https://github.com", "https://user:pw@github.com/a/b", "acme/api"} {
		if _, err := NormalizeRepo(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if !ValidPath("services/api") || ValidPath("../etc") || ValidPath("/abs") || ValidPath("a;rm") {
		t.Error("ValidPath")
	}
	if !ValidBranch("feature/x-1") || ValidBranch("a..b") || ValidBranch("-x") || ValidBranch("a b") {
		t.Error("ValidBranch")
	}
	if c, plain := gitContext("https://github.com/acme/api.git", "main", sha1); c != "git://github.com/acme/api.git#refs/heads/main#"+sha1 || plain {
		t.Errorf("context %s", c)
	}
}

func sign(body []byte, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func TestParsePush(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"` + sha2 + `","head_commit":{"id":"` + sha2 + `","message":"fix: things\n\nlong body"}}`)
	h := http.Header{}
	h.Set("X-GitHub-Event", "push")
	h.Set("X-Hub-Signature-256", "sha256="+sign(body, "s3cret"))
	p, err := ParsePush(h, body, "s3cret")
	if err != nil || p.Branch != "main" || p.Commit != sha2 || p.Message != "fix: things" || p.Provider != "github" {
		t.Fatalf("github: %+v %v", p, err)
	}
	if _, err := ParsePush(h, body, "other"); err != ErrSignature {
		t.Fatalf("bad signature: %v", err)
	}
	h.Set("X-GitHub-Event", "ping")
	if p, err := ParsePush(h, body, "s3cret"); err != nil || !p.Ping {
		t.Fatalf("ping %+v %v", p, err)
	}
	// GitLab: token header, checkout_sha, commits
	gl := []byte(`{"ref":"refs/heads/dev","checkout_sha":"` + sha1 + `","commits":[{"id":"` + sha1 + `","message":"feat: gl"}]}`)
	h = http.Header{}
	h.Set("X-Gitlab-Event", "Push Hook")
	h.Set("X-Gitlab-Token", "s3cret")
	if p, err := ParsePush(h, gl, "s3cret"); err != nil || p.Branch != "dev" || p.Commit != sha1 || p.Message != "feat: gl" {
		t.Fatalf("gitlab %+v %v", p, err)
	}
	h.Set("X-Gitlab-Token", "nope")
	if _, err := ParsePush(h, gl, "s3cret"); err != ErrSignature {
		t.Fatal("gitlab token")
	}
	// Gitea: hex HMAC without prefix
	h = http.Header{}
	h.Set("X-Gitea-Event", "push")
	h.Set("X-Gitea-Signature", sign(body, "s3cret"))
	if p, err := ParsePush(h, body, "s3cret"); err != nil || p.Provider != "gitea" || p.Commit != sha2 {
		t.Fatalf("gitea %+v %v", p, err)
	}
	// branch deleted
	del := []byte(`{"ref":"refs/heads/main","after":"0000000000000000000000000000000000000000","deleted":true}`)
	h = http.Header{}
	h.Set("X-GitHub-Event", "push")
	h.Set("X-Hub-Signature-256", "sha256="+sign(del, "s3cret"))
	if p, err := ParsePush(h, del, "s3cret"); err != nil || !p.Deleted {
		t.Fatalf("deleted %+v %v", p, err)
	}
}

// ------------------------------------------------------------------ builder --

type fakeK struct {
	*kubefake.Fake
	logs string
	hold atomic.Bool // while set, build Jobs are still running (no status yet)
}

func (f *fakeK) GetObject(ctx context.Context, kind, ns, name string, out any) error {
	if kind != "Job" || !f.hold.Load() {
		return f.Fake.GetObject(ctx, kind, ns, name, out)
	}
	var o kube.Object
	if err := f.Fake.GetObject(ctx, kind, ns, name, &o); err != nil {
		return err
	}
	delete(o, "status")
	b, _ := json.Marshal(o)
	return json.Unmarshal(b, out)
}

func (f *fakeK) List(ctx context.Context, kind, ns, sel string) ([]kube.Object, error) {
	if kind == "Pod" && strings.HasPrefix(sel, "job-name=") {
		return []kube.Object{{"kind": "Pod", "metadata": map[string]any{"name": strings.TrimPrefix(sel, "job-name=") + "-x", "namespace": ns}}}, nil
	}
	return f.Fake.List(ctx, kind, ns, sel)
}

func (f *fakeK) PodLogs(context.Context, string, string, string, int, bool) (string, error) {
	return f.logs, nil
}

func TestBuilder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "b.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, err := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image",
		Config: store.AppConfig{Image: "registry.nectar.local/prj-shop/api:pending", Port: 80, Replicas: 1}})
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{4}, 32), "builds")
	tok, _ := box.Seal("tok")
	hook, _ := box.Seal("hook")
	git := gitServer(t)
	if _, err := st.SaveAppSource(ctx, store.AppSource{AppID: app.ID, RepoURL: git.URL + "/acme/private.git", Branch: "main", Dockerfile: "Dockerfile",
		ContextDir: "services/api", Token: tok, WebhookSecret: hook, AutoDeploy: true}); err != nil {
		t.Fatal(err)
	}
	// registry API: one old build tag to prune
	var deleted []string
	regAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags/list"):
			json.NewEncoder(w).Encode(map[string]any{"tags": []string{"build-1", "build-2", "build-3", "build-4", "build-5", "build-6", "build-7", "cache"}})
		case r.Method == http.MethodHead:
			w.Header().Set("Docker-Content-Digest", "sha256:"+strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], "build-"))
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer regAPI.Close()
	k := &fakeK{Fake: kubefake.New(), logs: "INFO Pushing image\nINFO Pushed registry"}
	reg := &registry.Registry{Kube: k, Namespace: "nectar-system", Password: "pw", APIBase: regAPI.URL}
	b := &Builder{Store: st, Kube: k, Registry: reg, Box: box, Poll: 5 * time.Millisecond,
		Creds: func(context.Context) ([]registry.Credential, error) {
			return []registry.Credential{{Host: "ghcr.io", Username: "me", Password: "ghp"}}, nil
		}}
	k.hold.Store(true) // the first build keeps running until the second start was refused
	bl, err := b.Start(ctx, Request{App: app, Project: p, Trigger: "manual", Deploy: true})
	if err != nil {
		t.Fatal(err)
	}
	if bl.Number != 1 || bl.CommitSHA != sha1 || bl.Image != "registry.nectar.local/prj-shop/api:build-1" {
		t.Fatalf("build %+v", bl)
	}
	if _, err := b.Start(ctx, Request{App: app, Project: p, Trigger: "manual"}); !errors.Is(err, store.ErrBuildRunning) {
		t.Fatalf("second start while a build runs: %v", err)
	}
	k.hold.Store(false)
	b.Wait()
	var job kube.Object
	if err := k.Fake.GetObject(ctx, "Job", "nectar-builds", JobName(bl.ID), &job); err != nil {
		t.Fatal(err)
	}
	c := kube.Slice(job, "spec", "template", "spec", "containers")[0]
	args := fmt.Sprint(kube.Field(c, "args"))
	for _, want := range []string{"--context=git://" + strings.TrimPrefix(git.URL, "http://") + "/acme/private.git#refs/heads/main#" + sha1,
		"--context-sub-path=services/api", "--destination=nectar-registry.nectar-system.svc:5000/prj-shop/api:build-1",
		"--insecure-registry=nectar-registry.nectar-system.svc:5000", "--cache-repo=nectar-registry.nectar-system.svc:5000/prj-shop/api/cache"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %s: %s", want, args)
		}
	}
	env := fmt.Sprint(kube.Field(c, "env"))
	if !strings.Contains(env, "GIT_PASSWORD") || !strings.Contains(env, "GIT_PULL_METHOD") { // the test server is plain http
		t.Errorf("env %s", env)
	}
	if kube.Str(c, "resources", "limits", "cpu") != "2000m" {
		t.Error("default build cpu")
	}
	// the docker config holds the built-in registry and the private logins; the Secret is gone after the build
	if k.Fake.Has("Secret", "nectar-builds", JobName(bl.ID)) {
		t.Error("build secret not removed")
	}
	got, _ := st.GetBuild(ctx, bl.ID)
	if got.Status != store.BuildSucceeded || !strings.Contains(got.Log, "Pushed") {
		t.Fatalf("finished %+v", got)
	}
	a2, _ := st.GetApp(ctx, app.ID)
	if a2.Config.Image != bl.Image {
		t.Errorf("app image %s", a2.Config.Image)
	}
	// keeps 5 newest + in use (build-1 is the app's image now)
	if len(deleted) != 1 || !strings.HasSuffix(deleted[0], "sha256:2") {
		t.Errorf("deleted %v", deleted)
	}
	if !k.Fake.Has("Namespace", "", "nectar-builds") || !k.Fake.Has("NetworkPolicy", "nectar-builds", "deny-ingress") {
		t.Error("build namespace")
	}

	// a failing build keeps the image and reports the error line
	k.Fake.JobOK = false
	k.logs = "INFO Retrieving image\nerror building image: parsing dockerfile: unknown instruction: FORM"
	bl2, err := b.Start(ctx, Request{App: a2, Project: p, Trigger: "webhook", Commit: sha2, Message: "oops", Deploy: true})
	if err != nil {
		t.Fatal(err)
	}
	b.Wait()
	got, _ = st.GetBuild(ctx, bl2.ID)
	if got.Status != store.BuildFailed || !strings.Contains(got.Message, "unknown instruction") || got.CommitSHA != sha2 || got.Number != 2 {
		t.Fatalf("failed build %+v", got)
	}
	if a3, _ := st.GetApp(ctx, app.ID); a3.Config.Image != bl.Image {
		t.Error("a failed build must not change the image")
	}
	list, _ := st.ListBuilds(ctx, app.ID, 10)
	if len(list) != 2 || list[0].Number != 2 || list[0].Log != "" {
		t.Errorf("list %+v", list)
	}
	// without registry
	b.Registry = &registry.Registry{}
	if _, err := b.Start(ctx, Request{App: a2, Project: p}); err != registry.ErrDisabled {
		t.Errorf("no registry: %v", err)
	}
}

func TestParsePullRequests(t *testing.T) {
	sign := func(body, secret string) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(body))
		return hex.EncodeToString(m.Sum(nil))
	}
	gh := `{"action":"synchronize","number":12,"pull_request":{"title":"Fix","html_url":"https://github.com/acme/api/pull/12","user":{"login":"ann"},
		"head":{"sha":"` + strings.Repeat("c", 40) + `","ref":"fix","repo":{"full_name":"ann/api"}},"base":{"ref":"main","repo":{"full_name":"acme/api"}}}}`
	h := http.Header{"X-Github-Event": {"pull_request"}, "X-Hub-Signature-256": {"sha256=" + sign(gh, "s")}}
	p, err := ParsePush(h, []byte(gh), "s")
	if err != nil || p.PR == nil || p.PR.Action != PRUpdate || p.PR.HeadRef != "refs/pull/12/head" || !p.PR.Fork || p.PR.BaseBranch != "main" || p.Commit != strings.Repeat("c", 40) {
		t.Fatalf("github: %v %+v", err, p.PR)
	}
	gl := `{"object_kind":"merge_request","user":{"username":"bob"},"object_attributes":{"iid":5,"title":"MR","url":"https://gitlab.com/acme/api/-/merge_requests/5",
		"action":"update","target_branch":"main","source_project_id":1,"target_project_id":1,"last_commit":{"id":"` + strings.Repeat("d", 40) + `"}}}`
	h = http.Header{"X-Gitlab-Event": {"Merge Request Hook"}, "X-Gitlab-Token": {"s"}}
	if p, err = ParsePush(h, []byte(gl), "s"); err != nil || p.PR.Action != "" {
		t.Fatalf("gitlab update without new commits: %v %+v", err, p.PR)
	}
	gl = strings.Replace(gl, `"action":"update"`, `"action":"merge"`, 1)
	if p, err = ParsePush(h, []byte(gl), "s"); err != nil || p.PR.Action != PRClose || p.PR.HeadRef != "refs/merge-requests/5/head" || p.PR.Fork || p.PR.Author != "bob" {
		t.Fatalf("gitlab merge: %v %+v", err, p.PR)
	}
	if _, err := ParsePush(h, []byte(gl), "wrong"); err != ErrSignature {
		t.Fatalf("gitlab token: %v", err)
	}
	if ctx, _ := gitContext("https://github.com/acme/api.git", "refs/pull/12/head", "abc"); ctx != "git://github.com/acme/api.git#refs/pull/12/head#abc" {
		t.Fatalf("git context: %s", ctx)
	}
}

func TestPodStatus(t *testing.T) {
	ctx := context.Background()
	k := kubefake.New()
	b := &Builder{Kube: k}
	bl := store.Build{ID: 7, Status: store.BuildRunning}
	if s := b.PodStatus(ctx, bl); !strings.Contains(s, "waiting for the build pod") {
		t.Fatalf("no pod: %q", s)
	}
	pod := kube.Object{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "build-7-x", "namespace": DefaultNamespace,
		"labels": map[string]any{"job-name": "build-7"}},
		"status": map[string]any{"phase": "Pending", "conditions": []any{map[string]any{"type": "PodScheduled", "status": "False",
			"message": "0/1 nodes are available: 1 Insufficient memory."}}}}
	if _, err := k.Apply(ctx, pod); err != nil {
		t.Fatal(err)
	}
	if s := b.PodStatus(ctx, bl); !strings.Contains(s, "Insufficient memory") {
		t.Fatalf("unschedulable: %q", s)
	}
	if s := b.PodStatus(ctx, store.Build{ID: 7, Status: store.BuildSucceeded}); s != "" {
		t.Fatalf("finished build: %q", s)
	}
}
