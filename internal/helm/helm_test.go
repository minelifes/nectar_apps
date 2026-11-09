package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

// helmSecret builds a release Secret the way Helm 3 stores it (base64(gzip(json)), then base64 again by
// the API).
func helmSecret(t *testing.T, ns, name string, rev int, status, chartVer string) kube.Object {
	t.Helper()
	rel := map[string]any{"name": name, "version": rev,
		"info":  map[string]any{"status": status, "last_deployed": time.Date(2026, 11, 1, 10, rev, 0, 0, time.UTC), "description": "Upgrade complete", "notes": "Visit http://x"},
		"chart": map[string]any{"metadata": map[string]any{"name": "nginx", "version": chartVer, "appVersion": "1.27"}}}
	js, _ := json.Marshal(rel)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(js)
	zw.Close()
	helmData := base64.StdEncoding.EncodeToString(gz.Bytes())
	return kube.Object{"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]any{"name": "sh.helm.release.v1." + name + ".v" + string(rune('0'+rev)), "namespace": ns,
			"labels": map[string]any{"owner": "helm", "name": name}},
		"data": map[string]any{"release": base64.StdEncoding.EncodeToString([]byte(helmData))}}
}

func TestHistoryFromReleaseSecrets(t *testing.T) {
	k := kubefake.New()
	ctx := context.Background()
	k.Apply(ctx, helmSecret(t, "p-shop", "web", 1, "superseded", "18.1.0"))
	k.Apply(ctx, helmSecret(t, "p-shop", "web", 2, "deployed", "18.2.0"))
	k.Apply(ctx, helmSecret(t, "p-shop", "other", 1, "deployed", "1.0.0"))
	h, err := History(ctx, k, "p-shop", "web")
	if err != nil || len(h) != 2 {
		t.Fatalf("history: %v %v", h, err)
	}
	if h[0].Revision != 2 || h[0].Status != "deployed" || h[0].ChartVersion != "18.2.0" || h[0].AppVersion != "1.27" ||
		h[0].Chart != "nginx" || h[0].Notes != "Visit http://x" || h[1].Status != "superseded" {
		t.Fatalf("history: %+v", h)
	}
}

func TestOpCheck(t *testing.T) {
	good := Op{Kind: "install", Name: "web", Repo: "https://charts.bitnami.com/bitnami", Chart: "nginx", Version: "^18"}
	if err := good.Check(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Op{
		{Name: "Web", Repo: good.Repo, Chart: "nginx"},
		{Name: "web", Repo: "file:///etc", Chart: "nginx"},
		{Name: "web", Repo: "https://x/$(id)", Chart: "nginx"},
		{Name: "web", Repo: good.Repo, Chart: "../x"},
		{Name: "web", Repo: good.Repo, Chart: "nginx", Version: "1; rm -rf /"},
		{Name: strings.Repeat("a", 54), Repo: good.Repo, Chart: "nginx"},
	} {
		if bad.Check() == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

// Run: namespace-only access, a Job with the operation in its environment and the values in a Secret
// that is gone afterwards.
func TestRunCreatesAJobInTheNamespace(t *testing.T) {
	k := kubefake.New()
	ctx := context.Background()
	r := &Runner{Kube: k, Poll: time.Millisecond}
	out, err := r.Run(ctx, "p-shop", Op{Kind: "install", Name: "web", Repo: "oci://registry-1.docker.io/bitnamicharts", Chart: "nginx",
		Values: "replicaCount: 2\n", Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	if !k.Has("ServiceAccount", "p-shop", ServiceAccount) || !k.Has("Role", "p-shop", ServiceAccount) || !k.Has("RoleBinding", "p-shop", ServiceAccount) {
		t.Fatal("access objects missing")
	}
	if k.Has("ClusterRoleBinding", "", ServiceAccount) {
		t.Fatal("cluster-wide access")
	}
	jobs, _ := k.List(ctx, "Job", "p-shop", "")
	if len(jobs) != 1 {
		t.Fatalf("jobs: %d", len(jobs))
	}
	js, _ := json.Marshal(jobs[0])
	for _, want := range []string{`"serviceAccountName":"nectar-helm"`, `{"name":"OP","value":"install"}`, `{"name":"REPO","value":"oci://registry-1.docker.io/bitnamicharts"}`,
		`"image":"` + Image + `"`, `"runAsNonRoot":true`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("job lacks %s", want)
		}
	}
	if strings.Contains(string(js), "replicaCount") || strings.Contains(string(js), `"p"}`) {
		t.Error("values or password in the Job itself")
	}
	secrets, _ := k.List(ctx, "Secret", "p-shop", "")
	if len(secrets) != 0 {
		t.Fatalf("input secret left: %d", len(secrets))
	}

	k.JobOK = false
	if _, err := r.Run(ctx, "p-shop", Op{Kind: "uninstall", Name: "web"}); err == nil {
		t.Fatal("failed job not reported")
	}
}
