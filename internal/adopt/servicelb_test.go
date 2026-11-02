package adopt

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

// While the cutover waits for ServiceLB, it says why: the Service type, the helm-install job (which applies
// the HelmChartConfig of the bundled Traefik) and the ServiceLB pods.
func TestServiceLBStateExplainsTheWait(t *testing.T) {
	ctx := context.Background()
	k := kubefake.New()
	e := &Env{Kube: storedJobs{k}}
	apply := func(o kube.Object) {
		o["apiVersion"] = "v1"
		if _, err := k.Apply(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	apply(obj("Service", "kube-system", "traefik", map[string]any{"spec": map[string]any{"type": "LoadBalancer"}}))
	ds := obj("DaemonSet", "kube-system", "svclb-traefik-abc", nil)
	ds["metadata"].(map[string]any)["labels"] = map[string]any{"svccontroller.k3s.cattle.io/svcname": "traefik", "svccontroller.k3s.cattle.io/svcnamespace": "kube-system"}
	apply(ds)

	if s := e.serviceLBState(ctx, "kube-system", "traefik", true); !strings.Contains(s, "Service type LoadBalancer") ||
		!strings.Contains(s, "helm-install-traefik not started yet") || !strings.Contains(s, "1 ServiceLB DaemonSet") {
		t.Fatalf("before the job: %s", s)
	}
	apply(obj("Job", "kube-system", "helm-install-traefik", map[string]any{"status": map[string]any{"conditions": []any{
		map[string]any{"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded", "message": "Job has reached the specified backoff limit"}}}}))
	if s := e.serviceLBState(ctx, "kube-system", "traefik", true); !strings.Contains(s, "helm-install-traefik failed: BackoffLimitExceeded") ||
		!strings.Contains(s, "logs job/helm-install-traefik") {
		t.Fatalf("failed job: %s", s)
	}
	apply(obj("Job", "kube-system", "helm-install-traefik", map[string]any{"status": map[string]any{"succeeded": 1}}))
	k.Delete(ctx, "DaemonSet", "kube-system", "svclb-traefik-abc")
	if s := e.serviceLBState(ctx, "kube-system", "traefik", true); !strings.Contains(s, "helm-install-traefik done") || !strings.Contains(s, "ServiceLB pods gone") {
		t.Fatalf("done: %s", s)
	}
}

// storedJobs returns Jobs as stored (the fake otherwise reports every Job as succeeded or failed).
type storedJobs struct{ *kubefake.Fake }

func (s storedJobs) GetObject(ctx context.Context, kind, ns, name string, out any) error {
	if kind != "Job" {
		return s.Fake.GetObject(ctx, kind, ns, name, out)
	}
	o := s.Get(kind, ns, name)
	if o == nil {
		return &kube.StatusError{Code: 404, Message: "not found"}
	}
	b, _ := json.Marshal(o)
	return json.Unmarshal(b, out)
}

// k3s 1.36 bundles Traefik chart v40, which reads the Service type from service.spec.type: the old
// service.type was silently ignored and the cutover waited for a ServiceLB release that never came.
func TestTraefikChartVersionPicksTheServiceTypeKey(t *testing.T) {
	for _, c := range []struct {
		version, chart string
		want           int
	}{
		{"", "https://%{KUBERNETES_API}%/static/charts/traefik-40.1.3+up40.1.0.tgz", 40},
		{"", "https://%{KUBERNETES_API}%/static/charts/traefik-27.0.201+up27.0.2.tgz", 27},
		{"v39.0.0", "", 39},
		{"", "somewhere/else.tgz", 0},
	} {
		if got := chartMajor(c.version, c.chart); got != c.want {
			t.Errorf("chartMajor(%q, %q) = %d, want %d", c.version, c.chart, got, c.want)
		}
	}
	v40 := traefikClusterIPValues(map[string]any{"service": map[string]any{"spec": map[string]any{"ipFamilyPolicy": "PreferDualStack"}}}, 40)
	svc := v40["service"].(map[string]any)
	if spec := svc["spec"].(map[string]any); spec["type"] != "ClusterIP" || spec["ipFamilyPolicy"] != "PreferDualStack" || svc["type"] != nil {
		t.Fatalf("v40 values: %v", v40)
	}
	v39 := traefikClusterIPValues(map[string]any{}, 39)
	if s := v39["service"].(map[string]any); s["type"] != "ClusterIP" || s["spec"] != nil {
		t.Fatalf("v39 values: %v", v39)
	}
}

// When k3s finished re-installing Traefik but the Service kept its type, the cutover stops at once with the
// reason instead of waiting out its timeout.
func TestHelmAppliedButIgnoredFailsFast(t *testing.T) {
	ctx := context.Background()
	k := kubefake.New()
	now := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)
	e := &Env{Kube: storedJobs{k}, Now: func() time.Time { return now }}
	put := func(o kube.Object) {
		o["apiVersion"] = "v1"
		k.Apply(ctx, o)
	}
	since := now.Add(-time.Minute)
	put(obj("Service", "kube-system", "traefik", map[string]any{"spec": map[string]any{"type": "LoadBalancer"}}))
	// the job of an earlier install does not count
	put(obj("Job", "kube-system", "helm-install-traefik", map[string]any{"status": map[string]any{"succeeded": 1, "completionTime": now.Add(-time.Hour).Format(time.RFC3339)}}))
	if err := e.helmAppliedButIgnored(ctx, "kube-system", "traefik", since); err != nil {
		t.Fatalf("old job: %v", err)
	}
	// finished 10 s ago: the Service may still be updating
	put(obj("Job", "kube-system", "helm-install-traefik", map[string]any{"status": map[string]any{"succeeded": 1, "completionTime": now.Add(-10 * time.Second).Format(time.RFC3339)}}))
	if err := e.helmAppliedButIgnored(ctx, "kube-system", "traefik", since); err != nil {
		t.Fatalf("just finished: %v", err)
	}
	now = now.Add(30 * time.Second)
	if err := e.helmAppliedButIgnored(ctx, "kube-system", "traefik", since); err == nil || !strings.Contains(err.Error(), "still LoadBalancer") {
		t.Fatalf("ignored: %v", err)
	}
	put(obj("Service", "kube-system", "traefik", map[string]any{"spec": map[string]any{"type": "ClusterIP"}}))
	if err := e.helmAppliedButIgnored(ctx, "kube-system", "traefik", since); err != nil {
		t.Fatalf("applied: %v", err)
	}
}
