package update

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/store"

	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

// cluster wraps the fake: helper pods finish, pods of stopped deployments disappear, pods get a node IP.
type cluster struct {
	*kubefake.Fake
	mu         sync.Mutex
	helperFail string // op whose helper pod fails
	helpers    []string
}

func (c *cluster) GetObject(ctx context.Context, kind, ns, name string, out any) error {
	if kind == "Pod" && strings.HasPrefix(name, "nectar-update-") {
		var o kube.Object
		if err := c.Fake.GetObject(ctx, kind, ns, name, &o); err != nil {
			return err
		}
		phase := "Succeeded"
		c.mu.Lock()
		if c.helperFail != "" && strings.HasSuffix(name, "-"+c.helperFail) {
			phase = "Failed"
		}
		c.mu.Unlock()
		o["status"] = map[string]any{"phase": phase}
		b, _ := json.Marshal(o)
		return json.Unmarshal(b, out)
	}
	return c.Fake.GetObject(ctx, kind, ns, name, out)
}

func (c *cluster) Apply(ctx context.Context, o kube.Object) (kube.Object, error) {
	if k, _, name := kube.Meta(o); k == "Pod" {
		c.mu.Lock()
		c.helpers = append(c.helpers, name)
		c.mu.Unlock()
	}
	return c.Fake.Apply(ctx, o)
}

func (c *cluster) List(ctx context.Context, kind, ns, sel string) ([]kube.Object, error) {
	list, err := c.Fake.List(ctx, kind, ns, sel)
	if kind != "Pod" || err != nil {
		return list, err
	}
	var out []kube.Object
	for _, p := range list {
		name := kube.Str(p, "metadata", "name")
		var dep kube.Object
		if c.Fake.GetObject(ctx, "Deployment", ns, strings.TrimSuffix(name, "-pod"), &dep) == nil && kube.Int(dep, "spec", "replicas") == 0 {
			continue
		}
		if st, ok := p["status"].(map[string]any); ok {
			st["hostIP"] = "127.0.0.1"
		}
		out = append(out, p)
	}
	return out, nil
}

func (c *cluster) PodLogs(_ context.Context, _, pod, _ string, _ int, _ bool) (string, error) {
	if strings.HasSuffix(pod, "-snapshot") {
		return "snapshotting\nresult: /data/update-backups/pre-update-" + pod + ".db\n", nil
	}
	if strings.HasSuffix(pod, "-restore") {
		return "result: restored\n", nil
	}
	return "boom: bad thing", nil
}

func (c *cluster) image(name string) string {
	var dep kube.Object
	_ = c.Fake.GetObject(context.Background(), "Deployment", "nectar-system", name, &dep)
	for _, ct := range kube.Slice(dep, "spec", "template", "spec", "containers") {
		return kube.Str(ct, "image")
	}
	return ""
}

func (c *cluster) obj(kind, name string) kube.Object {
	var o kube.Object
	if c.Fake.GetObject(context.Background(), kind, "nectar-system", name, &o) != nil {
		return nil
	}
	return o
}

