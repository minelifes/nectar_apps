package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/gateway"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// ---------------------------------------------------------------- settings --

// Settings are panel-wide options editable in the UI (stored in the settings table; env vars are defaults).
type Settings struct {
	BaseDomain string   `json:"base_domain"` // apps get <app>-<project>.<base_domain>
	PublicIPs  []string `json:"public_ips"`  // where DNS records must point (empty = node addresses)
	// DNSAuto: create DNS records of new domains through nectar-gateway's DNS provider (one *.<base>
	// record for hosts under the base domain).
	DNSAuto bool `json:"dns_auto"`
}

const settingsKey = "general"

func (s *Server) loadSettings(ctx context.Context) Settings {
	st := Settings{BaseDomain: s.cfg.BaseDomain, PublicIPs: []string{}}
	_ = s.store.GetSetting(ctx, settingsKey, &st)
	if st.PublicIPs == nil {
		st.PublicIPs = []string{}
	}
	return st
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	st := s.loadSettings(r.Context())
	gw := map[string]any{"configured": s.syncer.Enabled(), "routing": s.cfg.Routing}
	if s.gateway != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if v, err := s.gateway.Version(ctx); err != nil {
			gw["reachable"], gw["error"] = false, err.Error()
		} else {
			gw["reachable"], gw["version"], gw["api_version"] = true, v.Version, v.APIVersion
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"base_domain": st.BaseDomain, "public_ips": st.PublicIPs, "gateway": gw,
		"server_ips": s.serverIPs(r.Context(), st), "dns_auto": st.DNSAuto, "dns_available": s.dnsAvailable(r.Context())})
}

var hostRe = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	in := s.loadSettings(r.Context()) // fields left out keep their value
	if !decode(w, r, &in) {
		return
	}
	in.BaseDomain = store.NormalizeHost(in.BaseDomain)
	if in.BaseDomain != "" && (strings.HasPrefix(in.BaseDomain, "*.") || !hostRe.MatchString(in.BaseDomain)) {
		writeError(w, http.StatusBadRequest, "base_domain must be a domain like apps.example.com")
		return
	}
	for _, ip := range in.PublicIPs {
		if net.ParseIP(ip) == nil {
			writeError(w, http.StatusBadRequest, "invalid IP address "+ip)
			return
		}
	}
	if in.PublicIPs == nil {
		in.PublicIPs = []string{}
	}
	old := s.loadSettings(r.Context())
	if in.DNSAuto && !old.DNSAuto && !s.dnsAvailable(r.Context()) {
		writeError(w, http.StatusConflict, "automatic DNS records need a DNS provider in nectar-gateway (Gateway → Certificates → DNS provider, gateway 1.3 or newer)")
		return
	}
	var dns map[string]any
	if in.DNSAuto && in.BaseDomain != "" && (!old.DNSAuto || old.BaseDomain != in.BaseDomain) {
		// one wildcard record serves every <app>-<project>.<base>; a conflict is reported, not fatal
		res, err := s.ensureBaseDNS(r.Context(), in)
		switch {
		case err != nil:
			writeError(w, http.StatusBadGateway, "DNS record *."+in.BaseDomain+": "+err.Error())
			return
		case res.Conflict != "":
			dns = map[string]any{"name": "*." + in.BaseDomain, "conflict": res.Conflict}
		default:
			dns = map[string]any{"name": "*." + in.BaseDomain, "records": res.Records}
		}
	}
	if err := s.store.SetSetting(r.Context(), settingsKey, in); err != nil {
		s.serverError(w, r, err)
		return
	}
	if in.DNSAuto && !slices.Equal(old.PublicIPs, in.PublicIPs) {
		s.refreshDNS(in) // the server moved: re-point the records Nectar manages
	}
	s.audit(r, p, "settings.update", "", in)
	writeJSON(w, http.StatusOK, map[string]any{"base_domain": in.BaseDomain, "public_ips": in.PublicIPs, "dns_auto": in.DNSAuto, "dns": dns})
}

// serverIPs returns the addresses DNS records should point to.
func (s *Server) serverIPs(ctx context.Context, st Settings) []string {
	if len(st.PublicIPs) > 0 {
		return st.PublicIPs
	}
	out := []string{}
	if s.kube == nil {
		return out
	}
	nodes, err := s.kube.ListNodes(ctx)
	if err != nil {
		return out
	}
	for _, n := range nodes {
		if n.ExternalIP != "" {
			out = append(out, n.ExternalIP)
		} else if n.InternalIP != "" {
			out = append(out, n.InternalIP)
		}
	}
	return out
}

