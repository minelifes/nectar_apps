package adopt

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"nectar_apps/internal/kube"
)

// Cluster is a read-only snapshot of the objects the Kubernetes translators need.
type Cluster struct {
	Ingresses      []kube.Object
	IngressClasses []kube.Object
	Services       []kube.Object
	Secrets        []kube.Object // TLS / basic-auth secrets referenced by routes (data is read for basic auth)
	IngressRoutes  []kube.Object // traefik.io and traefik.containo.us
	Middlewares    []kube.Object
	HTTPRoutes     []kube.Object
	Gateways       []kube.Object
	GatewayClasses []kube.Object
}

// K8sOptions selects which objects are served by the proxy that is being migrated.
type K8sOptions struct {
	// Controller is the edge controller being moved behind the gateway: "traefik", "ingress-nginx", …
	// It decides controller-specific defaults (ingress-nginx redirects to HTTPS when TLS is set).
	Controller string
	// Classes are the IngressClass names served by that controller; "*" = every Ingress.
	Classes []string
	// ClusterDomain for service DNS names (default cluster.local).
	ClusterDomain string
}

func (o K8sOptions) clusterDomain() string {
	if o.ClusterDomain == "" {
		return "cluster.local"
	}
	return o.ClusterDomain
}

func (o K8sOptions) servesClass(class string, defaultClass string) bool {
	for _, c := range o.Classes {
		if c == "*" || c == class || (class == "" && c == defaultClass && defaultClass != "") {
			return true
		}
	}
	return false
}

func (c Cluster) find(list []kube.Object, ns, name string) kube.Object {
	for _, o := range list {
		if kube.Str(o, "metadata", "namespace") == ns && kube.Str(o, "metadata", "name") == name {
			return o
		}
	}
	return nil
}

// DefaultIngressClass returns the IngressClass marked as the cluster default ("" = none).
func (c Cluster) DefaultIngressClass() string {
	for _, ic := range c.IngressClasses {
		if kube.Str(ic, "metadata", "annotations", "ingressclass.kubernetes.io/is-default-class") == "true" {
			return kube.Str(ic, "metadata", "name")
		}
	}
	return ""
}

// serviceTarget resolves a Service backend to a gateway target (named ports, headless and ExternalName).
func (c Cluster) serviceTarget(r *Route, ns, name string, port any, opt K8sOptions) bool {
	svc := c.find(c.Services, ns, name)
	if svc == nil {
		r.fallback("backend Service %s/%s not found", ns, name)
		return false
	}
	want := 0
	wantName := ""
	switch p := port.(type) {
	case float64:
		want = int(p)
	case int:
		want = p
	case int64:
		want = int(p)
	case string:
		if n, err := strconv.Atoi(p); err == nil {
			want = n
		} else {
			wantName = p
		}
	}
	host := fmt.Sprintf("%s.%s.svc.%s", name, ns, opt.clusterDomain())
	switch kube.Str(svc, "spec", "type") {
	case "ExternalName":
		ext := kube.Str(svc, "spec", "externalName")
		if want == 0 {
			r.fallback("ExternalName Service %s/%s needs a numeric port", ns, name)
			return false
		}
		r.Target = Target{Host: ext, Port: want}
		r.warn("ExternalName Service %s/%s: the gateway connects to %s directly", ns, name, ext)
		return true
	}
	headless := kube.Str(svc, "spec", "clusterIP") == "None"
	ports := kube.Slice(svc, "spec", "ports")
	for _, sp := range ports {
		if (want != 0 && int(kube.Int(sp, "port")) == want) || (wantName != "" && kube.Str(sp, "name") == wantName) {
			p := int(kube.Int(sp, "port"))
			if headless {
				// a headless Service resolves to pod IPs: connect to the target (container) port
				switch tp := sp["targetPort"].(type) {
				case float64:
					p = int(tp)
				case string:
					r.fallback("headless Service %s/%s with a named targetPort %q", ns, name, tp)
					return false
				}
				r.warn("headless Service %s/%s: the gateway connects to the pod IPs from DNS", ns, name)
			}
			if strings.EqualFold(kube.Str(sp, "appProtocol"), "kubernetes.io/h2c") {
				r.BackendProtocol = "h2c"
			}
			r.Target = Target{Host: host, Port: p}
			return true
		}
	}
	if want != 0 && len(ports) == 0 {
		r.Target = Target{Host: host, Port: want}
		return true
	}
	r.fallback("port %v not found on Service %s/%s", port, ns, name)
	return false
}