func (c *cluster) helperRan(suffix string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, h := range c.helpers {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

// world is a fake cluster with a panel and a gateway plus an HTTP server answering their version, the canary
// and the route list. Versions come from the image tags (ghcr/p:0.7.0 → 0.7.0).
type world struct {
	c         *cluster
	srv       *httptest.Server
	badCanary atomic.Value // gateway image whose canary fails
	routes    atomic.Int32
	lossy     atomic.Value // gateway image that lost all but one route
}

func newWorld(t *testing.T) *world {
	w := &world{c: &cluster{Fake: kubefake.New()}}
	w.routes.Store(3)
	w.badCanary.Store("")
	w.lossy.Store("")
	ns := "nectar-system"
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		tag := func(dep string) string { img := w.c.image(dep); return img[strings.LastIndexByte(img, ':')+1:] }
		switch {
		case r.URL.Path == "/panel/api/version":
			json.NewEncoder(rw).Encode(map[string]any{"version": tag("nectar-apps")})
		case r.URL.Path == "/gw/api/version":
			json.NewEncoder(rw).Encode(map[string]any{"version": tag("nectar-gateway"), "api_version": 1})
		case r.URL.Path == "/gw/api/routes":
			if r.Header.Get("Authorization") != "Bearer gwtoken" {
				rw.WriteHeader(401)
				return
			}
			n := w.routes.Load()
			if w.c.image("nectar-gateway") == w.lossy.Load().(string) {
				n = 1
			}
			list := make([]map[string]any, n)
			json.NewEncoder(rw).Encode(list)
		case r.URL.Path == "/__nectar/canary":
			if w.c.image("nectar-gateway") == w.badCanary.Load().(string) {
				rw.WriteHeader(502)
				return
			}
			rw.Write([]byte("ok"))
		default:
			rw.WriteHeader(404)
		}
	}))
	t.Cleanup(w.srv.Close)
	port := w.srv.URL[strings.LastIndexByte(w.srv.URL, ':')+1:]
	dep := func(name, container, image string, env []any) kube.Object {
		return kube.Object{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": name, "namespace": ns},
			"spec": map[string]any{"replicas": 1, "selector": map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": name}},
				"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app.kubernetes.io/name": name}},
					"spec": map[string]any{
						"containers": []any{map[string]any{"name": container, "image": image, "env": env,
							"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/data"}}}},
						"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": name + "-data"}}}}}}}
	}
	ctx := context.Background()
	w.c.Fake.Apply(ctx, dep("nectar-apps", "panel", "ghcr/p:0.6.0", []any{map[string]any{"name": "NECTAR_DATA_DIR", "value": "/data"}}))
	w.c.Fake.Apply(ctx, dep("nectar-gateway", "gateway", "ghcr/g:1.2.1", []any{
		map[string]any{"name": "PROXY_ADDR", "value": ":" + port}, map[string]any{"name": "DB_PATH", "value": "/data/nectar.db"}}))
	return w
}

func (w *world) runner(steps ...Step) *Runner {
	return &Runner{Kube: w.c, Poll: 5 * time.Millisecond, GatewayToken: "gwtoken",
		Spec: RunSpec{ID: 7, Namespace: "nectar-system", Steps: steps, HelperImage: "ghcr/p:0.7.0",
			PanelURL: w.srv.URL + "/panel", GatewayAdminURL: w.srv.URL + "/gw", HealthTimeout: 1}}
}