// DefaultHost returns the automatic domain of an app, e.g. api-shop.apps.example.com ("" without base domain).
func DefaultHost(app, project, base string) string {
	if base == "" {
		return ""
	}
	label := app + "-" + project
	if len(label) > 63 {
		label = strings.TrimRight(label[:63], "-")
	}
	return label + "." + base
}

// addDefaultDomain creates <app>-<project>.<base> for new web apps (best effort).
func (s *Server) addDefaultDomain(ctx context.Context, app store.App, proj store.Project) {
	if app.Kind != runtimes.KindWeb {
		return
	}
	host := DefaultHost(app.Slug, proj.Slug, s.loadSettings(ctx).BaseDomain)
	if host == "" {
		return
	}
	d, err := s.store.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: host, TLS: true, ForceHTTPS: true, IsDefault: true})
	if err != nil {
		s.log.Warn("default domain", "host", host, "err", err)
		return
	}
	s.autoDNS(d) // covered by *.<base> with dns_auto
}

// ----------------------------------------------------------------- domains --

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	ds, err := s.store.ListDomains(r.Context(), app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

type domainInput struct {
	Host       string `json:"host"`
	PathPrefix string `json:"path_prefix"`
	TargetPort int    `json:"target_port"`
	TLS        *bool  `json:"tls"`
	ForceHTTPS *bool  `json:"force_https"`

	// gateway options (nil = default on create / unchanged on update)
	Kind            *string               `json:"kind"`        // app | redirect
	RedirectTo      *string               `json:"redirect_to"` // host or URL for kind=redirect
	RedirectCode    *int                  `json:"redirect_code"`
	BackendProtocol *string               `json:"backend_protocol"` // http1 | h2c (gRPC)
	TimeoutSeconds  *int                  `json:"timeout_seconds"`
	MaxBodyMB       *int                  `json:"max_body_mb"`
	Web             *gateway.Web          `json:"web"`              // {} clears the options
	RateLimit       *int                  `json:"rate_limit"`       // requests/s per client IP, 0 = unlimited; over it the gateway blocks the IP for 24 h
	BasicAuthUsers  *[]auth.BasicAuthUser `json:"basic_auth_users"` // [] removes basic auth
	// WithWWW (create only) also adds www.<host> redirecting to https://<host>.
	WithWWW bool `json:"with_www"`
}

// applyOptions merges the input onto o and validates. It returns an error message or "".
// applyOptions validates and applies domain options. minRate is the lowest accepted rate limit (see
// minRateLimit).
func applyOptions(in domainInput, o *store.DomainOptions, minRate int) string {
	if in.Kind != nil {
		o.Kind = *in.Kind
	}
	if o.Kind == "" {
		o.Kind = store.DomainApp
	}
	if o.Kind != store.DomainApp && o.Kind != store.DomainRedirect {
		return "kind must be app or redirect"
	}
	if in.RedirectTo != nil {
		o.RedirectTo = strings.TrimSpace(*in.RedirectTo)
		if o.RedirectTo != "" && !strings.Contains(o.RedirectTo, "://") {
			o.RedirectTo = "https://" + o.RedirectTo
		}
	}
	if in.RedirectCode != nil {
		o.RedirectCode = *in.RedirectCode
	}
	if o.RedirectCode == 0 {
		o.RedirectCode = 301
	}
	if o.Kind == store.DomainRedirect {
		u, err := url.Parse(o.RedirectTo)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "redirect_to must be a domain or an http(s) URL, e.g. example.com or https://example.com/new"
		}
		if !slices.Contains([]int{301, 302, 307, 308}, o.RedirectCode) {
			return "redirect_code must be 301, 302, 307 or 308"
		}
	} else {
		o.RedirectTo = ""
	}
	if in.BackendProtocol != nil {
		o.BackendProtocol = *in.BackendProtocol
	}
	if o.BackendProtocol == "" {
		o.BackendProtocol = "http1"
	}
	if o.BackendProtocol != "http1" && o.BackendProtocol != "h2c" {
		return "backend_protocol must be http1 or h2c (gRPC)"
	}
	if in.TimeoutSeconds != nil {
		o.TimeoutSeconds = *in.TimeoutSeconds
	}
	if o.TimeoutSeconds < 0 || o.TimeoutSeconds > 3600 {
		return "timeout_seconds must be 0–3600 (0 = 60 s default)"
	}
	if in.MaxBodyMB != nil {
		o.MaxBodyMB = *in.MaxBodyMB
	}
	if o.MaxBodyMB < 0 || o.MaxBodyMB > 102400 {
		return "max_body_mb must be 0–102400 (0 = unlimited)"
	}
	if in.RateLimit != nil {
		o.RateLimit = *in.RateLimit
	}
	if o.RateLimit < 0 || o.RateLimit > 100000 {
		return "rate_limit must be 0–100000 requests per second (0 = unlimited)"
	}
	// gateways before rate-limit-429 block a client over the limit for 24 h on every domain, and one page
	// load can make dozens of requests: low limits would lock real visitors out
	if in.RateLimit != nil && o.RateLimit > 0 && o.RateLimit < minRate { // only when it is being set: older domains keep theirs
		return fmt.Sprintf("rate_limit must be 0 (unlimited) or at least %d requests per second: this nectar-gateway blocks clients over it for 24 hours (update it to answer 429 instead)", minRate)
	}
	if in.Web != nil {
		if err := in.Web.Normalize(); err != nil {
			return err.Error()
		}
		o.Web = in.Web
		if o.Web.IsZero() {
			o.Web = nil
		}
	}
	if in.BasicAuthUsers != nil {
		h, err := auth.Htpasswd(*in.BasicAuthUsers, o.BasicAuth)
		if err != nil {
			return "basic auth: " + err.Error()
		}
		o.BasicAuth = h
	}
	return ""
}