// -------------------------------------------------------------------- Ingress --

var (
	// annotations that change nothing the gateway does differently (or only the controller's own setup)
	ignoredAnnotationPrefixes = []string{
		"kubectl.kubernetes.io/", "meta.helm.sh/", "cert-manager.io/", "acme.cert-manager.io/", "argocd.argoproj.io/",
		"fluxcd.io/", "kustomize.toolkit.fluxcd.io/", "helm.sh/", "external-dns.alpha.kubernetes.io/", "field.cattle.io/",
		"kubernetes.io/ingress.class", "kubernetes.io/tls-acme", "deployment.kubernetes.io/", "nectar.dev/",
	}
	nginxIgnored = map[string]bool{
		"proxy-connect-timeout": true, "proxy-buffering": true, "proxy-buffer-size": true, "proxy-buffers-number": true,
		"proxy-request-buffering": true, "proxy-http-version": true, "proxy-max-temp-file-size": true,
		"proxy-next-upstream": true, "proxy-next-upstream-timeout": true, "proxy-next-upstream-tries": true,
		"proxy-redirect-from": true, "proxy-redirect-to": true, "client-body-buffer-size": true, "ssl-prefer-server-ciphers": true,
		"preserve-trailing-slash": true, "use-http2": true, "http2-push-preload": true, "service-upstream": true,
		"enable-access-log": true, "enable-rewrite-log": true, "upstream-vhost": false, "load-balance": true,
		"server-alias": false, "x-forwarded-prefix": false, "connection-proxy-header": true, "custom-http-errors": false,
	}
	traefikIgnored = map[string]bool{
		"router.entrypoints": true, "router.priority": true, "router.tls": true, "router.tls.options": true,
		"router.tls.domains.0.main": true, "router.tls.domains.0.sans": true, "service.passhostheader": true,
		"service.sticky.cookie": false, "router.observability.accesslogs": true, "router.observability.metrics": true,
		"router.observability.tracing": true,
	}
)

// Ingresses translates networking.k8s.io/v1 Ingress objects served by the migrated controller.
// Ingresses of other classes are skipped (they are not served on this proxy's ports).
func Ingresses(c Cluster, opt K8sOptions) []Route {
	var out []Route
	defClass := c.DefaultIngressClass()
	for _, ing := range c.Ingresses {
		ns, name := kube.Str(ing, "metadata", "namespace"), kube.Str(ing, "metadata", "name")
		class := kube.Str(ing, "spec", "ingressClassName")
		if class == "" {
			class = kube.Str(ing, "metadata", "annotations", "kubernetes.io/ingress.class")
		}
		if !opt.servesClass(class, defClass) {
			continue
		}
		out = append(out, ingressRoutes(c, ing, ns, name, opt)...)
	}
	return out
}

