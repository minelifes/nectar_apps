package store

import (
	"context"
	"errors"
	"testing"
)

// Migration 0024 rebuilds domains with an optional app_id: rows survive it, deleting an app still
// removes its domains, and proxy domains (no app) are stored and listed apart.
func TestProxyDomainsAndRebuild(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	p, err := s.CreateProject(ctx, "shop", "Shop", nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateApp(ctx, App{ProjectID: p.ID, Slug: "web", Name: "Web", Kind: "web", RuntimeID: "image"})
	if err != nil {
		t.Fatal(err)
	}
	ad, err := s.CreateDomain(ctx, Domain{AppID: a.ID, Host: "shop.example.com", TLS: true, DomainOptions: DomainOptions{RateLimit: 20}})
	if err != nil {
		t.Fatal(err)
	}
	pd, err := s.CreateDomain(ctx, Domain{Host: "nas.example.com", TargetHost: "127.0.0.1", TargetPort: 5000, TargetHTTPS: true,
		DomainOptions: DomainOptions{Kind: DomainProxy}})
	if err != nil || pd.AppID != 0 || pd.TargetHost != "127.0.0.1" || !pd.TargetHTTPS || pd.TargetPort != 5000 {
		t.Fatalf("proxy domain: %+v %v", pd, err)
	}
	if _, err := s.CreateDomain(ctx, Domain{Host: "NAS.example.com", TargetHost: "x", TargetPort: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate host: %v", err)
	}

	// run the rebuild again on the filled table
	body, err := migrationsFS.ReadFile("migrations/0024_proxy_domains.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got, err := s.GetDomain(ctx, ad.ID); err != nil || got.AppID != a.ID || got.RateLimit != 20 || got.Host != "shop.example.com" {
		t.Fatalf("app domain after rebuild: %+v %v", got, err)
	}
	if ps, err := s.ListProxyDomains(ctx); err != nil || len(ps) != 1 || ps[0].ID != pd.ID {
		t.Fatalf("proxies: %+v %v", ps, err)
	}
	if got, err := s.UpdateDomainTarget(ctx, pd.ID, " 10.0.0.5 ", 8080, false); err != nil || got.TargetHost != "10.0.0.5" || got.TargetPort != 8080 || got.TargetHTTPS {
		t.Fatalf("update target: %+v %v", got, err)
	}
	if err := s.DeleteApp(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDomain(ctx, ad.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("app domain kept after the app was deleted: %v", err)
	}
	if _, err := s.GetDomain(ctx, pd.ID); err != nil {
		t.Fatalf("proxy domain lost: %v", err)
	}
}
