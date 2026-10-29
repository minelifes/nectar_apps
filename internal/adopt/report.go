package adopt

import (
	"fmt"
	"strings"
)

// Markdown renders the migration report (nectar-migration-report.md).
func (r *Report) Markdown() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	esc := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

	w("# Nectar Apps — migration report")
	w("")
	w("Generated %s on `%s`. **Discovery is read-only: nothing was changed.**", r.GeneratedAt.Format("2006-01-02 15:04 MST"), r.Hostname)
	w("")
	w("## Summary")
	w("")
	w("| | |")
	w("|---|---|")
	w("| k3s | %s, datastore %s, %d node(s) |", orDash(r.K3s.Version), r.K3s.Datastore, len(r.K3s.Nodes))
	w("| Ports 80 / 443 | %s / %s |", r.Edges[80].describe(), r.Edges[443].describe())
	w("| Routes | %d ✓ migrate · %d ⚠ migrate with notes · %d ✗ stay on the old proxy |", r.Counts[StatusOK], r.Counts[StatusWarn], r.Counts[StatusFallback])
	w("| Hosts moving to nectar-gateway | %d |", len(r.Hosts))
	if r.CertManager {
		w("| cert-manager | installed (ClusterIssuers: %s) |", orDash(strings.Join(r.Issuers, ", ")))
	}
	w("| TLS Secrets used by routes | %d |", r.TLSSecrets)
	if r.Nectar {
		w("| Nectar Apps | already installed |")
	}
	w("")

	w("## Strategies")
	w("")
	w("| Strategy | Possible | |")
	w("|---|---|---|")
	for _, s := range r.Strategies {
		mark := "no"
		if s.Possible {
			mark = "yes"
		}
		name := "`" + s.Name + "`"
		if s.Recommended {
			name += " **(recommended)**"
		}
		w("| %s | %s | %s |", name, mark, esc(s.Reason))
	}
	w("")
	if len(r.Blockers) > 0 {
		w("### Blocking the cutover")
		w("")
		for _, x := range r.Blockers {
			w("- ✗ %s", x)
		}
		w("")
	}
	if len(r.Warnings) > 0 {
		w("### Warnings")
		w("")
		for _, x := range r.Warnings {
			w("- ⚠ %s", x)
		}
		w("")
	}

	w("## Routes")
	w("")
	w("✓ migrated as is · ⚠ migrated with an approximation · ✗ not migrated: the host keeps being served by the old proxy (fallback). Nothing is dropped.")
	w("")
	if len(r.Routes) == 0 {
		w("_No routes found._")
	} else {
		w("| | Host | Path | Target | TLS | Source | Notes |")
		w("|---|---|---|---|---|---|---|")
		for _, rt := range r.Routes {
			target := rt.Target.String()
			if rt.RedirectURL != "" {
				target = fmt.Sprintf("redirect %d → %s", rt.RedirectCode, rt.RedirectURL)
			}
			tls := ""
			switch {
			case rt.TLS && rt.Cert != nil && rt.Cert.Secret != "":
				tls = "Secret " + rt.Cert.Secret
			case rt.TLS && rt.Cert != nil:
				tls = "file"
			case rt.TLS:
				tls = "ACME"
			}
			if rt.ForceHTTPS {
				tls += " (→https)"
			}
			if rt.BasicAuth != "" {
				tls += " 🔒"
			}
			w("| %s | %s | `%s` | %s | %s | %s %s | %s |", rt.Status.Symbol(), esc(orDash(rt.Host)), rt.PathPrefix, esc(orDash(target)),
				esc(orDash(strings.TrimSpace(tls))), rt.Source, esc(rt.Ref), esc(strings.Join(rt.Notes, "; ")))
		}
	}
	w("")

	if len(r.Controllers) > 0 {
		w("## Ingress controllers in the cluster")
		w("")
		w("| Kind | Workload | Service | Classes | Serves 80/443 |")
		w("|---|---|---|---|---|")
		for _, c := range r.Controllers {
			svc := c.Service
			if c.ServiceType != "" {
				svc += " (" + c.ServiceType + ")"
			}
			edge := ""
			if c.HoldsEdge {
				edge = "yes"
			}
			kind := c.Kind
			if c.K3sBundled {
				kind += " (k3s bundled)"
			}
			w("| %s | %s/%s | %s | %s | %s |", kind, c.Namespace, c.Workload, orDash(svc), orDash(strings.Join(c.Classes, ", ")), edge)
		}
		w("")
	}
	if len(r.HostProxies) > 0 {
		w("## Proxies on the host")
		w("")
		w("| Kind | Service | Config | Ports |")
		w("|---|---|---|---|")
		for _, h := range r.HostProxies {
			w("| %s | %s | `%s` | %v |", h.Kind, orDash(h.Unit), h.Config, h.Ports)
		}
		w("")
	}
	w("## Workloads")
	w("")
	w("Existing workloads are never modified. They appear under **External** in the panel and can be adopted as projects later.")
	w("")
	w("| Namespace | Deployments | StatefulSets | DaemonSets | Services | Ingresses |")
	w("|---|---|---|---|---|---|")
	for _, n := range r.Namespaces {
		name := n.Name
		if n.System {
			name += " (system)"
		}
		w("| %s | %d | %d | %d | %d | %d |", name, n.Deployments, n.StatefulSets, n.DaemonSets, n.Services, n.Ingresses)
	}
	if len(r.Errors) > 0 {
		w("")
		w("## Discovery problems")
		w("")
		for _, e := range r.Errors {
			w("- %s", e)
		}
	}
	w("")
	w("## Next steps")
	w("")
	switch rec := r.Recommended(); rec {
	case "":
		w("No strategy can run automatically yet: fix the blocking items above and run the discovery again.")
	default:
		w("```")
		w("install.sh --adopt --strategy=%s", rec)
		w("```")
		w("")
		w("It backs up the cluster and proxy configs first, installs Nectar in shadow mode on ports 18080/18443, compares every host, and only then moves 80/443 (with automatic rollback). `install.sh --adopt --rollback` undoes everything.")
	}
	return b.String()
}

func (e EdgeHolder) describe() string {
	if e.Kind == "none" || e.Kind == "" {
		return "free"
	}
	return e.Name + " (" + e.Detail + ")"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