func ingressRoutes(c Cluster, ing kube.Object, ns, name string, opt K8sOptions) []Route {
	ref := ns + "/" + name
	base := Route{Source: SourceIngress, Ref: ref, Status: StatusOK}
	ann, _ := kube.Field(ing, "metadata", "annotations").(map[string]any)
	tlsHosts := map[string]string{} // host → secret ("" = controller default certificate)
	for _, t := range kube.Slice(ing, "spec", "tls") {
		secret := kube.Str(t, "secretName")
		hosts, _ := t["hosts"].([]any)
		for _, h := range hosts {
			if hs, ok := h.(string); ok {
				tlsHosts[normHost(hs)] = secret
			}
		}
	}
	applyIngressAnnotations(c, &base, ns, ann, len(tlsHosts) > 0, opt)

	var out []Route
	if db := kube.Field(ing, "spec", "defaultBackend"); db != nil {
		r := base
		r.fallback("defaultBackend (catch-all) stays on the old proxy")
		out = append(out, r)
	}
	for _, rule := range kube.Slice(ing, "spec", "rules") {
		host := normHost(kube.Str(rule, "host"))
		paths := kube.Slice(rule, "http", "paths")
		if len(paths) == 0 {
			r := base
			r.Host = host
			r.fallback("rule without http paths")
			out = append(out, r)
			continue
		}
		for _, p := range paths {
			r := base
			r.Notes = append([]string(nil), base.Notes...)
			r.Host, r.PathPrefix = host, normPath(kube.Str(p, "path"))
			switch kube.Str(p, "pathType") {
			case "Exact":
				if r.PathPrefix != "/" {
					r.warn("pathType Exact %s is served as a prefix", r.PathPrefix)
				}
			case "ImplementationSpecific", "":
				if strings.ContainsAny(r.PathPrefix, "*()[]{}^$|?+\\") {
					r.fallback("regular-expression path %q", r.PathPrefix)
				}
			}
			if strings.HasPrefix(host, "*.") {
				r.warn("wildcard host: certificates are issued per subdomain")
			}
			if svc := kube.Field(p, "backend", "service"); svc != nil && r.RedirectURL == "" {
				port := kube.Field(p, "backend", "service", "port", "number")
				if pn := kube.Str(p, "backend", "service", "port", "name"); pn != "" {
					port = pn
				}
				c.serviceTarget(&r, ns, kube.Str(p, "backend", "service", "name"), port, opt)
			} else if kube.Field(p, "backend", "resource") != nil {
				r.fallback("resource backend (not a Service)")
			}
			if secret, ok := tlsHosts[host]; ok {
				r.TLS = true
				if secret != "" {
					r.Cert = &CertRef{Secret: ns + "/" + secret}
					if c.find(c.Secrets, ns, secret) == nil && c.Secrets != nil {
						r.warn("TLS Secret %s/%s does not exist yet: the gateway uses its own ACME certificate until it does", ns, secret)
					}
				} else {
					r.warn("TLS without a secretName (controller default certificate): the gateway issues its own certificate")
				}
			}
			if r.ForceHTTPS && !r.TLS {
				r.ForceHTTPS = false // redirect only applies to TLS hosts (ingress-nginx semantics)
			}
			out = append(out, r)
		}
	}
	return out
}

func applyIngressAnnotations(c Cluster, r *Route, ns string, ann map[string]any, hasTLS bool, opt K8sOptions) {
	nginx := opt.Controller == "ingress-nginx"
	if nginx && hasTLS {
		r.ForceHTTPS = true // ingress-nginx default: ssl-redirect=true when the Ingress has TLS
	}
	keys := make([]string, 0, len(ann))
	for k := range ann {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := ann[k].(string)
		if ignoredAnnotation(k) {
			continue
		}
		switch {
		case strings.HasPrefix(k, "nginx.ingress.kubernetes.io/"):
			nginxAnnotation(c, r, ns, strings.TrimPrefix(k, "nginx.ingress.kubernetes.io/"), v, ann)
		case strings.HasPrefix(k, "traefik.ingress.kubernetes.io/"):
			traefikAnnotation(c, r, ns, strings.TrimPrefix(k, "traefik.ingress.kubernetes.io/"), v)
		case strings.Contains(k, "ingress") || strings.Contains(k, "haproxy") || strings.Contains(k, "konghq") ||
			strings.Contains(k, "projectcontour") || strings.Contains(k, "alb."):
			r.fallback("annotation %s is not supported", k)
		}
	}
}

