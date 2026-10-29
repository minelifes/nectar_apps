package routing

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
)

// Kube is the subset of the Kubernetes client used by the Ingress provider.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
}

// Ingress labels and annotations.
const (
	labelManagedBy   = "app.kubernetes.io/managed-by"
	labelDomainID    = "nectar.dev/domain-id"
	annotationRoute  = "nectar.dev/route" // the desired gateway route, for exact drift detection
	annotationIssuer = "cert-manager.io/cluster-issuer"
)

// IngressProvider implements Gateway with standard Kubernetes Ingress objects instead of
// nectar-gateway (install strategy B: keep the cluster's existing ingress controller, e.g. Traefik
// or ingress-nginx). Certificates come from cert-manager when an issuer is configured.
//
// Ingress has no portable way to express redirects, basic auth, gRPC backends, body limits,
// timeouts or maintenance pages; such domains are rejected with a clear message (they need
// nectar-gateway).
type IngressProvider struct {
	Kube          Kube
	ClassName     string // IngressClass ("" = the cluster default)
	ClusterIssuer string // cert-manager ClusterIssuer for TLS ("" = no TLS)
}

// ErrUnsupported is returned for route features an Ingress cannot express.
var ErrUnsupported = errors.New("not supported with the Kubernetes Ingress provider — use nectar-gateway")

func ingressName(domainID int64) string { return "nectar-domain-" + strconv.FormatInt(domainID, 10) }

func domainIDFromOwner(owner string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(owner, "domain:"), 10, 64)
	if err != nil || !strings.HasPrefix(owner, "domain:") {
		return 0, fmt.Errorf("unexpected owner %q", owner)
	}
	return id, nil
}

// splitService turns "<svc>.<namespace>.svc.cluster.local" into its parts.
func splitService(target string) (svc, ns string, err error) {
	parts := strings.Split(target, ".")
	if len(parts) < 3 || parts[2] != "svc" {
		return "", "", fmt.Errorf("target %q is not an in-cluster service", target)
	}
	return parts[0], parts[1], nil
}

func (p *IngressProvider) check(r gateway.Route) error {
	switch {
	case r.Mode == "redirect":
		return fmt.Errorf("redirect domains are %w", ErrUnsupported)
	case r.BasicAuthHtpasswd != nil && *r.BasicAuthHtpasswd != "":
		return fmt.Errorf("basic auth is %w", ErrUnsupported)
	case r.BackendProtocol == "h2c":
		return fmt.Errorf("gRPC (h2c) backends are %w", ErrUnsupported)
	case r.TimeoutSeconds != 0 || r.MaxBodyMB != 0 || r.RateLimit != 0:
		return fmt.Errorf("timeouts, body and rate limits are %w", ErrUnsupported)
	case strings.HasPrefix(r.Domain, "*.") && r.ACME:
		return errors.New("wildcard certificates need a DNS-01 issuer; disable TLS for this domain")
	}
	return nil
}

func (p *IngressProvider) build(r gateway.Route) (kube.Object, error) {
	id, err := domainIDFromOwner(r.Owner)
	if err != nil {
		return nil, err
	}
	svc, ns, err := splitService(r.TargetIP)
	if err != nil {
		return nil, err
	}
	desired := r
	desired.ID, desired.CertNotAfter, desired.CertSource = id, 0, ""
	routeJSON, _ := json.Marshal(desired)
	path := r.PathPrefix
	if path == "" {
		path = "/"
	}
	ann := map[string]any{annotationRoute: string(routeJSON)}
	spec := map[string]any{
		"rules": []any{map[string]any{
			"host": r.Domain,
			"http": map[string]any{"paths": []any{map[string]any{
				"path": path, "pathType": "Prefix",
				"backend": map[string]any{"service": map[string]any{"name": svc, "port": map[string]any{"number": r.TargetPort}}},
			}}},
		}},
	}
	if p.ClassName != "" {
		spec["ingressClassName"] = p.ClassName
	}
	if r.ACME && p.ClusterIssuer != "" {
		ann[annotationIssuer] = p.ClusterIssuer
		spec["tls"] = []any{map[string]any{"hosts": []any{r.Domain}, "secretName": ingressName(id) + "-tls"}}
	}
	if r.ForceSSL && r.ACME {
		// widely supported hints; controllers ignore annotations they do not know
		ann["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
		ann["traefik.ingress.kubernetes.io/router.entrypoints"] = "web,websecure"
	}
	return kube.Object{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]any{
			"name": ingressName(id), "namespace": ns,
			"labels":      map[string]any{labelManagedBy: gateway.ManagedBy, labelDomainID: strconv.FormatInt(id, 10)},
			"annotations": ann,
		},
		"spec": spec,
	}, nil
}

