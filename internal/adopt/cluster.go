package adopt

import (
	"context"
	"sort"
	"strings"

	"nectar_apps/internal/kube"
)

// Kube is the part of the Kubernetes client adopt mode uses (kube.Client and kubefake.Fake).
type Kube interface {
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	Patch(ctx context.Context, kind, namespace, name string, patch any) (kube.Object, error)
	Delete(ctx context.Context, kind, namespace, name string) error
}

// list returns nil (not an error) when the kind's CRD is not installed.
func list(ctx context.Context, k Kube, kind string) ([]kube.Object, error) {
	items, err := k.List(ctx, kind, "", "")
	if kube.IsNotFound(err) {
		return nil, nil
	}
	return items, err
}

// LoadCluster reads everything the translators need, plus the Secrets that routes reference
// (TLS and basic auth). Missing CRDs (no Traefik, no Gateway API) are fine.
func LoadCluster(ctx context.Context, k Kube) (Cluster, error) {
	var c Cluster
	var err error
	for _, x := range []struct {
		kind string
		dst  *[]kube.Object
	}{
		{"Ingress", &c.Ingresses}, {"IngressClass", &c.IngressClasses}, {"Service", &c.Services},
		{"IngressRoute", &c.IngressRoutes}, {"Middleware", &c.Middlewares},
		{"HTTPRoute", &c.HTTPRoutes}, {"Gateway", &c.Gateways},
	} {
		if *x.dst, err = list(ctx, k, x.kind); err != nil {
			return c, err
		}
	}
	// older Traefik installs (v2) use the traefik.containo.us group
	if legacy, err := list(ctx, k, "IngressRoute.containo.us"); err == nil {
		c.IngressRoutes = append(c.IngressRoutes, legacy...)
	}
	if legacy, err := list(ctx, k, "Middleware.containo.us"); err == nil {
		c.Middlewares = append(c.Middlewares, legacy...)
	}
	if gcs, err := listGatewayClasses(ctx, k); err == nil {
		c.GatewayClasses = gcs
	}
	c.Secrets = []kube.Object{}
	for _, ref := range secretRefs(c) {
		ns, name, _ := strings.Cut(ref, "/")
		var s kube.Object
		if err := k.GetObject(ctx, "Secret", ns, name, &s); err == nil {
			s["kind"] = "Secret"
			c.Secrets = append(c.Secrets, s)
		} else if !kube.IsNotFound(err) {
			return c, err
		}
	}
	return c, nil
}

func listGatewayClasses(ctx context.Context, k Kube) ([]kube.Object, error) {
	if !kube.HasKind("GatewayClass") {
		return nil, nil
	}
	return list(ctx, k, "GatewayClass")
}

// secretRefs lists "namespace/name" of the Secrets referenced by Ingresses, IngressRoutes, Middlewares
// and Gateways (TLS certificates and basic-auth users).
func secretRefs(c Cluster) []string {
	set := map[string]bool{}
	add := func(ns, name string) {
		if ns != "" && name != "" {
			set[ns+"/"+name] = true
		}
	}
	for _, ing := range c.Ingresses {
		ns := kube.Str(ing, "metadata", "namespace")
		for _, t := range kube.Slice(ing, "spec", "tls") {
			add(ns, kube.Str(t, "secretName"))
		}
		if v := kube.Str(ing, "metadata", "annotations", "nginx.ingress.kubernetes.io/auth-secret"); v != "" {
			if a, b, ok := strings.Cut(v, "/"); ok {
				add(a, b)
			} else {
				add(ns, v)
			}
		}
	}
	for _, ir := range c.IngressRoutes {
		add(kube.Str(ir, "metadata", "namespace"), kube.Str(ir, "spec", "tls", "secretName"))
	}
	for _, mw := range c.Middlewares {
		add(kube.Str(mw, "metadata", "namespace"), kube.Str(mw, "spec", "basicAuth", "secret"))
	}
	for _, gw := range c.Gateways {
		gns := kube.Str(gw, "metadata", "namespace")
		for _, l := range kube.Slice(gw, "spec", "listeners") {
			for _, r := range kube.Slice(l, "tls", "certificateRefs") {
				ns := kube.Str(r, "namespace")
				if ns == "" {
					ns = gns
				}
				add(ns, kube.Str(r, "name"))
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