func ignoredAnnotation(k string) bool {
	for _, p := range ignoredAnnotationPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func nginxAnnotation(c Cluster, r *Route, ns, k, v string, ann map[string]any) {
	switch k {
	case "ssl-redirect", "force-ssl-redirect":
		r.ForceHTTPS = v == "true"
	case "proxy-body-size":
		if mb, ok := parseSizeMB(v); ok {
			r.MaxBodyMB = mb
		} else {
			r.warn("proxy-body-size %q not understood, no limit applied", v)
		}
	case "proxy-read-timeout", "proxy-send-timeout":
		if n, err := strconv.Atoi(strings.TrimSuffix(v, "s")); err == nil && n > r.TimeoutSeconds {
			r.TimeoutSeconds = n
		}
	case "backend-protocol":
		switch strings.ToUpper(v) {
		case "HTTP":
		case "HTTPS":
			r.Target.HTTPS = true
			r.warn("HTTPS backend: the gateway does not verify the backend certificate")
		case "GRPC", "H2C":
			r.BackendProtocol = "h2c"
		default:
			r.fallback("backend-protocol %s is not supported", v)
		}
	case "permanent-redirect", "temporal-redirect":
		r.RedirectURL = v
		r.RedirectCode = 301
		if k == "temporal-redirect" {
			r.RedirectCode = 302
		}
		if code, _ := ann["nginx.ingress.kubernetes.io/"+k+"-code"].(string); code != "" {
			r.RedirectCode, _ = strconv.Atoi(code)
		}
	case "permanent-redirect-code", "temporal-redirect-code":
	case "auth-type":
		if v != "basic" {
			r.fallback("auth-type %s is not supported", v)
		}
	case "auth-secret":
		secretNS, secretName := ns, v
		if a, b, ok := strings.Cut(v, "/"); ok {
			secretNS, secretName = a, b
		}
		format, _ := ann["nginx.ingress.kubernetes.io/auth-secret-type"].(string)
		htpasswd, err := c.basicAuthFromSecret(secretNS, secretName, format == "auth-map")
		if err != nil {
			r.fallback("basic auth: %v", err)
		} else {
			r.Htpasswd, r.BasicAuth = htpasswd, "Secret "+secretNS+"/"+secretName
		}
	case "auth-secret-type", "auth-realm":
	case "affinity", "session-cookie-name", "session-cookie-path", "session-cookie-expires", "session-cookie-max-age", "affinity-mode":
		if k == "affinity" {
			r.warn("sticky sessions (affinity) are not kept: requests are balanced by the Service")
		}
	default:
		if nginxIgnored[k] {
			return
		}
		r.fallback("annotation nginx.ingress.kubernetes.io/%s is not supported", k)
	}
}

func traefikAnnotation(c Cluster, r *Route, ns, k, v string) {
	switch {
	case k == "router.middlewares":
		for _, ref := range strings.Split(v, ",") {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			// "<namespace>-<name>@kubernetescrd"
			nsName, provider, _ := strings.Cut(ref, "@")
			if provider != "" && provider != "kubernetescrd" {
				r.fallback("middleware %s from provider %s", nsName, provider)
				continue
			}
			mw := c.middlewareByRef(nsName)
			if mw == nil {
				r.fallback("middleware %s not found", ref)
				continue
			}
			c.applyMiddleware(r, mw)
		}
	case k == "router.tls.certresolver":
		r.warn("certificate came from Traefik's ACME resolver %q: the gateway issues its own (passthrough to Traefik until then)", v)
		r.TLS = true
	case k == "service.serversscheme":
		switch v {
		case "https":
			r.Target.HTTPS = true
			r.warn("HTTPS backend: the gateway does not verify the backend certificate")
		case "h2c":
			r.BackendProtocol = "h2c"
		}
	case k == "router.pathmatcher":
		if v != "PathPrefix" {
			r.fallback("router.pathmatcher %s", v)
		}
	default:
		if traefikIgnored[k] {
			if k == "router.tls" && v == "true" {
				r.TLS = true
			}
			return
		}
		r.fallback("annotation traefik.ingress.kubernetes.io/%s is not supported", k)
	}
}

// middlewareByRef finds "<namespace>-<name>" (Traefik's annotation form; namespaces may contain dashes).
func (c Cluster) middlewareByRef(nsName string) kube.Object {
	for _, mw := range c.Middlewares {
		if kube.Str(mw, "metadata", "namespace")+"-"+kube.Str(mw, "metadata", "name") == nsName {
			return mw
		}
	}
	return nil
}

// applyMiddleware maps the Traefik middlewares the gateway can do; anything else keeps the host on Traefik.
func (c Cluster) applyMiddleware(r *Route, mw kube.Object) {
	ns, name := kube.Str(mw, "metadata", "namespace"), kube.Str(mw, "metadata", "name")
	spec, _ := mw["spec"].(map[string]any)
	for kind := range spec {
		switch kind {
		case "redirectScheme":
			if kube.Str(mw, "spec", "redirectScheme", "scheme") == "https" {
				r.ForceHTTPS = true
			} else {
				r.fallback("middleware %s/%s: redirectScheme to %s", ns, name, kube.Str(mw, "spec", "redirectScheme", "scheme"))
			}
		case "basicAuth":
			secret := kube.Str(mw, "spec", "basicAuth", "secret")
			htpasswd, err := c.basicAuthFromSecret(ns, secret, false)
			if err != nil {
				r.fallback("middleware %s/%s: %v", ns, name, err)
				continue
			}
			r.Htpasswd, r.BasicAuth = htpasswd, "Middleware "+ns+"/"+name
			if kube.Field(mw, "spec", "basicAuth", "removeHeader") == true {
				r.warn("middleware %s/%s: the Authorization header is still passed to the backend", ns, name)
			}
		case "compress", "retry", "buffering":
			if kind == "buffering" {
				if mb := kube.Int(mw, "spec", "buffering", "maxRequestBodyBytes"); mb > 0 {
					r.MaxBodyMB = int((mb + (1 << 20) - 1) >> 20)
				}
			}
		default:
			r.fallback("middleware %s/%s (%s) is not supported", ns, name, kind)
		}
	}
}

// basicAuthFromSecret reads htpasswd lines from a Secret: key "auth" or "users" (htpasswd file), a
// kubernetes.io/basic-auth Secret (username/password, hashed here), or an auth-map (user → hash keys).
// Only bcrypt hashes are accepted (that is what the gateway verifies).
func (c Cluster) basicAuthFromSecret(ns, name string, authMap bool) (string, error) {
	s := c.find(c.Secrets, ns, name)
	if s == nil {
		return "", fmt.Errorf("Secret %s/%s not found", ns, name)
	}
	data, _ := s["data"].(map[string]any)
	dec := func(k string) string {
		v, _ := data[k].(string)
		b, _ := base64.StdEncoding.DecodeString(v)
		return string(b)
	}
	var lines []string
	switch {
	case kube.Str(s, "type") == "kubernetes.io/basic-auth":
		u, p := dec("username"), dec("password")
		if u == "" || p == "" {
			return "", fmt.Errorf("Secret %s/%s has no username/password", ns, name)
		}
		h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		lines = append(lines, u+":"+string(h))
	case authMap:
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, k+":"+strings.TrimSpace(dec(k)))
		}
	default:
		text := dec("auth")
		if text == "" {
			text = dec("users")
		}
		for _, l := range strings.Split(text, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("Secret %s/%s has no users", ns, name)
	}
	return checkHtpasswd(lines)
}