// ListManaged returns the routes represented by the panel's Ingress objects.
func (p *IngressProvider) ListManaged(ctx context.Context) ([]gateway.Route, error) {
	items, err := p.Kube.List(ctx, "Ingress", "", labelManagedBy+"="+gateway.ManagedBy)
	if err != nil {
		return nil, err
	}
	out := []gateway.Route{}
	for _, it := range items {
		var r gateway.Route
		if err := json.Unmarshal([]byte(kube.Str(it, "metadata", "annotations", annotationRoute)), &r); err != nil {
			// hand-edited or foreign: report it with an owner so the syncer replaces it
			id, _ := strconv.ParseInt(kube.Str(it, "metadata", "labels", labelDomainID), 10, 64)
			r = gateway.Route{ID: id, Owner: Owner(id)}
		}
		_, ns, name := kube.Meta(it)
		if len(kube.Slice(it, "spec", "tls")) > 0 {
			r.CertNotAfter = p.certExpiry(ctx, ns, name+"-tls")
		}
		out = append(out, r)
	}
	return out, nil
}

// certExpiry reads the certificate cert-manager stored for the Ingress (0 = not issued yet).
func (p *IngressProvider) certExpiry(ctx context.Context, ns, secret string) int64 {
	var s kube.Object
	if err := p.Kube.GetObject(ctx, "Secret", ns, secret, &s); err != nil {
		return 0
	}
	der, err := base64.StdEncoding.DecodeString(kube.Str(s, "data", "tls.crt"))
	if err != nil {
		return 0
	}
	block, _ := pem.Decode(der)
	if block == nil {
		return 0
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0
	}
	return c.NotAfter.Unix()
}

// Create applies an Ingress for the route.
func (p *IngressProvider) Create(ctx context.Context, r gateway.Route) (gateway.Route, error) {
	if err := p.check(r); err != nil {
		return gateway.Route{}, err
	}
	o, err := p.build(r)
	if err != nil {
		return gateway.Route{}, err
	}
	if _, err := p.Kube.Apply(ctx, o); err != nil {
		return gateway.Route{}, err
	}
	r.ID, _ = domainIDFromOwner(r.Owner)
	return r, nil
}

// Update re-applies the Ingress (it may move namespaces when the target changed).
func (p *IngressProvider) Update(ctx context.Context, r gateway.Route) (gateway.Route, error) {
	if err := p.Delete(ctx, r.ID); err != nil {
		return gateway.Route{}, err
	}
	return p.Create(ctx, r)
}

// Delete removes the Ingress of a domain (missing is fine).
func (p *IngressProvider) Delete(ctx context.Context, id int64) error {
	items, err := p.Kube.List(ctx, "Ingress", "", labelDomainID+"="+strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	for _, it := range items {
		_, ns, name := kube.Meta(it)
		if err := p.Kube.Delete(ctx, "Ingress", ns, name); err != nil {
			return err
		}
	}
	return nil
}

// IssueCert is handled by cert-manager.
func (p *IngressProvider) IssueCert(ctx context.Context, id int64) (gateway.Route, error) {
	if p.ClusterIssuer == "" {
		return gateway.Route{}, errors.New("no cert-manager ClusterIssuer configured (NECTAR_INGRESS_ISSUER)")
	}
	return gateway.Route{}, errors.New("certificates are issued automatically by cert-manager (" + p.ClusterIssuer + ")")
}
