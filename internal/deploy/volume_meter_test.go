package deploy

import (
	"context"
	"strings"
	"testing"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

// meterKube completes the measuring pods like a node would: du prints the folder sizes.
type meterKube struct {
	*kubefake.Fake
	out     string
	created []kube.Object
}

func (m *meterKube) Apply(ctx context.Context, o kube.Object) (kube.Object, error) {
	if kind, _, name := kube.Meta(o); kind == "Pod" && strings.HasPrefix(name, "nectar-du-") {
		m.created = append(m.created, o)
		o["status"] = map[string]any{"phase": "Succeeded"}
	}
	return m.Fake.Apply(ctx, o)
}

func (m *meterKube) PodLogs(_ context.Context, _, pod, _ string, _ int, _ bool) (string, error) {
	return m.out, nil
}

func localPV(name, ns, claim, path, node string) kube.Object {
	return kube.Object{"apiVersion": "v1", "kind": "PersistentVolume", "metadata": map[string]any{"name": name},
		"spec": map[string]any{
			"claimRef": map[string]any{"namespace": ns, "name": claim},
			"hostPath": map[string]any{"path": path},
			"nodeAffinity": map[string]any{"required": map[string]any{"nodeSelectorTerms": []any{map[string]any{
				"matchExpressions": []any{map[string]any{"key": "kubernetes.io/hostname", "operator": "In", "values": []any{node}}}}}}},
		}}
}

func pvc(ns, name string) kube.Object {
	return kube.Object{"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": map[string]any{LabelManagedBy: ManagedBy}},
		"spec":     map[string]any{"resources": map[string]any{"requests": map[string]any{"storage": "2Gi"}}}}
}

// The real size of local-path volumes (folders on the node) is measured with du and shown instead of the
// whole disk: a database with a few MB is not "5.8 GiB of 2 GiB".
func TestVolumeMeterMeasuresLocalPathFolders(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	mk := &meterKube{Fake: e.k, out: "40960\t/v/0\n2048\t/v/1\n"}
	e.d.kube = mk
	for _, o := range []kube.Object{
		pvc("prj-shop", "data-db-0"), pvc("prj-shop", "web-files"), pvc("prj-shop", "elsewhere"),
		localPV("pvc-1", "prj-shop", "data-db-0", "/var/lib/rancher/k3s/storage/pvc-1_prj-shop_data-db-0", "n1"),
		localPV("pvc-2", "prj-shop", "web-files", "/var/lib/rancher/k3s/storage/pvc-2_prj-shop_web-files", "n1"),
		// network storage: no host path, the kubelet reports it correctly
		{"apiVersion": "v1", "kind": "PersistentVolume", "metadata": map[string]any{"name": "pvc-3"},
			"spec": map[string]any{"claimRef": map[string]any{"namespace": "prj-shop", "name": "elsewhere"}, "csi": map[string]any{"driver": "driver.longhorn.io"}}},
		// not a Nectar volume
		localPV("pvc-4", "kube-system", "other", "/var/lib/rancher/k3s/storage/pvc-4", "n1"),
	} {
		if _, err := e.k.Apply(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.d.MeasureLocalVolumes(ctx); err != nil {
		t.Fatal(err)
	}
	if len(mk.created) != 1 {
		t.Fatalf("measuring pods: %d", len(mk.created))
	}
	pod := mk.created[0]
	c := kube.Slice(pod, "spec", "containers")[0]
	vols := kube.Slice(pod, "spec", "volumes")
	if kube.Str(pod, "spec", "nodeName") != "n1" || len(vols) != 2 ||
		kube.Str(vols[0], "hostPath", "path") != "/var/lib/rancher/k3s/storage/pvc-1_prj-shop_data-db-0" ||
		kube.Field(kube.Slice(c, "volumeMounts")[0], "readOnly") != true ||
		kube.Field(c, "securityContext", "readOnlyRootFilesystem") != true ||
		len(kube.Field(c, "securityContext", "capabilities", "add").([]any)) != 1 {
		t.Fatalf("pod: %v", pod)
	}
	if e.k.Has("Pod", "nectar-system", kube.Str(pod, "metadata", "name")) {
		t.Fatal("the measuring pod was not removed")
	}
	vols2, err := e.d.AllVolumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, v := range vols2 {
		got[v.Claim] = v.UsedB
	}
	if got["data-db-0"] != 40960*1024 || got["web-files"] != 2048*1024 || got["elsewhere"] != -1 {
		t.Fatalf("usage: %v", got)
	}
}

func TestParseDu(t *testing.T) {
	got := parseDu("12\t/v/0\ndu: cannot read /v/1/x: Permission denied\n7\t/v/1\nnoise\n")
	if len(got) != 2 || got[0] != 12*1024 || got[1] != 7*1024 {
		t.Fatalf("%v", got)
	}
}