// checkHtpasswd validates htpasswd lines and joins them (bcrypt only).
func checkHtpasswd(lines []string) (string, error) {
	for _, l := range lines {
		user, hash, ok := strings.Cut(l, ":")
		if !ok || user == "" {
			return "", fmt.Errorf("invalid htpasswd line")
		}
		if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
			return "", fmt.Errorf("user %q uses a non-bcrypt hash (re-create it with `htpasswd -B`)", user)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// parseSizeMB parses nginx sizes ("8m", "1g", "512k", "0" = unlimited) into whole MiB (rounded up).
func parseSizeMB(v string) (int, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 0, false
	}
	mult := 1.0 / (1 << 20)
	switch v[len(v)-1] {
	case 'k':
		mult, v = 1.0/1024, v[:len(v)-1]
	case 'm':
		mult, v = 1, v[:len(v)-1]
	case 'g':
		mult, v = 1024, v[:len(v)-1]
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	mb := f * mult
	n := int(mb)
	if float64(n) < mb {
		n++
	}
	return n, true
}

// --------------------------------------------------------------- IngressRoute --

var (
	matcherRe = regexp.MustCompile("^(Host|PathPrefix|Path)\\(\\s*((?:`[^`]*`\\s*,?\\s*)+)\\)$")
	backtick  = regexp.MustCompile("`([^`]*)`")
)

// parseTraefikMatch understands Host(`a`[,`b`]) and PathPrefix/Path joined by && and ||-joined Host
// alternatives in parentheses: `Host(`a`) && PathPrefix(`/x`)`, `(Host(`a`) || Host(`b`)) && Path(`/x`)`.
func parseTraefikMatch(match string) (hosts, prefixes []string, exact bool, err error) {
	m := strings.TrimSpace(match)
	for _, part := range splitTop(m, "&&") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
			part = strings.TrimSpace(part[1 : len(part)-1])
		}
		for _, alt := range splitTop(part, "||") {
			alt = strings.TrimSpace(alt)
			mm := matcherRe.FindStringSubmatch(alt)
			if mm == nil {
				return nil, nil, false, fmt.Errorf("matcher %q is not supported", alt)
			}
			var vals []string
			for _, b := range backtick.FindAllStringSubmatch(mm[2], -1) {
				vals = append(vals, b[1])
			}
			switch mm[1] {
			case "Host":
				hosts = append(hosts, vals...)
			case "PathPrefix":
				prefixes = append(prefixes, vals...)
			case "Path":
				prefixes = append(prefixes, vals...)
				exact = true
			}
		}
	}
	if len(hosts) == 0 {
		return nil, nil, false, fmt.Errorf("rule without Host()")
	}
	if len(prefixes) == 0 {
		prefixes = []string{"/"}
	}
	return hosts, prefixes, exact, nil
}

