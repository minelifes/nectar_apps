package deploy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

type env struct {
	st  *store.Store
	k   *kubefake.Fake
	d   *Deployer
	box *secretbox.Box
	p   store.Project
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "t.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cat, err := runtimes.LoadBuiltin()
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{1}, 32), "env")
	k := kubefake.New()
	d := New(st, k, cat, box, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		InternalURL: "http://nectar-apps.nectar-system.svc", SigningKey: []byte("k"),
		PollInterval: 5 * time.Millisecond, RolloutTimeout: 2 * time.Second, JobTimeout: 2 * time.Second,
	})
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	return &env{st: st, k: k, d: d, box: box, p: p}
}

func (e *env) app(t *testing.T, a store.App) (store.App, int64) {
	t.Helper()
	ctx := context.Background()
	a.ProjectID = e.p.ID
	a, err := e.st.CreateApp(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := e.box.Seal("postgres://db")
	e.st.ReplaceEnv(ctx, a.ID, []store.EnvVar{{Key: "DATABASE_URL", ValueEnc: enc, IsSecret: true}})
	art, _ := e.st.CreateArtifact(ctx, a.ID, strings.Repeat("ab", 32), "app.jar", 100, map[string]any{"type": "jar"}, nil)
	return a, art.ID
}

func (e *env) waitRelease(t *testing.T, id int64) store.Release {
	t.Helper()
	e.d.Wait()
	r, err := e.st.GetRelease(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func javaApp() store.App {
	return store.App{Slug: "api", Name: "API", Kind: "web", RuntimeID: "java", RuntimeVersion: "21",
		Config: store.AppConfig{Port: 8080, Replicas: 2, Resources: store.Resources{CPUMillicores: 500, MemoryMB: 512},
			Health: store.HealthConfig{Type: "http", Path: "/actuator/health"}, Expose: []store.ExposedPort{{Port: 1883, Protocol: "tcp"}}}}
}

func TestDeployJavaWebApp(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, artID := e.app(t, javaApp())

	// no artifact_id and nothing live yet → the newest upload is used
	rel, err := e.d.Deploy(ctx, DeployRequest{App: a})
	if err != nil {
		t.Fatal(err)
	}
	if rel = e.waitRelease(t, rel.ID); rel.Status != store.ReleaseLive || rel.ArtifactID == nil || *rel.ArtifactID != artID {
		t.Fatalf("release = %s %q %v", rel.Status, rel.Message, rel.ArtifactID)
	}
	if a, _ = e.st.GetApp(ctx, a.ID); a.Status != StatusRunning {
		t.Fatalf("app status = %s", a.Status)
	}

	ns := "prj-shop"
	for _, k := range []string{"Namespace//prj-shop", "LimitRange/" + ns + "/defaults", "NetworkPolicy/" + ns + "/nectar-isolation",
		"ServiceAccount/" + ns + "/app", "Secret/" + ns + "/api-env", "Deployment/" + ns + "/api", "Service/" + ns + "/api", "Service/" + ns + "/api-ports"} {
		if parts := strings.SplitN(k, "/", 3); !e.k.Has(parts[0], parts[1], parts[2]) {
			t.Errorf("missing %s", k)
		}
	}
	// network policy allows the node IP (gateway uses hostNetwork)
	np, _ := json.Marshal(e.k.Get("NetworkPolicy", ns, "nectar-isolation"))
	if !strings.Contains(string(np), "10.0.0.5/32") || !strings.Contains(string(np), "nectar-system") {
		t.Errorf("network policy: %s", np)
	}

	dep := e.k.Get("Deployment", ns, "api")
	pod := kube.Field(dep, "spec", "template", "spec").(map[string]any)
	c := pod["containers"].([]any)[0].(map[string]any)
	if c["image"] != "eclipse-temurin:21-jre" || kube.Int(dep, "spec", "replicas") != 2 {
		t.Errorf("container: %v", c["image"])
	}
	if kube.Str(c, "resources", "limits", "memory") != "512Mi" || kube.Str(c, "startupProbe", "httpGet", "path") != "/actuator/health" {
		t.Errorf("resources/probe: %v %v", c["resources"], c["startupProbe"])
	}
	init := pod["initContainers"].([]any)[0].(map[string]any)
	envs, _ := json.Marshal(init["env"])
	if !strings.Contains(string(envs), "/internal/artifacts/"+strings.Repeat("ab", 32)+"?sig=") || !strings.Contains(string(envs), `"app.jar"`) {
		t.Errorf("init env: %s", envs)
	}
	if kube.Int(pod, "securityContext", "runAsUser") != 65532 || pod["automountServiceAccountToken"] != false {
		t.Errorf("pod security: %v", pod["securityContext"])
	}
	// secret holds user env + PORT, base64-encoded
	sec := e.k.Get("Secret", ns, "api-env")
	if v, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "DATABASE_URL")); string(v) != "postgres://db" {
		t.Errorf("DATABASE_URL = %q", v)
	}
	if v, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "PORT")); string(v) != "8080" {
		t.Errorf("PORT = %q", v)
	}

	// second deploy without artifact reuses the live artifact; removing expose prunes the NodePort service
	a.Config.Expose = nil
	a, _ = e.st.SaveApp(ctx, a)
	rel2, err := e.d.Deploy(ctx, DeployRequest{App: a})
	if err != nil {
		t.Fatal(err)
	}
	if rel2 = e.waitRelease(t, rel2.ID); rel2.Status != store.ReleaseLive || *rel2.ArtifactID != artID {
		t.Fatalf("rel2 = %+v", rel2)
	}
	if e.k.Get("Service", ns, "api-ports") != nil {
		t.Error("NodePort service should have been pruned")
	}
	if r1, _ := e.st.GetRelease(ctx, rel.ID); r1.Status != store.ReleaseSuperseded {
		t.Errorf("rel1 = %s", r1.Status)
	}
}

