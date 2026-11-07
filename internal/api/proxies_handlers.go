package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/store"
)

// Proxy domains: a host (+ path) the gateway forwards to any address it can reach — a port on the
// server (127.0.0.1:9000; the gateway runs with the host network), another machine (10.0.0.5:8080) or a
// Service Nectar does not manage (name.ns.svc.cluster.local:80) — without creating an app. They are
// domains without an app (kind proxy, or redirect), so certificates, DNS records, web options, basic
// auth and rate limits work as for app domains. Server-wide: listed with server.view, changed with
// settings.manage.

// proxyInput is domainInput plus where to forward.
type proxyInput struct {
	domainInput
	TargetHost  *string `json:"target_host"`  // IP or DNS name
	TargetHTTPS *bool   `json:"target_https"` // connect to the target over HTTPS (certificate not verified)
}

var targetHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?)*$`)

// validTargetHost accepts an IP address or a DNS name (localhost and cluster names included).
func validTargetHost(h string) bool {
	if net.ParseIP(h) != nil {
		return true
	}
	return len(h) <= 253 && targetHostRe.MatchString(h)
}

// proxyOptions applies kind (proxy | redirect) and the gateway options. Returns an error message or "".
func (s *Server) proxyOptions(r *http.Request, in proxyInput, o *store.DomainOptions) string {
	kind := o.Kind
	if kind == "" || kind == store.DomainApp {
		kind = store.DomainProxy
	}
	if in.Kind != nil {
		kind = *in.Kind
	}
	if kind != store.DomainProxy && kind != store.DomainRedirect {
		return "kind must be proxy or redirect"
	}
	// applyOptions knows app | redirect; a proxy takes the same options as an app domain
	k := kind
	if k == store.DomainProxy {
		k = store.DomainApp
	}
	in.Kind = &k
	o.Kind = k
	if msg := applyOptions(in.domainInput, o, s.minRateLimit(r.Context())); msg != "" {
		return msg
	}
	o.Kind = kind
	return ""
}

// proxyTarget validates host:port (+https) for kind proxy.
func proxyTarget(kind, host string, port int, https bool, protocol string) string {
	if kind != store.DomainProxy {
		return ""
	}
	switch {
	case !validTargetHost(host):
		return "target_host must be an IP address or a host name, e.g. 127.0.0.1, 10.0.0.5 or nas.local"
	case port < 1 || port > 65535:
		return "target_port must be 1–65535"
	case https && protocol == "h2c":
		return "gRPC (h2c) is for plain-text targets: turn off HTTPS to the target or use HTTP/1.1"
	}
	return ""
}

func (s *Server) listProxies(w http.ResponseWriter, r *http.Request, _ *principal) {
	ds, err := s.store.ListProxyDomains(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

func (s *Server) createProxy(w http.ResponseWriter, r *http.Request, p *principal) {
	var in proxyInput
	if !decode(w, r, &in) {
		return
	}
	host := store.NormalizeHost(in.Host)
	if !hostRe.MatchString(host) {
		writeError(w, http.StatusBadRequest, "host must be a domain like nas.example.com (or *.example.com)")
		return
	}
	path := store.NormalizePath(in.PathPrefix)
	if strings.ContainsAny(path, " ?#*") || len(path) > 200 {
		writeError(w, http.StatusBadRequest, "invalid path prefix")
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
		tls, force = false, false // wildcard hosts are served by a wildcard certificate (Gateway → Certificates)
	}
	var opts store.DomainOptions
	if msg := s.proxyOptions(r, in, &opts); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	target, https := "", false
	if in.TargetHost != nil {
		target = strings.ToLower(strings.TrimSpace(*in.TargetHost))
	}
	if in.TargetHTTPS != nil {
		https = *in.TargetHTTPS
	}
	port := in.TargetPort
	if opts.Kind != store.DomainProxy {
		target, port, https = "", 0, false
	}
	if msg := proxyTarget(opts.Kind, target, port, https, opts.BackendProtocol); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if in.WithWWW {
		writeError(w, http.StatusBadRequest, "with_www is for app domains: add www as a redirect proxy")
		return
	}
	d, err := s.store.CreateDomain(r.Context(), store.Domain{Host: host, PathPrefix: path, TargetPort: port, TLS: tls,
		ForceHTTPS: force, DomainOptions: opts, TargetHost: target, TargetHTTPS: https})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this host and path are already used (by an app or another proxy)")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.autoDNS(d)
	s.resync(r)
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	s.audit(r, p, "proxy.create", host+path, map[string]any{"kind": opts.Kind, "target": target, "port": port, "https": https})
	writeJSON(w, http.StatusCreated, d)
}

// proxyDomain loads {did} and checks it is a proxy domain.
func (s *Server) proxyDomain(w http.ResponseWriter, r *http.Request) (store.Domain, bool) {
	id, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad proxy id")
		return store.Domain{}, false
	}
	d, err := s.store.GetDomain(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.AppID != 0) {
		writeError(w, http.StatusNotFound, "proxy not found")
		return d, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return d, false
	}
	return d, true
}

func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request, p *principal) {
	d, ok := s.proxyDomain(w, r)
	if !ok {
		return
	}
	var in proxyInput
	if !decode(w, r, &in) {
		return
	}
	host, path := d.Host, d.PathPrefix
	if in.Host != "" {
		host = store.NormalizeHost(in.Host)
		if !hostRe.MatchString(host) {
			writeError(w, http.StatusBadRequest, "host must be a domain like nas.example.com (or *.example.com)")
			return
		}
	}
	if in.PathPrefix != "" {
		path = store.NormalizePath(in.PathPrefix)
		if strings.ContainsAny(path, " ?#*") || len(path) > 200 {
			writeError(w, http.StatusBadRequest, "invalid path prefix")
			return
		}
	}
	tls, force := d.TLS, d.ForceHTTPS
	if in.TLS != nil {
		tls = *in.TLS
	}
	if in.ForceHTTPS != nil {
		force = *in.ForceHTTPS
	}
	if strings.HasPrefix(host, "*.") {
		tls, force = false, false
	}
	opts := d.DomainOptions
	if msg := s.proxyOptions(r, in, &opts); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	target, port, https := d.TargetHost, d.TargetPort, d.TargetHTTPS
	if in.TargetHost != nil {
		target = strings.ToLower(strings.TrimSpace(*in.TargetHost))
	}
	if in.TargetPort != 0 {
		port = in.TargetPort
	}
	if in.TargetHTTPS != nil {
		https = *in.TargetHTTPS
	}
	if opts.Kind != store.DomainProxy {
		target, port, https = "", 0, false
	}
	if msg := proxyTarget(opts.Kind, target, port, https, opts.BackendProtocol); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if in.WithWWW {
		writeError(w, http.StatusBadRequest, "with_www is for app domains: add www as a redirect proxy")
		return
	}
	want := store.Domain{Host: host, PathPrefix: path, TargetPort: port, TLS: tls, ForceHTTPS: force, DomainOptions: opts,
		TargetHost: target, TargetHTTPS: https}
	if host != d.Host || path != d.PathPrefix {
		// a new host or path is a new route (a certificate never moves to another host)
		nd, err := s.store.CreateDomain(r.Context(), want)
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "this host and path are already used (by an app or another proxy)")
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if err := s.store.DeleteDomain(r.Context(), d.ID); err != nil {
			_ = s.store.DeleteDomain(r.Context(), nd.ID)
			s.serverError(w, r, err)
			return
		}
		s.releaseDNS(r.Context(), d)
		s.autoDNS(nd)
		s.resync(r)
		nd, _ = s.store.GetDomain(r.Context(), nd.ID)
		s.audit(r, p, "proxy.update", host+path, map[string]any{"from": d.Host + d.PathPrefix, "target": target, "port": port})
		writeJSON(w, http.StatusOK, nd)
		return
	}
	if _, err := s.store.UpdateDomainSettings(r.Context(), d.ID, port, tls, force); err != nil {
		s.serverError(w, r, err)
		return
	}
	if _, err := s.store.UpdateDomainOptions(r.Context(), d.ID, opts); err != nil {
		s.serverError(w, r, err)
		return
	}
	if _, err := s.store.UpdateDomainTarget(r.Context(), d.ID, target, port, https); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.resync(r)
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	s.audit(r, p, "proxy.update", host+path, map[string]any{"kind": opts.Kind, "target": target, "port": port, "https": https, "tls": tls})
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteProxy(w http.ResponseWriter, r *http.Request, p *principal) {
	d, ok := s.proxyDomain(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteDomain(r.Context(), d.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.releaseDNS(r.Context(), d)
	s.resync(r)
	s.audit(r, p, "proxy.delete", d.Host+d.PathPrefix, nil)
	w.WriteHeader(http.StatusNoContent)
}

// checkProxy compares the host's DNS with the server's addresses.
func (s *Server) checkProxy(w http.ResponseWriter, r *http.Request, _ *principal) {
	d, ok := s.proxyDomain(w, r)
	if !ok {
		return
	}
	expected := s.serverIPs(r.Context(), s.loadSettings(r.Context()))
	host := strings.TrimPrefix(d.Host, "*.")
	if strings.HasPrefix(d.Host, "*.") {
		host = "nectar-dns-check." + host
	}
	res := dnsCheck(r.Context(), host, expected)
	res["host"] = d.Host
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) issueProxyCert(w http.ResponseWriter, r *http.Request, p *principal) {
	d, ok := s.proxyDomain(w, r)
	if !ok {
		return
	}
	if !d.TLS {
		writeError(w, http.StatusBadRequest, "enable TLS for this proxy first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	d, err := s.syncer.IssueCertificate(ctx, d.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "proxy.certificate", d.Host, nil)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) createProxyDNS(w http.ResponseWriter, r *http.Request, p *principal) {
	d, ok := s.proxyDomain(w, r)
	if !ok {
		return
	}
	if !s.dnsAvailable(r.Context()) {
		writeError(w, http.StatusConflict, "nectar-gateway has no DNS provider: set one under Gateway → Certificates → DNS provider (needs the latest nectar-gateway)")
		return
	}
	state, msg := s.ensureDomainDNS(r.Context(), d, s.loadSettings(r.Context()))
	s.audit(r, p, "proxy.dns", d.Host, map[string]string{"state": state})
	d, _ = s.store.GetDomain(r.Context(), d.ID)
	code := http.StatusOK
	if state == dnsError {
		code = http.StatusBadGateway
	}
	writeJSON(w, code, map[string]any{"domain": d, "state": state, "message": msg})
}
