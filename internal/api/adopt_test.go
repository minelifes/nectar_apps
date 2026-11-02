package api

import (
	"context"
	"sort"
	"strings"
	"testing"

	"nectar_apps/internal/adopt"
	"nectar_apps/internal/kube"
)

func TestExternalNamespacesAndAdopt(t *testing.T) {
	e := newEnv(t)
	e.api.cluster = e.kube // the adopt endpoints read the fake cluster
	o := e.owner()
	ctx := context.Background()
	for _, ns := range []string{"kube-system", "legacy", "blog"} {
		e.kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}})
	}
	e.kube.Apply(ctx, kube.Object{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "wp", "namespace": "legacy"},
		"spec": map[string]any{"replicas": float64(2), "template": map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "wp", "image": "wordpress:6"}}}}}, "status": map[string]any{"readyReplicas": float64(2)}})
	e.kube.Apply(ctx, kube.Object{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": map[string]any{"name": "wp", "namespace": "legacy"},
		"spec": map[string]any{"rules": []any{map[string]any{"host": "blog.example.com"}}}})
	e.kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": map[string]any{"name": "app", "namespace": "blog"}})
	if code, _, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "own", "name": "Own"}); code != 201 {
		t.Fatal("create project")
	}

	code, _, list := e.do(o, "GET", "/api/external/namespaces", nil)
	if code != 200 || len(list) != 2 { // blog, legacy (not kube-system, not prj-own)
		t.Fatalf("external: %d %v", code, list)
	}
	leg := list[1].(map[string]any)
	wl := leg["workloads"].([]any)[0].(map[string]any)
	if leg["name"] != "legacy" || wl["name"] != "wp" || wl["ready"].(float64) != 2 || leg["hosts"].([]any)[0] != "blog.example.com" {
		t.Fatalf("legacy: %v", leg)
	}

	// a namespace with its own "app" ServiceAccount is refused
	if code, _, _ := e.do(o, "POST", "/api/external/namespaces/blog/adopt", nil); code != 409 {
		t.Fatalf("adopt blog: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/external/namespaces/kube-system/adopt", nil); code != 400 {
		t.Fatalf("adopt kube-system: %d", code)
	}
	code, proj, _ := e.do(o, "POST", "/api/external/namespaces/legacy/adopt", map[string]any{"name": "Legacy blog"})
	if code != 201 || proj["namespace"] != "legacy" || proj["slug"] != "legacy" || proj["adopted"] != true {
		t.Fatalf("adopt: %d %v", code, proj)
	}
	ns := e.kube.Get("Namespace", "", "legacy")
	if kube.Str(ns, "metadata", "labels", "nectar.dev/adopted") != "true" || kube.Str(ns, "metadata", "labels", "pod-security.kubernetes.io/enforce") != "" {
		t.Fatalf("namespace labels: %v", ns["metadata"])
	}
	if e.kube.Has("NetworkPolicy", "legacy", "nectar-isolation") || e.kube.Has("LimitRange", "legacy", "defaults") {
		t.Fatal("adopted namespace must not get isolation / defaults")
	}
	if code, _, _ := e.do(o, "POST", "/api/external/namespaces/legacy/adopt", nil); code != 409 {
		t.Fatalf("adopt twice: %d", code)
	}
	// the adopted project shows what already runs in its namespace (not the panel's own objects)
	code, ext, _ := e.do(o, "GET", "/api/projects/"+itoa(proj["id"].(float64))+"/external-workloads", nil)
	if code != 200 || ext["name"] != "legacy" || len(ext["workloads"].([]any)) != 1 || ext["hosts"].([]any)[0] != "blog.example.com" {
		t.Fatalf("project external workloads: %d %v", code, ext)
	}
	// a project the panel created has none
	if code, ext, _ := e.do(o, "GET", "/api/projects/1/external-workloads", nil); code != 200 || len(ext["workloads"].([]any)) != 0 {
		t.Fatalf("own project: %d %v", code, ext)
	}
	// deleting the project keeps the namespace and the user's workloads
	if code, _, _ := e.do(o, "DELETE", "/api/projects/"+itoa(proj["id"].(float64)), nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if !e.kube.Has("Namespace", "", "legacy") || !e.kube.Has("Deployment", "legacy", "wp") || e.kube.Has("ServiceAccount", "legacy", "app") {
		t.Fatal("delete of an adopted project must only remove Nectar objects")
	}
	if kube.Str(e.kube.Get("Namespace", "", "legacy"), "metadata", "labels", "nectar.dev/adopted") != "" {
		t.Fatal("adopted label not removed")
	}

	code, st, _ := e.do(o, "GET", "/api/adopt", nil)
	if code != 200 || st["state"] != nil {
		t.Fatalf("adopt status without a migration: %d %v", code, st)
	}
	e.kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "nectar-adopt", "namespace": ""},
		"data": map[string]any{"state.json": `{"phase":"soaking"}`, "report.md": "# r"}})
	code, st, _ = e.do(o, "GET", "/api/adopt", nil)
	if code != 200 || st["state"].(map[string]any)["phase"] != "soaking" || st["report_markdown"] != "# r" {
		t.Fatalf("adopt status: %d %v", code, st)
	}
}

// install.sh --adopt asks for projects through a ConfigMap; the panel adopts them and removes the request.
func TestAdoptRequestsFromInstaller(t *testing.T) {
	e := newEnv(t)
	e.api.cluster = e.kube
	ctx := context.Background()
	for _, ns := range []string{"family", "portainer", "kube-system"} {
		e.kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}})
	}
	e.kube.Apply(ctx, kube.Object{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "web", "namespace": "family"}})
	names, err := adopt.AdoptableNamespaces(ctx, e.kube, e.api.cfg.SystemNamespace)
	if err != nil || strings.Join(names, " ") != "family portainer" {
		t.Fatalf("adoptable: %v %v", names, err)
	}
	if err := adopt.RequestProjects(ctx, e.kube, e.api.cfg.SystemNamespace, []string{"family", "kube-system"}); err != nil {
		t.Fatal(err)
	}
	if err := adopt.RequestProjects(ctx, e.kube, e.api.cfg.SystemNamespace, []string{"portainer", "family"}); err != nil {
		t.Fatal(err)
	}
	cm := e.kube.Get("ConfigMap", e.api.cfg.SystemNamespace, adopt.ProjectRequestsConfigMap)
	if kube.Str(cm, "data", "namespaces") != "family kube-system portainer" {
		t.Fatalf("requests: %v", cm["data"])
	}
	e.api.processAdoptRequests(ctx)
	ps, _ := e.api.store.ListProjects(ctx)
	got := []string{}
	for _, p := range ps {
		if p.Adopted {
			got = append(got, p.Namespace)
		}
	}
	sort.Strings(got)
	if strings.Join(got, " ") != "family portainer" {
		t.Fatalf("adopted: %v", got)
	}
	if e.kube.Has("ConfigMap", e.api.cfg.SystemNamespace, adopt.ProjectRequestsConfigMap) {
		t.Fatal("handled requests must be removed (kube-system is refused for good)")
	}
	if !e.kube.Has("Deployment", "family", "web") {
		t.Fatal("existing workloads stay")
	}
}
