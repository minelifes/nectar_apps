package deploy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

func TestVolumesFilesScaleRestartPending(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a := javaApp()
	a.Config.Expose = nil
	a.Config.Volumes = []store.Volume{{Name: "data", MountPath: "/data", SizeMB: 1024}}
	a, _ = e.app(t, a)
	enc, _ := e.box.Seal("key: value\n")
	if err := e.st.ReplaceFiles(ctx, a.ID, []store.AppFile{{Path: "/app/config/application.yml", ContentEnc: enc}}); err != nil {
		t.Fatal(err)
	}

	if err := e.d.Scale(ctx, a, e.p, 3); err != ErrNotDeployed {
		t.Fatalf("scale before deploy: %v", err)
	}
	if p, _ := e.d.Pending(ctx, a); len(p) != 0 {
		t.Fatalf("pending before the first deploy: %v", p)
	}
	rel, err := e.d.Deploy(ctx, DeployRequest{App: a})
	if err != nil {
		t.Fatal(err)
	}
	if rel = e.waitRelease(t, rel.ID); rel.Status != store.ReleaseLive {
		t.Fatalf("release %s %s", rel.Status, rel.Message)
	}
	ns := e.p.Namespace
	pvc := e.k.Get("PersistentVolumeClaim", ns, "api-data")
	if pvc == nil || kube.Str(pvc, "spec", "resources", "requests", "storage") != "1024Mi" || kube.Str(pvc, "metadata", "labels", LabelVolume) != "data" {
		t.Fatalf("pvc: %v", pvc)
	}
	sec := e.k.Get("Secret", ns, "api-userfiles")
	if v, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "file-00")); string(v) != "key: value\n" {
		t.Fatalf("user file secret: %v", sec)
	}
	pod, _ := json.Marshal(kube.Field(e.k.Get("Deployment", ns, "api"), "spec", "template", "spec"))
	for _, want := range []string{`"claimName":"api-data"`, `"mountPath":"/data"`, `"mountPath":"/app/config/application.yml","name":"userfiles","readOnly":true,"subPath":"file-00"`} {
		if !strings.Contains(string(pod), want) {
			t.Errorf("pod spec lacks %s", want)
		}
	}

	// a PVC is never re-applied (its spec is immutable)
	e.k.Get("PersistentVolumeClaim", ns, "api-data")["marker"] = "kept"

	// scale and stop/start keep working without a release
	if err := e.d.Scale(ctx, a, e.p, 0); err != nil {
		t.Fatal(err)
	}
	if kube.Int(e.k.Get("Deployment", ns, "api"), "spec", "replicas") != 0 {
		t.Fatal("not scaled to 0")
	}
	a, _ = e.st.GetApp(ctx, a.ID)
	if a.Config.Replicas != 0 || a.Config.ResumeReplicas != 2 || a.Status != StatusStopped {
		t.Fatalf("after stop: %+v %s", a.Config, a.Status)
	}
	if err := e.d.Restart(ctx, a, e.p); err == nil {
		t.Fatal("restart of a stopped app")
	}
	if err := e.d.Scale(ctx, a, e.p, 2); err != nil {
		t.Fatal(err)
	}
	a, _ = e.st.GetApp(ctx, a.ID)
	if err := e.d.Restart(ctx, a, e.p); err != nil {
		t.Fatal(err)
	}
	if kube.Str(e.k.Get("Deployment", ns, "api"), "spec", "template", "metadata", "annotations", "nectar.dev/restarted-at") == "" {
		t.Fatal("restart annotation missing")
	}
	if p, _ := e.d.Pending(ctx, a); len(p) != 0 {
		t.Fatalf("scaling must not count as a pending change: %v", p)
	}

	// settings and env changes are pending until the next deploy
	time.Sleep(1100 * time.Millisecond) // env_changed_at has second resolution
	e.st.ReplaceEnv(ctx, a.ID, nil)
	a.Config.Health.Path = "/health"
	a, _ = e.st.SaveApp(ctx, a)
	if p, _ := e.d.Pending(ctx, a); !slices.Equal(p, []string{"settings", "environment"}) {
		t.Fatalf("pending: %v", p)
	}

	// volumes: removing one from the settings keeps its data until it is deleted explicitly
	a.Config.Volumes = nil
	a, _ = e.st.SaveApp(ctx, a)
	if err := e.d.DeleteVolume(ctx, a, e.p, "data"); err == nil {
		t.Fatal("deleted a volume the live release mounts")
	}
	rel, _ = e.d.Deploy(ctx, DeployRequest{App: a})
	e.waitRelease(t, rel.ID)
	if e.k.Get("PersistentVolumeClaim", ns, "api-data")["marker"] != "kept" {
		t.Fatal("the deploy touched or pruned the volume")
	}
	vols, err := e.d.Volumes(ctx, a, e.p)
	if err != nil || len(vols) != 1 || vols[0].InConfig || vols[0].Mounted || vols[0].RequestedMB != 1024 {
		t.Fatalf("volumes: %+v %v", vols, err)
	}
	// finished backup/restore pods that mount the claim would keep it Terminating: they go first;
	// a running pod (and pods of other claims) stay
	mkPod := func(name, phase, claim string) kube.Object {
		return kube.Object{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": ns},
			"spec":   map[string]any{"volumes": []any{map[string]any{"name": "v", "persistentVolumeClaim": map[string]any{"claimName": claim}}}},
			"status": map[string]any{"phase": phase}}
	}
	for _, p := range []kube.Object{mkPod("backup-done", "Succeeded", "api-data"), mkPod("restore-failed", "Failed", "api-data"),
		mkPod("other", "Succeeded", "web-data"), mkPod("busy", "Running", "api-data")} {
		e.k.Apply(ctx, p)
	}
	if err := e.d.DeleteVolume(ctx, a, e.p, "data"); err != nil || e.k.Has("PersistentVolumeClaim", ns, "api-data") {
		t.Fatalf("delete volume: %v", err)
	}
	if e.k.Has("Pod", ns, "backup-done") || e.k.Has("Pod", ns, "restore-failed") || !e.k.Has("Pod", ns, "other") || !e.k.Has("Pod", ns, "busy") {
		t.Fatal("releaseClaim deleted the wrong pods")
	}
}
