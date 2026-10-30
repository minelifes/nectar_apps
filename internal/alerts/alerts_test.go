package alerts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

type fakeKube struct {
	mu    sync.Mutex
	nodes []kube.Node
	deps  []kube.Object
	pods  []kube.Object
	err   error
}

func (f *fakeKube) ListNodes(context.Context) ([]kube.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nodes, f.err
}

func (f *fakeKube) List(_ context.Context, kind, ns, _ string) ([]kube.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if ns != "prj-shop" {
		return nil, nil
	}
	if kind == "Deployment" {
		return f.deps, nil
	}
	return f.pods, nil
}

type fakeGateway struct {
	down   bool
	routes []gateway.Route
}

func (g *fakeGateway) Version(context.Context) (gateway.Version, error) {
	if g.down {
		return gateway.Version{}, errors.New("connection refused")
	}
	return gateway.Version{Version: "1.0.0"}, nil
}

func (g *fakeGateway) ListManaged(context.Context) ([]gateway.Route, error) { return g.routes, nil }

func obj(js string) kube.Object {
	var o kube.Object
	if err := json.Unmarshal([]byte(js), &o); err != nil {
		panic(err)
	}
	return o
}

type ntfyRecorder struct {
	mu   sync.Mutex
	msgs []string // "title|priority|body"
}

func (r *ntfyRecorder) handler(w http.ResponseWriter, req *http.Request) {
	b, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.msgs = append(r.msgs, req.Header.Get("Title")+"|"+req.Header.Get("Priority")+"|"+string(b))
	r.mu.Unlock()
}

func (r *ntfyRecorder) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.msgs
	r.msgs = nil
	return m
}

