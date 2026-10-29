package adopt

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"nectar_apps/internal/kube"
)

// Strategy names (install.sh --strategy=…).
const (
	StrategyMigrate      = "migrate"       // A: nectar-gateway takes over 80/443, the old proxy is the fallback
	StrategyKeep         = "keep"          // B: keep the existing controller, the panel writes Ingress objects
	StrategyAdoptGateway = "adopt-gateway" // C: import a standalone nectar_gateway
)

// K3sInfo describes the k3s installation.
type K3sInfo struct {
	Version       string     `json:"version"`
	Datastore     string     `json:"datastore"` // sqlite | etcd | external
	ConfigFiles   []string   `json:"config_files,omitempty"`
	Disabled      []string   `json:"disabled,omitempty"` // --disable components
	ClusterDomain string     `json:"cluster_domain"`
	Nodes         []NodeInfo `json:"nodes"`
}

// NodeInfo is one cluster node.
type NodeInfo struct {
	Name       string   `json:"name"`
	Ready      bool     `json:"ready"`
	InternalIP string   `json:"internal_ip"`
	Roles      []string `json:"roles,omitempty"`
}

// Controller is an ingress controller / proxy running in the cluster.
type Controller struct {
	Kind        string   `json:"kind"` // traefik, ingress-nginx, haproxy, kong, contour, istio, envoy-gateway, cilium, caddy, nectar-gateway
	Namespace   string   `json:"namespace"`
	Workload    string   `json:"workload"` // Deployment/x or DaemonSet/x
	Service     string   `json:"service,omitempty"`
	ServiceType string   `json:"service_type,omitempty"`
	ExtraPorts  []int    `json:"extra_ports,omitempty"` // exposed ports other than 80/443
	Classes     []string `json:"classes,omitempty"`     // IngressClass names
	K3sBundled  bool     `json:"k3s_bundled,omitempty"` // k3s HelmChart kube-system/traefik
	HostNetwork bool     `json:"host_network,omitempty"`
	HostPorts   []int    `json:"host_ports,omitempty"`
	ServiceLB   bool     `json:"servicelb,omitempty"` // exposed on the node through k3s ServiceLB (svclb)
	LBIPs       []string `json:"lb_ips,omitempty"`    // LoadBalancer IPs that are not node IPs (MetalLB, …)
	HoldsEdge   bool     `json:"holds_edge"`          // serves the node's 80/443
}

