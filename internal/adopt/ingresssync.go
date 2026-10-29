package adopt

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
)

// IngressSyncManagedBy marks gateway routes created from Kubernetes Ingress / IngressRoute / HTTPRoute objects.
const IngressSyncManagedBy = "nectar-ingress"

// SyncGateway is the gateway API the Ingress sync needs (gateway.Client).
type SyncGateway interface {
	ListManagedBy(ctx context.Context, managedBy string) ([]gateway.Route, error)
	Create(ctx context.Context, r gateway.Route) (gateway.Route, error)
	Update(ctx context.Context, r gateway.Route) (gateway.Route, error)
	Delete(ctx context.Context, id int64) error
	UploadCert(ctx context.Context, id int64, certPEM, keyPEM string) (gateway.Route, error)
}

// IngressSync keeps gateway routes in line with the Kubernetes routing objects of the classes it serves,
// so Helm charts, GitOps and `kubectl apply` keep working after a migration. TLS Secrets (e.g. issued by
// cert-manager) are pushed to the gateway when they change. Annotations that cannot be translated are
// reported; those hosts keep using the fallback (the old controller).
type IngressSync struct {
	Kube    Kube
	Gateway SyncGateway
	Options K8sOptions
	Log     *slog.Logger
	// Reserved returns host+path keys served by the panel's own domains (they win over Ingress objects).
	Reserved func(ctx context.Context) map[string]bool
	// Address is written to status.loadBalancer of every Ingress the gateway serves (the edge node's IP),
	// like other controllers do: `kubectl get ingress` shows it and GitOps tools (Argo CD) see the
	// Ingress as healthy. Empty or nil leaves the status alone.
	Address func(ctx context.Context) string

	mu      sync.Mutex
	last    []Route
	lastAt  time.Time
	lastErr string
	trigger chan struct{}
}

// SyncStatus is what the last sync did (for the panel API).
type SyncStatus struct {
	At      time.Time      `json:"at"`
	Error   string         `json:"error,omitempty"`
	Classes []string       `json:"classes"`
	Routes  []Route        `json:"routes"`
	Counts  map[Status]int `json:"counts"`
}

// Status returns the result of the last sync.
func (s *IngressSync) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SyncStatus{At: s.lastAt, Error: s.lastErr, Classes: s.Options.Classes, Routes: s.last, Counts: Count(s.last)}
}

// Trigger requests a sync soon (non-blocking).
func (s *IngressSync) Trigger() {
	s.mu.Lock()
	if s.trigger == nil {
		s.trigger = make(chan struct{}, 1)
	}
	t := s.trigger
	s.mu.Unlock()
	select {
	case t <- struct{}{}:
	default:
	}
}

// Run syncs every interval (and on Trigger) until ctx ends.
func (s *IngressSync) Run(ctx context.Context, interval time.Duration) {
	s.mu.Lock()
	if s.trigger == nil {
		s.trigger = make(chan struct{}, 1)
	}
	t := s.trigger
	s.mu.Unlock()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			s.Log.Warn("ingress sync", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-t:
		}
	}
}

func ingressOwner(r Route) string { return r.Source + "/" + r.Ref + "|" + r.Key() }

// Sync reconciles once.
func (s *IngressSync) Sync(ctx context.Context) error {
	err := s.sync(ctx)
	s.mu.Lock()
	s.lastAt = time.Now().UTC()
	s.lastErr = ""
	if err != nil {
		s.lastErr = err.Error()
	}
	s.mu.Unlock()
	return err
}