func readStatus(t *testing.T, c *cluster, id int64) RunStatus {
	var cm kube.Object
	if c.Fake.GetObject(context.Background(), "ConfigMap", "nectar-system", StatusName(id), &cm) != nil {
		t.Fatal("no status ConfigMap")
	}
	var st RunStatus
	if err := json.Unmarshal([]byte(kube.Str(cm, "data", "status")), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRunnerSuccess(t *testing.T) {
	w := newWorld(t)
	r := w.runner(
		Step{Component: Panel, From: "0.6.0", To: "0.7.0", FromImage: "ghcr/p:0.6.0", ToImage: "ghcr/p:0.7.0", Migrations: true},
		Step{Component: Gateway, From: "1.2.1", To: "1.3.0", FromImage: "ghcr/g:1.2.1", ToImage: "ghcr/g:1.3.0"})
	st := r.Run(context.Background())
	if st.State != RunSucceeded {
		t.Fatalf("state %s: %s\n%+v", st.State, st.Error, st.Log)
	}
	if w.c.image("nectar-apps") != "ghcr/p:0.7.0" || w.c.image("nectar-gateway") != "ghcr/g:1.3.0" {
		t.Error("images not switched")
	}
	for _, s := range st.Steps {
		if s.State != StepDone || !strings.Contains(s.Snapshot, "pre-update-") {
			t.Errorf("step %+v", s)
		}
	}
	if got := readStatus(t, w.c, 7); got.State != RunSucceeded || got.FinishedAt == nil {
		t.Errorf("saved status %+v", got)
	}
	// the gateway helper runs as root, the panel helper as the panel user
	if !w.c.helperRan("panel-snapshot") || !w.c.helperRan("gateway-snapshot") || w.c.helperRan("restore") {
		t.Errorf("helpers %v", w.c.helpers)
	}
	if w.c.Fake.Has("Pod", "nectar-system", "nectar-update-7-panel-snapshot") {
		t.Error("helper pod not cleaned up")
	}
	// running the same plan again skips the steps
	st = w.runner(r.Spec.Steps...).Run(context.Background())
	if st.State != RunSucceeded || st.Steps[0].State != StepSkipped {
		t.Errorf("second run %+v", st.Steps)
	}
}

func TestRunnerRollback(t *testing.T) {
	w := newWorld(t)
	w.badCanary.Store("ghcr/g:1.3.0")
	r := w.runner(
		Step{Component: Panel, From: "0.6.0", To: "0.7.0", FromImage: "ghcr/p:0.6.0", ToImage: "ghcr/p:0.7.0", Migrations: true},
		Step{Component: Gateway, From: "1.2.1", To: "1.3.0", FromImage: "ghcr/g:1.2.1", ToImage: "ghcr/g:1.3.0"})
	st := r.Run(context.Background())
	if st.State != RunRolledBack {
		t.Fatalf("state %s: %s", st.State, st.Error)
	}
	if w.c.image("nectar-apps") != "ghcr/p:0.6.0" || w.c.image("nectar-gateway") != "ghcr/g:1.2.1" {
		t.Errorf("not rolled back: %s %s", w.c.image("nectar-apps"), w.c.image("nectar-gateway"))
	}
	if !strings.Contains(st.Error, "canary") || len(st.Bad) != 1 || st.Bad[0] != "gateway@1.3.0" {
		t.Errorf("error %q bad %v", st.Error, st.Bad)
	}
	// the panel ran migrations: its database was restored; the gateway had none
	if !w.c.helperRan("panel-restore") || w.c.helperRan("gateway-restore") {
		t.Errorf("helpers %v", w.c.helpers)
	}
	if st.Steps[0].State != StepRolledBack || st.Steps[1].State != StepRolledBack {
		t.Errorf("steps %+v", st.Steps)
	}
	if kube.Int(w.c.obj("Deployment", "nectar-apps"), "spec", "replicas") != 1 {
		t.Error("panel not scaled back up")
	}
}

func TestRunnerPreflightFailure(t *testing.T) {
	w := newWorld(t)
	w.c.helperFail = "snapshot"
	st := w.runner(Step{Component: Gateway, From: "1.2.1", To: "1.3.0", ToImage: "ghcr/g:1.3.0"}).Run(context.Background())
	if st.State != RunFailed || !strings.Contains(st.Error, "backup") || w.c.image("nectar-gateway") != "ghcr/g:1.2.1" {
		t.Errorf("state %s %q", st.State, st.Error)
	}
	w.c.helperFail = ""
	st = w.runner(Step{Component: "nope", To: "1", ToImage: "x"}).Run(context.Background())
	if st.State != RunFailed {
		t.Error("unknown component")
	}
}

func TestRunnerLostRoutes(t *testing.T) {
	w := newWorld(t)
	r := w.runner(Step{Component: Gateway, From: "1.2.1", To: "1.3.0", ToImage: "ghcr/g:1.3.0"})
	// the new gateway loses routes after the switch
	w.lossy.Store("ghcr/g:1.3.0")
	st := r.Run(context.Background())
	if st.State != RunRolledBack || !strings.Contains(st.Error, "routes") {
		t.Errorf("state %s %q", st.State, st.Error)
	}
}

func TestSnapshotRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nectar.db")
	db, err := sql.Open(sqliteDriver, "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE TABLE t (v TEXT)`)
	db.Exec(`INSERT INTO t VALUES ('before')`)
	ctx := context.Background()
	var snap string
	for i := 0; i < 4; i++ {
		if snap, err = Snapshot(ctx, sqliteDriver, dbPath, filepath.Join(dir, "update-backups"), fmt.Sprintf("run-%d", i), 3); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "update-backups"))
	if len(entries) != 3 {
		t.Errorf("kept %d snapshots", len(entries))
	}
	db.Exec(`UPDATE t SET v = 'after'`)
	db.Close()
	if err := RestoreFile(snap, dbPath); err != nil {
		t.Fatal(err)
	}
	db, _ = sql.Open(sqliteDriver, "file:"+dbPath)
	defer db.Close()
	var v string
	if err := db.QueryRow(`SELECT v FROM t`).Scan(&v); err != nil || v != "before" {
		t.Errorf("restored %q %v", v, err)
	}
	if RestoreFile(dbPath, dbPath) == nil {
		t.Error("restoring a non-snapshot must fail")
	}
}

// ---------------------------------------------------------------- manager --

type gw struct{ v string }

func (g gw) Version(context.Context) (string, int, error) { return g.v, 1, nil }

func feedServer(t *testing.T, panel, gateway []Release) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		comp, list := Panel, panel
		if strings.Contains(r.URL.Path, "gateway") {
			comp, list = Gateway, gateway
		}
		json.NewEncoder(rw).Encode(Feed{Schema: 1, Components: map[string]map[string][]Release{comp: {Stable: list}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newManager(t *testing.T, w *world, feed *httptest.Server) (*Manager, *[]string) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "m.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var notes []string
	var mu sync.Mutex
	m := &Manager{Store: st, Kube: w.c, Gateway: gw{"1.2.1"}, Fetcher: &Fetcher{}, Namespace: "nectar-system",
		PanelVersion: "0.6.0", PanelAPI: 1, PanelRequires: Requires{GatewayAPI: []int{1}}, Poll: 5 * time.Millisecond,
		Notify: func(_ context.Context, title, _ string, _ bool) { mu.Lock(); notes = append(notes, title); mu.Unlock() }}
	set := Defaults
	set.Panel.FeedURL = feed.URL + "/panel.json"
	set.Gateway.FeedURL = feed.URL + "/gateway.json"
	set.K3s.Policy = PolicyOff
	if _, err := m.SaveSettings(ctx, set); err != nil {
		t.Fatal(err)
	}
	return m, &notes
}

func TestManagerCheckPlanStart(t *testing.T) {
	w := newWorld(t)
	feed := feedServer(t,
		[]Release{{Version: "0.7.0", Image: "ghcr/p:0.7.0", Migrations: true, Requires: Requires{GatewayAPI: []int{1}, MinGateway: "1.3.0"}},
			{Version: "0.6.1", Image: "ghcr/p:0.6.1"}, {Version: "0.6.0", Image: "ghcr/p:0.6.0"}},
		[]Release{{Version: "1.3.0", Image: "ghcr/g:1.3.0", APIVersion: intp(1)}, {Version: "1.2.1", Image: "ghcr/g:1.2.1", APIVersion: intp(1)}})
	m, notes := newManager(t, w, feed)
	ctx := context.Background()
	st, err := m.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pc, gc := st.Components[Panel], st.Components[Gateway]
	if pc.Error != "" || len(pc.Available) != 2 || pc.Latest.Version != "0.7.0" || pc.Installed.Image != "ghcr/p:0.6.0" || pc.Current == nil {
		t.Fatalf("panel %+v", pc)
	}
	if len(gc.Available) != 1 || gc.Installed.Version != "1.2.1" {
		t.Fatalf("gateway %+v", gc)
	}
	if pc.Auto != nil {
		t.Error("notify policy must not pick automatic updates")
	}
	// panel 0.7.0 needs gateway 1.3.0: completed automatically
	if _, err := m.Plan(ctx, PlanRequest{Panel: "0.7.0"}, false); err == nil {
		t.Fatal("plan without completion should fail")
	}
	plan, err := m.Plan(ctx, PlanRequest{Panel: "0.7.0"}, true)
	if err != nil || len(plan.Steps) != 2 || plan.Steps[0].Component != Gateway {
		t.Fatalf("%+v %v", plan, err)
	}
	run, err := m.Start(ctx, plan, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(ctx, plan, "manual", nil, nil); err != ErrBusy {
		t.Errorf("second start: %v", err)
	}
	job := w.c.obj("Job", JobName(run.ID))
	if job == nil {
		t.Fatal("no job")
	}
	c := kube.Slice(job, "spec", "template", "spec", "containers")[0]
	if kube.Str(c, "image") != "ghcr/p:0.7.0" {
		t.Errorf("the new panel image runs the update, got %s", kube.Str(c, "image"))
	}
	var spec RunSpec
	for _, e := range kube.Slice(c, "env") {
		if kube.Str(e, "name") == "NECTAR_UPDATE_RUN" {
			json.Unmarshal([]byte(kube.Str(e, "value")), &spec)
		}
	}
	if spec.ID != run.ID || len(spec.Steps) != 2 {
		t.Fatalf("spec %+v", spec)
	}
	// the Job runs (in-process here) and the manager picks up its result
	w.badCanary.Store("ghcr/g:1.3.0")
	r := w.runner(spec.Steps...)
	r.Spec = spec
	r.Spec.PanelURL, r.Spec.GatewayAdminURL, r.Spec.HealthTimeout = w.srv.URL+"/panel", w.srv.URL+"/gw", 1
	final := r.Run(ctx)
	if final.State != RunRolledBack {
		t.Fatalf("run %s %s", final.State, final.Error)
	}
	m.Wait()
	got, _ := m.Store.GetUpdateRun(ctx, run.ID)
	if got.State != RunRolledBack || got.FinishedAt == nil {
		t.Fatalf("run %+v", got)
	}
	if bad := m.Bad(ctx); bad["gateway@1.3.0"] == "" {
		t.Errorf("bad %v", bad)
	}
	if len(*notes) != 1 || !strings.Contains((*notes)[0], "rolled back") {
		t.Errorf("notes %v", *notes)
	}
}

func TestManagerAutoAndRollback(t *testing.T) {
	w := newWorld(t)
	feed := feedServer(t, []Release{{Version: "0.6.1", Image: "ghcr/p:0.6.1", Migrations: true}, {Version: "0.6.0", Image: "ghcr/p:0.6.0"}}, nil)
	m, _ := newManager(t, w, feed)
	ctx := context.Background()
	set, _ := m.Settings(ctx)
	set.Panel.Policy = PolicyPatch
	set.Window = Window{Start: "03:00", End: "04:00"}
	m.SaveSettings(ctx, set)
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	m.Now = func() time.Time { return at }
	m.Tick(ctx)
	st := m.State(ctx)
	if st.Components[Panel].Auto == nil || st.Components[Panel].Auto.Version != "0.6.1" {
		t.Fatalf("auto %+v", st.Components[Panel])
	}
	if runs, _ := m.Store.ListUpdateRuns(ctx, 10); len(runs) != 0 {
		t.Fatal("started outside the window")
	}
	if !strings.Contains(m.State(ctx).Components[Panel].Waiting, "maintenance window") {
		t.Errorf("waiting %q", m.State(ctx).Components[Panel].Waiting)
	}
	at = time.Date(2026, 10, 29, 3, 15, 0, 0, time.UTC)
	m.Tick(ctx)
	runs, _ := m.Store.ListUpdateRuns(ctx, 10)
	if len(runs) != 1 || runs[0].Trigger != "auto" {
		t.Fatalf("runs %+v", runs)
	}
	// run it
	var spec RunSpec
	c := kube.Slice(w.c.obj("Job", JobName(runs[0].ID)), "spec", "template", "spec", "containers")[0]
	json.Unmarshal([]byte(kube.Str(kube.Slice(c, "env")[0], "value")), &spec)
	r := w.runner()
	r.Spec, r.Spec.PanelURL, r.Spec.GatewayAdminURL = spec, w.srv.URL+"/panel", w.srv.URL+"/gw"
	if st := r.Run(ctx); st.State != RunSucceeded {
		t.Fatalf("%s %s", st.State, st.Error)
	}
	m.Wait()
	// roll back: the panel had migrations → its database is restored from the pre-update snapshot
	rb, err := m.Rollback(ctx, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	c = kube.Slice(w.c.obj("Job", JobName(rb.ID)), "spec", "template", "spec", "containers")[0]
	json.Unmarshal([]byte(kube.Str(kube.Slice(c, "env")[0], "value")), &spec)
	if spec.Trigger != "rollback" || spec.Steps[0].ToImage != "ghcr/p:0.6.0" || spec.Restore[Panel] == "" || kube.Str(c, "image") != "ghcr/p:0.6.1" {
		t.Fatalf("rollback spec %+v image %s", spec, kube.Str(c, "image"))
	}
	r = w.runner()
	r.Spec, r.Spec.PanelURL, r.Spec.GatewayAdminURL = spec, w.srv.URL+"/panel", w.srv.URL+"/gw"
	if st := r.Run(ctx); st.State != RunSucceeded || w.c.image("nectar-apps") != "ghcr/p:0.6.0" || !w.c.helperRan("panel-restore") {
		t.Fatalf("rollback %s %s", st.State, st.Error)
	}
	m.Wait()
}

func TestK3sPlans(t *testing.T) {
	set := Defaults
	set.K3s = ComponentSettings{Policy: PolicyAuto, Channel: "v1.31"}
	set.Window = Window{Days: []string{"sat"}, Start: "02:00", End: "04:00", TimeZone: "Europe/Kyiv"}
	ps := k3sPlans(set, "")
	if len(ps) != 2 || kube.Str(ps[0], "spec", "channel") != K3sChannelURL+"v1.31" || kube.Str(ps[0], "spec", "window", "timeZone") != "Europe/Kyiv" {
		t.Fatalf("%+v", ps[0])
	}
	if kube.Str(ps[1], "spec", "prepare", "args") == "" && kube.Field(ps[1], "spec", "prepare") == nil {
		t.Error("agent plan waits for the server plan")
	}
	ps = k3sPlans(set, "v1.31.5+k3s1")
	if kube.Str(ps[0], "spec", "version") != "v1.31.5+k3s1" || kube.Field(ps[0], "spec", "window") != nil {
		t.Errorf("%+v", ps[0])
	}
	if !k3sLess("v1.31.4+k3s1", "v1.31.4+k3s2") || !k3sLess("v1.30.9+k3s1", "v1.31.0+k3s1") || k3sLess("v1.31.4+k3s2", "v1.31.4+k3s1") {
		t.Error("k3sLess")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://github.com/k3s-io/k3s/releases/tag/v1.31.5%2Bk3s1", http.StatusFound)
	}))
	defer srv.Close()
	f := &Fetcher{HTTP: srv.Client()}
	// point the channel URL at the test server through a transport rewrite
	f.HTTP.Transport = rewrite{srv.URL}
	v, err := f.K3sLatest(context.Background(), "stable")
	if err != nil || v != "v1.31.5+k3s1" {
		t.Errorf("latest %q %v", v, err)
	}
}

type rewrite struct{ base string }

func (rw rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme, u.Host = "http", strings.TrimPrefix(rw.base, "http://")
	r2 := r.Clone(r.Context())
	r2.URL = &u
	r2.Host = u.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func TestFetcherGitHubFallback(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.json":
			rw.WriteHeader(404)
		case "/repos/minelifes/nectar_apps/releases":
			json.NewEncoder(rw).Encode([]map[string]any{
				{"tag_name": "v0.7.0", "body": "notes", "assets": []any{map[string]any{"name": "release.json", "browser_download_url": srv.URL + "/a/0.7.0"}}},
				{"tag_name": "v0.8.0-beta.1", "prerelease": true, "assets": []any{map[string]any{"name": "release.json", "browser_download_url": srv.URL + "/a/0.8.0-beta.1"}}},
			})
		case "/a/0.7.0":
			json.NewEncoder(rw).Encode(Release{Component: Panel, Version: "0.7.0", Image: "p@sha256:7"})
		case "/a/0.8.0-beta.1":
			json.NewEncoder(rw).Encode(Release{Component: Panel, Version: "0.8.0-beta.1", Image: "p@sha256:8"})
		}
	}))
	defer srv.Close()
	f := &Fetcher{GitHubAPI: srv.URL}
	list, src, err := f.Releases(context.Background(), Panel, Stable, Source{FeedURL: srv.URL + "/feed.json", Repo: "minelifes/nectar_apps"})
	if err != nil || src != "github" || len(list) != 1 || list[0].Notes != "notes" {
		t.Fatalf("%+v %s %v", list, src, err)
	}
	list, _, _ = f.Releases(context.Background(), Panel, Beta, Source{Repo: "minelifes/nectar_apps"})
	if len(list) != 2 || list[0].Version != "0.8.0-beta.1" {
		t.Errorf("beta %+v", list)
	}
}
