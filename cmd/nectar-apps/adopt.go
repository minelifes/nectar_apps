package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nectar_apps/internal/adopt"
	"nectar_apps/internal/kube"
)

const adoptUsage = `Adopt an existing k3s server (run on the server as root; install.sh --adopt calls these):

  nectar-apps adopt discover   [--json]       read-only discovery + migration report
  nectar-apps adopt plan       [--strategy S] shell variables for install.sh (strategy, routing, classes)
  nectar-apps adopt backup                    back up the datastore, routing objects and proxy configs
  nectar-apps adopt shadow                    configure the shadow gateway (18080/18443) and import routes
  nectar-apps adopt verify                    compare the old proxy with the shadow gateway for every host
  nectar-apps adopt cutover    [--force]      move 80/443 to nectar-gateway (health gate + auto-rollback)
  nectar-apps adopt rollback   [--purge]      undo the cutover / shadow changes
  nectar-apps adopt finalize   [--force] [--restart-k3s]   stop depending on the old proxy
  nectar-apps adopt status                    phase, history and hosts still using the old proxy

Common flags: --kubeconfig PATH (default /etc/rancher/k3s/k3s.yaml), --namespace NS, --state-dir DIR,
              --probe-ip IP (address used for the comparison requests; default this node's IP)
`

func adoptCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(os.Stderr, adoptUsage)
		return nil
	}
	sub := args[0]
	fs := flag.NewFlagSet("adopt "+sub, flag.ContinueOnError)
	kubeconfig := fs.String("kubeconfig", envOr("KUBECONFIG", kube.K3sKubeconfig), "kubeconfig of the cluster")
	namespace := fs.String("namespace", "nectar-system", "Nectar system namespace")
	stateDir := fs.String("state-dir", "/var/lib/nectar", "state and backup directory")
	root := fs.String("root", "", "host root directory (tests)")
	probeIP := fs.String("probe-ip", "", "IP used to reach both proxies during verification")
	asJSON := fs.Bool("json", false, "discover: print the report as JSON")
	force := fs.Bool("force", false, "cutover/finalize: skip safety checks")
	purge := fs.Bool("purge", false, "rollback: also delete the routes imported into the gateway")
	restartK3s := fs.Bool("restart-k3s", false, "finalize: restart k3s now (removes a disabled Traefik)")
	noRollback := fs.Bool("no-rollback", false, "cutover: keep the new state when the health gate fails (debugging)")
	refresh := fs.Bool("refresh", false, "re-run discovery even after the shadow install")
	strategy := fs.String("strategy", "", "shadow: migrate | adopt-gateway (default: the recommended one)")
	quiet := fs.Duration("quiet-for", 24*time.Hour, "finalize: hosts must not have used the fallback for this long")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	changes := sub != "discover" && sub != "status" && sub != "verify" && sub != "plan"
	if changes && *root == "" && os.Geteuid() != 0 {
		return adopt.ErrNotRoot
	}
	kc, err := kube.FromKubeconfig(*kubeconfig)
	if err != nil {
		return err
	}
	adopt.GatewayDBDriver = sqliteDriver
	env := &adopt.Env{
		Kube: kc, Host: adopt.Host{Root: *root}, SystemNamespace: *namespace, StateDir: *stateDir, ProbeIP: *probeIP,
		SQLDriver: sqliteDriver, Logf: func(format string, a ...any) { fmt.Printf("⬢ "+format+"\n", a...) },
	}
	st, err := env.LoadState()
	if err != nil {
		return err
	}
	reportFile := filepath.Join(*stateDir, "adopt", "report.json")

	// the report from the first discovery is the plan: after the shadow install the gateway changes what
	// discovery would see, so later phases reuse the stored report
	loadReport := func() (*adopt.Report, error) {
		fresh := sub == "discover" || sub == "backup" || sub == "plan" || *refresh ||
			st.Phase == "" || st.Phase == adopt.PhaseDiscovered || st.Phase == adopt.PhaseRolledBack
		if !fresh {
			if b, err := env.Host.ReadFile(reportFile); err == nil {
				var rep adopt.Report
				if err := json.Unmarshal(b, &rep); err == nil {
					return &rep, nil
				}
			}
		}
		return adopt.Discover(ctx, kc, env.Host, *namespace)
	}
	rep, err := loadReport()
	if err != nil {
		return err
	}
	if env.ProbeIP == "" {
		env.ProbeIP = nodeIP(rep)
	}
	save := func() error { return env.SaveState(ctx, st, rep) }

	switch sub {
	case "discover":
		if *asJSON {
			b, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(rep.Markdown())
		}
		if st.Phase == "" {
			st.Phase = adopt.PhaseDiscovered
		}
		if err := save(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save the report: %v\n", err)
		} else if !*asJSON {
			fmt.Printf("Report saved to %s/adopt/nectar-migration-report.md\n", *stateDir)
		}
		return nil
	case "plan":
		name := *strategy
		if name == "" {
			name = rep.Recommended()
		}
		opt, ok := rep.Strategy(name)
		if !ok {
			return fmt.Errorf("unknown strategy %q (migrate, keep, adopt-gateway)", name)
		}
		routing, class, sync, ctrl := "gateway", "", "nectar", rep.Options.Controller
		if name == adopt.StrategyKeep {
			routing, sync = "ingress", "off"
			if c := rep.EdgeController(); c != nil && len(c.Classes) > 0 {
				class = c.Classes[0]
			}
		} else if len(rep.Options.Classes) > 0 {
			sync = strings.Join(append(append([]string{}, rep.Options.Classes...), "nectar"), ",")
		}
		possible := 0
		if opt.Possible {
			possible = 1
		}
		for _, kv := range [][2]string{{"STRATEGY", name}, {"POSSIBLE", fmt.Sprint(possible)}, {"REASON", opt.Reason},
			{"EDGE", rep.Edge}, {"ROUTING", routing}, {"INGRESS_CLASS", class}, {"INGRESS_SYNC", sync}, {"INGRESS_CONTROLLER", ctrl},
			{"MIGRATED_HOSTS", fmt.Sprint(len(rep.Hosts))}, {"FALLBACK_ROUTES", fmt.Sprint(rep.Counts[adopt.StatusFallback])},
			{"NECTAR_INSTALLED", fmt.Sprint(map[bool]int{true: 1}[rep.Nectar])}, {"PHASE", st.Phase}} {
			fmt.Printf("%s='%s'\n", kv[0], strings.ReplaceAll(kv[1], "'", "'\\''"))
		}
		return nil
	case "backup":
		if err := env.Backup(ctx, rep, st); err != nil {
			return err
		}
		return save()
	case "shadow":
		if st.BackupDir == "" {
			return errors.New("run `nectar-apps adopt backup` first")
		}
		if *strategy != "" {
			if *strategy != adopt.StrategyMigrate && *strategy != adopt.StrategyAdoptGateway {
				return fmt.Errorf("--strategy must be %s or %s (strategy %s needs no shadow install)", adopt.StrategyMigrate, adopt.StrategyAdoptGateway, adopt.StrategyKeep)
			}
			st.Strategy = *strategy
		}
		if st.Strategy == "" {
			st.Strategy = adopt.StrategyMigrate
			if rep.Recommended() == adopt.StrategyAdoptGateway {
				st.Strategy = adopt.StrategyAdoptGateway
			}
		}
		if st.Strategy == adopt.StrategyAdoptGateway {
			if hp := rep.EdgeHostProxy(); hp == nil || hp.Kind == "nectar_gateway" {
				for _, h := range rep.HostProxies {
					if h.Kind == "nectar_gateway" {
						_, blocked, err := adopt.GatewayDB(ctx, sqliteDriver, env.Host.Root+h.Config)
						if err == nil {
							rep.Blocked = blocked
						}
					}
				}
			}
		}
		err := env.Shadow(ctx, rep, st)
		if serr := save(); serr != nil && err == nil {
			err = serr
		}
		return err
	case "verify":
		res := env.Verify(ctx, rep, st)
		fmt.Print(res.Text())
		if err := save(); err != nil {
			return err
		}
		if res.Failed > 0 {
			return fmt.Errorf("%d host(s) answer differently through the gateway", res.Failed)
		}
		return nil
	case "cutover":
		err := env.Cutover(ctx, rep, st, adopt.CutoverOptions{Force: *force, NoRollback: *noRollback})
		if st.Gate != nil {
			fmt.Print(st.Gate.Text())
		}
		if serr := save(); serr != nil && err == nil {
			err = serr
		}
		return err
	case "rollback":
		err := env.ConnectGateway(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: gateway not reachable (%v); its settings are not restored\n", err)
			env.Gateway = nil
		}
		err = env.Rollback(ctx, rep, st, *purge)
		if serr := save(); serr != nil && err == nil {
			err = serr
		}
		if err == nil && st.BackupDir != "" {
			fmt.Printf("Backups stay in %s (see manifest.json there for restoring the datastore).\n", st.BackupDir)
		}
		return err
	case "finalize":
		err := env.Finalize(ctx, rep, st, adopt.FinalizeOptions{Force: *force, RestartK3s: *restartK3s, QuietFor: *quiet})
		if serr := save(); serr != nil && err == nil {
			err = serr
		}
		return err
	case "status":
		fmt.Printf("Strategy: %s\nPhase:    %s\n", orNone(st.Strategy), orNone(st.Phase))
		if st.BackupDir != "" {
			fmt.Printf("Backup:   %s\n", st.BackupDir)
		}
		if st.Fallback.HTTP != "" {
			fmt.Printf("Fallback: %s / %s\n", st.Fallback.HTTP, st.Fallback.TLS)
		}
		n := len(st.History)
		for _, h := range st.History[max(0, n-15):] {
			fmt.Println("  " + h)
		}
		if env.ConnectGateway(ctx) == nil {
			if hits, err := env.Gateway.FallbackHits(ctx); err == nil {
				if len(hits) == 0 {
					fmt.Println("No host used the old proxy through the gateway.")
				} else {
					fmt.Println("Hosts still served by the old proxy (fallback):")
					for _, h := range hits {
						fmt.Printf("  %-40s %6d HTTP %6d TLS  last %s\n", h.Host, h.HTTP, h.TLS, h.LastSeen.Format(time.RFC3339))
					}
				}
			}
		}
		return nil
	}
	return fmt.Errorf("unknown adopt command %q\n\n%s", sub, adoptUsage)
}

// nodeIP returns the InternalIP of the node this command runs on (for probing both proxies).
func nodeIP(rep *adopt.Report) string {
	for _, n := range rep.K3s.Nodes {
		if strings.EqualFold(n.Name, rep.Hostname) && n.InternalIP != "" {
			return n.InternalIP
		}
	}
	if len(rep.K3s.Nodes) == 1 && rep.K3s.Nodes[0].InternalIP != "" {
		return rep.K3s.Nodes[0].InternalIP
	}
	return "127.0.0.1"
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
