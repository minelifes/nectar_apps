//go:build integration

// Integration test against a real Kubernetes API (e.g. k3s): every object the panel generates must be
// accepted by the API server's schema validation.
//
//	kubectl proxy --port 8001 &
//	NECTAR_KUBE_API=http://127.0.0.1:8001 go test -tags integration ./internal/deploy -run Integration -v
package deploy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

func TestIntegrationManifestsAccepted(t *testing.T) {
	api := os.Getenv("NECTAR_KUBE_API")
	if api == "" {
		t.Skip("NECTAR_KUBE_API not set")
	}
	k, err := kube.New(api)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cat, _ := runtimes.LoadBuiltin()

	slug := "it" + strings.ToLower(time.Now().Format("150405"))
	p := store.Project{ID: 1, Slug: slug, Name: "IT", Namespace: "prj-" + slug, Quota: &store.Quota{CPUMillicores: 4000, MemoryMB: 4096, Pods: 20}}
	for _, o := range ProjectObjects(ProjectSpec{Project: p, NodeIPs: []string{"10.0.0.5"}, SystemNamespace: "nectar-system"}) {
		if _, err := k.Apply(ctx, o); err != nil {
			t.Fatalf("project object rejected: %v", err)
		}
	}
	if os.Getenv("NECTAR_IT_KEEP") == "" {
		t.Cleanup(func() { _ = k.Delete(context.Background(), "Namespace", "", p.Namespace) })
	}

	art := &store.Artifact{SHA256: strings.Repeat("ab", 32), Filename: "x"}
	cases := []struct {
		app     store.App
		version string
	}{
		{store.App{ID: 1, Slug: "api", Kind: "web", RuntimeID: "java", Config: store.AppConfig{Port: 8080, Replicas: 1,
			Resources: store.Resources{CPUMillicores: 500, MemoryMB: 512}, Health: store.HealthConfig{Type: "http", Path: "/health"},
			Expose: []store.ExposedPort{{Port: 1883, Protocol: "TCP"}, {Port: 5683, Protocol: "UDP"}}, ReleaseCommand: "echo migrate"}}, "21"},
		{store.App{ID: 2, Slug: "site", Kind: "web", RuntimeID: "static", Config: store.AppConfig{Port: 8080, Replicas: 1, Health: store.HealthConfig{Type: "http"}}}, "nginx"},
		{store.App{ID: 3, Slug: "worker", Kind: "worker", RuntimeID: "native", Config: store.AppConfig{Replicas: 1, Resources: store.Resources{CPUMillicores: 100, MemoryMB: 64, NoLimit: true}}}, "static"},
		{store.App{ID: 4, Slug: "nightly", Kind: "cron", RuntimeID: "python", Config: store.AppConfig{Replicas: 1, Schedule: "0 3 * * *"}}, "3.13"},
		{store.App{ID: 5, Slug: "php", Kind: "web", RuntimeID: "php", Config: store.AppConfig{Port: 80, Replicas: 1, Health: store.HealthConfig{Type: "tcp"}}}, "8.4"},
		{store.App{ID: 6, Slug: "img", Kind: "web", RuntimeID: "image", Config: store.AppConfig{Image: "traefik/whoami:v1.10", Port: 80, Replicas: 1}}, ""},
	}
	for _, c := range cases {
		rt, _ := cat.Get(c.app.RuntimeID)
		image := c.app.Config.Image
		var a *store.Artifact
		if rt.Artifact.Kind != runtimes.ArtifactNone {
			v, _ := rt.Version(c.version)
			image, a = v.Image, art
		}
		spec := AppSpec{App: c.app, Project: p, Runtime: rt, Image: image, Config: c.app.Config, Release: 1,
			Env: map[string]string{"FOO": "bar"}, Artifact: a, ArtifactURL: "http://nectar-apps.nectar-system.svc/internal/artifacts/x?sig=y", ArchiveFormat: "zip"}
		objs, err := AppObjects(spec)
		if err != nil {
			t.Fatalf("%s: %v", c.app.Slug, err)
		}
		if c.app.Config.ReleaseCommand != "" {
			job, _ := ReleaseJob(spec)
			objs = append(objs, job)
		}
		task, _ := TaskJob(spec, "abc", []string{"sh", "-c", "echo hi"})
		objs = append(objs, task)
		for _, o := range objs {
			if _, err := k.Apply(ctx, o); err != nil {
				t.Errorf("%s: %v", c.app.Slug, err)
			}
		}
	}
	// the NodePort service got server ports assigned
	var svc kube.Object
	if err := k.GetObject(ctx, "Service", p.Namespace, "api-ports", &svc); err != nil {
		t.Fatal(err)
	}
	for _, port := range kube.Slice(svc, "spec", "ports") {
		if np := kube.Int(port, "nodePort"); np < 30000 || np > 32767 {
			t.Errorf("nodePort %d", np)
		}
	}
	// listing by label works on a real server
	items, err := k.List(ctx, "Deployment", p.Namespace, LabelManagedBy+"="+ManagedBy)
	if err != nil || len(items) != 5 { // api, site, worker, php, img (cron is a CronJob)
		t.Errorf("deployments = %d, %v", len(items), err)
	}
}
