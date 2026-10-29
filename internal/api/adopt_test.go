package api

import (
	"context"
	"testing"

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