func TestFailedRolloutRollsBack(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, artID := e.app(t, javaApp())
	rel, _ := e.d.Deploy(ctx, DeployRequest{App: a, ArtifactID: &artID})
	e.waitRelease(t, rel.ID)

	// next release crash-loops
	e.k.RolloutOK, e.k.PodIssue = false, "CrashLoopBackOff"
	e.d.opt.RolloutTimeout = 300 * time.Millisecond
	rel2, _ := e.d.Deploy(ctx, DeployRequest{App: a, ArtifactID: &artID})
	rel2 = e.waitRelease(t, rel2.ID)
	if rel2.Status != store.ReleaseFailed || rel2.Message == "" {
		t.Fatalf("rel2 = %s %q", rel2.Status, rel2.Message)
	}
	a, _ = e.st.GetApp(ctx, a.ID)
	if a.Status != StatusDegraded || !strings.Contains(a.StatusMessage, "still serving #1") {
		t.Fatalf("app = %s %q", a.Status, a.StatusMessage)
	}
	// the deployment template points back at release 1
	dep := e.k.Get("Deployment", "prj-shop", "api")
	if kube.Str(dep, "spec", "template", "metadata", "labels", LabelRelease) != "1" {
		t.Errorf("not rolled back: %v", kube.Field(dep, "spec", "template", "metadata", "labels"))
	}
	if live, _ := e.st.LiveRelease(ctx, a.ID); live.ID != rel.ID {
		t.Error("release 1 should still be live")
	}
}

func TestReleaseCommandFailureStopsDeploy(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	app := javaApp()
	app.Config.ReleaseCommand = "java -jar /app/app.jar migrate"
	a, artID := e.app(t, app)
	e.k.JobOK = false
	rel, _ := e.d.Deploy(ctx, DeployRequest{App: a, ArtifactID: &artID})
	rel = e.waitRelease(t, rel.ID)
	if rel.Status != store.ReleaseFailed || !strings.Contains(rel.Message, "release command failed") {
		t.Fatalf("rel = %s %q", rel.Status, rel.Message)
	}
	if e.k.Get("Deployment", "prj-shop", "api") != nil {
		t.Error("workload must not be applied when the release command fails")
	}
	if job := e.k.Get("Job", "prj-shop", "api-release-1"); job == nil {
		t.Error("release job missing")
	}
}