// HostProxy is a reverse proxy running directly on the host.
type HostProxy struct {
	Kind    string `json:"kind"` // nginx, caddy, apache, haproxy, nectar_gateway
	Unit    string `json:"unit,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Config  string `json:"config,omitempty"` // main config file (or the DB of a standalone nectar_gateway)
	Ports   []int  `json:"ports"`
}

// NamespaceInfo summarizes a namespace.
type NamespaceInfo struct {
	Name         string `json:"name"`
	System       bool   `json:"system,omitempty"`
	Deployments  int    `json:"deployments"`
	StatefulSets int    `json:"statefulsets"`
	DaemonSets   int    `json:"daemonsets"`
	Services     int    `json:"services"`
	Ingresses    int    `json:"ingresses"`
}

// StrategyOption says whether a strategy fits this server.
type StrategyOption struct {
	Name        string `json:"name"`
	Possible    bool   `json:"possible"`
	Recommended bool   `json:"recommended,omitempty"`
	Reason      string `json:"reason"`
}

// Report is the result of a read-only discovery.
type Report struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Hostname    string             `json:"hostname,omitempty"`
	K3s         K3sInfo            `json:"k3s"`
	Listeners   []Listener         `json:"listeners"` // processes on 80/443 (pods with hostNetwork included)
	Controllers []Controller       `json:"controllers"`
	HostProxies []HostProxy        `json:"host_proxies"`
	Edge        string             `json:"edge"` // who serves 80/443: "controller:<kind>", "host:<kind>", "none", "mixed"
	Routes      []Route            `json:"routes"`
	Hosts       []string           `json:"hosts"` // hosts that move to the gateway
	TCPRoutes   int                `json:"tcp_routes,omitempty"`
	TLSSecrets  int                `json:"tls_secrets"`
	CertManager bool               `json:"cert_manager"`
	Issuers     []string           `json:"cluster_issuers,omitempty"`
	Namespaces  []NamespaceInfo    `json:"namespaces"`
	Strategies  []StrategyOption   `json:"strategies"`
	Blockers    []string           `json:"blockers,omitempty"` // things that block the cutover of strategy A
	Warnings    []string           `json:"warnings,omitempty"`
	Nectar      bool               `json:"nectar_installed"`
	Counts      map[Status]int     `json:"counts"`
	Errors      []string           `json:"errors,omitempty"` // non-fatal discovery problems
	Options     K8sOptions         `json:"k8s_options"`      // what the translators were run with
	Blocked     []BlockedIP        `json:"blocked_ips,omitempty"`
	Edges       map[int]EdgeHolder `json:"edge_ports"`
}

// EdgeHolder is what serves one edge port.
type EdgeHolder struct {
	Kind   string `json:"kind"`   // controller | host | none
	Name   string `json:"name"`   // traefik, nginx, …
	Detail string `json:"detail"` // e.g. "ServiceLB kube-system/traefik", "nginx.service (pid 812)"
}

// edge ports
var edgePorts = []int{80, 443}

// controller detection: image keywords → kind
var controllerImages = []struct{ kind, match string }{
	{"nectar-gateway", "nectar-gateway"},
	{"traefik", "traefik"},
	{"ingress-nginx", "ingress-nginx/controller"},
	{"ingress-nginx", "nginx-ingress-controller"},
	{"ingress-nginx", "nginx/nginx-ingress"},
	{"haproxy", "haproxy-ingress"},
	{"haproxy", "haproxytech/kubernetes-ingress"},
	{"kong", "kong"},
	{"contour", "projectcontour/contour"},
	{"envoy-gateway", "envoyproxy/gateway"},
	{"istio", "istio/proxyv2"},
	{"caddy", "caddy"},
}

// IngressClass controller names → kind
var classControllers = map[string]string{
	"traefik.io/ingress-controller":          "traefik",
	"k8s.io/ingress-nginx":                   "ingress-nginx",
	"nginx.org/ingress-controller":           "ingress-nginx",
	"haproxy.org/ingress-controller":         "haproxy",
	"haproxy-ingress.github.io/controller":   "haproxy",
	"ingress-controllers.konghq.com/kong":    "kong",
	"projectcontour.io/ingress-controller":   "contour",
	"istio.io/ingress-controller":            "istio",
	"caddy.ingress.kubernetes.io/controller": "caddy",
	"nectar.dev/gateway":                     "nectar-gateway",
}

var hostProxyProcs = map[string]string{
	"nginx": "nginx", "caddy": "caddy", "apache2": "apache", "httpd": "apache", "haproxy": "haproxy",
	"nectar_gateway": "nectar_gateway", "nectar-gateway": "nectar_gateway",
}

// Discover inspects the server and the cluster without changing anything.
func Discover(ctx context.Context, k Kube, h Host, systemNamespace string) (*Report, error) {
	rep := &Report{GeneratedAt: time.Now().UTC(), Edges: map[int]EdgeHolder{}}
	if b, err := h.ReadFile("/etc/hostname"); err == nil {
		rep.Hostname = strings.TrimSpace(string(b))
	}
	errf := func(format string, args ...any) { rep.Errors = append(rep.Errors, fmt.Sprintf(format, args...)) }

	discoverK3s(ctx, k, h, rep)
	rep.Listeners = h.Listeners(edgePorts...)

	// --- cluster objects
	nsList, err := k.List(ctx, "Namespace", "", "")
	if err != nil {
		return nil, fmt.Errorf("kubernetes API: %w", err)
	}
	deps, _ := k.List(ctx, "Deployment", "", "")
	dss, _ := k.List(ctx, "DaemonSet", "", "")
	sts, _ := k.List(ctx, "StatefulSet", "", "")
	svcs, _ := k.List(ctx, "Service", "", "")
	ings, _ := list(ctx, k, "Ingress")
	classes, _ := list(ctx, k, "IngressClass")
	nodeIPs := map[string]bool{}
	for _, n := range rep.K3s.Nodes {
		nodeIPs[n.InternalIP] = true
	}
	for _, ns := range nsList {
		name := kube.Str(ns, "metadata", "name")
		if name == systemNamespace {
			rep.Nectar = true
		}
		ni := NamespaceInfo{Name: name, System: strings.HasPrefix(name, "kube-") || name == systemNamespace}
		for _, d := range deps {
			if kube.Str(d, "metadata", "namespace") == name {
				ni.Deployments++
			}
		}
		for _, d := range sts {
			if kube.Str(d, "metadata", "namespace") == name {
				ni.StatefulSets++
			}
		}
		for _, d := range dss {
			if kube.Str(d, "metadata", "namespace") == name {
				ni.DaemonSets++
			}
		}
		for _, d := range svcs {
			if kube.Str(d, "metadata", "namespace") == name {
				ni.Services++
			}
		}
		for _, d := range ings {
			if kube.Str(d, "metadata", "namespace") == name {
				ni.Ingresses++
			}
		}
		rep.Namespaces = append(rep.Namespaces, ni)
	}

	// --- controllers
	var hc kube.Object
	bundledTraefik := k.GetObject(ctx, "HelmChart", "kube-system", "traefik", &hc) == nil
	for _, w := range append(append([]kube.Object{}, deps...), dss...) {
		kind := controllerKind(w)
		if kind == "" {
			continue
		}
		ns, name := kube.Str(w, "metadata", "namespace"), kube.Str(w, "metadata", "name")
		if ns == systemNamespace && kind == "nectar-gateway" {
			continue // our own gateway (re-run)
		}
		c := Controller{Kind: kind, Namespace: ns, Workload: kube.Str(w, "kind") + "/" + name,
			HostNetwork: kube.Field(w, "spec", "template", "spec", "hostNetwork") == true}
		c.K3sBundled = kind == "traefik" && bundledTraefik && ns == "kube-system"
		for _, ctr := range kube.Slice(w, "spec", "template", "spec", "containers") {
			for _, p := range kube.Slice(ctr, "ports") {
				if hp := int(kube.Int(p, "hostPort")); hp != 0 {
					c.HostPorts = append(c.HostPorts, hp)
				}
			}
		}
		labels, _ := kube.Field(w, "spec", "template", "metadata", "labels").(map[string]any)
		for _, s := range svcs {
			if kube.Str(s, "metadata", "namespace") != ns || !selectorMatches(s, labels) {
				continue
			}
			typ := kube.Str(s, "spec", "type")
			if typ != "LoadBalancer" && typ != "NodePort" && c.Service != "" {
				continue
			}
			c.Service = ns + "/" + kube.Str(s, "metadata", "name")
			c.ServiceType = typ
			c.ExtraPorts = nil
			for _, p := range kube.Slice(s, "spec", "ports") {
				if pn := int(kube.Int(p, "port")); pn != 80 && pn != 443 {
					c.ExtraPorts = append(c.ExtraPorts, pn)
				}
			}
			for _, ing := range kube.Slice(s, "status", "loadBalancer", "ingress") {
				if ip := kube.Str(ing, "ip"); ip != "" && !nodeIPs[ip] {
					c.LBIPs = append(c.LBIPs, ip)
				}
			}
		}
		for _, ic := range classes {
			if classControllers[kube.Str(ic, "spec", "controller")] == kind {
				c.Classes = append(c.Classes, kube.Str(ic, "metadata", "name"))
			}
		}
		rep.Controllers = append(rep.Controllers, c)
	}
	// k3s ServiceLB: one svclb DaemonSet per LoadBalancer Service
	for _, ds := range dss {
		svcName := kube.Str(ds, "metadata", "labels", "svccontroller.k3s.cattle.io/svcname")
		svcNS := kube.Str(ds, "metadata", "labels", "svccontroller.k3s.cattle.io/svcnamespace")
		if svcName == "" {
			continue
		}
		if svcNS == "" {
			svcNS = "kube-system"
		}
		for i := range rep.Controllers {
			if rep.Controllers[i].Service == svcNS+"/"+svcName {
				rep.Controllers[i].ServiceLB = true
			}
		}
	}

	// --- who serves 80/443
	for _, p := range edgePorts {
		eh := EdgeHolder{Kind: "none"}
		for i, c := range rep.Controllers {
			if c.ServiceLB && hasPort(svcs, c.Service, p) || containsInt(c.HostPorts, p) || (c.HostNetwork && listenerOn(rep.Listeners, p, true)) {
				rep.Controllers[i].HoldsEdge = true
				detail := "ServiceLB " + c.Service
				switch {
				case containsInt(c.HostPorts, p):
					detail = "hostPort on " + c.Workload
				case c.HostNetwork:
					detail = "hostNetwork " + c.Workload
				}
				eh = EdgeHolder{Kind: "controller", Name: c.Kind, Detail: detail}
			}
		}
		for _, l := range rep.Listeners {
			if l.Port != p || l.InPod || isK3sProcess(l.Process) {
				continue
			}
			kind := hostProxyProcs[l.Process]
			if kind == "" {
				kind = l.Process
			}
			detail := l.Process
			if l.Unit != "" {
				detail = l.Unit
			}
			eh = EdgeHolder{Kind: "host", Name: kind, Detail: fmt.Sprintf("%s (pid %d)", detail, l.PID)}
		}
		rep.Edges[p] = eh
	}
	switch e80, e443 := rep.Edges[80], rep.Edges[443]; {
	case e80.Kind == "none" && e443.Kind == "none":
		rep.Edge = "none"
	case e80.Kind == e443.Kind && e80.Name == e443.Name, e443.Kind == "none":
		rep.Edge = e80.Kind + ":" + e80.Name
	case e80.Kind == "none":
		rep.Edge = e443.Kind + ":" + e443.Name
	default:
		rep.Edge = "mixed"
		rep.Blockers = append(rep.Blockers, fmt.Sprintf("port 80 is served by %s and 443 by %s: adopt mode needs one proxy on both", e80.Detail, e443.Detail))
	}

	// --- host proxies
	seenPID := map[int]int{}
	for _, l := range rep.Listeners {
		if l.InPod {
			continue
		}
		kind := hostProxyProcs[l.Process]
		if kind == "" {
			if isK3sProcess(l.Process) {
				continue
			}
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("port %d is used by %s (pid %d), which adopt mode does not know", l.Port, l.Process, l.PID))
			if rep.Edges[l.Port].Name == l.Process {
				rep.Blockers = append(rep.Blockers, fmt.Sprintf("unknown process %s holds port %d", l.Process, l.Port))
			}
			continue
		}
		if i, ok := seenPID[l.PID]; ok {
			if !containsInt(rep.HostProxies[i].Ports, l.Port) {
				rep.HostProxies[i].Ports = append(rep.HostProxies[i].Ports, l.Port)
			}
			continue
		}
		hp := HostProxy{Kind: kind, Unit: l.Unit, PID: l.PID, Command: l.Command, Ports: []int{l.Port}}
		hp.Config = hostProxyConfig(h, hp)
		seenPID[l.PID] = len(rep.HostProxies)
		rep.HostProxies = append(rep.HostProxies, hp)
	}
	// a standalone nectar_gateway that is installed but not on 80/443 still counts for strategy C
	if !hasHostKind(rep.HostProxies, "nectar_gateway") {
		for _, unit := range []string{"/etc/systemd/system/nectar_gateway.service", "/etc/systemd/system/nectar-gateway.service"} {
			if h.Exists(unit) {
				hp := HostProxy{Kind: "nectar_gateway", Unit: strings.TrimPrefix(unit, "/etc/systemd/system/")}
				hp.Config = hostProxyConfig(h, hp)
				rep.HostProxies = append(rep.HostProxies, hp)
			}
		}
	}

	// --- routes
	edgeCtrl := rep.edgeController()
	opt := K8sOptions{ClusterDomain: rep.K3s.ClusterDomain}
	if edgeCtrl != nil {
		opt.Controller = edgeCtrl.Kind
		opt.Classes = edgeCtrl.Classes
		if len(opt.Classes) == 0 && edgeCtrl.Kind == "traefik" {
			opt.Classes = []string{"traefik"}
		}
	}
	rep.Options = opt
	cl, err := LoadCluster(ctx, k)
	if err != nil {
		errf("load routes: %v", err)
	}
	var routes []Route
	if edgeCtrl != nil {
		routes = append(routes, Kubernetes(cl, opt)...)
	}
	for _, hp := range rep.HostProxies {
		if !containsInt(hp.Ports, 80) && !containsInt(hp.Ports, 443) && hp.Kind != "nectar_gateway" {
			continue
		}
		rs, err := HostRoutes(ctx, h, hp)
		if err != nil {
			errf("%s config %s: %v", hp.Kind, hp.Config, err)
			continue
		}
		routes = append(routes, rs...)
	}
	rep.Routes = Finalize(routes)
	rep.Hosts = Hosts(rep.Routes)
	rep.Counts = Count(rep.Routes)
	if tcp, _ := list(ctx, k, "IngressRouteTCP"); len(tcp) > 0 {
		rep.TCPRoutes += len(tcp)
	}
	if udp, _ := list(ctx, k, "IngressRouteUDP"); len(udp) > 0 {
		rep.TCPRoutes += len(udp)
	}

	// --- TLS
	if crds, err := list(ctx, k, "CustomResourceDefinition"); err == nil {
		for _, c := range crds {
			if kube.Str(c, "metadata", "name") == "certificates.cert-manager.io" {
				rep.CertManager = true
			}
		}
	}
	if rep.CertManager {
		if iss, err := list(ctx, k, "ClusterIssuer"); err == nil {
			for _, i := range iss {
				rep.Issuers = append(rep.Issuers, kube.Str(i, "metadata", "name"))
			}
		}
	}
	for _, s := range cl.Secrets {
		if kube.Str(s, "type") == "kubernetes.io/tls" {
			rep.TLSSecrets++
		}
	}

	rep.assess()
	return rep, nil
}

func (r *Report) edgeController() *Controller {
	for i := range r.Controllers {
		if r.Controllers[i].HoldsEdge {
			return &r.Controllers[i]
		}
	}
	return nil
}

// EdgeController returns the in-cluster controller serving 80/443 (nil if none).
func (r *Report) EdgeController() *Controller { return r.edgeController() }

// EdgeHostProxy returns the host proxy serving 80/443 (nil if none).
func (r *Report) EdgeHostProxy() *HostProxy {
	for i := range r.HostProxies {
		if containsInt(r.HostProxies[i].Ports, 80) || containsInt(r.HostProxies[i].Ports, 443) {
			return &r.HostProxies[i]
		}
	}
	return nil
}

// HostRoutes translates the configuration of a host proxy.
func HostRoutes(ctx context.Context, h Host, hp HostProxy) ([]Route, error) {
	switch hp.Kind {
	case "nginx":
		rs, _, err := Nginx(h, hp.Config)
		return rs, err
	case "caddy":
		if strings.HasSuffix(hp.Config, ".json") {
			return nil, fmt.Errorf("Caddy JSON configs are not supported (use a Caddyfile)")
		}
		return Caddy(h, hp.Config)
	case "haproxy":
		return HAProxy(h, hp.Config)
	case "apache":
		rs, _, err := Apache(h, hp.Config)
		return rs, err
	case "nectar_gateway":
		if GatewayDBDriver == "" {
			return nil, fmt.Errorf("no SQLite driver")
		}
		rs, _, err := GatewayDB(ctx, GatewayDBDriver, h.p(hp.Config))
		return rs, err
	}
	return nil, fmt.Errorf("unknown proxy %s", hp.Kind)
}

// GatewayDBDriver is the database/sql driver used to read a standalone nectar_gateway DB (set by main).
var GatewayDBDriver = ""

var flagRe = regexp.MustCompile(`(?:^|\s)-(?:c|f|-config|-conf)[=\s]+(\S+)`)

func hostProxyConfig(h Host, hp HostProxy) string {
	if m := flagRe.FindStringSubmatch(hp.Command); m != nil && hp.Kind != "nectar_gateway" {
		return m[1]
	}
	candidates := map[string][]string{
		"nginx":   {"/etc/nginx/nginx.conf", "/usr/local/nginx/conf/nginx.conf", "/usr/local/etc/nginx/nginx.conf"},
		"caddy":   {"/etc/caddy/Caddyfile", "/etc/caddy/caddy.json"},
		"apache":  {"/etc/apache2/apache2.conf", "/etc/httpd/conf/httpd.conf"},
		"haproxy": {"/etc/haproxy/haproxy.cfg"},
	}[hp.Kind]
	if hp.Kind == "nectar_gateway" {
		// DB_PATH from the process environment or the unit file, relative to its working directory
		dbPath, wd := "", ""
		if hp.PID > 0 {
			if env, err := h.ReadFile(fmt.Sprintf("/proc/%d/environ", hp.PID)); err == nil {
				for _, kv := range strings.Split(string(env), "\x00") {
					if v, ok := strings.CutPrefix(kv, "DB_PATH="); ok {
						dbPath = v
					}
				}
			}
		}
		if hp.Unit != "" {
			if b, err := h.ReadFile("/etc/systemd/system/" + hp.Unit); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					l = strings.TrimSpace(l)
					if v, ok := strings.CutPrefix(l, "WorkingDirectory="); ok {
						wd = v
					}
					if v, ok := strings.CutPrefix(l, "Environment="); ok {
						v = strings.Trim(v, `"`)
						if p, ok := strings.CutPrefix(v, "DB_PATH="); ok && dbPath == "" {
							dbPath = p
						}
					}
				}
			}
		}
		if dbPath == "" {
			dbPath = "nectar.db"
		}
		if !strings.HasPrefix(dbPath, "/") {
			if wd == "" {
				wd = "/opt/nectar_gateway"
			}
			dbPath = wd + "/" + dbPath
		}
		return dbPath
	}
	for _, c := range candidates {
		if h.Exists(c) {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func discoverK3s(ctx context.Context, k Kube, h Host, rep *Report) {
	if out, err := h.Run(ctx, "k3s", "--version"); err == nil {
		rep.K3s.Version = strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	}
	rep.K3s.ClusterDomain = "cluster.local"
	var cfgs []string
	if h.Exists("/etc/rancher/k3s/config.yaml") {
		cfgs = append(cfgs, "/etc/rancher/k3s/config.yaml")
	}
	cfgs = append(cfgs, h.Glob("/etc/rancher/k3s/config.yaml.d/*.yaml")...)
	rep.K3s.ConfigFiles = cfgs
	external := false
	for _, f := range cfgs {
		b, err := h.ReadFile(f)
		if err != nil {
			continue
		}
		var m map[string]any
		if yaml.Unmarshal(b, &m) != nil {
			continue
		}
		for key, v := range m {
			switch strings.TrimSuffix(key, "+") {
			case "disable":
				rep.K3s.Disabled = append(rep.K3s.Disabled, toStrings(v)...)
			case "cluster-domain":
				if s, ok := v.(string); ok && s != "" {
					rep.K3s.ClusterDomain = s
				}
			case "datastore-endpoint":
				external = true
			}
		}
	}
	// flags in the service unit (INSTALL_K3S_EXEC)
	for _, unit := range []string{"/etc/systemd/system/k3s.service", "/etc/init.d/k3s"} {
		b, err := h.ReadFile(unit)
		if err != nil {
			continue
		}
		f := strings.Fields(strings.NewReplacer("'", " ", "\\", " ", "\"", " ").Replace(string(b)))
		for i, x := range f {
			switch {
			case x == "--disable" && i+1 < len(f):
				rep.K3s.Disabled = append(rep.K3s.Disabled, strings.Split(f[i+1], ",")...)
			case strings.HasPrefix(x, "--disable="):
				rep.K3s.Disabled = append(rep.K3s.Disabled, strings.Split(strings.TrimPrefix(x, "--disable="), ",")...)
			case strings.HasPrefix(x, "--datastore-endpoint"):
				external = true
			case x == "--cluster-domain" && i+1 < len(f):
				rep.K3s.ClusterDomain = f[i+1]
			}
		}
	}
	rep.K3s.Disabled = dedupe(rep.K3s.Disabled)
	switch {
	case external:
		rep.K3s.Datastore = "external"
	case h.Exists("/var/lib/rancher/k3s/server/db/etcd/member"): // sqlite installs also create db/etcd/name
		rep.K3s.Datastore = "etcd"
	default:
		rep.K3s.Datastore = "sqlite"
	}
	nodes, _ := k.List(ctx, "Node", "", "")
	for _, n := range nodes {
		ni := NodeInfo{Name: kube.Str(n, "metadata", "name")}
		for _, c := range kube.Slice(n, "status", "conditions") {
			if kube.Str(c, "type") == "Ready" {
				ni.Ready = kube.Str(c, "status") == "True"
			}
		}
		for _, a := range kube.Slice(n, "status", "addresses") {
			if kube.Str(a, "type") == "InternalIP" && ni.InternalIP == "" {
				ni.InternalIP = kube.Str(a, "address")
			}
		}
		if labels, ok := kube.Field(n, "metadata", "labels").(map[string]any); ok {
			for l := range labels {
				if r, ok := strings.CutPrefix(l, "node-role.kubernetes.io/"); ok {
					ni.Roles = append(ni.Roles, r)
				}
			}
			sort.Strings(ni.Roles)
		}
		rep.K3s.Nodes = append(rep.K3s.Nodes, ni)
	}
}

// isK3sProcess reports processes of k3s itself (its ServiceLB / hostPort plumbing is reported via the cluster).
func isK3sProcess(comm string) bool {
	return comm == "k3s" || comm == "k3s-server" || comm == "k3s-agent" || strings.HasPrefix(comm, "containerd")
}

func controllerKind(w kube.Object) string {
	for _, c := range kube.Slice(w, "spec", "template", "spec", "containers") {
		img := strings.ToLower(kube.Str(c, "image"))
		for _, ci := range controllerImages {
			if strings.Contains(img, ci.match) {
				if ci.kind == "istio" && !strings.Contains(kube.Str(w, "metadata", "name"), "ingress") {
					return "" // sidecars and egress gateways are not edge proxies
				}
				if ci.kind == "kong" && strings.Contains(img, "kong-gateway-operator") {
					return ""
				}
				return ci.kind
			}
		}
	}
	return ""
}

func selectorMatches(svc kube.Object, labels map[string]any) bool {
	sel, _ := kube.Field(svc, "spec", "selector").(map[string]any)
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func hasPort(svcs []kube.Object, ref string, port int) bool {
	ns, name, _ := strings.Cut(ref, "/")
	for _, s := range svcs {
		if kube.Str(s, "metadata", "namespace") == ns && kube.Str(s, "metadata", "name") == name {
			for _, p := range kube.Slice(s, "spec", "ports") {
				if int(kube.Int(p, "port")) == port {
					return true
				}
			}
		}
	}
	return false
}

func listenerOn(ls []Listener, port int, inPod bool) bool {
	for _, l := range ls {
		if l.Port == port && l.InPod == inPod {
			return true
		}
	}
	return false
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func hasHostKind(hps []HostProxy, kind string) bool {
	for _, h := range hps {
		if h.Kind == kind {
			return true
		}
	}
	return false
}

func toStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return strings.Split(x, ",")
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// assess decides which strategies fit and what blocks a cutover.
func (r *Report) assess() {
	ctrl := r.edgeController()
	hp := r.EdgeHostProxy()
	gwHost := hasHostKind(r.HostProxies, "nectar_gateway")
	if ctrl != nil {
		if len(ctrl.LBIPs) > 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf("%s is reached through LoadBalancer IP %s (not a node IP): move DNS to the node first, or keep it (strategy keep)",
				ctrl.Service, strings.Join(ctrl.LBIPs, ", ")))
		}
		if len(ctrl.ExtraPorts) > 0 && ctrl.ServiceLB {
			r.Blockers = append(r.Blockers, fmt.Sprintf("%s also exposes port(s) %v; switching it to ClusterIP would close them. Expose them with a separate LoadBalancer Service first",
				ctrl.Service, ctrl.ExtraPorts))
		}
		if ctrl.HostNetwork || len(ctrl.HostPorts) > 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf("%s binds the node ports directly (hostNetwork/hostPort): change it to a ClusterIP Service before the cutover", ctrl.Workload))
		}
		if !ctrl.ServiceLB && !ctrl.HostNetwork && len(ctrl.HostPorts) == 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf("%s does not use k3s ServiceLB; adopt mode cannot release its ports automatically", ctrl.Workload))
		}
	}
	if r.TCPRoutes > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d Traefik TCP/UDP route(s) are not migrated; they keep working only on ports Traefik still exposes", r.TCPRoutes))
	}
	if len(r.K3s.Nodes) > 1 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d nodes: the gateway runs on this node only (label nectar.dev/edge=true); make sure DNS points here", len(r.K3s.Nodes)))
	}
	migrate := StrategyOption{Name: StrategyMigrate, Possible: len(r.Blockers) == 0}
	keep := StrategyOption{Name: StrategyKeep, Possible: ctrl != nil && len(ctrl.Classes) > 0}
	adoptGW := StrategyOption{Name: StrategyAdoptGateway, Possible: gwHost}
	switch {
	case r.Edge == "none":
		migrate.Reason = "ports 80/443 are free: nectar-gateway binds them directly"
	case ctrl != nil:
		migrate.Reason = fmt.Sprintf("%s keeps running behind nectar-gateway as the fallback", ctrl.Kind)
	case hp != nil && hp.Kind == "nectar_gateway":
		migrate.Possible = false
		migrate.Reason = "a standalone nectar_gateway serves 80/443: use adopt-gateway"
	case hp != nil:
		migrate.Reason = fmt.Sprintf("host %s is moved to internal ports and keeps running behind nectar-gateway as the fallback", hp.Kind)
	}
	if !migrate.Possible && migrate.Reason == "" {
		migrate.Reason = "blocked: " + strings.Join(r.Blockers, "; ")
	} else if len(r.Blockers) > 0 {
		migrate.Reason += " — blocked: " + strings.Join(r.Blockers, "; ")
	}
	if keep.Possible {
		keep.Reason = fmt.Sprintf("the panel writes Ingress objects (class %s) for your %s", ctrl.Classes[0], ctrl.Kind)
	} else {
		keep.Reason = "needs an in-cluster ingress controller with an IngressClass"
	}
	if gwHost {
		adoptGW.Reason = "imports routes, certificates and blocked IPs of the standalone nectar_gateway into the in-cluster one"
	} else {
		adoptGW.Reason = "no standalone nectar_gateway found"
	}
	switch {
	case adoptGW.Possible:
		adoptGW.Recommended = true
	case migrate.Possible:
		migrate.Recommended = true
	case keep.Possible:
		keep.Recommended = true
	}
	r.Strategies = []StrategyOption{migrate, keep, adoptGW}
}

// Recommended returns the recommended strategy ("" if none is possible).
func (r *Report) Recommended() string {
	for _, s := range r.Strategies {
		if s.Recommended {
			return s.Name
		}
	}
	return ""
}

// Strategy returns a strategy option by name.
func (r *Report) Strategy(name string) (StrategyOption, bool) {
	for _, s := range r.Strategies {
		if s.Name == name {
			return s, true
		}
	}
	return StrategyOption{}, false
}
