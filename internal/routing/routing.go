// Package routing keeps nectar-gateway's routes in sync with the panel's domains.
//
// The panel is the source of truth: every domain of a deployed web app becomes one gateway route
// (managed_by=nectar-apps, owner=domain:<id>). Sync creates missing routes, fixes drifted ones and
// deletes routes whose domain no longer exists. It runs after changes and periodically (drift repair).
package routing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// Gateway is the subset of the gateway client used here (a fake implements it in tests).
type Gateway interface {
	ListManaged(ctx context.Context) ([]gateway.Route, error)
	Create(ctx context.Context, r gateway.Route) (gateway.Route, error)
	Update(ctx context.Context, r gateway.Route) (gateway.Route, error)
	Delete(ctx context.Context, id int64) error
	IssueCert(ctx context.Context, id int64) (gateway.Route, error)
}

// Domain statuses.
const (
	StatusActive  = "active"  // route exists in the gateway
	StatusWaiting = "waiting" // app not deployed yet
	StatusError   = "error"
)

// Syncer reconciles domains → gateway routes.
type Syncer struct {
	store   *store.Store
	gw      Gateway // nil = gateway not configured
	log     *slog.Logger
	mu      sync.Mutex // one sync at a time
	trigger chan struct{}
}

// New creates a syncer. gw may be nil (domains are stored but marked "gateway not configured").
func New(st *store.Store, gw Gateway, log *slog.Logger) *Syncer {
	return &Syncer{store: st, gw: gw, log: log, trigger: make(chan struct{}, 1)}
}

// Enabled reports whether a gateway is configured.
func (s *Syncer) Enabled() bool { return s != nil && s.gw != nil }

// Trigger requests a sync soon (non-blocking).
func (s *Syncer) Trigger() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Run syncs on Trigger and every interval until ctx ends.
func (s *Syncer) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		sctx, cancel := context.WithTimeout(ctx, time.Minute)
		if err := s.Sync(sctx); err != nil {
			s.log.Warn("route sync", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.trigger:
		}
	}
}

// Owner returns the gateway owner reference of a domain.
func Owner(domainID int64) string { return "domain:" + strconv.FormatInt(domainID, 10) }

// ServiceHost is the in-cluster DNS name of an app's Service.
func ServiceHost(app store.App, p store.Project) string {
	return app.Slug + "." + p.Namespace + ".svc.cluster.local"
}

// desiredRoute builds the gateway route for a domain, or returns a status explaining why there is none.
func (s *Syncer) desiredRoute(ctx context.Context, d store.Domain) (*gateway.Route, string, string) {
	app, err := s.store.GetApp(ctx, d.AppID)
	if err != nil {
		return nil, StatusError, "app not found"
	}
	wildcard := strings.HasPrefix(d.Host, "*.")
	htpasswd := d.BasicAuth
	if d.Kind == store.DomainRedirect {
		// redirects do not depend on the app being deployed
		return &gateway.Route{
			Domain: d.Host, PathPrefix: d.PathPrefix, Mode: "redirect", RedirectURL: d.RedirectTo, RedirectCode: d.RedirectCode, BackendProtocol: "http1",
			ACME: d.TLS && !wildcard, ForceSSL: d.TLS && d.ForceHTTPS && !wildcard,
			ManagedBy: gateway.ManagedBy, Owner: Owner(d.ID), HCInterval: 30, HCPath: "/",
			BasicAuthHtpasswd: &htpasswd, AuthFingerprint: gateway.AuthFingerprint(htpasswd),
			Web: webFor(d),
		}, StatusActive, ""
	}
	if app.Kind != runtimes.KindWeb {
		return nil, StatusError, "only web apps can have domains"
	}
	live, err := s.store.LiveRelease(ctx, app.ID)
	if err != nil {
		return nil, StatusWaiting, "deploy the app to activate this domain"
	}
	p, err := s.store.GetProject(ctx, app.ProjectID)
	if err != nil {
		return nil, StatusError, "project not found"
	}
	cfg := live.Config
	port := d.TargetPort
	if port == 0 {
		port = cfg.Port
	}
	r := &gateway.Route{
		Domain:      d.Host,
		PathPrefix:  d.PathPrefix,
		Mode:        "proxy",
		TargetIP:    ServiceHost(app, p),
		TargetPort:  port,
		ACME:        d.TLS && !wildcard,
		ForceSSL:    d.TLS && d.ForceHTTPS && !wildcard,
		Maintenance: cfg.Replicas == 0, // stopped apps show the maintenance page
		ManagedBy:   gateway.ManagedBy,
		Owner:       Owner(d.ID),
		HCInterval:  30,
		HCPath:      "/",

		BackendProtocol:   d.BackendProtocol,
		TimeoutSeconds:    d.TimeoutSeconds,
		MaxBodyMB:         d.MaxBodyMB,
		RateLimit:         d.RateLimit,
		BasicAuthHtpasswd: &htpasswd,
		AuthFingerprint:   gateway.AuthFingerprint(htpasswd),
		Web:               webFor(d),
	}
	if r.BackendProtocol == "" {
		r.BackendProtocol = "http1"
	}
	if cfg.Health.Type == "http" && cfg.Replicas > 0 && r.BackendProtocol != "h2c" {
		r.HCEnabled = true
		r.HCPath = cfg.Health.Path
		if r.HCPath == "" {
			r.HCPath = "/"
		}
	}
	return r, StatusActive, ""
}