// splitTop splits s on sep outside parentheses and backticks.
func splitTop(s, sep string) []string {
	var out []string
	depth, inTick, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '`':
			inTick = !inTick
		case '(':
			if !inTick {
				depth++
			}
		case ')':
			if !inTick {
				depth--
			}
		default:
			if !inTick && depth == 0 && strings.HasPrefix(s[i:], sep) {
				out = append(out, s[start:i])
				i += len(sep) - 1
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// IngressRoutes translates Traefik IngressRoute objects (HTTP routers).
func IngressRoutes(c Cluster, opt K8sOptions) []Route {
	var out []Route
	for _, ir := range c.IngressRoutes {
		ns, name := kube.Str(ir, "metadata", "namespace"), kube.Str(ir, "metadata", "name")
		tlsSecret := kube.Str(ir, "spec", "tls", "secretName")
		hasTLS := kube.Field(ir, "spec", "tls") != nil
		resolver := kube.Str(ir, "spec", "tls", "certResolver")
		for i, rt := range kube.Slice(ir, "spec", "routes") {
			base := Route{Source: SourceIngressRoute, Ref: fmt.Sprintf("%s/%s#%d", ns, name, i), Status: StatusOK}
			if kind := kube.Str(rt, "kind"); kind != "" && kind != "Rule" {
				base.fallback("route kind %s", kind)
			}
			hosts, prefixes, exact, err := parseTraefikMatch(kube.Str(rt, "match"))
			if err != nil {
				base.fallback("%v", err)
				out = append(out, base)
				continue
			}
			if exact {
				base.warn("Path() is served as a prefix")
			}
			for _, mref := range kube.Slice(rt, "middlewares") {
				mns := kube.Str(mref, "namespace")
				if mns == "" {
					mns = ns
				}
				mw := c.find(c.Middlewares, mns, kube.Str(mref, "name"))
				if mw == nil {
					base.fallback("middleware %s/%s not found", mns, kube.Str(mref, "name"))
					continue
				}
				c.applyMiddleware(&base, mw)
			}
			svcs := kube.Slice(rt, "services")
			switch {
			case len(svcs) != 1:
				base.fallback("%d services (weighted or mirrored routing) is not supported", len(svcs))
			case kube.Str(svcs[0], "kind") != "" && kube.Str(svcs[0], "kind") != "Service":
				base.fallback("service kind %s", kube.Str(svcs[0], "kind"))
			}
			if hasTLS {
				base.TLS = true
				switch {
				case tlsSecret != "":
					base.Cert = &CertRef{Secret: ns + "/" + tlsSecret}
				case resolver != "":
					base.warn("certificate came from Traefik's ACME resolver %q: the gateway issues its own (passthrough to Traefik until then)", resolver)
				default:
					base.warn("TLS with Traefik's default certificate: the gateway issues its own")
				}
			}
			for _, h := range hosts {
				for _, p := range prefixes {
					r := base
					r.Notes = append([]string(nil), base.Notes...)
					r.Host, r.PathPrefix = normHost(h), normPath(p)
					if len(svcs) == 1 && r.Migratable() {
						sns := kube.Str(svcs[0], "namespace")
						if sns == "" {
							sns = ns
						}
						c.serviceTarget(&r, sns, kube.Str(svcs[0], "name"), svcs[0]["port"], opt)
						switch kube.Str(svcs[0], "scheme") {
						case "https":
							r.Target.HTTPS = true
							r.warn("HTTPS backend: the gateway does not verify the backend certificate")
						case "h2c":
							r.BackendProtocol = "h2c"
						}
					}
					out = append(out, r)
				}
			}
		}
	}
	return out
}

// ------------------------------------------------------------------ HTTPRoute --

// HTTPRoutes translates Gateway API HTTPRoutes attached to Gateways of the migrated controller.
func HTTPRoutes(c Cluster, opt K8sOptions) []Route {
	classController := map[string]string{}
	for _, gc := range c.GatewayClasses {
		classController[kube.Str(gc, "metadata", "name")] = kube.Str(gc, "spec", "controllerName")
	}
	var out []Route
	for _, hr := range c.HTTPRoutes {
		ns, name := kube.Str(hr, "metadata", "namespace"), kube.Str(hr, "metadata", "name")
		var gws []kube.Object
		served := false
		for _, pr := range kube.Slice(hr, "spec", "parentRefs") {
			gns := kube.Str(pr, "namespace")
			if gns == "" {
				gns = ns
			}
			gw := c.find(c.Gateways, gns, kube.Str(pr, "name"))
			if gw == nil {
				continue
			}
			gws = append(gws, gw)
			ctrl := classController[kube.Str(gw, "spec", "gatewayClassName")]
			if opt.Controller != "" && strings.Contains(strings.ToLower(ctrl), strings.ToLower(strings.TrimPrefix(opt.Controller, "ingress-"))) {
				served = true
			}
		}
		if !served {
			continue
		}
		var hostnames []string
		if hs, ok := kube.Field(hr, "spec", "hostnames").([]any); ok {
			for _, h := range hs {
				if s, ok := h.(string); ok {
					hostnames = append(hostnames, s)
				}
			}
		}
		for i, rule := range kube.Slice(hr, "spec", "rules") {
			base := Route{Source: SourceHTTPRoute, Ref: fmt.Sprintf("%s/%s#%d", ns, name, i), Status: StatusOK}
			if len(hostnames) == 0 {
				base.fallback("HTTPRoute without hostnames")
			}
			prefixes := []string{"/"}
			if ms := kube.Slice(rule, "matches"); len(ms) > 0 {
				prefixes = nil
				for _, m := range ms {
					for k := range m {
						if k != "path" {
							base.fallback("%s matches are not supported", k)
						}
					}
					switch kube.Str(m, "path", "type") {
					case "", "PathPrefix":
					case "Exact":
						base.warn("Exact path is served as a prefix")
					default:
						base.fallback("path type %s", kube.Str(m, "path", "type"))
					}
					v := kube.Str(m, "path", "value")
					if v == "" {
						v = "/"
					}
					prefixes = append(prefixes, v)
				}
			}
			for _, f := range kube.Slice(rule, "filters") {
				switch kube.Str(f, "type") {
				case "RequestRedirect":
					if kube.Str(f, "requestRedirect", "scheme") == "https" && kube.Str(f, "requestRedirect", "hostname") == "" &&
						kube.Field(f, "requestRedirect", "path") == nil {
						base.ForceHTTPS = true
					} else {
						base.fallback("RequestRedirect filter other than scheme https")
					}
				default:
					base.fallback("%s filter is not supported", kube.Str(f, "type"))
				}
			}
			refs := kube.Slice(rule, "backendRefs")
			if len(refs) > 1 {
				base.fallback("%d backendRefs (traffic splitting) is not supported", len(refs))
			}
			if len(refs) == 1 && kube.Str(refs[0], "kind") != "" && kube.Str(refs[0], "kind") != "Service" {
				base.fallback("backendRef kind %s", kube.Str(refs[0], "kind"))
			}
			for _, h := range hostnames {
				for _, p := range prefixes {
					r := base
					r.Notes = append([]string(nil), base.Notes...)
					r.Host, r.PathPrefix = normHost(h), normPath(p)
					if len(refs) == 1 && r.Migratable() {
						bns := kube.Str(refs[0], "namespace")
						if bns == "" {
							bns = ns
						}
						c.serviceTarget(&r, bns, kube.Str(refs[0], "name"), refs[0]["port"], opt)
					} else if len(refs) == 0 && !r.ForceHTTPS {
						r.fallback("rule without backendRefs")
					}
					if cert := gatewayCert(gws, r.Host); cert != "" {
						r.TLS, r.Cert = true, &CertRef{Secret: cert}
					}
					out = append(out, r)
				}
			}
		}
	}
	return mergeRedirectOnly(out)
}

// mergeRedirectOnly folds HTTP→HTTPS redirect rules (typically a separate HTTPRoute on the http listener)
// into the real rule of the same host and path.
func mergeRedirectOnly(rs []Route) []Route {
	redirect := map[string]bool{}
	for _, r := range rs {
		if r.ForceHTTPS && r.Target.Host == "" && r.Migratable() {
			redirect[r.Key()] = true
		}
	}
	var out []Route
	for _, r := range rs {
		if r.ForceHTTPS && r.Target.Host == "" && r.Migratable() {
			continue
		}
		if redirect[r.Key()] {
			r.ForceHTTPS = true
		}
		out = append(out, r)
	}
	return out
}

// gatewayCert returns "namespace/secret" of the first HTTPS listener matching host.
func gatewayCert(gws []kube.Object, host string) string {
	for _, gw := range gws {
		gns := kube.Str(gw, "metadata", "namespace")
		for _, l := range kube.Slice(gw, "spec", "listeners") {
			if kube.Str(l, "protocol") != "HTTPS" {
				continue
			}
			lh := normHost(kube.Str(l, "hostname"))
			if lh != "" && lh != host && !(strings.HasPrefix(lh, "*.") && strings.HasSuffix(host, lh[1:])) {
				continue
			}
			refs := kube.Slice(l, "tls", "certificateRefs")
			if len(refs) == 0 {
				continue
			}
			ns := kube.Str(refs[0], "namespace")
			if ns == "" {
				ns = gns
			}
			return ns + "/" + kube.Str(refs[0], "name")
		}
	}
	return ""
}

// Kubernetes translates everything served by the migrated controller.
func Kubernetes(c Cluster, opt K8sOptions) []Route {
	var out []Route
	out = append(out, Ingresses(c, opt)...)
	if opt.Controller == "traefik" {
		out = append(out, IngressRoutes(c, opt)...)
	}
	out = append(out, HTTPRoutes(c, opt)...)
	return out
}
