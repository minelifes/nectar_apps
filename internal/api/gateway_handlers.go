package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/store"
)

// Gateway observability and settings, proxied from nectar-gateway's admin API so the panel is the
// only UI (PLAN.md §5: the gateway runs headless).

func (s *Server) needGateway(w http.ResponseWriter) bool {
	if s.gateway == nil {
		writeError(w, http.StatusServiceUnavailable, "gateway not configured (NECTAR_GATEWAY_URL / NECTAR_GATEWAY_TOKEN)")
		return false
	}
	return true
}

func gwCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 10*time.Second)
}

func (s *Server) gatewayErr(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadGateway, err.Error())
}

// relay forwards a GET to the gateway, keeping only allowed query parameters.
func (s *Server) relay(path string, params ...string) handlerWithPrincipal {
	return func(w http.ResponseWriter, r *http.Request, _ *principal) {
		if !s.needGateway(w) {
			return
		}
		q := url.Values{}
		for _, p := range params {
			if v := r.URL.Query().Get(p); v != "" {
				q.Set(p, v)
			}
		}
		target := path
		if len(q) > 0 {
			target += "?" + q.Encode()
		}
		ctx, cancel := gwCtx(r)
		defer cancel()
		raw, err := s.gateway.Raw(ctx, target)
		if err != nil {
			s.gatewayErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}
}

func (s *Server) unblockIP(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needGateway(w) {
		return
	}
	ip := r.PathValue("ip")
	ctx, cancel := gwCtx(r)
	defer cancel()
	if _, err := s.gateway.Send(ctx, http.MethodDelete, "/api/blocked/"+url.PathEscape(ip), nil); err != nil {
		s.gatewayErr(w, err)
		return
	}
	s.audit(r, p, "gateway.unblock", ip, nil)
	w.WriteHeader(http.StatusNoContent)
}

// blockIP blocks a client IP at the gateway (ttl_seconds 0 = 10 years).
func (s *Server) blockIP(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needGateway(w) {
		return
	}
	var in struct {
		IP         string `json:"ip"`
		Reason     string `json:"reason"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := net.ParseIP(strings.TrimSpace(in.IP))
	if ip == nil {
		writeError(w, http.StatusBadRequest, "ip must be an IPv4 or IPv6 address")
		return
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.String() == clientIP(r) {
		writeError(w, http.StatusBadRequest, "refusing to block a private address or your own")
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "blocked by " + p.user.Email
	}
	ctx, cancel := gwCtx(r)
	defer cancel()
	if _, err := s.gateway.Send(ctx, http.MethodPost, "/api/blocked", map[string]any{"ip": ip.String(), "reason": reason, "ttl_seconds": in.TTLSeconds}); err != nil {
		s.gatewayErr(w, err)
		return
	}
	s.audit(r, p, "gateway.block", ip.String(), map[string]any{"reason": reason, "ttl_seconds": in.TTLSeconds})
	w.WriteHeader(http.StatusNoContent)
}

// putGatewaySettings changes the gateway's runtime settings (fallback upstream / TLS passthrough).
func (s *Server) putGatewaySettings(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needGateway(w) {
		return
	}
	var in struct {
		FallbackHTTP *string `json:"fallback_http"`
		FallbackTLS  *string `json:"fallback_tls"`
		ACMEEmail    *string `json:"acme_email"`
	}
	if !decode(w, r, &in) {
		return
	}
	body := map[string]string{}
	if in.FallbackHTTP != nil {
		body["fallback_http"] = *in.FallbackHTTP
	}
	if in.FallbackTLS != nil {
		body["fallback_tls"] = *in.FallbackTLS
	}
	if in.ACMEEmail != nil {
		body["acme_email"] = strings.TrimSpace(*in.ACMEEmail)
	}
	ctx, cancel := gwCtx(r)
	defer cancel()
	out, err := s.gateway.Send(ctx, http.MethodPut, "/api/settings", body)
	if err != nil {
		if se, ok := err.(*gateway.StatusError); ok && se.Code == http.StatusBadRequest {
			writeError(w, http.StatusBadRequest, se.Message)
			return
		}
		s.gatewayErr(w, err)
		return
	}
	s.audit(r, p, "gateway.settings", "", body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// appHosts returns the distinct hosts of an app's domains.
func (s *Server) appHosts(r *http.Request, app store.App) ([]string, error) {
	ds, err := s.store.ListDomains(r.Context(), app.ID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range ds {
		if !seen[d.Host] {
			seen[d.Host] = true
			out = append(out, d.Host)
		}
	}
	return out, nil
}

// appMetrics returns request statistics for every domain of the app.
func (s *Server) appMetrics(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	if !s.needGateway(w) {
		return
	}
	hosts, err := s.appHosts(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	minutes, _ := strconv.Atoi(r.URL.Query().Get("minutes"))
	if minutes <= 0 || minutes > 60 {
		minutes = 60
	}
	ctx, cancel := gwCtx(r)
	defer cancel()
	out := []gateway.Summary{}
	for _, h := range hosts {
		sums, err := s.gateway.Metrics(ctx, minutes, h, r.URL.Query().Get("series") == "1")
		if err != nil {
			s.gatewayErr(w, err)
			return
		}
		out = append(out, sums...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"minutes": minutes, "domains": out})
}

// appAccessLog merges the access logs of the app's domains, newest first.
func (s *Server) appAccessLog(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	if !s.needGateway(w) {
		return
	}
	hosts, err := s.appHosts(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	ctx, cancel := gwCtx(r)
	defer cancel()
	var all []map[string]any
	for _, h := range hosts {
		q := url.Values{"domain": {h}, "limit": {strconv.Itoa(limit)}}
		if st := r.URL.Query().Get("status"); st != "" {
			q.Set("status", st)
		}
		entries, err := s.gateway.AccessLog(ctx, q)
		if err != nil {
			s.gatewayErr(w, err)
			return
		}
		all = append(all, entries...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		ti, _ := all[i]["time"].(string)
		tj, _ := all[j]["time"].(string)
		return ti > tj // RFC 3339 UTC sorts lexically
	})
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, all)
}