// webFor returns the domain's web options for the route (nil = none). Redirect domains only use HSTS and
// the allowlist; the gateway checks redirect rules before the route's own redirect anyway.
func webFor(d store.Domain) *gateway.Web {
	if d.Web.IsZero() {
		return nil
	}
	w := *d.Web
	return &w
}

func differs(want, have gateway.Route) bool {
	if want.Web.Encode() != have.Web.Encode() {
		return true
	}
	return want.Domain != have.Domain || want.PathPrefix != have.PathPrefix || want.TargetIP != have.TargetIP ||
		want.TargetPort != have.TargetPort || want.ACME != have.ACME || want.ForceSSL != have.ForceSSL ||
		want.Maintenance != have.Maintenance || want.HCEnabled != have.HCEnabled || want.HCPath != have.HCPath ||
		want.Mode != have.Mode || want.RedirectURL != have.RedirectURL || want.RedirectCode != have.RedirectCode ||
		want.BackendProtocol != orDefault(have.BackendProtocol, "http1") || want.TimeoutSeconds != have.TimeoutSeconds ||
		want.MaxBodyMB != have.MaxBodyMB || want.RateLimit != have.RateLimit || want.AuthFingerprint != have.AuthFingerprint
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// hasFeature asks the gateway whether it supports an optional feature (false when it cannot say).
func (s *Syncer) hasFeature(ctx context.Context, feature string) bool {
	v, ok := s.gw.(interface {
		Version(context.Context) (gateway.Version, error)
	})
	if !ok {
		return true // test fakes: everything
	}
	ver, err := v.Version(ctx)
	return err == nil && ver.Has(feature)
}

// Sync reconciles once.
func (s *Syncer) Sync(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	domains, err := s.store.ListDomains(ctx, 0)
	if err != nil {
		return err
	}
	if s.gw == nil {
		for _, d := range domains {
			_ = s.store.SetDomainState(ctx, d.ID, StatusError, "gateway not configured (NECTAR_GATEWAY_URL / NECTAR_GATEWAY_TOKEN)", nil, 0)
		}
		return nil
	}
	actual, err := s.gw.ListManaged(ctx)
	if err != nil {
		for _, d := range domains {
			_ = s.store.SetDomainState(ctx, d.ID, StatusError, "gateway unreachable: "+err.Error(), d.GatewayRouteID, certUnix(d))
		}
		return err
	}
	byOwner := map[string]gateway.Route{}
	for _, r := range actual {
		if old, dup := byOwner[r.Owner]; dup { // duplicates (e.g. after a crash): keep one
			_ = s.gw.Delete(ctx, old.ID)
		}
		byOwner[r.Owner] = r
	}

	webOK := s.hasFeature(ctx, gateway.FeatureWebOptions)
	var errs []error
	wanted := map[string]bool{}
	for _, d := range domains {
		want, status, msg := s.desiredRoute(ctx, d)
		owner := Owner(d.ID)
		if want == nil {
			_ = s.store.SetDomainState(ctx, d.ID, status, msg, nil, 0)
			continue // its route (if any) is deleted below
		}
		wanted[owner] = true
		have, exists := byOwner[owner]
		webNote := ""
		if want.Web != nil && !webOK {
			// an older gateway would silently ignore the options: serve the route without them and say so
			want.Web = nil
			webNote = "redirect rules, allowlist, headers, CORS, compression and HSTS need nectar-gateway 1.3 or newer (update it)"
		}
		if want.Web == nil && have.Web != nil {
			want.Web = &gateway.Web{} // clears the options on the gateway ("web" omitted would keep them)
		}
		var got gateway.Route
		switch {
		case !exists:
			got, err = s.gw.Create(ctx, *want)
		case differs(*want, have):
			want.ID = have.ID
			got, err = s.gw.Update(ctx, *want)
		default:
			got = have
		}
		if err != nil {
			msg := err.Error()
			if gateway.IsConflict(err) {
				msg = "this host and path are already used by another gateway route"
			}
			_ = s.store.SetDomainState(ctx, d.ID, StatusError, msg, nil, 0)
			errs = append(errs, fmt.Errorf("%s%s: %w", d.Host, d.PathPrefix, err))
			continue
		}
		note := ""
		notAfter := got.CertNotAfter
		if notAfter == 0 && got.CoveredUntil > 0 {
			notAfter = got.CoveredUntil // served by a wildcard certificate (e.g. *.<base domain>)
			note = "HTTPS by the certificate " + got.CoveredBy
		}
		if want.ACME && notAfter == 0 {
			note = "waiting for the certificate — the domain must point to this server"
		}
		if webNote != "" {
			note = strings.TrimPrefix(note+"; "+webNote, "; ")
		}
		id := got.ID
		_ = s.store.SetDomainState(ctx, d.ID, StatusActive, note, &id, notAfter)
	}
	for owner, r := range byOwner {
		if !wanted[owner] {
			if err := s.gw.Delete(ctx, r.ID); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// IssueCertificate syncs, then asks the gateway to obtain a certificate for the domain right away.
func (s *Syncer) IssueCertificate(ctx context.Context, domainID int64) (store.Domain, error) {
	if s.gw == nil {
		return store.Domain{}, errors.New("gateway not configured")
	}
	if err := s.Sync(ctx); err != nil {
		s.log.Warn("route sync before certificate", "err", err)
	}
	d, err := s.store.GetDomain(ctx, domainID)
	if err != nil {
		return d, err
	}
	if d.GatewayRouteID == nil {
		return d, errors.New("the domain has no gateway route yet: " + d.StatusMessage)
	}
	r, err := s.gw.IssueCert(ctx, *d.GatewayRouteID)
	if err != nil {
		_ = s.store.SetDomainState(ctx, d.ID, StatusActive, "certificate request failed: "+err.Error(), d.GatewayRouteID, 0)
		return d, err
	}
	_ = s.store.SetDomainState(ctx, d.ID, StatusActive, "", d.GatewayRouteID, r.CertNotAfter)
	return s.store.GetDomain(ctx, domainID)
}

func certUnix(d store.Domain) int64 {
	if d.CertNotAfter == nil {
		return 0
	}
	return d.CertNotAfter.Unix()
}

// ReservedKeys returns host+path of every panel domain. The Ingress sync leaves these to the panel, so an
// app's own domain always wins over an Ingress object for the same host.
func ReservedKeys(ctx context.Context, st *store.Store) map[string]bool {
	ds, err := st.ListDomains(ctx, 0)
	if err != nil {
		return nil
	}
	out := make(map[string]bool, len(ds))
	for _, d := range ds {
		p := d.PathPrefix
		if p == "" {
			p = "/"
		}
		out[strings.ToLower(d.Host)+p] = true
	}
	return out
}
