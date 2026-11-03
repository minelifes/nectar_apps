package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"nectar_apps/internal/kube"
)

// A Deployment of an adopted namespace becomes a Nectar app: config translated, original scaled to 0,
// its Service repointed, the PVC reused; deleting the app brings the original back.
func TestImportWorkload(t *testing.T) {
	e := newEnv(t)
	e.api.cluster = e.kube
	o := e.owner()
	ctx := context.Background()
	ap := func(obj kube.Object) { e.kube.Apply(ctx, obj) }
	ap(kube.Object{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "family"}})
	ap(kube.Object{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "db", "namespace": "family"},
		"data": map[string]any{"password": base64.StdEncoding.EncodeToString([]byte("s3cret"))}})
	ap(kube.Object{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "cfg", "namespace": "family"},
		"data": map[string]any{"MODE": "prod", "app.conf": "listen 80;"}})
	ap(kube.Object{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "web-data", "namespace": "family"},
		"spec": map[string]any{"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "2Gi"}}}})
	ap(kube.Object{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "web", "namespace": "family"},
		"spec": map[string]any{"replicas": float64(2), "selector": map[string]any{"matchLabels": map[string]any{"app": "web"}},
			"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "web"}}, "spec": map[string]any{
				"containers": []any{map[string]any{"name": "web", "image": "ghcr.io/me/family:3", "args": []any{"--port", "80"},
					"ports": []any{map[string]any{"name": "http", "containerPort": float64(80)}},
					"env": []any{map[string]any{"name": "TZ", "value": "UTC"},
						map[string]any{"name": "DB_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "db", "key": "password"}}},
						map[string]any{"name": "POD_IP", "valueFrom": map[string]any{"fieldRef": map[string]any{"fieldPath": "status.podIP"}}}},
					"envFrom":        []any{map[string]any{"configMapRef": map[string]any{"name": "cfg"}}},
					"resources":      map[string]any{"limits": map[string]any{"cpu": "500m", "memory": "1Gi"}},
					"readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/healthz", "port": "http"}},
					"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/data"},
						map[string]any{"name": "conf", "mountPath": "/app/app.conf", "subPath": "app.conf"},
						map[string]any{"name": "tmp", "mountPath": "/tmp"}}}},
				"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "web-data"}},
					map[string]any{"name": "conf", "configMap": map[string]any{"name": "cfg"}},
					map[string]any{"name": "tmp", "emptyDir": map[string]any{}}}}}}})
	ap(kube.Object{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "web", "namespace": "family"},
		"spec": map[string]any{"type": "ClusterIP", "selector": map[string]any{"app": "web"},
			"ports": []any{map[string]any{"port": float64(80), "targetPort": "http"}}}})
	ap(kube.Object{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": map[string]any{"name": "mixed", "namespace": "family"},
		"spec": map[string]any{"rules": []any{map[string]any{"host": "mixed.example.com", "http": map[string]any{"paths": []any{
			map[string]any{"path": "/", "backend": map[string]any{"service": map[string]any{"name": "web", "port": map[string]any{"number": float64(80)}}}},
			map[string]any{"path": "/api", "backend": map[string]any{"service": map[string]any{"name": "api", "port": map[string]any{"number": float64(80)}}}}}}}}}})
	ap(kube.Object{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": map[string]any{"name": "web", "namespace": "family"},
		"spec": map[string]any{"tls": []any{map[string]any{"hosts": []any{"family.example.com"}, "secretName": "family-tls"}}, "rules": []any{map[string]any{"host": "family.example.com", "http": map[string]any{"paths": []any{
			map[string]any{"path": "/", "backend": map[string]any{"service": map[string]any{"name": "web", "port": map[string]any{"number": float64(80)}}}}}}}}}})
	ap(kube.Object{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "sidecar", "namespace": "family"},
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{
			map[string]any{"name": "a", "image": "a"}, map[string]any{"name": "b", "image": "b"}}}}}})

	code, proj, _ := e.do(o, "POST", "/api/external/namespaces/family/adopt", nil)
	if code != 201 {
		t.Fatalf("adopt: %d %v", code, proj)
	}
	base := "/api/projects/" + itoa(proj["id"].(float64)) + "/external-workloads/"

	code, plan, _ := e.do(o, "GET", base+"deployment/web/import", nil)
	if code != 200 || len(plan["problems"].([]any)) != 0 {
		t.Fatalf("preview: %d %v", code, plan)
	}
	cfg := plan["config"].(map[string]any)
	if plan["slug"] != "web-app" || plan["kind"] != "web" || cfg["image"] != "ghcr.io/me/family:3" || cfg["port"].(float64) != 80 ||
		cfg["replicas"].(float64) != 2 || cfg["health"].(map[string]any)["path"] != "/healthz" ||
		cfg["resources"].(map[string]any)["cpu_millicores"].(float64) != 500 || cfg["resources"].(map[string]any)["memory_mb"].(float64) != 1024 {
		t.Fatalf("plan: %v", plan)
	}
	vol := cfg["volumes"].([]any)[0].(map[string]any)
	if vol["claim"] != "web-data" || vol["mount_path"] != "/data" || vol["size_mb"].(float64) != 2048 {
		t.Fatalf("volume: %v", vol)
	}
	if strings.Join(toStrings(plan["env"]), ",") != "DB_PASSWORD,MODE,TZ,app.conf" {
		t.Fatalf("env: %v", plan["env"])
	}
	if toStrings(plan["files"])[0] != "/app/app.conf" || toStrings(plan["services"])[0] != "web" || strings.Join(toStrings(plan["hosts"]), ",") != "mixed.example.com" {
		t.Fatalf("files/services/hosts: %v", plan)
	}
	if doms := plan["domains"].([]any); len(doms) != 1 || jsonOf(doms[0]) != `{"host":"family.example.com","path_prefix":"/","tls":true}` {
		t.Fatalf("domains: %v", plan["domains"])
	}
	if w := strings.Join(toStrings(plan["warnings"]), "\n"); !strings.Contains(w, "POD_IP") || !strings.Contains(w, "/tmp") {
		t.Fatalf("warnings: %s", w)
	}
	if code, bad, _ := e.do(o, "GET", base+"deployment/sidecar/import", nil); code != 200 || !strings.Contains(strings.Join(toStrings(bad["problems"]), ""), "2 containers") {
		t.Fatalf("sidecar preview: %d %v", code, bad)
	}
	if code, _, _ := e.do(o, "POST", base+"deployment/sidecar/import", nil); code != 400 {
		t.Fatalf("sidecar import: %d", code)
	}

	code, res, _ := e.do(o, "POST", base+"deployment/web/import", map[string]any{"name": "Family web"})
	if code != 201 {
		t.Fatalf("import: %d %v", code, res)
	}
	e.dep.Wait()
	app := res["app"].(map[string]any)
	appID := itoa(app["id"].(float64))
	if _, _, doms := e.do(o, "GET", "/api/apps/"+appID+"/domains", nil); len(doms) != 1 || doms[0].(map[string]any)["host"] != "family.example.com" ||
		doms[0].(map[string]any)["force_https"] != true {
		t.Fatalf("app domains: %v", doms)
	}
	old := e.kube.Get("Deployment", "family", "web")
	if kube.Int(old, "spec", "replicas") != 0 || kube.Str(old, "metadata", "annotations", annImportedAs) != "web-app" {
		t.Fatalf("original not scaled down: %v", old["spec"])
	}
	svc := e.kube.Get("Service", "family", "web")
	sel := svc["spec"].(map[string]any)["selector"].(map[string]any)
	if sel["app"] != nil || sel["nectar.dev/app"] != "web-app" || kube.Int(kube.Slice(svc, "spec", "ports")[0], "targetPort") != 80 {
		t.Fatalf("service not repointed: %v", svc["spec"])
	}
	dep := e.kube.Get("Deployment", "family", "web-app")
	pod := kube.Field(dep, "spec", "template", "spec").(map[string]any)
	if !strings.Contains(jsonOf(pod), `"claimName":"web-data"`) ||
		!strings.Contains(jsonOf(pod), `"args":["--port","80"]`) || e.kube.Has("PersistentVolumeClaim", "family", "web-app-data") {
		t.Fatalf("app pod: %s", jsonOf(pod))
	}
	if _, _, envList := e.do(o, "GET", "/api/apps/"+appID+"/env", nil); len(envList) != 4 || !strings.Contains(jsonOf(envList), `"DB_PASSWORD"`) {
		t.Fatalf("env: %v", envList)
	}
	if code, ext, _ := e.do(o, "GET", "/api/projects/"+itoa(proj["id"].(float64))+"/external-workloads", nil); code != 200 || strings.Contains(jsonOf(ext["workloads"]), `"name":"web"`) || strings.Contains(jsonOf(ext["services"]), `"name":"web"`) {
		t.Fatalf("imported workload still listed: %v", ext)
	}
	if code, _, _ := e.do(o, "POST", base+"deployment/web/import", nil); code != 409 {
		t.Fatalf("second import: %d", code)
	}

	// deleting the app restores the original
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+appID, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	old = e.kube.Get("Deployment", "family", "web")
	svc = e.kube.Get("Service", "family", "web")
	sel = svc["spec"].(map[string]any)["selector"].(map[string]any)
	if kube.Int(old, "spec", "replicas") != 2 || kube.Str(old, "metadata", "annotations", annImportedAs) != "" ||
		sel["app"] != "web" || sel["nectar.dev/app"] != nil || kube.Str(kube.Slice(svc, "spec", "ports")[0], "targetPort") != "http" {
		t.Fatalf("not restored: %v / %v", old["spec"], svc["spec"])
	}
	if !e.kube.Has("PersistentVolumeClaim", "family", "web-data") {
		t.Fatal("the imported claim must survive the app")
	}
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
