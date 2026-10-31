// Command nectar-apps is the Nectar Apps control panel.
//
//	nectar-apps serve                                   run the API + web UI
//	nectar-apps admin reset-password --email E [--password P]
//	nectar-apps admin make-owner --email E
//	nectar-apps adopt …                                 adopt an existing k3s server (host side)
//	nectar-apps update run|snapshot|restore             the updater Job and its helper pods (started by the panel)
//	nectar-apps version
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"nectar_apps/internal/adopt"
	"nectar_apps/internal/alerts"
	"nectar_apps/internal/api"
	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/auth"
	"nectar_apps/internal/backup"
	"nectar_apps/internal/build"
	"nectar_apps/internal/config"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/gateway"
	"nectar_apps/internal/housekeep"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/logstore"
	"nectar_apps/internal/mail"
	"nectar_apps/internal/observe"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/rekey"
	"nectar_apps/internal/routing"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
	"nectar_apps/internal/update"
	"nectar_apps/internal/version"
	"nectar_apps/web"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "admin":
		err = admin(os.Args[2:])
	case "adopt":
		err = adoptCmd(os.Args[2:])
	case "update":
		err = updateCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("nectar-apps %s (%s), API v%d\n", version.Version, version.Commit, version.APIVersion)
	case "help", "--help", "-h":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Nectar Apps control panel

Usage:
  nectar-apps serve                                      run the API and web UI
  nectar-apps admin reset-password --email E [--password P]
  nectar-apps admin rotate-master-key [--key K] [--no-restart]
  nectar-apps admin make-owner --email E                 grant the Owner flag (recovery)
  nectar-apps adopt discover|backup|shadow|verify|cutover|rollback|finalize|status
                                                         migrate an existing k3s server (see adopt --help)
  nectar-apps update run|snapshot|restore                internal: the updater Job started by Settings → Updates
  nectar-apps version

