package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/store"
)

// Automatic DNS records: with a DNS provider in nectar-gateway (Gateway → Certificates → DNS) and
// "dns_auto" in the settings, the panel asks the gateway to create the A/AAAA records of new domains.
// Hosts under the base domain are served by one wildcard record *.<base domain> instead of a record each.
// Only records the gateway created are ever changed or deleted.

// DNS states of a domain.
const (
	dnsManaged  = "managed"  // the gateway created the records
	dnsCovered  = "covered"  // served by the *.<base domain> record
	dnsConflict = "conflict" // the name has records someone else created
	dnsError    = "error"
)

// dnsAvailable reports whether the gateway can manage DNS records.
func (s *Server) dnsAvailable(ctx context.Context) bool {
	if s.gateway == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, err := s.gateway.Version(ctx)
	return err == nil && v.Has(gateway.FeatureDNSRecords)
}

// underBase reports whether host is covered by the *.<base> wildcard record (one label below base).
func underBase(host, base string) bool {
	if base == "" || !strings.HasSuffix(host, "."+base) {
		return false
	}
	label := strings.TrimSuffix(host, "."+base)
	return label != "" && !strings.Contains(label, ".") && !strings.HasPrefix(label, "*")
}

// ensureDomainDNS creates or updates the records of one domain and stores the outcome.
func (s *Server) ensureDomainDNS(ctx context.Context, d store.Domain, st Settings) (state, msg string) {
	switch {
	case underBase(d.Host, st.BaseDomain) && st.DNSAuto:
		state, msg = dnsCovered, "served by the DNS record *."+st.BaseDomain
	default:
		ips := s.serverIPs(ctx, st)
		if len(ips) == 0 {
			state, msg = dnsError, "the server's public IP is unknown: set it in the setup wizard (Settings → Domain, HTTPS email and sample app)"
			break
		}
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		res, err := s.gateway.EnsureDNS(ctx, d.Host, ips)
		switch {
		case err != nil:
			state, msg = dnsError, err.Error()
		case res.Conflict != "":
			state, msg = dnsConflict, res.Conflict
		default:
			state, msg = dnsManaged, "points to "+strings.Join(ips, ", ")
		}
	}
	if err := s.store.SetDomainDNS(ctx, d.ID, state, msg); err != nil {
		s.log.Warn("dns state", "host", d.Host, "err", err)
	}
	return state, msg
}

// autoDNS runs after a domain was created (in the background: the provider can take seconds).
func (s *Server) autoDNS(d store.Domain) {
	ctx := context.Background()
	st := s.loadSettings(ctx)
	if !st.DNSAuto || !s.dnsAvailable(ctx) {
		return
	}
	go func() {
		if state, msg := s.ensureDomainDNS(ctx, d, st); state == dnsError || state == dnsConflict {
			s.log.Warn("automatic DNS record", "host", d.Host, "state", state, "msg", msg)
		}
	}()
}

// releaseDNS removes the records of a deleted domain when no other domain uses the host.
func (s *Server) releaseDNS(ctx context.Context, d store.Domain) {
	if d.DNSState != dnsManaged || s.gateway == nil {
		return
	}
	all, err := s.store.ListDomains(ctx, 0)
	if err != nil {
		return
	}
	for _, o := range all {
		if o.ID != d.ID && o.Host == d.Host {
			return // another path on the same host still needs it
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := s.gateway.DeleteDNS(ctx, d.Host); err != nil {
			s.log.Warn("delete DNS record", "host", d.Host, "err", err)
		}
	}()
}

// ensureBaseDNS points *.<base domain> at the server (dns_auto). Returns a message for the UI.
func (s *Server) ensureBaseDNS(ctx context.Context, st Settings) (gateway.DNSResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.gateway.EnsureDNS(ctx, "*."+st.BaseDomain, s.serverIPs(ctx, st))
}

// refreshDNS re-points every managed record (after the server's IPs changed), in the background.
func (s *Server) refreshDNS(st Settings) {
	go func() {
		ctx := context.Background()
		if !s.dnsAvailable(ctx) {
			return
		}
		if st.DNSAuto && st.BaseDomain != "" {
			if res, err := s.ensureBaseDNS(ctx, st); err != nil || res.Conflict != "" {
				s.log.Warn("DNS record for the base domain", "err", err, "conflict", res.Conflict)
			}
		}
		all, err := s.store.ListDomains(ctx, 0)
		if err != nil {
			return
		}
		for _, d := range all {
			if d.DNSState == dnsManaged || d.DNSState == dnsError {
				s.ensureDomainDNS(ctx, d, st)
			}
		}
	}()
}

// createDomainDNS: POST /api/apps/{id}/domains/{did}/dns — create the records now (domain.manage).
func (s *Server) createDomainDNS(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	d, ok := s.appDomain(w, r, app)
	if !ok {
		return
	}
	if !s.dnsAvailable(r.Context()) {
		writeError(w, http.StatusConflict, "nectar-gateway has no DNS provider: set one under Gateway → Certificates → DNS provider (needs gateway 1.3 or newer)")
		return
	}
	st := s.loadSettings(r.Context())
	state, msg := s.ensureDomainDNS(r.Context(), d, st)
	s.audit(r, p, "domain.dns", proj.Slug+"/"+app.Slug, map[string]string{"host": d.Host, "state": state})
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	code := http.StatusOK
	if state == dnsError {
		code = http.StatusBadGateway
	}
	writeJSON(w, code, map[string]any{"domain": d, "state": state, "message": msg})
}

// domainRow is one entry of the domains overview.
type domainRow struct {
	store.Domain
	AppSlug     string `json:"app_slug"`
	AppName     string `json:"app_name"`
	ProjectID   int64  `json:"project_id"`
	ProjectSlug string `json:"project_slug"`
	ProjectName string `json:"project_name"`
	CanManage   bool   `json:"can_manage"`
}

// listAllDomains: GET /api/domains — every domain the caller may see, with its app and project, plus what
// DNS records should look like.
func (s *Server) listAllDomains(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	byProject := map[int64]store.Project{}
	for _, pr := range projects {
		byProject[pr.ID] = pr
	}
	apps, err := s.store.ListApps(ctx, 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	byApp := map[int64]store.App{}
	for _, a := range apps {
		byApp[a.ID] = a
	}
	domains, err := s.store.ListDomains(ctx, 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := []domainRow{}
	for _, d := range domains {
		a, ok := byApp[d.AppID]
		if !ok || !p.eff.Can("domain.view", &a.ProjectID) {
			continue
		}
		pr := byProject[a.ProjectID]
		rows = append(rows, domainRow{Domain: d, AppSlug: a.Slug, AppName: a.Name, ProjectID: pr.ID, ProjectSlug: pr.Slug,
			ProjectName: pr.Name, CanManage: p.eff.Can("domain.manage", &a.ProjectID)})
	}
	st := s.loadSettings(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"domains":       rows,
		"base_domain":   st.BaseDomain,
		"server_ips":    s.serverIPs(ctx, st),
		"dns_auto":      st.DNSAuto,
		"dns_available": s.dnsAvailable(ctx),
		"base_host":     baseTarget(st.BaseDomain),
	})
}

// baseTarget is a host under the base domain that custom domains can CNAME to ("" without one).
func baseTarget(base string) string {
	if base == "" {
		return ""
	}
	return "nectar." + base
}