// resync applies domain changes to the gateway right away (bounded wait; the loop retries anyway).
func (s *Server) resync(r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.syncer.Sync(ctx); err != nil {
		s.log.Warn("route sync", "err", err)
	}
}

func (s *Server) createDomain(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if app.Kind != runtimes.KindWeb {
		writeError(w, http.StatusBadRequest, "only web apps can have domains")
		return
	}
	var in domainInput
	if !decode(w, r, &in) {
		return
	}
	host := store.NormalizeHost(in.Host)
	if !hostRe.MatchString(host) {
		writeError(w, http.StatusBadRequest, "host must be a domain like shop.example.com (or *.example.com)")
		return
	}
	path := store.NormalizePath(in.PathPrefix)
	if strings.ContainsAny(path, " ?#*") || len(path) > 200 {
		writeError(w, http.StatusBadRequest, "invalid path prefix")
		return
	}
	if in.TargetPort < 0 || in.TargetPort > 65535 {
		writeError(w, http.StatusBadRequest, "target_port must be 0–65535 (0 = the app's port)")
		return
	}
	tls, force := true, true
	if in.TLS != nil {
		tls = *in.TLS
	}
	if in.ForceHTTPS != nil {
		force = *in.ForceHTTPS
	}
	if strings.HasPrefix(host, "*.") {
		tls, force = false, false // wildcard certificates need DNS-01 (later); upload one in the gateway
	}
	if tls && !p.eff.Can("domain.tls", &app.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: domain.tls (or create the domain with tls=false)")
		return
	}
	var opts store.DomainOptions
	if msg := applyOptions(in, &opts, s.minRateLimit(r.Context())); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	wwwHost := ""
	if in.WithWWW {
		if strings.HasPrefix(host, "*.") || strings.HasPrefix(host, "www.") || opts.Kind != store.DomainApp {
			writeError(w, http.StatusBadRequest, "with_www needs a plain app domain like example.com")
			return
		}
		wwwHost = "www." + host
	}
	d, err := s.store.CreateDomain(r.Context(), store.Domain{AppID: app.ID, Host: host, PathPrefix: path, TargetPort: in.TargetPort, TLS: tls, ForceHTTPS: force, DomainOptions: opts})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this host and path are already used by another app")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if wwwHost != "" {
		redirect := store.DomainOptions{Kind: store.DomainRedirect, RedirectTo: "https://" + host, RedirectCode: 301}
		if !tls {
			redirect.RedirectTo = "http://" + host
		}
		if wd, err := s.store.CreateDomain(r.Context(), store.Domain{AppID: app.ID, Host: wwwHost, PathPrefix: "/", TLS: tls, ForceHTTPS: force, DomainOptions: redirect}); err != nil {
			s.log.Warn("www redirect domain", "host", wwwHost, "err", err)
		} else {
			s.autoDNS(wd)
		}
	}
	s.autoDNS(d)
	s.resync(r)
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	s.audit(r, p, "domain.create", proj.Slug+"/"+app.Slug, map[string]any{"host": host, "path": path, "kind": opts.Kind, "www": wwwHost != ""})
	writeJSON(w, http.StatusCreated, d)
}

// appDomain loads {did} and checks it belongs to the app.
func (s *Server) appDomain(w http.ResponseWriter, r *http.Request, app store.App) (store.Domain, bool) {
	id, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad domain id")
		return store.Domain{}, false
	}
	d, err := s.store.GetDomain(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.AppID != app.ID) {
		writeError(w, http.StatusNotFound, "domain not found")
		return d, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return d, false
	}
	return d, true
}

