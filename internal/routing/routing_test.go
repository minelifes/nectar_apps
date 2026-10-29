package routing

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/store"
)

// fakeGW is an in-memory gateway.
type fakeGW struct {
	mu     sync.Mutex
	routes map[int64]gateway.Route
	next   int64
	calls  []string
}

func newFakeGW() *fakeGW { return &fakeGW{routes: map[int64]gateway.Route{}} }

func (f *fakeGW) ListManaged(context.Context) ([]gateway.Route, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []gateway.Route
	for _, r := range f.routes {
		if r.ManagedBy == gateway.ManagedBy {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeGW) Create(_ context.Context, r gateway.Route) (gateway.Route, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.routes {
		if o.Domain == r.Domain && o.PathPrefix == r.PathPrefix {
			return r, &gateway.StatusError{Code: 409, Message: "exists"}
		}
	}
	f.next++
	r.ID = f.next
	f.routes[r.ID] = r
	f.calls = append(f.calls, "create "+r.Domain)
	return r, nil
}

func (f *fakeGW) Update(_ context.Context, r gateway.Route) (gateway.Route, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.CertNotAfter = f.routes[r.ID].CertNotAfter
	f.routes[r.ID] = r
	f.calls = append(f.calls, "update "+r.Domain)
	return r, nil
}

func (f *fakeGW) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "delete "+f.routes[id].Domain)
	delete(f.routes, id)
	return nil
}

func (f *fakeGW) IssueCert(_ context.Context, id int64) (gateway.Route, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.routes[id]
	r.CertNotAfter = 1_900_000_000
	f.routes[id] = r
	return r, nil
}

func (f *fakeGW) byDomain(d string) (gateway.Route, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.routes {
		if r.Domain == d {
			return r, true
		}
	}
	return gateway.Route{}, false
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "t.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	gw := newFakeGW()
	s := New(st, gw, slog.New(slog.NewTextHandler(io.Discard, nil)))

	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "java",
		Config: store.AppConfig{Port: 8080, Replicas: 1, Health: store.HealthConfig{Type: "http", Path: "/health"}}})
	d, _ := st.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: "API-Shop.apps.example.com.", TLS: true, ForceHTTPS: true})
	if d.Host != "api-shop.apps.example.com" || d.PathPrefix != "/" {
		t.Fatalf("normalized: %+v", d)
	}

	// not deployed yet → waiting, no route
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ = st.GetDomain(ctx, d.ID); d.Status != StatusWaiting || len(gw.routes) != 0 {
		t.Fatalf("before deploy: %+v %v", d, gw.routes)
	}

	// deployed → route created with the service DNS name and health check
	rel, _ := st.CreateRelease(ctx, app.ID, nil, "img", app.Config, nil)
	st.SetReleaseStatus(ctx, rel.ID, store.ReleaseLive, "")
	s.Sync(ctx)
	r, ok := gw.byDomain("api-shop.apps.example.com")
	if !ok || r.TargetIP != "api.prj-shop.svc.cluster.local" || r.TargetPort != 8080 || !r.ACME || !r.ForceSSL ||
		!r.HCEnabled || r.HCPath != "/health" || r.Owner != Owner(d.ID) {
		t.Fatalf("route: %+v", r)
	}
	if d, _ = st.GetDomain(ctx, d.ID); d.Status != StatusActive || d.GatewayRouteID == nil || d.StatusMessage == "" {
		t.Fatalf("active (cert pending note expected): %+v", d)
	}

	// idempotent: a second sync changes nothing
	n := len(gw.calls)
	s.Sync(ctx)
	if len(gw.calls) != n {
		t.Fatalf("second sync made calls: %v", gw.calls[n:])
	}

	// drift: someone changed the route in the gateway → repaired
	gw.mu.Lock()
	r.TargetPort = 9999
	gw.routes[r.ID] = r
	gw.mu.Unlock()
	s.Sync(ctx)
	if r, _ = gw.byDomain("api-shop.apps.example.com"); r.TargetPort != 8080 {
		t.Fatalf("drift not repaired: %+v", r)
	}

	// stopping the app (new live release with 0 replicas) → maintenance page
	cfg := app.Config
	cfg.Replicas = 0
	rel2, _ := st.CreateRelease(ctx, app.ID, nil, "img", cfg, nil)
	st.SetReleaseStatus(ctx, rel2.ID, store.ReleaseLive, "")
	s.Sync(ctx)
	if r, _ = gw.byDomain("api-shop.apps.example.com"); !r.Maintenance || r.HCEnabled {
		t.Fatalf("stopped app: %+v", r)
	}

	// certificate on demand
	if d, err = s.IssueCertificate(ctx, d.ID); err != nil || d.CertNotAfter == nil {
		t.Fatalf("certificate: %+v %v", d, err)
	}

	// conflict with an unmanaged route
	gw.Create(ctx, gateway.Route{Domain: "taken.example.com", PathPrefix: "/", ManagedBy: ""})
	d2, _ := st.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: "taken.example.com"})
	s.Sync(ctx)
	if d2, _ = st.GetDomain(ctx, d2.ID); d2.Status != StatusError {
		t.Fatalf("conflict: %+v", d2)
	}

	// deleting the domain deletes its route; unmanaged routes are never touched
	st.DeleteDomain(ctx, d.ID)
	st.DeleteDomain(ctx, d2.ID)
	s.Sync(ctx)
	if _, ok := gw.byDomain("api-shop.apps.example.com"); ok {
		t.Fatal("orphan route not deleted")
	}
	if _, ok := gw.byDomain("taken.example.com"); !ok {
		t.Fatal("unmanaged route deleted")
	}

	// no gateway configured
	none := New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d3, _ := st.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: "x.example.com"})
	none.Sync(ctx)
	if d3, _ = st.GetDomain(ctx, d3.ID); d3.Status != StatusError {
		t.Fatalf("no gateway: %+v", d3)
	}
}