func TestStaticSiteCronAndTask(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// static site: archive extraction + rendered nginx config with SPA fallback
	site, siteArt := e.app(t, store.App{Slug: "web", Name: "Web", Kind: "web", RuntimeID: "static", RuntimeVersion: "nginx",
		Config: store.AppConfig{Port: 8080, Replicas: 1, Health: store.HealthConfig{Type: "http", Path: "/"}}})
	e.st.DB().Exec(`UPDATE artifacts SET detect = ? WHERE id = ?`, `{"type":"zip","archive":{"format":"zip","strip_prefix":"dist/"}}`, siteArt)
	rel, err := e.d.Deploy(ctx, DeployRequest{App: site, ArtifactID: &siteArt})
	if err != nil {
		t.Fatal(err)
	}
	if rel = e.waitRelease(t, rel.ID); rel.Status != store.ReleaseLive {
		t.Fatalf("static: %s %s", rel.Status, rel.Message)
	}
	cm := e.k.Get("ConfigMap", "prj-shop", "web-files")
	b, _ := json.Marshal(cm)
	if !strings.Contains(string(b), "try_files $uri $uri/ /index.html") || !strings.Contains(string(b), "listen 8080") {
		t.Errorf("nginx config: %s", b)
	}
	initEnv, _ := json.Marshal(kube.Field(e.k.Get("Deployment", "prj-shop", "web"), "spec", "template", "spec", "initContainers"))
	if !strings.Contains(string(initEnv), `"ARTIFACT_STRIP","value":"dist"`) || !strings.Contains(string(initEnv), `"archive"`) {
		t.Errorf("init: %s", initEnv)
	}

	// cron job
	cron, cronArt := e.app(t, store.App{Slug: "nightly", Name: "Nightly", Kind: "cron", RuntimeID: "java", RuntimeVersion: "21",
		Config: store.AppConfig{Replicas: 1, Schedule: "0 3 * * *"}})
	rel, _ = e.d.Deploy(ctx, DeployRequest{App: cron, ArtifactID: &cronArt})
	if rel = e.waitRelease(t, rel.ID); rel.Status != store.ReleaseLive {
		t.Fatalf("cron: %s", rel.Message)
	}
	cj := e.k.Get("CronJob", "prj-shop", "nightly")
	if kube.Str(cj, "spec", "schedule") != "0 3 * * *" || e.k.Get("Service", "prj-shop", "nightly") != nil {
		t.Errorf("cronjob: %v", cj)
	}
	if a, _ := e.st.GetApp(ctx, cron.ID); a.Status != StatusScheduled {
		t.Errorf("cron status = %s", a.Status)
	}

	// task app: deploy creates no workload; RunTask starts a Job
	task, taskArt := e.app(t, store.App{Slug: "report", Name: "Report", Kind: "task", RuntimeID: "java", RuntimeVersion: "21", Config: store.AppConfig{Replicas: 1}})
	rel, _ = e.d.Deploy(ctx, DeployRequest{App: task, ArtifactID: &taskArt})
	e.waitRelease(t, rel.ID)
	name, err := e.d.RunTask(ctx, task, nil)
	if err != nil || !strings.HasPrefix(name, "report-run-") || e.k.Get("Job", "prj-shop", name) == nil {
		t.Fatalf("RunTask = %q %v", name, err)
	}

	// deleting the site removes its objects only
	if err := e.d.DeleteApp(ctx, site, e.p); err != nil {
		t.Fatal(err)
	}
	if e.k.Get("Deployment", "prj-shop", "web") != nil || e.k.Get("CronJob", "prj-shop", "nightly") == nil {
		t.Error("DeleteApp removed the wrong objects")
	}
}

func TestCapacityAndValidation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	app := javaApp()
	app.Config.Replicas = 10
	app.Config.Resources.CPUMillicores = 1000 // 10 CPU > 4 allocatable
	a, artID := e.app(t, app)
	if _, err := e.d.Deploy(ctx, DeployRequest{App: a, ArtifactID: &artID}); err == nil || !strings.Contains(err.Error(), "not enough CPU") {
		t.Fatalf("capacity: %v", err)
	}
	if _, err := e.d.Deploy(ctx, DeployRequest{App: a, ArtifactID: &artID, Force: true}); err != nil {
		t.Fatalf("force: %v", err)
	}
	e.d.Wait()

	// project quota
	q := store.Quota{MemoryMB: 256}
	e.p, _ = e.st.UpdateProject(ctx, e.p.ID, nil, true, &q)
	small := javaApp()
	small.Slug, small.Config.Replicas = "small", 1
	s, sArt := e.app(t, small)
	if _, err := e.d.Deploy(ctx, DeployRequest{App: s, ArtifactID: &sArt}); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("quota: %v", err)
	}

	// cron without schedule, image app without image, missing artifact
	c := store.App{Slug: "c", Name: "C", Kind: "cron", RuntimeID: "java", RuntimeVersion: "21", ProjectID: e.p.ID}
	c, _ = e.st.CreateApp(ctx, c)
	if _, err := e.d.Deploy(ctx, DeployRequest{App: c}); err == nil {
		t.Error("deploy without artifact accepted")
	}
	img := store.App{Slug: "img", Name: "Img", Kind: "web", RuntimeID: "image", ProjectID: e.p.ID}
	img, _ = e.st.CreateApp(ctx, img)
	if _, err := e.d.Deploy(ctx, DeployRequest{App: img}); err == nil || !strings.Contains(err.Error(), "image") {
		t.Errorf("image app without image: %v", err)
	}
}

func TestArtifactSignature(t *testing.T) {
	sig := SignArtifact([]byte("key"), "abc")
	if !VerifyArtifact([]byte("key"), "abc", sig) || VerifyArtifact([]byte("key"), "abd", sig) || VerifyArtifact([]byte("other"), "abc", sig) {
		t.Fatal("signature verification broken")
	}
}