func (s *Server) updateDomain(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	d, ok := s.appDomain(w, r, app)
	if !ok {
		return
	}
	var in domainInput
	if !decode(w, r, &in) {
		return
	}
	if in.Host != "" || in.PathPrefix != "" {
		writeError(w, http.StatusBadRequest, "host and path cannot be changed — add a new domain instead")
		return
	}
	port, tls, force := d.TargetPort, d.TLS, d.ForceHTTPS
	if in.TargetPort != 0 {
		port = in.TargetPort
	}
	if in.TLS != nil && *in.TLS != tls {
		if !p.eff.Can("domain.tls", &app.ProjectID) {
			writeError(w, http.StatusForbidden, "missing permission: domain.tls")
			return
		}
		tls = *in.TLS
	}
	if in.ForceHTTPS != nil {
		force = *in.ForceHTTPS
	}
	if strings.HasPrefix(d.Host, "*.") {
		tls, force = false, false
	}
	opts := d.DomainOptions
	if msg := applyOptions(in, &opts, s.minRateLimit(r.Context())); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if in.WithWWW {
		writeError(w, http.StatusBadRequest, "with_www is only for new domains — add www as its own redirect domain")
		return
	}
	d, err := s.store.UpdateDomainSettings(r.Context(), d.ID, port, tls, force)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if d, err = s.store.UpdateDomainOptions(r.Context(), d.ID, opts); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.resync(r)
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	s.audit(r, p, "domain.update", proj.Slug+"/"+app.Slug, map[string]any{"host": d.Host, "tls": tls, "force_https": force, "port": port})
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	d, ok := s.appDomain(w, r, app)
	if !ok {
		return
	}
	if err := s.store.DeleteDomain(r.Context(), d.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.releaseDNS(r.Context(), d)
	s.resync(r)
	s.audit(r, p, "domain.delete", proj.Slug+"/"+app.Slug, map[string]string{"host": d.Host, "path": d.PathPrefix})
	w.WriteHeader(http.StatusNoContent)
}

// checkDomain resolves the host and compares it with the server's addresses.
func (s *Server) checkDomain(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	d, ok := s.appDomain(w, r, app)
	if !ok {
		return
	}
	expected := s.serverIPs(r.Context(), s.loadSettings(r.Context()))
	host := strings.TrimPrefix(d.Host, "*.")
	if strings.HasPrefix(d.Host, "*.") {
		host = "nectar-dns-check." + host // any name under the wildcard
	}
	res := dnsCheck(r.Context(), host, expected)
	res["host"] = d.Host
	writeJSON(w, http.StatusOK, res)
}

// dnsCheck resolves host and compares the answer with the server's addresses.
func dnsCheck(ctx context.Context, host string, expected []string) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if addrs == nil {
		addrs = []string{}
	}
	res := map[string]any{"host": host, "resolves_to": addrs, "expected": expected}
	switch {
	case err != nil:
		res["ok"], res["message"] = false, "the domain does not resolve yet: "+err.Error()
	case len(expected) == 0:
		res["ok"], res["message"] = true, "resolves; set the server's public IP in settings to verify the target"
	default:
		match := false
		for _, a := range addrs {
			if slices.Contains(expected, a) {
				match = true
			}
		}
		res["ok"] = match
		if match {
			res["message"] = "DNS points to this server"
		} else {
			res["message"] = fmt.Sprintf("DNS points to %s, but should point to %s", strings.Join(addrs, ", "), strings.Join(expected, ", "))
		}
	}
	return res
}

func (s *Server) issueDomainCert(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	d, ok := s.appDomain(w, r, app)
	if !ok {
		return
	}
	if !d.TLS {
		writeError(w, http.StatusBadRequest, "enable TLS for this domain first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	d, err := s.syncer.IssueCertificate(ctx, d.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "domain.certificate", proj.Slug+"/"+app.Slug, map[string]string{"host": d.Host})
	writeJSON(w, http.StatusOK, d)
}

// MinRateLimit is the lowest per-domain rate limit (requests/s per IP) for gateways that block clients
// over it for 24 h (before the rate-limit-429 feature).
const MinRateLimit = 10

// minRateLimit is 1 when the gateway answers 429 over the limit, else MinRateLimit.
func (s *Server) minRateLimit(ctx context.Context) int {
	if s.gateway == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if v, err := s.gateway.Version(ctx); err == nil && v.Has(gateway.FeatureRateLimit429) {
		return 1
	}
	return MinRateLimit
}
