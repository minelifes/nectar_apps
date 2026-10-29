package adopt

import (
	"encoding/json"
	"strings"

	"nectar_apps/internal/gateway"
)

// GatewayRoute converts a migratable route into a nectar-gateway route.
func (r Route) GatewayRoute(managedBy, owner string) gateway.Route {
	wildcard := strings.HasPrefix(r.Host, "*.")
	g := gateway.Route{
		Domain:          r.Host,
		PathPrefix:      normPath(r.PathPrefix),
		Mode:            "proxy",
		TargetIP:        r.Target.Host,
		TargetPort:      r.Target.Port,
		TargetHTTPS:     r.Target.HTTPS,
		ForceSSL:        r.ForceHTTPS && r.TLS,
		ACME:            r.TLS && r.Cert == nil && !wildcard,
		ManagedBy:       managedBy,
		Owner:           owner,
		BackendProtocol: r.BackendProtocol,
		TimeoutSeconds:  r.TimeoutSeconds,
		MaxBodyMB:       r.MaxBodyMB,
	}
	if g.BackendProtocol == "" {
		g.BackendProtocol = "http1"
	}
	if r.RedirectURL != "" {
		g.Mode, g.RedirectURL, g.RedirectCode = "redirect", r.RedirectURL, r.RedirectCode
		if g.RedirectCode == 0 {
			g.RedirectCode = 301
		}
		g.TargetIP, g.TargetPort = "", 0
	}
	ht := r.Htpasswd
	g.BasicAuthHtpasswd = &ht
	return g
}

// GatewayPayload is GatewayRoute as a JSON object with the extra fields of a standalone gateway
// (rate limit, health checks) merged in.
func (r Route) GatewayPayload(managedBy, owner string) map[string]any {
	b, _ := json.Marshal(r.GatewayRoute(managedBy, owner))
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for k, v := range r.Extra {
		m[k] = v
	}
	return m
}
