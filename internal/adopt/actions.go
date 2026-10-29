package adopt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
)

// Shadow ports used by the gateway until the cutover.
const (
	ShadowHTTPPort  = 18080
	ShadowHTTPSPort = 18443
)

// ImportedOwner marks gateway routes created from host proxy configs (they are normal, editable routes).
const ImportedOwner = "adopt-import"

// Env holds what the adopt commands work with.
type Env struct {
	Kube            Kube
	Host            Host
	Gateway         *gateway.Client // nil until the gateway is installed
	SystemNamespace string
	StateDir        string // /var/lib/nectar
	ProbeIP         string // address the probes use for both proxies (default: this node's InternalIP)
	Prober          Prober
	Logf            func(format string, args ...any)
	Now             func() time.Time
	Sleep           func(time.Duration)
	Timeout         time.Duration // waits for rollouts / ServiceLB (default 4 minutes)
	SQLDriver       string        // to copy the k3s sqlite datastore (VACUUM INTO)
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

func (e *Env) logf(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

func (e *Env) sleep(d time.Duration) {
	if e.Sleep != nil {
		e.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (e *Env) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return 4 * time.Minute
}

func (e *Env) probe() Prober {
	if e.Prober != nil {
		return e.Prober
	}
	return DefaultProber
}

// waitFor polls cond until it returns true, an error, or the timeout passes.
func (e *Env) waitFor(ctx context.Context, what string, cond func() (bool, error)) error {
	deadline := e.now().Add(e.timeout())
	for {
		ok, err := cond()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if e.now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", what)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.sleep(2 * time.Second)
	}
}

// =================================================================== backup ==

// Backup saves everything adopt mode may touch: the k3s datastore, the routing-related objects, host
// proxy configuration directories and k3s config. Nothing is changed.
func (e *Env) Backup(ctx context.Context, rep *Report, st *State) error {
	st.restart()
	dir := filepath.Join(e.StateDir, "backup", e.now().Format("20060102-150405"))
	if err := os.MkdirAll(e.Host.p(dir), 0o700); err != nil {
		return err
	}
	manifest := map[string]any{"created_at": e.now(), "k3s": rep.K3s, "edge": rep.Edge}
	var notes []string

	// datastore
	switch rep.K3s.Datastore {
	case "sqlite":
		src := "/var/lib/rancher/k3s/server/db/state.db"
		dst := filepath.Join(dir, "k3s-state.db")
		if err := e.copySQLite(ctx, src, dst); err != nil {
			return fmt.Errorf("back up the k3s datastore: %w", err)
		}
		notes = append(notes, "k3s sqlite datastore → k3s-state.db (restore: stop k3s, copy it to "+src+", remove state.db-wal/-shm, start k3s)")
	case "etcd":
		name := "nectar-adopt-" + e.now().Format("20060102-150405")
		if out, err := e.Host.Run(ctx, "k3s", "etcd-snapshot", "save", "--name", name); err != nil {
			return fmt.Errorf("k3s etcd-snapshot save: %v: %s", err, out)
		}
		notes = append(notes, "etcd snapshot "+name+" in /var/lib/rancher/k3s/server/db/snapshots (restore: k3s server --cluster-reset --cluster-reset-restore-path=<file>)")
	default:
		notes = append(notes, "external datastore: back it up with your database tools — adopt mode cannot")
	}

	// objects
	objDir := filepath.Join(dir, "objects")
	for _, kind := range []string{"Ingress", "IngressClass", "IngressRoute", "IngressRoute.containo.us", "Middleware",
		"Middleware.containo.us", "IngressRouteTCP", "IngressRouteUDP", "HTTPRoute", "Gateway", "GatewayClass", "Certificate",
		"ClusterIssuer", "Service", "HelmChart", "HelmChartConfig", "Deployment", "DaemonSet"} {
		items, err := list(ctx, e.Kube, kind)
		if err != nil {
			notes = append(notes, fmt.Sprintf("could not list %s: %v", kind, err))
			continue
		}
		if len(items) == 0 {
			continue
		}
		b, _ := json.MarshalIndent(items, "", " ")
		if err := e.Host.WriteFile(filepath.Join(objDir, kind+".json"), b); err != nil {
			return err
		}
	}
	// TLS / auth secrets referenced by routes (contain private keys: the backup dir is root-only)
	if cl, err := LoadCluster(ctx, e.Kube); err == nil && len(cl.Secrets) > 0 {
		b, _ := json.MarshalIndent(cl.Secrets, "", " ")
		if err := e.Host.WriteFile(filepath.Join(objDir, "Secret.json"), b); err != nil {
			return err
		}
	}

	// files: host proxy configs, a standalone gateway DB, k3s config
	var paths []string
	for _, hp := range rep.HostProxies {
		switch hp.Kind {
		case "nectar_gateway":
			paths = append(paths, hp.Config)
			if hp.Unit != "" {
				paths = append(paths, "/etc/systemd/system/"+hp.Unit)
			}
		default:
			if hp.Config != "" {
				paths = append(paths, filepath.Dir(hp.Config))
			}
		}
	}
	paths = append(paths, "/etc/rancher/k3s/config.yaml", "/etc/rancher/k3s/config.yaml.d")
	for _, p := range paths {
		if !e.Host.Exists(p) {
			continue
		}
		if err := e.copyTree(p, filepath.Join(dir, "files", p)); err != nil {
			return fmt.Errorf("back up %s: %w", p, err)
		}
	}
	manifest["notes"] = notes
	manifest["files"] = paths
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err := e.Host.WriteFile(filepath.Join(dir, "manifest.json"), b); err != nil {
		return err
	}
	st.BackupDir = dir
	if st.Phase == PhaseDiscovered || st.Phase == "" || st.Phase == PhaseRolledBack {
		st.Phase = PhaseBackedUp
	}
	st.log(e, "backup written to %s", dir)
	for _, n := range notes {
		e.logf("  %s", n)
	}
	return nil
}

// copySQLite makes a consistent copy of a live SQLite database (VACUUM INTO), falling back to a file copy.
func (e *Env) copySQLite(ctx context.Context, src, dst string) error {
	if !e.Host.Exists(src) {
		return fmt.Errorf("%s not found", src)
	}
	if e.SQLDriver != "" {
		db, err := sql.Open(e.SQLDriver, "file:"+e.Host.p(src)+"?mode=ro")
		if err == nil {
			_, err = db.ExecContext(ctx, `VACUUM INTO ?`, e.Host.p(dst))
			db.Close()
			if err == nil {
				return nil
			}
		}
		e.logf("  VACUUM INTO failed (%v), copying the files instead", err)
	}
	for _, suf := range []string{"", "-wal", "-shm"} {
		if e.Host.Exists(src + suf) {
			b, err := e.Host.ReadFile(src + suf)
			if err != nil {
				return err
			}
			if err := e.Host.WriteFile(dst+suf, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Env) copyTree(src, dst string) error {
	return filepath.Walk(e.Host.p(src), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(e.Host.p(src), path)
		target := filepath.Join(e.Host.p(dst), rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return nil // sockets, symlinks (sites-enabled links are re-read from sites-available)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
}

// ============================================================ gateway access ==

// ConnectGateway builds the gateway client from the installed manifests (token Secret + admin port).
func (e *Env) ConnectGateway(ctx context.Context) error {
	var sec kube.Object
	if err := e.Kube.GetObject(ctx, "Secret", e.SystemNamespace, "nectar-secrets", &sec); err != nil {
		return fmt.Errorf("Nectar is not installed yet (secret %s/nectar-secrets: %w)", e.SystemNamespace, err)
	}
	tok, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "gateway-api-token"))
	port := 8081
	if dep, err := e.gatewayDeployment(ctx); err == nil {
		if a := containerEnv(dep, "ADMIN_ADDR"); a != "" {
			if i := strings.LastIndex(a, ":"); i >= 0 {
				if n, err := strconv.Atoi(a[i+1:]); err == nil {
					port = n
				}
			}
		}
	}
	ip := e.ProbeIP
	if ip == "" {
		ip = "127.0.0.1"
	}
	e.Gateway = gateway.New(fmt.Sprintf("http://%s:%d", ip, port), string(tok))
	return e.waitFor(ctx, "the gateway admin API", func() (bool, error) {
		_, err := e.Gateway.Version(ctx)
		return err == nil, nil
	})
}

func (e *Env) gatewayDeployment(ctx context.Context) (kube.Object, error) {
	var dep kube.Object
	err := e.Kube.GetObject(ctx, "Deployment", e.SystemNamespace, "nectar-gateway", &dep)
	return dep, err
}

func containerEnv(dep kube.Object, name string) string {
	for _, c := range kube.Slice(dep, "spec", "template", "spec", "containers") {
		for _, ev := range kube.Slice(c, "env") {
			if kube.Str(ev, "name") == name {
				return kube.Str(ev, "value")
			}
		}
	}
	return ""
}

// setGatewayPorts changes PROXY_ADDR / HTTPS_ADDR of the gateway Deployment and waits for the rollout.
func (e *Env) setGatewayPorts(ctx context.Context, httpAddr, httpsAddr string) error {
	dep, err := e.gatewayDeployment(ctx)
	if err != nil {
		return fmt.Errorf("gateway deployment: %w", err)
	}
	ctrs := kube.Slice(dep, "spec", "template", "spec", "containers")
	changed := false
	for _, c := range ctrs {
		if kube.Str(c, "name") != "gateway" {
			continue
		}
		envs := kube.Slice(c, "env")
		set := func(k, v string) {
			for _, ev := range envs {
				if kube.Str(ev, "name") == k {
					if kube.Str(ev, "value") != v {
						ev["value"], changed = v, true
					}
					return
				}
			}
			envs = append(envs, kube.Object{"name": k, "value": v})
			changed = true
		}
		set("PROXY_ADDR", httpAddr)
		set("HTTPS_ADDR", httpsAddr)
		arr := make([]any, len(envs))
		for i := range envs {
			arr[i] = envs[i]
		}
		c["env"] = arr
	}
	if !changed {
		return nil
	}
	arr := make([]any, len(ctrs))
	for i := range ctrs {
		arr[i] = ctrs[i]
	}
	if _, err := e.Kube.Patch(ctx, "Deployment", e.SystemNamespace, "nectar-gateway",
		map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": arr}}}}); err != nil {
		return err
	}
	return e.waitRollout(ctx, "Deployment", e.SystemNamespace, "nectar-gateway")
}

func (e *Env) waitRollout(ctx context.Context, kind, ns, name string) error {
	return e.waitFor(ctx, kind+" "+ns+"/"+name+" rollout", func() (bool, error) {
		var d kube.Object
		if err := e.Kube.GetObject(ctx, kind, ns, name, &d); err != nil {
			return false, err
		}
		want := kube.Int(d, "spec", "replicas")
		if want == 0 {
			want = 1
		}
		return kube.Int(d, "status", "observedGeneration") >= kube.Int(d, "metadata", "generation") &&
			kube.Int(d, "status", "updatedReplicas") == want && kube.Int(d, "status", "availableReplicas") >= want, nil
	})
}

// =================================================================== shadow ==

// Fallbacks returns where the gateway sends hosts it has no route for: the old edge proxy.
// rebound=true gives the addresses after a host proxy was moved to the internal ports.
func Fallbacks(rep *Report, rebound bool) (httpAddr, tlsAddr string) {
	if c := rep.EdgeController(); c != nil && c.Service != "" {
		ns, name, _ := strings.Cut(c.Service, "/")
		host := fmt.Sprintf("%s.%s.svc.%s", name, ns, rep.K3s.ClusterDomain)
		return host + ":80", host + ":443"
	}
	if hp := rep.EdgeHostProxy(); hp != nil && hp.Kind != "nectar_gateway" {
		if rebound {
			return fmt.Sprintf("127.0.0.1:%d", RebindHTTPPort), fmt.Sprintf("127.0.0.1:%d", RebindHTTPSPort)
		}
		return "127.0.0.1:80", "127.0.0.1:443"
	}
	return "", ""
}

// Shadow configures the shadow gateway (fallback to the old proxy) and imports the routes of host proxies
// and of a standalone nectar_gateway. Kubernetes routes (Ingress, IngressRoute, HTTPRoute) are synced by the
// panel's Ingress controller; Shadow waits until they are in the gateway.
func (e *Env) Shadow(ctx context.Context, rep *Report, st *State) error {
	if e.Gateway == nil {
		if err := e.ConnectGateway(ctx); err != nil {
			return err
		}
	}
	fh, ft := Fallbacks(rep, false)
	if fh != "" {
		old, _ := e.Gateway.GetSettings(ctx)
		if old.FallbackHTTP != fh || old.FallbackTLS != ft {
			b, _ := json.Marshal(old)
			st.record(e, Step{Kind: stepGatewaySettings, Target: "gateway", Before: b})
			if _, err := e.Gateway.PutSettings(ctx, gateway.Settings{FallbackHTTP: fh, FallbackTLS: ft}); err != nil {
				return fmt.Errorf("gateway fallback settings: %w", err)
			}
		}
		st.Fallback.HTTP, st.Fallback.TLS = fh, ft
		st.log(e, "gateway fallback → %s (HTTP), %s (TLS passthrough)", fh, ft)
	}

	// host proxy routes: imported once as normal (editable) gateway routes
	existing, err := e.Gateway.ListManagedBy(ctx, "")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, r := range existing {
		pp := r.PathPrefix
		if pp == "" {
			pp = "/"
		}
		have[r.Domain+pp] = true
	}
	imported := 0
	var failed []string
	for _, r := range rep.Routes {
		if !r.Migratable() || isK8sSource(r.Source) || have[r.Key()] {
			continue
		}
		id, err := e.importRoute(ctx, r, st)
		if id != 0 {
			have[r.Key()] = true
			st.ImportedRoutes = append(st.ImportedRoutes, id)
			imported++
		}
		if err != nil {
			st.log(e, "✗ import %s%s (%s %s): %v", r.Host, r.PathPrefix, r.Source, r.Ref, err)
			failed = append(failed, r.Host+r.PathPrefix)
		}
	}
	if imported > 0 {
		b, _ := json.Marshal(st.ImportedRoutes)
		st.record(e, Step{Kind: stepImportedRoutes, Target: "gateway", Before: b})
		st.log(e, "imported %d route(s) from host proxy configuration", imported)
	}
	if len(rep.Blocked) > 0 {
		n := 0
		for _, b := range rep.Blocked {
			ttl := time.Until(b.ExpiresAt)
			if ttl <= 0 {
				continue
			}
			if err := e.Gateway.Block(ctx, b.IP, b.Reason, ttl); err == nil {
				n++
			}
		}
		st.log(e, "imported %d blocked IP(s)", n)
	}

	if len(failed) > 0 {
		b, _ := json.Marshal(st.ImportedRoutes)
		st.record(e, Step{Kind: stepImportedRoutes, Target: "gateway", Before: b})
		return fmt.Errorf("%d route(s) could not be imported (%s); they stay on the old proxy — fix and re-run shadow", len(failed), strings.Join(failed, ", "))
	}

	// Kubernetes routes come from the panel's Ingress sync
	want := map[string]bool{}
	for _, r := range rep.Routes {
		if r.Migratable() && isK8sSource(r.Source) {
			want[r.Key()] = true
		}
	}
	if len(want) > 0 {
		e.logf("waiting for the panel to sync %d Kubernetes route(s) to the gateway…", len(want))
		var missing []string
		err := e.waitFor(ctx, "Kubernetes routes in the gateway", func() (bool, error) {
			rs, err := e.Gateway.ListManagedBy(ctx, IngressSyncManagedBy)
			if err != nil {
				return false, nil
			}
			got := map[string]bool{}
			for _, r := range rs {
				pp := r.PathPrefix
				if pp == "" {
					pp = "/"
				}
				got[r.Domain+pp] = true
			}
			missing = missing[:0]
			for k := range want {
				if !got[k] {
					missing = append(missing, k)
				}
			}
			return len(missing) == 0, nil
		})
		if err != nil {
			sort.Strings(missing)
			return fmt.Errorf("%w; missing: %s (check the panel log: kubectl -n %s logs deploy/nectar-apps)", err, strings.Join(missing, ", "), e.SystemNamespace)
		}
	}
	st.Phase, st.ShadowAt = PhaseShadow, e.now()
	st.log(e, "shadow gateway ready on :%d/:%d", ShadowHTTPPort, ShadowHTTPSPort)
	return nil
}

// importRoute creates a gateway route from a host proxy route. A certificate is uploaded before the HTTPS
// redirect is switched on (the gateway refuses force_ssl without a certificate); if the certificate cannot be
// read, the gateway's own ACME is used instead.
func (e *Env) importRoute(ctx context.Context, r Route, st *State) (int64, error) {
	payload := r.GatewayPayload("", ImportedOwner)
	force := payload["force_ssl"] == true
	payload["force_ssl"] = false
	created, err := e.Gateway.CreateRaw(ctx, payload)
	if err != nil {
		return 0, err
	}
	if r.Cert != nil {
		certPEM, keyPEM, err := e.readCert(r.Cert)
		if err == nil {
			_, err = e.Gateway.UploadCert(ctx, created.ID, certPEM, keyPEM)
		}
		if err != nil {
			st.log(e, "⚠ certificate for %s: %v (the gateway uses ACME instead)", r.Host, err)
			payload["acme"] = r.TLS && !strings.HasPrefix(r.Host, "*.")
		}
	}
	if force {
		payload["force_ssl"] = true
		payload["id"] = created.ID
		if _, err := e.Gateway.Send(ctx, http.MethodPut, fmt.Sprintf("/api/routes/%d", created.ID), payload); err != nil {
			return created.ID, fmt.Errorf("enable the HTTPS redirect: %w", err)
		}
	} else if payload["acme"] == true && r.Cert != nil {
		payload["id"] = created.ID
		_, _ = e.Gateway.Send(ctx, http.MethodPut, fmt.Sprintf("/api/routes/%d", created.ID), payload)
	}
	return created.ID, nil
}

func isK8sSource(s string) bool {
	return s == SourceIngress || s == SourceIngressRoute || s == SourceHTTPRoute
}

// readCert reads a certificate from host files (HAProxy combined PEMs are split) or a DB.
func (e *Env) readCert(c *CertRef) (string, string, error) {
	if c.CertPEM != "" {
		return c.CertPEM, c.KeyPEM, nil
	}
	cb, err := e.Host.ReadFile(c.CertFile)
	if err != nil {
		return "", "", err
	}
	kb := cb
	if c.KeyFile != c.CertFile {
		if kb, err = e.Host.ReadFile(c.KeyFile); err != nil {
			return "", "", err
		}
	}
	return splitPEM(cb, kb)
}

// splitPEM separates certificate blocks from the private key (they may share one file).
func splitPEM(certFile, keyFile []byte) (string, string, error) {
	var certs, key []byte
	scan := func(src []byte, wantCerts bool) {
		rest := src
		for {
			var blk *pem.Block
			blk, rest = pem.Decode(rest)
			if blk == nil {
				return
			}
			switch {
			case blk.Type == "CERTIFICATE" && wantCerts:
				certs = append(certs, pem.EncodeToMemory(blk)...)
			case strings.Contains(blk.Type, "PRIVATE KEY") && key == nil:
				key = pem.EncodeToMemory(blk)
			}
		}
	}
	scan(certFile, true)
	scan(keyFile, false)
	if len(certs) == 0 || key == nil {
		return "", "", errors.New("no certificate or private key in PEM")
	}
	return string(certs), string(key), nil
}

// =================================================================== verify ==

// Verify compares the old proxy (80/443) with the shadow gateway (18080/18443) for every host.
func (e *Env) Verify(ctx context.Context, rep *Report, st *State) *VerifyResult {
	res := &VerifyResult{At: e.now()}
	ip := e.probeIP()
	for _, p := range probeTargets(rep.Routes) {
		oldPort, newPort := 80, ShadowHTTPPort
		if p.Scheme == "https" {
			oldPort, newPort = 443, ShadowHTTPSPort
		}
		p.Old = e.probe()(ctx, p.Scheme, ip, oldPort, p.Host, p.Path)
		p.New = e.probe()(ctx, p.Scheme, ip, newPort, p.Host, p.Path)
		compare(&p)
		res.Probes = append(res.Probes, p)
	}
	summarize(res)
	st.Verify = res
	if st.Phase == PhaseShadow || st.Phase == PhaseVerified {
		st.Phase = PhaseVerified
	}
	st.log(e, "verification: %d ✓ %d ⚠ %d ✗", res.OK, res.Warn, res.Failed)
	return res
}

func (e *Env) probeIP() string {
	if e.ProbeIP != "" {
		return e.ProbeIP
	}
	return "127.0.0.1"
}

// ================================================================== cutover ==

// CutoverOptions tune the cutover.
type CutoverOptions struct {
	Force      bool          // ignore failed verification probes
	GateTries  int           // health-gate attempts before rolling back (default 3)
	GateDelay  time.Duration // between attempts (default 5s)
	NoRollback bool          // keep the new state even if the health gate fails (debugging)
}

// ErrRolledBack is returned when the health gate failed and the cutover was undone.
var ErrRolledBack = errors.New("health check after the cutover failed — everything was rolled back")

// Cutover moves 80/443 to the gateway. For an in-cluster controller behind k3s ServiceLB, the gateway binds
// 80/443 first (ServiceLB's iptables rules keep sending traffic to the old controller), then the controller's
// Service becomes ClusterIP, which hands the traffic to the gateway without a gap. A host proxy is moved to
// 127.0.0.1:10080/10443 first. Afterwards every host is checked; on regression everything is rolled back.
func (e *Env) Cutover(ctx context.Context, rep *Report, st *State, opt CutoverOptions) error {
	if len(rep.Blockers) > 0 && !opt.Force {
		return fmt.Errorf("cutover blocked: %s", strings.Join(rep.Blockers, "; "))
	}
	if st.Phase != PhaseVerified && st.Phase != PhaseShadow && !opt.Force {
		return fmt.Errorf("run the shadow installation and verification first (phase is %q)", st.Phase)
	}
	if st.Verify != nil && st.Verify.Failed > 0 && !opt.Force {
		return fmt.Errorf("%d host(s) answered differently in the shadow verification (see the report); fix them or use --force", st.Verify.Failed)
	}
	if err := e.checkEdgeUnchanged(ctx, rep); err != nil && !opt.Force {
		return err
	}
	if e.Gateway == nil {
		if err := e.ConnectGateway(ctx); err != nil {
			return err
		}
	}
	// fresh baseline of the old proxy right before the switch
	baseline := map[string]Response{}
	ip := e.probeIP()
	for _, p := range probeTargets(rep.Routes) {
		port := 80
		if p.Scheme == "https" {
			port = 443
		}
		baseline[p.Scheme+"://"+p.Host+p.Path] = e.probe()(ctx, p.Scheme, ip, port, p.Host, p.Path)
	}

	err := e.cutoverSteps(ctx, rep, st)
	if err == nil {
		err = e.healthGate(ctx, rep, st, baseline, opt)
	}
	if err != nil {
		if opt.NoRollback {
			st.log(e, "✗ cutover problem (no rollback requested): %v", err)
			return err
		}
		st.log(e, "✗ %v — rolling back", err)
		if rbErr := e.Rollback(ctx, rep, st, false); rbErr != nil {
			return fmt.Errorf("%v; ROLLBACK FAILED: %v (backup: %s)", err, rbErr, st.BackupDir)
		}
		return fmt.Errorf("%w: %v", ErrRolledBack, err)
	}
	st.Phase, st.CutoverAt = PhaseSoaking, e.now()
	if st.Fallback.HTTP == "" && st.Fallback.TLS == "" {
		st.log(e, "cutover complete: nectar-gateway serves 80/443")
	} else {
		st.log(e, "cutover complete: nectar-gateway serves 80/443, the old proxy is the fallback")
	}
	return nil
}

// checkEdgeUnchanged compares who serves 80/443 now with the report the plan was made from. A cutover based on
// a stale or incomplete report could hand the ports to a gateway that knows none of the hosts.
func (e *Env) checkEdgeUnchanged(ctx context.Context, rep *Report) error {
	var hostHolders []string
	for _, l := range e.Host.Listeners(80, 443) {
		if !l.InPod && !isK3sProcess(l.Process) && l.Process != "nectar-gateway" {
			hostHolders = append(hostHolders, fmt.Sprintf("%s (pid %d) on %d", l.Process, l.PID, l.Port))
		}
	}
	hp := rep.EdgeHostProxy()
	switch {
	case hp == nil && len(hostHolders) > 0:
		return fmt.Errorf("ports 80/443 are now used by %s, which the report does not know: run `nectar-apps adopt discover --refresh` and shadow again", strings.Join(dedupe(hostHolders), ", "))
	case hp != nil && hp.Kind != "nectar_gateway" && len(hostHolders) == 0:
		return fmt.Errorf("the report expects %s on 80/443, but nothing listens there now: run `nectar-apps adopt discover --refresh`", hp.Kind)
	}
	if rep.Edge != "none" && len(probeTargets(rep.Routes)) == 0 {
		return fmt.Errorf("the report lists no hosts for the proxy on 80/443 (%s): nothing could be verified, refusing to switch", rep.Edge)
	}
	return nil
}

func (e *Env) cutoverSteps(ctx context.Context, rep *Report, st *State) error {
	ctrl := rep.EdgeController()
	hp := rep.EdgeHostProxy()
	switch {
	case ctrl != nil:
		// 1. gateway binds 80/443 (ServiceLB DNAT still sends traffic to the controller)
		st.record(e, Step{Kind: stepGatewayPorts, Target: "nectar-gateway", Before: mustJSON(map[string]string{
			"proxy": fmt.Sprintf(":%d", ShadowHTTPPort), "https": fmt.Sprintf(":%d", ShadowHTTPSPort)})})
		st.log(e, "gateway → :80/:443")
		if err := e.setGatewayPorts(ctx, ":80", ":443"); err != nil {
			return err
		}
		// 2. release the controller's ServiceLB ports
		if ctrl.K3sBundled {
			if err := e.traefikClusterIP(ctx, st); err != nil {
				return err
			}
		} else if err := e.serviceClusterIP(ctx, st, ctrl.Service); err != nil {
			return err
		}
		ns, name, _ := strings.Cut(ctrl.Service, "/")
		st.log(e, "waiting for k3s ServiceLB to release 80/443 of %s", ctrl.Service)
		return e.waitFor(ctx, "ServiceLB release of "+ctrl.Service, func() (bool, error) {
			var svc kube.Object
			if err := e.Kube.GetObject(ctx, "Service", ns, name, &svc); err != nil {
				return false, err
			}
			if kube.Str(svc, "spec", "type") != "ClusterIP" {
				return false, nil
			}
			dss, err := e.Kube.List(ctx, "DaemonSet", "", "svccontroller.k3s.cattle.io/svcname="+name)
			if err != nil {
				return false, err
			}
			for _, ds := range dss {
				if sns := kube.Str(ds, "metadata", "labels", "svccontroller.k3s.cattle.io/svcnamespace"); sns == "" || sns == ns {
					return false, nil
				}
			}
			return true, nil
		})
	case hp != nil && hp.Kind == "nectar_gateway":
		if hp.Unit == "" {
			return fmt.Errorf("standalone nectar_gateway (pid %d) does not run as a systemd unit: stop it manually and re-run", hp.PID)
		}
		if out, err := e.Host.Run(ctx, "systemctl", "stop", hp.Unit); err != nil {
			return fmt.Errorf("systemctl stop %s: %v %s", hp.Unit, err, out)
		}
		st.record(e, Step{Kind: stepHostStop, Target: hp.Unit})
		st.log(e, "stopped %s", hp.Unit)
		if err := e.waitPortsFree(ctx); err != nil {
			return err
		}
	case hp != nil:
		if err := e.rebindHostProxy(ctx, rep, st, *hp); err != nil {
			return err
		}
		// the fallback now points at the rebound ports
		fh, ft := Fallbacks(rep, true)
		old, _ := e.Gateway.GetSettings(ctx)
		st.record(e, Step{Kind: stepGatewaySettings, Target: "gateway", Before: mustJSON(old)})
		if _, err := e.Gateway.PutSettings(ctx, gateway.Settings{FallbackHTTP: fh, FallbackTLS: ft}); err != nil {
			return err
		}
		st.Fallback.HTTP, st.Fallback.TLS = fh, ft
	}
	if ctrl == nil {
		st.record(e, Step{Kind: stepGatewayPorts, Target: "nectar-gateway", Before: mustJSON(map[string]string{
			"proxy": fmt.Sprintf(":%d", ShadowHTTPPort), "https": fmt.Sprintf(":%d", ShadowHTTPSPort)})})
		st.log(e, "gateway → :80/:443")
		return e.setGatewayPorts(ctx, ":80", ":443")
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// traefikClusterIP sets the bundled Traefik's Service to ClusterIP through a HelmChartConfig (the supported
// way: k3s would revert a direct Service edit). Existing values are kept.
func (e *Env) traefikClusterIP(ctx context.Context, st *State) error {
	var cur kube.Object
	err := e.Kube.GetObject(ctx, "HelmChartConfig", "kube-system", "traefik", &cur)
	switch {
	case kube.IsNotFound(err):
		st.record(e, Step{Kind: stepHelmChartConfig, Target: "kube-system/traefik", Before: json.RawMessage("null")})
		_, err = e.Kube.Apply(ctx, kube.Object{
			"apiVersion": "helm.cattle.io/v1", "kind": "HelmChartConfig",
			"metadata": map[string]any{"name": "traefik", "namespace": "kube-system", "labels": map[string]any{"app.kubernetes.io/managed-by": "nectar-apps"}},
			"spec":     map[string]any{"valuesContent": "service:\n  type: ClusterIP\n"},
		})
		if err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		values := map[string]any{}
		if vc := kube.Str(cur, "spec", "valuesContent"); vc != "" {
			if err := yaml.Unmarshal([]byte(vc), &values); err != nil {
				return fmt.Errorf("HelmChartConfig kube-system/traefik valuesContent: %w", err)
			}
		}
		svc, _ := values["service"].(map[string]any)
		if svc == nil {
			svc = map[string]any{}
		}
		svc["type"] = "ClusterIP"
		values["service"] = svc
		out, err := yaml.Marshal(values)
		if err != nil {
			return err
		}
		st.record(e, Step{Kind: stepHelmChartConfig, Target: "kube-system/traefik", Before: mustJSON(kube.Str(cur, "spec", "valuesContent"))})
		if _, err := e.Kube.Patch(ctx, "HelmChartConfig", "kube-system", "traefik",
			map[string]any{"spec": map[string]any{"valuesContent": string(out)}}); err != nil {
			return err
		}
	}
	st.log(e, "Traefik Service → ClusterIP (HelmChartConfig kube-system/traefik)")
	return nil
}

// serviceClusterIP switches a controller's LoadBalancer Service to ClusterIP (node ports removed).
func (e *Env) serviceClusterIP(ctx context.Context, st *State, ref string) error {
	ns, name, _ := strings.Cut(ref, "/")
	var svc kube.Object
	if err := e.Kube.GetObject(ctx, "Service", ns, name, &svc); err != nil {
		return err
	}
	spec, _ := svc["spec"].(map[string]any)
	before := map[string]any{}
	for _, k := range []string{"type", "ports", "externalTrafficPolicy", "allocateLoadBalancerNodePorts", "loadBalancerClass",
		"loadBalancerIP", "loadBalancerSourceRanges", "healthCheckNodePort"} {
		if v, ok := spec[k]; ok {
			before[k] = v
		}
	}
	var ports []any
	for _, p := range kube.Slice(svc, "spec", "ports") {
		cp := map[string]any{}
		for k, v := range p {
			if k != "nodePort" {
				cp[k] = v
			}
		}
		ports = append(ports, cp)
	}
	st.record(e, Step{Kind: stepServiceType, Target: ref, Before: mustJSON(before)})
	patch := map[string]any{"spec": map[string]any{"type": "ClusterIP", "ports": ports, "externalTrafficPolicy": nil,
		"allocateLoadBalancerNodePorts": nil, "loadBalancerClass": nil, "loadBalancerIP": nil, "loadBalancerSourceRanges": nil,
		"healthCheckNodePort": nil}}
	if _, err := e.Kube.Patch(ctx, "Service", ns, name, patch); err != nil {
		return err
	}
	st.log(e, "Service %s → ClusterIP", ref)
	if kube.Str(svc, "metadata", "labels", "app.kubernetes.io/managed-by") == "Helm" {
		e.logf("  ⚠ %s is managed by Helm: set its service type to ClusterIP in your values, or the next helm upgrade reverts it", ref)
	}
	return nil
}

// rebindHostProxy moves a host proxy to 127.0.0.1:10080/10443 (config rewrite, validation, reload).
func (e *Env) rebindHostProxy(ctx context.Context, rep *Report, st *State, hp HostProxy) error {
	files := []string{hp.Config}
	switch hp.Kind {
	case "nginx":
		if _, fs, err := Nginx(e.Host, hp.Config); err == nil {
			files = fs
		}
	case "apache":
		if _, fs, err := Apache(e.Host, hp.Config); err == nil {
			files = fs
		}
		for _, p := range []string{"/etc/apache2/ports.conf", "/etc/httpd/conf.d/ssl.conf"} {
			if e.Host.Exists(p) {
				files = append(files, p)
			}
		}
	}
	files = dedupe(files)
	unit := hp.Unit
	if unit == "" {
		unit = map[string]string{"nginx": "nginx.service", "caddy": "caddy.service", "apache": "apache2.service", "haproxy": "haproxy.service"}[hp.Kind]
	}
	// recorded before the edits: rollback undoes in reverse order, so it restores the files and then reloads
	st.record(e, Step{Kind: stepHostRebind, Target: unit})
	changed := 0
	for _, f := range files {
		b, err := e.Host.ReadFile(f)
		if err != nil {
			return err
		}
		out, ok := RebindConfig(hp.Kind, string(b))
		if !ok {
			continue
		}
		backup := filepath.Join(st.BackupDir, "rebind", f)
		if err := e.Host.WriteFile(backup, b); err != nil {
			return err
		}
		if err := e.Host.WriteFile(f, []byte(out)); err != nil {
			return err
		}
		st.record(e, Step{Kind: stepHostConfig, Target: f, Before: mustJSON(backup)})
		changed++
	}
	if changed == 0 {
		return fmt.Errorf("%s: found no listen directive for 80/443 to move", hp.Kind)
	}
	if cmd := validateCmd(hp.Kind, hp.Config); cmd != nil && e.Host.Has(cmd[0]) {
		if out, err := e.Host.Run(ctx, cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("%s configuration check failed after moving the ports: %v\n%s", hp.Kind, err, out)
		}
	}
	if out, err := e.Host.Run(ctx, "systemctl", "reload-or-restart", unit); err != nil {
		return fmt.Errorf("reload %s: %v %s", unit, err, out)
	}
	st.log(e, "%s moved to 127.0.0.1:%d / :%d (%d file(s) changed, reloaded %s)", hp.Kind, RebindHTTPPort, RebindHTTPSPort, changed, unit)
	return e.waitPortsFree(ctx)
}

// waitPortsFree waits until no host process listens on 80/443 any more.
func (e *Env) waitPortsFree(ctx context.Context) error {
	return e.waitFor(ctx, "ports 80/443 to be released", func() (bool, error) {
		for _, l := range e.Host.Listeners(80, 443) {
			if !l.InPod {
				return false, nil
			}
		}
		return true, nil
	})
}

// healthGate re-checks every host through 80/443 and compares with the pre-cutover baseline.
func (e *Env) healthGate(ctx context.Context, rep *Report, st *State, baseline map[string]Response, opt CutoverOptions) error {
	tries, delay := opt.GateTries, opt.GateDelay
	if tries <= 0 {
		tries = 3
	}
	if delay <= 0 {
		delay = 5 * time.Second
	}
	ip := e.probeIP()
	var res *VerifyResult
	for attempt := 1; attempt <= tries; attempt++ {
		res = &VerifyResult{At: e.now()}
		for _, p := range probeTargets(rep.Routes) {
			port := 80
			if p.Scheme == "https" {
				port = 443
			}
			p.Old = baseline[p.Scheme+"://"+p.Host+p.Path]
			p.New = e.probe()(ctx, p.Scheme, ip, port, p.Host, p.Path)
			compare(&p)
			res.Probes = append(res.Probes, p)
		}
		summarize(res)
		if res.Failed == 0 {
			break
		}
		if attempt < tries {
			e.logf("health gate: %d host(s) differ, retrying in %s", res.Failed, delay)
			e.sleep(delay)
		}
	}
	st.Gate = res
	st.log(e, "health gate: %d ✓ %d ⚠ %d ✗", res.OK, res.Warn, res.Failed)
	if res.Failed > 0 {
		var bad []string
		for _, p := range res.Probes {
			if p.Verdict == StatusFallback {
				bad = append(bad, fmt.Sprintf("%s://%s%s (%s)", p.Scheme, p.Host, p.Path, p.Note))
			}
		}
		return fmt.Errorf("health gate: %s", strings.Join(bad, ", "))
	}
	return nil
}

// ================================================================= rollback ==

// Rollback undoes the journal in reverse order (gateway back to shadow ports, controller Service /
// HelmChartConfig restored, host proxy config restored and reloaded, stopped units started).
// With purge, the imported gateway routes are removed too.
func (e *Env) Rollback(ctx context.Context, rep *Report, st *State, purge bool) error {
	var errs []string
	for i := len(st.Journal) - 1; i >= 0; i-- {
		s := st.Journal[i]
		if err := e.undo(ctx, s, purge); err != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", s.Kind, s.Target, err))
			continue
		}
		e.logf("  undone: %s %s", s.Kind, s.Target)
	}
	st.Journal = nil
	if purge {
		st.ImportedRoutes = nil
	}
	if hp := rep.EdgeHostProxy(); hp != nil && len(errs) == 0 {
		err := e.waitFor(ctx, hp.Kind+" back on 80/443", func() (bool, error) {
			for _, l := range e.Host.Listeners(80, 443) {
				if !l.InPod && hostProxyProcs[l.Process] == hp.Kind {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("%v — check `systemctl status %s`", err, hp.Unit))
		}
	}
	st.Phase, st.RolledBackAt = PhaseRolledBack, e.now()
	if len(errs) > 0 {
		st.log(e, "rollback finished with errors: %s", strings.Join(errs, "; "))
		return errors.New(strings.Join(errs, "; "))
	}
	if st.Fallback.HTTP == "" && st.Fallback.TLS == "" {
		st.log(e, "rollback complete: nectar-gateway is off ports 80/443 (there was no previous proxy)")
	} else {
		st.log(e, "rollback complete: the previous proxy serves 80/443 again")
	}
	return nil
}

func (e *Env) undo(ctx context.Context, s Step, purge bool) error {
	switch s.Kind {
	case stepGatewayPorts:
		var p map[string]string
		_ = json.Unmarshal(s.Before, &p)
		return e.setGatewayPorts(ctx, p["proxy"], p["https"])
	case stepGatewaySettings:
		if e.Gateway == nil {
			return nil
		}
		var old gateway.Settings
		_ = json.Unmarshal(s.Before, &old)
		_, err := e.Gateway.PutSettings(ctx, old)
		return err
	case stepHelmChartConfig:
		if string(s.Before) == "null" {
			return e.Kube.Delete(ctx, "HelmChartConfig", "kube-system", "traefik")
		}
		var vc string
		_ = json.Unmarshal(s.Before, &vc)
		_, err := e.Kube.Patch(ctx, "HelmChartConfig", "kube-system", "traefik", map[string]any{"spec": map[string]any{"valuesContent": vc}})
		return err
	case stepServiceType:
		ns, name, _ := strings.Cut(s.Target, "/")
		var before map[string]any
		_ = json.Unmarshal(s.Before, &before)
		// node ports may have been reused meanwhile: let Kubernetes allocate new ones
		if ports, ok := before["ports"].([]any); ok {
			for _, p := range ports {
				if pm, ok := p.(map[string]any); ok {
					delete(pm, "nodePort")
				}
			}
		}
		delete(before, "healthCheckNodePort")
		_, err := e.Kube.Patch(ctx, "Service", ns, name, map[string]any{"spec": before})
		return err
	case stepHostConfig:
		var backup string
		_ = json.Unmarshal(s.Before, &backup)
		b, err := e.Host.ReadFile(backup)
		if err != nil {
			return err
		}
		return e.Host.WriteFile(s.Target, b)
	case stepHostRebind:
		out, err := e.Host.Run(ctx, "systemctl", "reload-or-restart", s.Target)
		if err != nil {
			return fmt.Errorf("%v %s", err, out)
		}
		return nil
	case stepHostStop:
		out, err := e.Host.Run(ctx, "systemctl", "start", s.Target)
		if err != nil {
			return fmt.Errorf("%v %s", err, out)
		}
		return nil
	case stepImportedRoutes:
		if !purge || e.Gateway == nil {
			return nil
		}
		var ids []int64
		_ = json.Unmarshal(s.Before, &ids)
		for _, id := range ids {
			if err := e.Gateway.Delete(ctx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// ================================================================= finalize ==

// FinalizeOptions tune finalize.
type FinalizeOptions struct {
	Force      bool          // ignore recent fallback hits
	QuietFor   time.Duration // hosts must not have used the fallback for this long (default 24h)
	RestartK3s bool          // restart k3s now so that a disabled Traefik is removed
}

// Finalize removes the dependency on the old proxy: the bundled Traefik is disabled with a k3s drop-in
// (k3s's own config.yaml is never edited), a host proxy is stopped and disabled, and the gateway's fallback
// is cleared. It refuses while hosts still reach the old proxy, unless forced.
func (e *Env) Finalize(ctx context.Context, rep *Report, st *State, opt FinalizeOptions) error {
	if st.Phase != PhaseSoaking && !opt.Force {
		return fmt.Errorf("finalize needs a completed cutover (phase is %q)", st.Phase)
	}
	if e.Gateway == nil {
		if err := e.ConnectGateway(ctx); err != nil {
			return err
		}
	}
	quiet := opt.QuietFor
	if quiet <= 0 {
		quiet = 24 * time.Hour
	}
	hits, err := e.Gateway.FallbackHits(ctx)
	if err != nil {
		return fmt.Errorf("fallback hits: %w", err)
	}
	var recent []string
	for _, h := range hits {
		if e.now().Sub(h.LastSeen) < quiet {
			recent = append(recent, fmt.Sprintf("%s (%d HTTP, %d TLS, last %s)", h.Host, h.HTTP, h.TLS, h.LastSeen.Format(time.RFC3339)))
		}
	}
	if len(recent) > 0 && !opt.Force {
		return fmt.Errorf("these hosts still use the old proxy (last %s): %s — add routes for them, or use --force", quiet, strings.Join(recent, "; "))
	}
	ctrl := rep.EdgeController()
	hp := rep.EdgeHostProxy()
	switch {
	case ctrl != nil && ctrl.K3sBundled:
		dropin := "/etc/rancher/k3s/config.yaml.d/90-nectar.yaml"
		content := "# Written by nectar-apps adopt --finalize: nectar-gateway replaced the bundled Traefik.\n# Delete this file and restart k3s to bring Traefik back.\ndisable+:\n  - traefik\n"
		if err := e.Host.WriteFile(dropin, []byte(content)); err != nil {
			return err
		}
		st.log(e, "wrote %s (disable traefik)", dropin)
		if opt.RestartK3s {
			if out, err := e.Host.Run(ctx, "systemctl", "restart", "k3s"); err != nil {
				return fmt.Errorf("restart k3s: %v %s", err, out)
			}
			st.log(e, "k3s restarted: Traefik is being removed")
		} else {
			e.logf("  Traefik is removed the next time k3s restarts (systemctl restart k3s); running workloads are not affected")
		}
	case ctrl != nil:
		e.logf("  %s keeps running unused; uninstall it with the tool you installed it with (e.g. helm uninstall)", ctrl.Workload)
	case hp != nil && hp.Unit != "":
		if out, err := e.Host.Run(ctx, "systemctl", "disable", "--now", hp.Unit); err != nil {
			return fmt.Errorf("disable %s: %v %s", hp.Unit, err, out)
		}
		st.log(e, "%s stopped and disabled", hp.Unit)
	}
	if _, err := e.Gateway.PutSettings(ctx, gateway.Settings{}); err != nil {
		return fmt.Errorf("clear the gateway fallback: %w", err)
	}
	st.Fallback.HTTP, st.Fallback.TLS = "", ""
	st.Phase, st.FinalizedAt = PhaseFinalized, e.now()
	st.Journal = nil // nothing left to roll back automatically; the backup stays
	st.log(e, "finalized: unknown hosts now get the gateway's 410 page")
	return nil
}