func (s *IngressSync) sync(ctx context.Context) error {
	cl, err := LoadCluster(ctx, s.Kube)
	if err != nil {
		return err
	}
	routes := Finalize(Kubernetes(cl, s.Options))
	var reserved map[string]bool
	if s.Reserved != nil {
		reserved = s.Reserved(ctx)
	}
	have, err := s.Gateway.ListManagedBy(ctx, IngressSyncManagedBy)
	if err != nil {
		return err
	}
	byOwner := map[string]gateway.Route{}
	for _, r := range have {
		byOwner[r.Owner] = r
	}
	keep := map[string]bool{}
	var errs []string
	for i := range routes {
		r := &routes[i]
		if !r.Migratable() {
			continue
		}
		if reserved[r.Key()] {
			r.fallback("the panel serves this host and path (a domain of a Nectar app)")
			continue
		}
		owner := ingressOwner(*r)
		want := r.GatewayRoute(IngressSyncManagedBy, owner)
		final := want
		keep[owner] = true
		cur, exists := byOwner[owner]
		var got gateway.Route
		switch {
		case !exists:
			// the gateway refuses an HTTPS redirect before the certificate is there: upload it first
			if want.ForceSSL && r.Cert != nil {
				want.ForceSSL = false
			}
			got, err = s.Gateway.Create(ctx, want)
			if gateway.IsConflict(err) {
				r.fallback("another gateway route already serves %s%s", r.Host, r.PathPrefix)
				continue
			}
		case routeDiffers(want, cur):
			want.ID = cur.ID
			got, err = s.Gateway.Update(ctx, want)
		default:
			got = cur
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s%s: %v", r.Host, r.PathPrefix, err))
			continue
		}
		certOK := true
		if r.Cert != nil && r.Cert.Secret != "" {
			if err := s.syncCert(ctx, cl, got, r.Cert.Secret); err != nil {
				certOK = false
				r.warn("certificate from Secret %s: %v (ACME is used meanwhile)", r.Cert.Secret, err)
			}
		}
		if final.ForceSSL && !got.ForceSSL {
			final.ID = got.ID
			if !certOK {
				final.ACME = !strings.HasPrefix(final.Domain, "*.")
			}
			if _, err := s.Gateway.Update(ctx, final); err != nil {
				errs = append(errs, fmt.Sprintf("%s%s: enable the HTTPS redirect: %v", r.Host, r.PathPrefix, err))
			}
		}
	}
	for owner, r := range byOwner {
		if !keep[owner] {
			if err := s.Gateway.Delete(ctx, r.ID); err != nil {
				errs = append(errs, fmt.Sprintf("delete %s: %v", r.Domain, err))
			}
		}
	}
	s.mu.Lock()
	s.last = routes
	s.mu.Unlock()
	if err := s.publishStatus(ctx, cl, routes); err != nil {
		errs = append(errs, "ingress status: "+err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// statusWriter is the raw request the status subresource needs (kube.Client.Do).
type statusWriter interface {
	Do(ctx context.Context, method, path, contentType string, body io.Reader, out any) error
}

// publishStatus sets status.loadBalancer.ingress to the edge address on each Ingress whose routes the
// gateway serves (all of them: a host still on the fallback proxy keeps the old controller's status).
func (s *IngressSync) publishStatus(ctx context.Context, cl Cluster, routes []Route) error {
	w, ok := s.Kube.(statusWriter)
	if s.Address == nil || !ok {
		return nil
	}
	addr := s.Address(ctx)
	if addr == "" {
		return nil
	}
	served := map[string]bool{}
	for _, r := range routes {
		if r.Source != SourceIngress {
			continue
		}
		if _, seen := served[r.Ref]; !seen {
			served[r.Ref] = true
		}
		served[r.Ref] = served[r.Ref] && r.Migratable()
	}
	field := "ip"
	if net.ParseIP(addr) == nil {
		field = "hostname"
	}
	var errs []string
	for _, ing := range cl.Ingresses {
		_, ns, name := kube.Meta(ing)
		if !served[ns+"/"+name] {
			continue
		}
		cur := kube.Slice(ing, "status", "loadBalancer", "ingress")
		if len(cur) == 1 && kube.Str(cur[0], field) == addr {
			continue
		}
		body, _ := json.Marshal(map[string]any{"status": map[string]any{"loadBalancer": map[string]any{"ingress": []any{map[string]any{field: addr}}}}})
		path := "/apis/networking.k8s.io/v1/namespaces/" + url.PathEscape(ns) + "/ingresses/" + url.PathEscape(name) + "/status"
		if err := w.Do(ctx, http.MethodPatch, path, "application/merge-patch+json", bytes.NewReader(body), nil); err != nil {
			errs = append(errs, ns+"/"+name+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// syncCert uploads the Secret's certificate when the gateway has none or a different one (by expiry).
func (s *IngressSync) syncCert(ctx context.Context, cl Cluster, r gateway.Route, ref string) error {
	ns, name, _ := strings.Cut(ref, "/")
	sec := cl.find(cl.Secrets, ns, name)
	if sec == nil {
		return fmt.Errorf("not found")
	}
	crt, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "tls.crt"))
	key, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "tls.key"))
	if len(crt) == 0 || len(key) == 0 {
		return fmt.Errorf("no tls.crt / tls.key")
	}
	blk, _ := pem.Decode(crt)
	if blk == nil {
		return fmt.Errorf("tls.crt is not PEM")
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return err
	}
	if r.CertNotAfter == c.NotAfter.Unix() && r.CertSource != "" && r.CertSource != "acme" {
		return nil
	}
	_, err = s.Gateway.UploadCert(ctx, r.ID, string(crt), string(key))
	return err
}

func routeDiffers(want, have gateway.Route) bool {
	pp := func(p string) string {
		if p == "" {
			return "/"
		}
		return p
	}
	bp := func(p string) string {
		if p == "" {
			return "http1"
		}
		return p
	}
	ht := ""
	if want.BasicAuthHtpasswd != nil {
		ht = *want.BasicAuthHtpasswd
	}
	return want.Domain != have.Domain || pp(want.PathPrefix) != pp(have.PathPrefix) || want.Mode != have.Mode ||
		want.TargetIP != have.TargetIP || want.TargetPort != have.TargetPort || want.TargetHTTPS != have.TargetHTTPS ||
		want.ForceSSL != have.ForceSSL || want.ACME != have.ACME || bp(want.BackendProtocol) != bp(have.BackendProtocol) ||
		want.TimeoutSeconds != have.TimeoutSeconds || want.MaxBodyMB != have.MaxBodyMB || want.RedirectURL != have.RedirectURL ||
		want.RedirectCode != have.RedirectCode || gateway.AuthFingerprint(ht) != have.AuthFingerprint
}