func setup(t *testing.T) (*Engine, *store.Store, *fakeKube, *fakeGateway, *ntfyRecorder, *time.Time, store.App) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "a.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, _ := secretbox.New([]byte("0123456789abcdef0123456789abcdef"), "alerts")
	rec := &ntfyRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	t.Cleanup(srv.Close)
	sealed, err := SealConfig(box, ChannelConfig{URL: srv.URL, Topic: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAlertChannel(ctx, store.AlertChannel{Kind: Ntfy, Name: "phone", Config: sealed, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image", Status: "running"})
	if p.Namespace != "prj-shop" {
		t.Fatalf("namespace %q", p.Namespace)
	}
	k := &fakeKube{nodes: []kube.Node{{Name: "n1", Ready: true, Allocatable: kube.Resources{CPUMillicores: 2000, MemoryBytes: 4 << 30},
		Usage: &kube.Resources{CPUMillicores: 500, MemoryBytes: 1 << 30}}}}
	gw := &fakeGateway{}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	e := &Engine{Store: st, Kube: k, Gateway: gw, Box: box, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
	return e, st, k, gw, rec, &now, app
}

func TestAppDownFiresAfterDurationAndResolves(t *testing.T) {
	ctx := context.Background()
	e, st, k, _, rec, now, app := setup(t)
	k.deps = []kube.Object{obj(`{"metadata":{"name":"api","labels":{"nectar.dev/app":"api"}},"spec":{"replicas":2},"status":{"readyReplicas":0}}`)}

	if err := e.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if f, _ := st.FiringAlerts(ctx); len(f) != 0 || len(rec.take()) != 0 {
		t.Fatal("app_down must wait for its duration (120 s)")
	}
	*now = now.Add(2 * time.Minute)
	if err := e.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	f, _ := st.FiringAlerts(ctx)
	if len(f) != 1 || f[0].Rule != AppDown || f[0].AppID == nil || *f[0].AppID != app.ID || f[0].Notified != 1 {
		t.Fatalf("firing: %+v", f)
	}
	msgs := rec.take()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "App is down · shop/api|urgent|API has 0 of 2 replicas ready.") {
		t.Fatalf("notification: %q", msgs)
	}
	// still down: no duplicate
	*now = now.Add(time.Minute)
	_ = e.Evaluate(ctx)
	if len(rec.take()) != 0 {
		t.Fatal("duplicate notification")
	}
	// Kubernetes unreachable: the incident is neither resolved nor duplicated
	k.err = errors.New("boom")
	_ = e.Evaluate(ctx)
	if f, _ := st.FiringAlerts(ctx); len(f) != 1 {
		t.Fatal("a failed check must not resolve incidents")
	}
	k.err = nil
	k.deps[0]["status"] = map[string]any{"readyReplicas": float64(2)}
	*now = now.Add(time.Minute)
	_ = e.Evaluate(ctx)
	if f, _ := st.FiringAlerts(ctx); len(f) != 0 {
		t.Fatal("not resolved")
	}
	msgs = rec.take()
	if len(msgs) != 1 || !strings.HasPrefix(msgs[0], "Resolved: App is down · shop/api|default|Back to normal after 2m") {
		t.Fatalf("resolve notification: %q", msgs)
	}
	all, _ := st.ListAlerts(ctx, "", nil, 10, 0)
	if len(all) != 1 || all[0].State != store.AlertResolved || all[0].ResolvedAt == nil {
		t.Fatalf("history: %+v", all)
	}
}

func TestCrashLoopOOMNodesCertsGateway(t *testing.T) {
	ctx := context.Background()
	e, st, k, gw, rec, now, _ := setup(t)
	k.pods = []kube.Object{obj(`{"metadata":{"name":"api-1","labels":{"nectar.dev/app":"api"}},
		"spec":{"containers":[{"name":"app","resources":{"limits":{"memory":"256Mi"}}}]},
		"status":{"containerStatuses":[{"name":"app","restartCount":7,"state":{"waiting":{"reason":"CrashLoopBackOff"}},
			"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-10-28T11:58:00Z"}}}]}}`)}
	k.nodes[0].Usage.CPUMillicores = 1900 // 95 %
	if err := st.AddMetricSamples(ctx, now.Add(-15*time.Second), []store.MetricSample{{Series: "node:n1", DiskUsed: 90, DiskTotal: 100}}); err != nil {
		t.Fatal(err)
	}
	gw.routes = []gateway.Route{
		{Domain: "shop.example.com", ACME: true, CertNotAfter: now.Add(5 * 24 * time.Hour).Unix()},
		{Domain: "fine.example.com", ACME: true, CertNotAfter: now.Add(60 * 24 * time.Hour).Unix()},
	}
	if err := e.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	got := map[string]store.Alert{}
	f, _ := st.FiringAlerts(ctx)
	for _, a := range f {
		got[a.Rule] = a
	}
	for _, r := range []string{AppCrashLoop, AppOOM, NodeDisk, CertExpiry} {
		if _, ok := got[r]; !ok {
			t.Errorf("%s did not fire: %+v", r, f)
		}
	}
	if _, ok := got[NodeCPU]; ok {
		t.Error("node_cpu must wait 10 minutes")
	}
	if m := got[AppCrashLoop].Message; !strings.Contains(m, "api-1 restarted 7 times (last exit: OOMKilled, code 137)") {
		t.Errorf("crashloop message %q", m)
	}
	if m := got[AppOOM].Message; !strings.Contains(m, "limit 256 MiB") {
		t.Errorf("oom message %q", m)
	}
	if a := got[CertExpiry]; a.Subject != "shop.example.com" || !strings.Contains(a.Message, "expires in 5 days") {
		t.Errorf("cert %+v", a)
	}
	if n := len(rec.take()); n != 4 {
		t.Errorf("notifications: %d", n)
	}

	*now = now.Add(15 * time.Minute)
	_ = st.AddMetricSamples(ctx, now.Add(-15*time.Second), []store.MetricSample{{Series: "node:n1", DiskUsed: 90, DiskTotal: 100}})
	_ = e.Evaluate(ctx)
	f, _ = st.FiringAlerts(ctx)
	rules := map[string]bool{}
	for _, a := range f {
		rules[a.Rule] = true
	}
	if !rules[NodeCPU] {
		t.Error("node_cpu did not fire after 15 minutes")
	}
	if rules[AppOOM] {
		t.Error("an OOM kill older than 15 minutes must resolve")
	}
	rec.take()

	// gateway down: fires after 2 minutes; certificate incidents stay open (unknown while the gateway is down)
	gw.down = true
	_ = e.Evaluate(ctx)
	*now = now.Add(2 * time.Minute)
	_ = st.AddMetricSamples(ctx, now.Add(-15*time.Second), []store.MetricSample{{Series: "node:n1", DiskUsed: 90, DiskTotal: 100}})
	_ = e.Evaluate(ctx)
	f, _ = st.FiringAlerts(ctx)
	rules = map[string]bool{}
	for _, a := range f {
		rules[a.Rule] = true
	}
	if !rules[GatewayDown] || !rules[CertExpiry] {
		t.Errorf("gateway down / cert kept: %v", rules)
	}

	// disabling a rule closes its incidents without a notification
	rec.take()
	set, _ := LoadSettings(ctx, st)
	for i := range set.Rules {
		if set.Rules[i].Kind == GatewayDown {
			set.Rules[i].Enabled = false
		}
	}
	_ = st.SetSetting(ctx, SettingsKey, set)
	_ = e.Evaluate(ctx)
	for _, a := range must(st.FiringAlerts(ctx)) {
		if a.Rule == GatewayDown {
			t.Error("disabled rule still firing")
		}
	}
	for _, m := range rec.take() {
		if strings.Contains(m, "Gateway") {
			t.Errorf("notified about a disabled rule: %q", m)
		}
	}
}

func must[T any](v T, _ error) T { return v }

func TestSettingsNormalizeValidate(t *testing.T) {
	s := Settings{Rules: []Rule{{Kind: NodeCPU, Enabled: false, Threshold: 75, For: 60}, {Kind: "nope"}}}.Normalize()
	if len(s.Rules) != len(Catalog) {
		t.Fatalf("rules: %d", len(s.Rules))
	}
	if r := s.Rule(NodeCPU); r.Enabled || r.Threshold != 75 || r.For != 60 {
		t.Errorf("kept: %+v", r)
	}
	if r := s.Rule(NodeDisk); !r.Enabled || r.Threshold != 85 {
		t.Errorf("default: %+v", r)
	}
	if err := (Settings{Rules: []Rule{{Kind: NodeCPU, Threshold: 150}}}).Validate(); err == nil {
		t.Error("150 % accepted")
	}
	if err := (Settings{Rules: []Rule{{Kind: "x"}}}).Validate(); err == nil {
		t.Error("unknown rule accepted")
	}
}

func TestChannelConfig(t *testing.T) {
	c := ChannelConfig{Topic: "ops"}
	if err := c.Validate(Ntfy); err != nil || c.URL != "https://ntfy.sh" {
		t.Fatalf("ntfy default: %v %q", err, c.URL)
	}
	e := ChannelConfig{Host: "smtp.example.com", From: "Nectar <alerts@example.com>", To: "a@example.com, b@example.com"}
	if err := e.Validate(Email); err != nil || e.Port != 587 || e.Security != "starttls" {
		t.Fatalf("email defaults: %v %+v", err, e)
	}
	if err := (&ChannelConfig{Host: "h", From: "x@y.z", To: "not an address"}).Validate(Email); err == nil {
		t.Error("bad recipient accepted")
	}
	if err := (&ChannelConfig{BotToken: "nope", ChatID: "1"}).Validate(Telegram); err == nil {
		t.Error("bad bot token accepted")
	}
	m := ChannelConfig{BotToken: "1:abc", ChatID: "5", Password: ""}.Mask()
	if m.BotToken != Masked || m.Password != "" {
		t.Errorf("mask %+v", m)
	}
	kept := m.KeepSecrets(ChannelConfig{BotToken: "1:abc"})
	if kept.BotToken != "1:abc" {
		t.Errorf("keep %+v", kept)
	}
}

func TestTelegramAndWebhook(t *testing.T) {
	var got []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.URL.Path+" "+r.Header.Get("X-Nectar-Signature")+" "+string(b))
		mu.Unlock()
		if strings.Contains(r.URL.Path, "bad") {
			w.WriteHeader(401)
			io.WriteString(w, `{"ok":false,"description":"Unauthorized"}`)
		}
	}))
	defer srv.Close()
	s := &Sender{TelegramAPI: srv.URL}
	n := Notification{Title: "App is down · a/b", Message: "x < y", Severity: Critical, State: "firing", Link: "https://p/apps/1"}
	if err := s.Send(context.Background(), Telegram, ChannelConfig{BotToken: "1:abc", ChatID: "42"}, n); err != nil {
		t.Fatal(err)
	}
	err := s.Send(context.Background(), Telegram, ChannelConfig{BotToken: "1:bad", ChatID: "42"}, n)
	if err == nil || strings.Contains(err.Error(), "1:bad") || !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("telegram error %v", err)
	}
	if err := s.Send(context.Background(), Webhook, ChannelConfig{URL: srv.URL + "/hook", Secret: "s"}, n); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got[0], "/bot1:abc/sendMessage") || !strings.Contains(got[0], `x \u0026lt; y`) {
		t.Errorf("telegram request %q", got[0])
	}
	if !strings.Contains(got[2], "/hook sha256=") || !strings.Contains(got[2], `"state":"firing"`) {
		t.Errorf("webhook request %q", got[2])
	}
}

