// Package adopt migrates an existing k3s server to Nectar Apps without breaking what runs on it
// ("adopt mode", PLAN.md §2b): read-only discovery and a migration report, translators from existing
// proxy configurations to nectar-gateway routes, backups, shadow verification, cutover with automatic
// rollback, and finalize.
//
// The iron rule is "never break what is running": anything that cannot be translated faithfully stays
// on the old proxy, which keeps running behind the gateway as the fallback upstream. Because the
// fallback works per host (unknown hosts go to the old proxy), a host is migrated only when every one
// of its rules can be migrated.
package adopt

import (
	"fmt"
	"sort"
	"strings"
)

// Status is the migration verdict for a route.
type Status string

const (
	// StatusOK means the route is migrated as is (✓).
	StatusOK Status = "ok"
	// StatusWarn means the route is migrated with a documented approximation (⚠).
	StatusWarn Status = "warn"
	// StatusFallback means the route is not migrated; its host keeps being served by the old proxy (✗).
	StatusFallback Status = "fallback"
)

// Symbol returns ✓ / ⚠ / ✗.
func (s Status) Symbol() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusWarn:
		return "⚠"
	}
	return "✗"
}

// Sources of routes.
const (
	SourceIngress       = "Ingress"
	SourceIngressRoute  = "IngressRoute"
	SourceHTTPRoute     = "HTTPRoute"
	SourceNginx         = "nginx"
	SourceCaddy         = "caddy"
	SourceHAProxy       = "haproxy"
	SourceApache        = "apache"
	SourceNectarGateway = "nectar_gateway"
)

// Target is where a route sends requests.
type Target struct {
	Host  string `json:"host"` // IP or DNS name (e.g. web.shop.svc.cluster.local, 127.0.0.1)
	Port  int    `json:"port"`
	HTTPS bool   `json:"https,omitempty"`
}

func (t Target) String() string {
	if t.Host == "" {
		return ""
	}
	scheme := "http"
	if t.HTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, t.Host, t.Port)
}

// CertRef says where the certificate of a host comes from.
type CertRef struct {
	// Secret is "namespace/name" of a kubernetes.io/tls Secret.
	Secret string `json:"secret,omitempty"`
	// CertFile / KeyFile are host paths (nginx, Apache, certbot). For HAProxy both point to one PEM.
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
	// PEM is set when the certificate was read from a database (standalone nectar_gateway).
	CertPEM string `json:"-"`
	KeyPEM  string `json:"-"`
}

// Route is one migration candidate: a host (+ path prefix) and how the old proxy served it.
type Route struct {
	Source string `json:"source"`
	Ref    string `json:"ref"` // "namespace/name", "file:line", "route #id"

	Host       string `json:"host"`
	PathPrefix string `json:"path_prefix"` // "/" = everything

	Target       Target `json:"target"`
	RedirectURL  string `json:"redirect_url,omitempty"` // mode=redirect (path and query are kept)
	RedirectCode int    `json:"redirect_code,omitempty"`

	TLS             bool     `json:"tls"`                   // the old proxy served HTTPS for the host
	Cert            *CertRef `json:"cert,omitempty"`        // nil + TLS = the gateway issues its own (ACME)
	ForceHTTPS      bool     `json:"force_https,omitempty"` // HTTP → HTTPS redirect
	BackendProtocol string   `json:"backend_protocol,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty"`
	MaxBodyMB       int      `json:"max_body_mb,omitempty"`
	Htpasswd        string   `json:"-"` // basic auth (bcrypt htpasswd); never written to reports
	BasicAuth       string   `json:"basic_auth,omitempty"`
	// Extra gateway fields carried over from a standalone nectar_gateway.
	Extra map[string]any `json:"-"`

	Status Status   `json:"status"`
	Notes  []string `json:"notes,omitempty"`
}

// warn records an approximation (✓ → ⚠).
func (r *Route) warn(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
	if r.Status == StatusOK || r.Status == "" {
		r.Status = StatusWarn
	}
}

// fallback marks the route as not migratable (stays on the old proxy).
func (r *Route) fallback(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
	r.Status = StatusFallback
}

// Migratable reports whether the route goes to the gateway.
func (r Route) Migratable() bool { return r.Status == StatusOK || r.Status == StatusWarn }

// Key identifies a route in the gateway (host + path prefix).
func (r Route) Key() string { return r.Host + r.PathPrefix }

func normPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func normHost(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
}

// Finalize applies the per-host rule and removes duplicates; it returns the routes sorted by host.
//   - duplicates (same host + path) keep the first route; later ones are reported as fallback
//   - when any rule of a host stays on the fallback, the whole host does (the fallback works per host)
func Finalize(routes []Route) []Route {
	seen := map[string]int{}
	out := make([]Route, 0, len(routes))
	for _, r := range routes {
		r.Host, r.PathPrefix = normHost(r.Host), normPath(r.PathPrefix)
		if r.Status == "" {
			r.Status = StatusOK
		}
		if r.Host == "" {
			r.fallback("rule without a host (catch-all) stays on the old proxy")
		}
		if i, dup := seen[r.Key()]; dup {
			if out[i].Migratable() && r.Migratable() && out[i].Target == r.Target && out[i].RedirectURL == r.RedirectURL {
				continue // identical rule (e.g. the same Ingress applied twice under two classes)
			}
			r.fallback("duplicate of %s %s for the same host and path", out[i].Source, out[i].Ref)
		} else if r.Host != "" {
			seen[r.Key()] = len(out)
		}
		out = append(out, r)
	}
	blocked := map[string]string{}
	for _, r := range out {
		if r.Status == StatusFallback && r.Host != "" {
			if _, ok := blocked[r.Host]; !ok {
				blocked[r.Host] = r.Source + " " + r.Ref
			}
		}
	}
	for i := range out {
		if by, ok := blocked[out[i].Host]; ok && out[i].Migratable() {
			out[i].fallback("another rule of this host (%s) cannot be migrated, so the whole host stays on the old proxy", by)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].PathPrefix > out[j].PathPrefix // longest prefix first
	})
	return out
}

// Hosts returns the distinct hosts of the routes that will be migrated.
func Hosts(routes []Route) []string {
	set := map[string]bool{}
	for _, r := range routes {
		if r.Migratable() {
			set[r.Host] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Count returns the number of routes per status.
func Count(routes []Route) map[Status]int {
	m := map[Status]int{}
	for _, r := range routes {
		m[r.Status]++
	}
	return m
}
