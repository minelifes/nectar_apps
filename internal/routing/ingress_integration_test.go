//go:build integration

package routing

import (
	"context"
	"os"
	"testing"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
)

// The Ingress objects must be accepted by a real API server.
//
//	NECTAR_KUBE_API=http://127.0.0.1:8001 go test -tags integration ./internal/routing -run Integration -v
func TestIntegrationIngressAccepted(t *testing.T) {
	api := os.Getenv("NECTAR_KUBE_API")
	if api == "" {
		t.Skip("NECTAR_KUBE_API not set")
	}
	k, _ := kube.New(api)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ns := "prj-itingress"
	if _, err := k.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k.Delete(context.Background(), "Namespace", "", ns) })
	prov := &IngressProvider{Kube: k, ClassName: "traefik", ClusterIssuer: "letsencrypt"}
	empty := ""
	r := gateway.Route{Domain: "shop.example.com", PathPrefix: "/api", Mode: "proxy", TargetIP: "api." + ns + ".svc.cluster.local",
		TargetPort: 8080, ACME: true, ForceSSL: true, ManagedBy: gateway.ManagedBy, Owner: Owner(77), BasicAuthHtpasswd: &empty, BackendProtocol: "http1"}
	if _, err := prov.Create(ctx, r); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := prov.ListManaged(ctx)
	if err != nil || len(list) != 1 || list[0].ID != 77 || differs(r, list[0]) {
		t.Fatalf("list: %v %+v", err, list)
	}
	r.TargetPort = 9090
	if _, err := prov.Update(ctx, r); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := prov.Delete(ctx, 77); err != nil {
		t.Fatal(err)
	}
	if list, _ := prov.ListManaged(ctx); len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}