// TestEmail talks to a minimal SMTP server without TLS.
func TestEmail(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	data := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { io.WriteString(c, s+"\r\n") }
		say("220 test ESMTP")
		var body strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					data <- body.String()
					say("250 queued")
					continue
				}
				body.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-test\r\n250 8BITMIME")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				say("354 go")
			case strings.HasPrefix(cmd, "QUIT"):
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	c := ChannelConfig{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Security: "none", From: "alerts@example.com", To: "ops@example.com"}
	if err := c.Validate(Email); err != nil {
		t.Fatal(err)
	}
	s := &Sender{}
	err = s.Send(context.Background(), Email, c, Notification{Title: "Disk almost full · n1 — 92 %", Message: "line 1\nline 2", State: "firing", Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	msg := <-data
	if !strings.Contains(msg, "Subject: =?utf-8?b?") || !strings.Contains(msg, "line 1\r\nline 2") || !strings.Contains(msg, "To: <ops@example.com>") {
		t.Errorf("message:\n%s", msg)
	}
}

func TestEventRulesDefaults(t *testing.T) {
	s := Settings{}.Normalize()
	if !s.Rule(DeployFailed).Enabled || s.Rule(DeployOK).Enabled {
		t.Errorf("deploy_failed on, deploy_succeeded off by default: %+v %+v", s.Rule(DeployFailed), s.Rule(DeployOK))
	}
	if d, _ := Def(DeployOK); !d.Event {
		t.Error("deploy_succeeded is an event rule")
	}
}

func TestWildcardCertificateConditions(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	certs := []gateway.Certificate{
		{Domain: "*.apps.example.com", ACME: true, NotAfter: now.Add(2 * 24 * time.Hour).Unix(), LastError: "cloudflare: Authentication error (10000)", Serves: []string{"a", "b"}},
		{Domain: "*.ok.example.com", ACME: true, NotAfter: now.Add(60 * 24 * time.Hour).Unix()},
		{Domain: "*.new.example.com", ACME: true, LastError: "no zone", LastAttempt: now.Add(-time.Hour).Unix()},
		{Domain: "*.old-failure.example.com", ACME: true, LastError: "x", LastAttempt: now.Add(-72 * time.Hour).Unix()},
		{Domain: "*.pending.example.com", ACME: true},
	}
	out := wildcardConds(certs, 14, now)
	if len(out) != 2 {
		t.Fatalf("conditions %+v", out)
	}
	if out[0].Severity != Critical || !strings.Contains(out[0].Message, "serves 2 host(s)") || !strings.Contains(out[0].Message, "Authentication error") {
		t.Errorf("expiring: %+v", out[0])
	}
	if out[1].Subject != "*.new.example.com" || !strings.Contains(out[1].Message, "could not be issued: no zone") {
		t.Errorf("failed order: %+v", out[1])
	}
}

func TestVolumeConds(t *testing.T) {
	cs := volumeConds([]VolumeUsage{
		{Key: "a", Subject: "shop/web: data", UsedB: 950 << 20, SizeB: 1 << 30},
		{Key: "b", Subject: "shop/web: logs", UsedB: 100 << 20, SizeB: 1 << 30},
		{Key: "c", Subject: "shop/db (database)", UsedB: -1, SizeB: 1 << 30},
	}, 90)
	if len(cs) != 1 || cs[0].Key != VolumeFull+":a" || !strings.Contains(cs[0].Message, "93%") {
		t.Fatalf("conds: %+v", cs)
	}
}