Configuration is read from NECTAR_* environment variables (see internal/config).
`)
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.Dev {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

func openStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return store.Open(ctx, sqliteDriver, store.DSN(cfg.DBPath()))
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	if cfg.Dev {
		log.Warn("development mode: insecure cookies and a fixed dev master key may be in use")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	kc, err := kube.New(cfg.KubeAPI)
	switch {
	case errors.Is(err, kube.ErrNotConfigured):
		log.Warn("kubernetes not configured — server features disabled", "hint", "set NECTAR_KUBE_API=http://127.0.0.1:8001 with kubectl proxy")
		kc = nil
	case err != nil:
		return err
	default:
		vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if v, err := kc.ServerVersion(vctx); err != nil {
			log.Warn("kubernetes API not reachable yet", "err", err)
		} else {
			log.Info("connected to kubernetes", "version", v)
		}
		cancel()
	}

	// disaster recovery: install.sh --restore left a recovery kit → restore the panel data before opening it
	if kc != nil {
		if _, err := backup.RestorePanel(ctx, kc, cfg.SystemNamespace, cfg.DataDir, "nectar-apps-data", 65532, log); err != nil {
			return err
		}
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()
	// the master key must match the database; a pending rotation (master-key-next) is finished here,
	// before anything else reads or writes a secret
	ro := rekey.Options{Store: st, Namespace: cfg.SystemNamespace, Current: cfg.MasterKey, Log: log}
	if kc != nil {
		ro.Kube = kc
	}
	if cfg.MasterKey, err = rekey.Startup(ctx, ro); err != nil {
		return err
	}

	catalog, err := runtimes.LoadBuiltin()
	if err != nil {
		return err
	}
	arts, err := artifacts.New(cfg.ArtifactsDir())
	if err != nil {
		return fmt.Errorf("artifact store: %w", err)
	}
	envBox, err := secretbox.New(cfg.MasterKey, "env")
	if err != nil {
		return err
	}
	artifactKey, err := rekey.ArtifactKey(ctx, st, envBox)
	if err != nil {
		return err
	}
	// domains: routes are synced from the panel's domains to nectar-gateway (default) or to
	// Kubernetes Ingress objects (routing=ingress, for clusters that keep their own ingress controller)
	var gw *gateway.Client
	var gwSync routing.Gateway
	switch {
	case cfg.Routing == "ingress" && kc != nil:
		gwSync = &routing.IngressProvider{Kube: kc, ClassName: cfg.IngressClass, ClusterIssuer: cfg.IngressIssuer}
		log.Info("domains are served through Kubernetes Ingress", "class", cfg.IngressClass, "issuer", cfg.IngressIssuer)
	case cfg.Routing == "ingress":
		log.Warn("routing=ingress needs Kubernetes — domains will not be served")
	case cfg.GatewayURL != "" && cfg.GatewayToken != "":
		gw = gateway.New(cfg.GatewayURL, cfg.GatewayToken)
		gwSync = gw
		vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if v, err := gw.Version(vctx); err != nil {
			log.Warn("gateway not reachable yet", "url", cfg.GatewayURL, "err", err)
		} else {
			log.Info("connected to nectar-gateway", "version", v.Version, "api", v.APIVersion)
		}
		cancel()
	default:
		log.Warn("no gateway configured (NECTAR_GATEWAY_URL / NECTAR_GATEWAY_TOKEN) — domains will not be served")
	}
	syncer := routing.New(st, gwSync, log)
	go syncer.Run(ctx, time.Minute)

	// Ingress compatibility: Ingress / IngressRoute / HTTPRoute objects of the served classes become gateway
	// routes (adopt mode keeps Helm charts and kubectl workflows working after the migration)
	var ingSync *adopt.IngressSync
	if gw != nil && kc != nil && len(cfg.IngressSync) > 0 {
		ingSync = &adopt.IngressSync{Kube: kc, Gateway: gw, Log: log,
			Options:  adopt.K8sOptions{Controller: cfg.IngressController, Classes: cfg.IngressSync},
			Reserved: func(ctx context.Context) map[string]bool { return routing.ReservedKeys(ctx, st) },
			Address:  func(ctx context.Context) string { return edgeAddress(ctx, kc) }}
		go ingSync.Run(ctx, 30*time.Second)
		log.Info("syncing Kubernetes Ingress objects to the gateway", "classes", cfg.IngressSync, "controller", cfg.IngressController)
	}

	// resource usage history (server and app charts)
	if kc != nil {
		go (&observe.Collector{Kube: kc, Store: st, Log: log}).Run(ctx)
	}

	// private registry logins for app pods (set once the store and its box exist, see below)
	var pullSecret = func(context.Context) ([]byte, error) { return nil, nil }
	if box, err := secretbox.New(cfg.MasterKey, "registries"); err == nil {
		pullSecret = func(ctx context.Context) ([]byte, error) { return registry.PullSecret(ctx, st, box) }
	}
	var dep *deploy.Deployer
	var deployEvents atomic.Pointer[alerts.Engine] // set below; deploys notify through the alert channels
	if kc != nil {
		dep = deploy.New(st, kc, catalog, envBox, log, deploy.Options{
			SystemNamespace: cfg.SystemNamespace,
			InternalURL:     cfg.InternalURL,
			SigningKey:      artifactKey,
			OnDone:          func(int64) { syncer.Trigger() },
			PullSecret:      func(ctx context.Context) ([]byte, error) { return pullSecret(ctx) },
			OnResult: func(a store.App, rel store.Release, err error) {
				ev := deployEvents.Load()
				if ev == nil {
					return
				}
				ctx := context.Background()
				path := "/apps/" + strconv.FormatInt(a.ID, 10)
				if err != nil {
					ev.Event(ctx, alerts.DeployFailed, a.Name, fmt.Sprintf("Deploy #%d of %s failed: %v", rel.Number, a.Name, err), path)
					return
				}
				ev.Event(ctx, alerts.DeployOK, a.Name, fmt.Sprintf("Release #%d of %s is live.", rel.Number, a.Name), path)
			},
		})
		dep.RecoverInterrupted(ctx)
	}

	mfaBox, err := secretbox.New(cfg.MasterKey, "totp")
	if err != nil {
		return err
	}
	alertBox, err := secretbox.New(cfg.MasterKey, "alerts")
	if err != nil {
		return err
	}
	var backups *backup.Manager
	if kc != nil && dep != nil {
		backupBox, err := secretbox.New(cfg.MasterKey, "backups")
		if err != nil {
			return err
		}
		backups = &backup.Manager{Store: st, Kube: kc, Deployer: dep, Box: backupBox, Log: log, Namespace: cfg.SystemNamespace, DataDir: cfg.DataDir}
		go backups.Run(ctx)
		if backup.RestorePending(cfg.DataDir) {
			go func() { _ = backups.RestoreAll(ctx) }()
		}
	}
	keeper := &housekeep.Keeper{Store: st, Artifacts: arts, DataDir: cfg.DataDir, DBPath: cfg.DBPath(), Namespace: cfg.SystemNamespace, Log: log,
		InitImage: deploy.InitImage, Images: func(ctx context.Context) []string { return runtimeImages(ctx, st, catalog) }}
	alertEngine := &alerts.Engine{Store: st, Box: alertBox, Sender: &alerts.Sender{}, Log: log}
	deployEvents.Store(alertEngine)
	mailBox, err := secretbox.New(cfg.MasterKey, "smtp")
	if err != nil {
		return err
	}
	mailr := &mail.Mailer{Store: st, Box: mailBox, Sender: alertEngine.Sender}
	alertEngine.PanelSMTP = func(ctx context.Context) (alerts.ChannelConfig, error) {
		s, err := mailr.Load(ctx)
		return s.Channel(), err
	}
	if kc != nil {
		keeper.Kube, alertEngine.Kube = kc, kc
	}
	if kc != nil && dep != nil {
		alertEngine.Volumes = func(ctx context.Context) ([]alerts.VolumeUsage, error) { return volumeUsage(ctx, dep) }
	}
	if gw != nil {
		alertEngine.Gateway = gw
	}

	// built-in registry + Git builds (M7)
	registryBox, err := secretbox.New(cfg.MasterKey, "registries")
	if err != nil {
		return err
	}
	reg := &registry.Registry{Namespace: cfg.SystemNamespace, Password: cfg.RegistryPassword, Host: cfg.RegistryHost,
		NodePort: cfg.RegistryNodePort, SizeGB: cfg.RegistrySizeGB}
	var builder *build.Builder
	if kc != nil {
		reg.Kube = kc
		if reg.Enabled() {
			go func() {
				for i := 0; i < 10; i++ {
					if err := reg.Ensure(ctx); err == nil {
						log.Info("built-in registry ready", "host", cfg.RegistryHost)
						return
					} else if ctx.Err() == nil {
						log.Warn("built-in registry", "err", err)
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(30 * time.Second):
					}
				}
			}()
		}
		buildBox, err := secretbox.New(cfg.MasterKey, "builds")
		if err != nil {
			return err
		}
		builder = &build.Builder{Store: st, Kube: kc, Registry: reg, Deployer: dep, Box: buildBox, Log: log, Image: cfg.BuilderImage,
			Creds: func(ctx context.Context) ([]registry.Credential, error) {
				return registry.LoadCredentials(ctx, st, registryBox)
			}}
		builder.Recover(ctx)
	}

	// updates of the panel, the gateway and k3s (PLAN.md §5a)
	updates := &update.Manager{Store: st, Fetcher: &update.Fetcher{}, Log: log, Namespace: cfg.SystemNamespace,
		PanelVersion: version.Version, PanelAPI: version.APIVersion,
		PanelRequires: update.Requires{GatewayAPI: version.GatewayAPIs, MinGateway: version.MinGateway},
		PanelURL:      "http://nectar-apps." + cfg.SystemNamespace + ".svc", GatewayAdminURL: cfg.GatewayURL,
		Notify: func(ctx context.Context, title, message string, ok bool) {
			alertEngine.Announce(ctx, title, message, "/updates", ok)
		}}
	if kc != nil {
		updates.Kube = kc
	}
	if gw != nil {
		updates.Gateway = gatewayVersion{gw}
	}
	go updates.Run(ctx)

	// log history: app logs on the panel's disk, searchable after restarts (Settings → retention)
	var logStore *logstore.Store
	if kc != nil {
		if ls, err := logstore.New(filepath.Join(cfg.DataDir, "logs")); err != nil {
			log.Warn("log history off", "err", err)
		} else {
			logStore = ls
			lc := &logstore.Collector{Store: ls, Kube: kc, Log: log, Retention: func(ctx context.Context) (bool, int, int64) {
				h := housekeep.Load(ctx, st)
				return h.LogHistoryOff, h.LogDays, int64(h.LogMaxMB) << 20
			}}
			go lc.Run(ctx)
		}
	}

	srv := api.New(api.Deps{Config: cfg, Store: st, Kube: kc, Deployer: dep, Catalog: catalog,
		Artifacts: arts, EnvBox: envBox, Gateway: gw, Syncer: syncer, IngressSync: ingSync, Log: log, Web: web.Handler(),
		AlertBox: alertBox, AlertSender: alertEngine.Sender, Keeper: keeper, Backups: backups, MFABox: mfaBox, Mailer: mailr,
		Updates: updates, Builder: builder, Registry: reg, RegistryBox: registryBox, ArtifactKey: artifactKey, LogStore: logStore})
	if err := srv.SyncBuiltinRoles(ctx); err != nil {
		return err
	}
	if n, err := st.CountUsers(ctx); err == nil && n == 0 {
		if cfg.SetupToken == "" {
			log.Warn("no users yet and NECTAR_SETUP_TOKEN is empty: first-run setup is impossible; use `nectar-apps admin reset-password` or set the token")
		} else {
			log.Info("no users yet — open /setup with the setup token to create the first Owner")
		}
	}

	// retention and disk cleanup, alerts
	go func() {
		if err := keeper.SyncPrepull(ctx); err != nil {
			log.Warn("image pre-pull", "err", err)
		}
		keeper.Run(ctx)
	}()
	go alertEngine.Run(ctx)
	// sizes of node-local volumes (k3s local-path folders: the kubelet reports the whole disk for them)
	if kc != nil {
		go dep.RunVolumeMeter(ctx, 10*time.Minute)
	}
	alertEngine.Sender.Push = srv.SendPush // alert channels of kind "push" (Firebase; Settings → Push notifications)
	go srv.RunEdge(ctx)                    // gateway replicas on extra edge nodes follow the primary's image
	go srv.RunPreviews(ctx)                // delete pull request previews without activity

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "version", version.Version)
		errc <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := httpSrv.Shutdown(sctx)
		// running deploys are marked as interrupted on the next start (RecoverInterrupted)
		return err
	}
	return nil
}

// gatewayVersion adapts the gateway client for the updater.
type gatewayVersion struct{ c *gateway.Client }

func (g gatewayVersion) Version(ctx context.Context) (string, int, error) {
	v, err := g.c.Version(ctx)
	return v.Version, v.APIVersion, err
}

// updateCmd runs inside the updater Job (run) and its helper pods (snapshot, restore).
func updateCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nectar-apps update run | snapshot --db F --dir D --name N [--keep 3] | restore --db F --from S")
	}
	fs := flag.NewFlagSet("update "+args[0], flag.ContinueOnError)
	db := fs.String("db", "", "database file")
	dir := fs.String("dir", "", "snapshot directory")
	name := fs.String("name", "", "snapshot name")
	keep := fs.Int("keep", 3, "snapshots to keep")
	from := fs.String("from", "", "snapshot to restore")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	switch args[0] {
	case "snapshot":
		if *db == "" || *dir == "" || *name == "" {
			return errors.New("--db, --dir and --name are required")
		}
		out, err := update.Snapshot(ctx, sqliteDriver, *db, *dir, *name, *keep)
		if err != nil {
			return err
		}
		fmt.Println("result: " + out)
		return nil
	case "restore":
		if *db == "" || *from == "" {
			return errors.New("--db and --from are required")
		}
		if err := update.RestoreFile(*from, *db); err != nil {
			return err
		}
		fmt.Println("result: restored " + *from)
		return nil
	case "run":
		var spec update.RunSpec
		if err := json.Unmarshal([]byte(os.Getenv("NECTAR_UPDATE_RUN")), &spec); err != nil {
			return fmt.Errorf("NECTAR_UPDATE_RUN: %w", err)
		}
		kc, err := kube.New(os.Getenv("NECTAR_KUBE_API"))
		if err != nil {
			return err
		}
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
		// finish (or roll back) even when the Job is asked to stop; activeDeadlineSeconds kills it at the end
		r := &update.Runner{Kube: kc, Spec: spec, GatewayToken: os.Getenv("NECTAR_GATEWAY_TOKEN"), Log: log}
		st := r.Run(context.WithoutCancel(ctx))
		log.Info("update run finished", "state", st.State, "error", st.Error)
		if st.State != update.RunSucceeded {
			return errors.New(st.State + ": " + st.Error)
		}
		return nil
	}
	return fmt.Errorf("unknown update command %q", args[0])
}

// admin implements recovery commands. They work inside the pod:
//
//	kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin reset-password --email you@example.com
func admin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nectar-apps admin reset-password|make-owner|reset-mfa --email E | clear-ip-allowlist | rotate-master-key")
	}
	if args[0] == "rotate-master-key" {
		return rotateMasterKey(args[1:])
	}
	if args[0] == "clear-ip-allowlist" {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		ctx := context.Background()
		st, err := openStore(ctx, cfg)
		if err != nil {
			return err
		}
		defer st.Close()
		if err := st.SetSetting(ctx, "access", map[string]any{"ip_allowlist": []string{}}); err != nil {
			return err
		}
		_ = st.Audit(ctx, nil, "cli", "settings.ip_allowlist", "", "", map[string]any{"entries": []string{}})
		fmt.Println("The IP allowlist is cleared: every address may reach the panel again (within 30 seconds).")
		return nil
	}
	fs := flag.NewFlagSet("admin "+args[0], flag.ContinueOnError)
	email := fs.String("email", "", "user email")
	password := fs.String("password", "", "new password (default: generate a random one)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := st.GetUserByEmail(ctx, *email)
	if err != nil {
		return fmt.Errorf("user %s: %w", *email, err)
	}

	switch args[0] {
	case "reset-password":
		pw := *password
		generated := pw == ""
		if generated {
			b := make([]byte, 15)
			_, _ = rand.Read(b)
			pw = base64.RawURLEncoding.EncodeToString(b)
		}
		if err := auth.ValidatePassword(pw); err != nil {
			return err
		}
		h, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}
		enabled := false
		if _, err := st.UpdateUser(ctx, u.ID, store.UserUpdate{PasswordHash: &h, Disabled: &enabled}); err != nil {
			return err
		}
		_ = st.DeleteUserSessions(ctx, u.ID, 0)
		_ = st.Audit(ctx, nil, "cli", "user.password_reset", u.Email, "", nil)
		fmt.Printf("Password for %s was reset and all sessions were ended.\n", u.Email)
		if generated {
			fmt.Printf("New password: %s\n", pw)
		}
	case "reset-mfa":
		if err := st.SetMFA(ctx, u.ID, "", false, nil); err != nil {
			return err
		}
		_, _ = st.DeletePasskeys(ctx, u.ID, 0)
		_ = st.DeleteUserSessions(ctx, u.ID, 0)
		_ = st.Audit(ctx, nil, "cli", "user.mfa_reset", u.Email, "", nil)
		fmt.Printf("Two-factor authentication of %s is off, its passkeys are removed and all sessions were ended.\n", u.Email)
	case "make-owner":
		if err := st.SetOwner(ctx, u.ID, true); err != nil {
			return err
		}
		_ = st.Audit(ctx, nil, "cli", "user.make_owner", u.Email, "", nil)
		fmt.Printf("%s is now an Owner.\n", u.Email)
	default:
		return fmt.Errorf("unknown admin command %q", args[0])
	}
	return nil
}

// rotateMasterKey starts a master key rotation: the new key goes into Secret nectar-secrets as
// master-key-next and the panel restarts; while starting it re-seals every secret (rekey.Startup).
//
//	kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin rotate-master-key
func rotateMasterKey(args []string) error {
	fs := flag.NewFlagSet("admin rotate-master-key", flag.ContinueOnError)
	given := fs.String("key", "", "the new key (base64, 32+ bytes); default: a random one")
	noRestart := fs.Bool("no-restart", false, "do not restart the panel now (the rotation happens at its next start)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	kc, err := kube.New(cfg.KubeAPI)
	if err != nil {
		return fmt.Errorf("kubernetes: %w (run this inside the panel pod: kubectl -n %s exec deploy/nectar-apps -- nectar-apps admin rotate-master-key)", err, cfg.SystemNamespace)
	}
	ctx := context.Background()
	var sec kube.Object
	if err := kc.GetObject(ctx, "Secret", cfg.SystemNamespace, rekey.SecretName, &sec); err != nil {
		return fmt.Errorf("Secret %s: %w", rekey.SecretName, err)
	}
	if kube.Str(sec, "data", rekey.KeyNext) != "" {
		return errors.New("a rotation is already pending (" + rekey.KeyNext + " is set): restart the panel to finish it")
	}
	raw, enc := rekey.NewKey()
	if *given != "" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*given))
		if err != nil || len(b) < 32 {
			return errors.New("--key must be base64 of at least 32 bytes (openssl rand -base64 32)")
		}
		raw, enc = b, strings.TrimSpace(*given)
	}
	if rekey.KeyID(raw) == rekey.KeyID(cfg.MasterKey) {
		return errors.New("the new key is the current key")
	}
	data := map[string]any{rekey.KeyNext: base64.StdEncoding.EncodeToString([]byte(enc))}
	if _, err := kc.Patch(ctx, "Secret", cfg.SystemNamespace, rekey.SecretName, map[string]any{"data": data}); err != nil {
		return fmt.Errorf("store the new key: %w", err)
	}
	fmt.Println("NEW MASTER KEY — store it somewhere safe (password manager) and keep the old one until the steps below are done:")
	fmt.Println("  " + enc)
	fmt.Println()
	if *noRestart {
		fmt.Println("The rotation happens when the panel starts next (kubectl -n " + cfg.SystemNamespace + " rollout restart deploy/nectar-apps).")
	} else {
		patch := map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]any{
			"kubectl.kubernetes.io/restartedAt": time.Now().UTC().Format(time.RFC3339)}}}}}
		if _, err := kc.Patch(ctx, "Deployment", cfg.SystemNamespace, "nectar-apps", patch); err != nil {
			return fmt.Errorf("the new key is stored, but the panel did not restart (%w): kubectl -n %s rollout restart deploy/nectar-apps", err, cfg.SystemNamespace)
		}
		fmt.Println("The panel restarts now and re-seals every secret with the new key while starting (a few seconds).")
	}
	fmt.Println("Then:")
	fmt.Println("  1. Download a new recovery kit (Backups → target → Recovery kit): the old one only works with the old key.")
	fmt.Println("  2. Run the panel backup once (Backups → Panel data → Run now): older panel backups need the old key to restore.")
	fmt.Println("  3. Keep the old key until then; afterwards it is no longer needed.")
	return nil
}

// edgeAddress is the public address of the edge node (nectar-gateway runs there): its ExternalIP, else
// its InternalIP; "" when no node is labelled nectar.dev/edge=true.
func edgeAddress(ctx context.Context, kc *kube.Client) string {
	nodes, err := kc.ListNodes(ctx)
	if err != nil {
		return ""
	}
	for _, n := range nodes {
		if n.Edge {
			if n.ExternalIP != "" {
				return n.ExternalIP
			}
			return n.InternalIP
		}
	}
	return ""
}

// runtimeImages returns the runtime images the apps use (artifact apps; image apps bring their own image).
func runtimeImages(ctx context.Context, st *store.Store, catalog *runtimes.Catalog) []string {
	apps, err := st.ListApps(ctx, 0)
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range apps {
		rt, ok := catalog.Get(a.RuntimeID)
		if !ok {
			continue
		}
		if v, ok := rt.Version(a.RuntimeVersion); ok {
			out = append(out, v.Image)
		}
	}
	return out
}

// volumeUsage adapts the deployer's volume list for the volume_full alert rule.
func volumeUsage(ctx context.Context, dep *deploy.Deployer) ([]alerts.VolumeUsage, error) {
	vols, err := dep.AllVolumes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]alerts.VolumeUsage, 0, len(vols))
	for _, v := range vols {
		u := alerts.VolumeUsage{Key: v.Namespace + "/" + v.Claim, UsedB: v.UsedB, SizeB: v.RequestedB, Path: "/server"}
		switch {
		case v.Volume != "":
			u.Subject = v.Project + "/" + v.App + ": " + v.Volume
			if v.AppID > 0 {
				u.Path = "/apps/" + strconv.FormatInt(v.AppID, 10)
			}
		case v.Addon != "":
			u.Subject = v.Project + "/" + v.Addon + " (database)"
			u.Database = true
		default:
			u.Subject = v.Namespace + "/" + v.Claim
		}
		out = append(out, u)
	}
	return out, nil
}
