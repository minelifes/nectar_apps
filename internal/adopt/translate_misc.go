package adopt

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ===================================================================== Caddy ==

type cdNode struct {
	Args  []string
	Block []cdNode
	Line  int
}

// caddyTokens splits a Caddyfile into lines of tokens (quotes and comments handled), keeping braces as tokens.
func caddyTokens(src string) [][]string {
	var lines [][]string
	for _, raw := range strings.Split(src, "\n") {
		var toks []string
		var cur strings.Builder
		inQ := false
		flush := func() {
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
		}
		for i := 0; i < len(raw); i++ {
			c := raw[i]
			switch {
			case inQ && c == '"':
				inQ = false
				toks = append(toks, cur.String())
				cur.Reset()
			case inQ:
				cur.WriteByte(c)
			case c == '"':
				flush()
				inQ = true
			case c == '#' && cur.Len() == 0:
				i = len(raw)
			case c == ' ' || c == '\t' || c == '\r':
				flush()
			default:
				cur.WriteByte(c)
			}
		}
		flush()
		lines = append(lines, toks)
	}
	return lines
}

func caddyParse(lines [][]string, i *int, depth int) ([]cdNode, error) {
	var out []cdNode
	for *i < len(lines) {
		toks := lines[*i]
		line := *i + 1
		*i++
		if len(toks) == 0 {
			continue
		}
		if len(toks) == 1 && toks[0] == "}" {
			if depth == 0 {
				return nil, fmt.Errorf("line %d: unexpected }", line)
			}
			return out, nil
		}
		n := cdNode{Line: line}
		if toks[len(toks)-1] == "{" {
			n.Args = toks[:len(toks)-1]
			blk, err := caddyParse(lines, i, depth+1)
			if err != nil {
				return nil, err
			}
			n.Block = blk
			if n.Block == nil {
				n.Block = []cdNode{}
			}
		} else {
			n.Args = toks
		}
		out = append(out, n)
	}
	if depth > 0 {
		return nil, fmt.Errorf("unexpected end of Caddyfile (missing })")
	}
	return out, nil
}

var caddyIgnored = map[string]bool{"encode": true, "log": true, "metrics": true, "tracing": true, "skip_log": true, "log_skip": true}

// Caddy translates a Caddyfile (sites on the default HTTPS ports). JSON configs are not read.
func Caddy(h Host, file string) ([]Route, error) {
	b, err := h.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lines := caddyTokens(string(b))
	i := 0
	nodes, err := caddyParse(lines, &i, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var out []Route
	for idx, n := range nodes {
		if idx == 0 && len(n.Args) == 0 && n.Block != nil {
			// global options; custom ports change where sites listen
			for _, o := range n.Block {
				if len(o.Args) > 1 && (o.Args[0] == "http_port" || o.Args[0] == "https_port") && o.Args[1] != "80" && o.Args[1] != "443" {
					return nil, fmt.Errorf("%s: custom %s %s is not supported", file, o.Args[0], o.Args[1])
				}
			}
			continue
		}
		if len(n.Args) > 0 && strings.HasPrefix(n.Args[0], "(") {
			continue // snippet definition
		}
		if n.Block == nil {
			continue
		}
		out = append(out, caddySite(h, file, n)...)
	}
	return out, nil
}

func caddySite(h Host, file string, site cdNode) []Route {
	type addr struct {
		host string
		tls  bool
	}
	var addrs []addr
	var out []Route
	ref := fmt.Sprintf("%s:%d", file, site.Line)
	for _, a := range site.Args {
		for _, a := range strings.Split(a, ",") {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			tls := true
			switch {
			case strings.HasPrefix(a, "http://"):
				tls, a = false, strings.TrimPrefix(a, "http://")
			case strings.HasPrefix(a, "https://"):
				a = strings.TrimPrefix(a, "https://")
			}
			host, port := a, ""
			if hp, p, err := net.SplitHostPort(a); err == nil {
				host, port = hp, p
			}
			switch {
			case port != "" && port != "80" && port != "443":
				continue // not on the edge ports
			case port == "80":
				tls = false
			}
			if host == "" || host == "localhost" || net.ParseIP(host) != nil {
				continue // catch-all or IP site: unknown hosts keep going to Caddy
			}
			addrs = append(addrs, addr{normHost(host), tls})
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	matchers := map[string]string{} // @name → path prefix ("" = unsupported matcher)
	base := Route{Source: SourceCaddy, Ref: ref, Status: StatusOK}
	type rule struct {
		prefix string
		r      Route
	}
	var rules []rule
	addRule := func(prefix string, r Route) { rules = append(rules, rule{prefix, r}) }
	var walk func(nodes []cdNode, prefix string, r *Route)
	walk = func(nodes []cdNode, prefix string, cur *Route) {
		for _, d := range nodes {
			if len(d.Args) == 0 {
				continue
			}
			name := d.Args[0]
			args := d.Args[1:]
			// optional matcher token
			mprefix := prefix
			if len(args) > 0 {
				switch m := args[0]; {
				case strings.HasPrefix(m, "@"):
					p, ok := matchers[m]
					if !ok || p == "" {
						cur.fallback("matcher %s is not a simple path matcher", m)
						continue
					}
					mprefix, args = p, args[1:]
				case strings.HasPrefix(m, "/"):
					if name != "redir" || len(args) > 1 {
						p, ok := caddyPathPrefix(m)
						if !ok {
							cur.fallback("path matcher %s", m)
							continue
						}
						mprefix, args = p, args[1:]
					}
				case m == "*":
					args = args[1:]
				}
			}
			switch {
			case strings.HasPrefix(name, "@"):
				p, ok := "", false
				if len(args) == 2 && args[0] == "path" {
					p, ok = caddyPathPrefix(args[1])
				}
				if ok {
					matchers[name] = p
				} else {
					matchers[name] = ""
				}
			case name == "reverse_proxy":
				r := *cur
				r.Notes = append([]string(nil), cur.Notes...)
				caddyProxy(&r, args, d.Block)
				addRule(mprefix, r)
			case name == "handle":
				r := *cur
				r.Notes = append([]string(nil), cur.Notes...)
				walk(d.Block, mprefix, &r)
			case name == "redir":
				if len(args) == 0 {
					cur.fallback("redir without target")
					continue
				}
				r := *cur
				target := args[0]
				code := 302
				if len(args) > 1 {
					switch args[1] {
					case "permanent":
						code = 301
					case "temporary":
						code = 302
					default:
						code, _ = strconv.Atoi(args[1])
					}
				}
				switch {
				case target == "https://{host}{uri}":
					cur.ForceHTTPS = true
					continue
				case strings.HasSuffix(target, "{uri}") && !strings.Contains(strings.TrimSuffix(target, "{uri}"), "{"):
					r.RedirectURL, r.RedirectCode = strings.TrimSuffix(target, "{uri}"), code
				case !strings.Contains(target, "{") && strings.Contains(target, "://"):
					r.RedirectURL, r.RedirectCode = target, code
					r.warn("redirect to %s: the gateway appends the request path", target)
				default:
					r.fallback("redir %s", target)
				}
				addRule(mprefix, r)
			case name == "basicauth" || name == "basic_auth":
				var lines []string
				for _, u := range d.Block {
					if len(u.Args) >= 2 {
						hash := u.Args[1]
						if dec, err := base64.StdEncoding.DecodeString(hash); err == nil && strings.HasPrefix(string(dec), "$2") {
							hash = string(dec)
						}
						lines = append(lines, u.Args[0]+":"+hash)
					}
				}
				if ht, err := checkHtpasswd(lines); err != nil || mprefix != prefix {
					if err == nil {
						err = fmt.Errorf("basic auth on a sub-path")
					}
					cur.fallback("basicauth: %v", err)
				} else {
					cur.Htpasswd, cur.BasicAuth = ht, ref
				}
			case name == "request_body":
				for _, o := range d.Block {
					if len(o.Args) == 2 && o.Args[0] == "max_size" {
						if mb, ok := parseSizeMB(strings.TrimSuffix(strings.ToLower(o.Args[1]), "b")); ok {
							cur.MaxBodyMB = mb
						}
					}
				}
			case name == "tls":
				switch {
				case len(args) == 2 && !strings.Contains(args[0], "@"):
					cur.Cert = &CertRef{CertFile: args[0], KeyFile: args[1]}
				case len(args) == 1 && args[0] == "internal":
					cur.warn("Caddy's internal CA certificate is replaced by a public one from the gateway")
				}
			case name == "header":
				cur.warn("header directive is not applied by the gateway")
			case caddyIgnored[name]:
			case name == "import":
				cur.fallback("import %s is not expanded", strings.Join(args, " "))
			default:
				cur.fallback("directive %s is not supported", name)
			}
		}
	}
	walk(site.Block, "/", &base)
	if len(rules) == 0 {
		base.PathPrefix = "/"
		if base.Status != StatusFallback {
			base.fallback("site has no reverse_proxy")
		}
		rules = append(rules, rule{"/", base})
	}
	for _, a := range addrs {
		for _, ru := range rules {
			r := ru.r
			// settings from the site level apply to every rule
			if base.Htpasswd != "" && r.Htpasswd == "" {
				r.Htpasswd, r.BasicAuth = base.Htpasswd, base.BasicAuth
			}
			if base.Status == StatusFallback {
				r.Status, r.Notes = StatusFallback, append(r.Notes, base.Notes...)
			}
			if base.ForceHTTPS {
				r.ForceHTTPS = true
			}
			if r.MaxBodyMB == 0 {
				r.MaxBodyMB = base.MaxBodyMB
			}
			r.Host, r.PathPrefix, r.TLS = a.host, normPath(ru.prefix), a.tls
			if a.tls {
				r.ForceHTTPS = true // Caddy's automatic HTTPS redirects HTTP
				if base.Cert != nil {
					r.Cert = base.Cert
				} else {
					r.Notes = append(r.Notes, "certificate is managed by Caddy: the gateway issues its own (TLS passes through to Caddy until then)")
				}
			}
			out = append(out, r)
		}
	}
	return out
}

func caddyPathPrefix(m string) (string, bool) {
	switch {
	case strings.HasSuffix(m, "/*"):
		return strings.TrimSuffix(m, "*"), !strings.ContainsAny(strings.TrimSuffix(m, "/*"), "*?[")
	case strings.HasSuffix(m, "*"):
		return strings.TrimSuffix(m, "*"), !strings.ContainsAny(strings.TrimSuffix(m, "*"), "*?[")
	case !strings.ContainsAny(m, "*?["):
		return m, true
	}
	return "", false
}

func caddyProxy(r *Route, upstreams []string, block []cdNode) {
	if len(upstreams) == 0 {
		for _, b := range block {
			if len(b.Args) > 1 && b.Args[0] == "to" {
				upstreams = append(upstreams, b.Args[1:]...)
			}
		}
	}
	if len(upstreams) == 0 {
		r.fallback("reverse_proxy without upstream")
		return
	}
	if len(upstreams) > 1 {
		r.warn("%d upstreams: only %s is used (no load balancing / failover)", len(upstreams), upstreams[0])
	}
	u := upstreams[0]
	if strings.HasPrefix(u, "unix/") || strings.Contains(u, "{") {
		r.fallback("upstream %s", u)
		return
	}
	scheme := "http"
	if s, rest, ok := strings.Cut(u, "://"); ok {
		scheme, u = s, rest
	}
	host, port := u, ""
	if hp, p, err := net.SplitHostPort(u); err == nil {
		host, port = hp, p
	} else if strings.HasPrefix(u, ":") {
		host, port = "127.0.0.1", strings.TrimPrefix(u, ":")
	}
	if host == "localhost" || host == "" {
		host = "127.0.0.1"
	}
	n := 80
	switch scheme {
	case "https":
		n = 443
		r.Target.HTTPS = true
	case "h2c":
		r.BackendProtocol = "h2c"
	}
	if port != "" {
		n, _ = strconv.Atoi(port)
	}
	r.Target.Host, r.Target.Port = host, n
	for _, b := range block {
		if len(b.Args) == 0 {
			continue
		}
		switch b.Args[0] {
		case "to", "lb_policy", "lb_try_duration", "health_uri", "health_interval", "health_timeout", "flush_interval":
		case "header_up":
			if len(b.Args) > 1 && !strings.EqualFold(strings.TrimPrefix(b.Args[1], "+"), "X-Real-IP") &&
				!strings.EqualFold(b.Args[1], "Host") && !strings.HasPrefix(strings.ToLower(b.Args[1]), "x-forwarded") {
				r.warn("header_up %s is not sent to the backend", b.Args[1])
			}
			if len(b.Args) > 2 && strings.EqualFold(b.Args[1], "Host") && b.Args[2] != "{host}" && b.Args[2] != "{http.request.host}" {
				r.fallback("header_up Host %s", b.Args[2])
			}
		case "transport":
			for _, t := range b.Block {
				if len(t.Args) > 0 && (t.Args[0] == "tls" || t.Args[0] == "tls_insecure_skip_verify") {
					r.Target.HTTPS = true
				} else if len(t.Args) > 1 && t.Args[0] == "versions" && t.Args[1] == "h2c" {
					r.BackendProtocol = "h2c"
				}
			}
		default:
			r.warn("reverse_proxy option %s is not applied", b.Args[0])
		}
	}
}

// =================================================================== HAProxy ==

type hapSection struct {
	kind, name string
	line       int
	lines      [][]string
}

func haproxySections(src string) []hapSection {
	var out []hapSection
	sc := bufio.NewScanner(strings.NewReader(src))
	n := 0
	for sc.Scan() {
		n++
		l := sc.Text()
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "global", "defaults", "frontend", "backend", "listen", "userlist", "resolvers", "peers", "cache", "program", "mailers":
			s := hapSection{kind: f[0], line: n}
			if len(f) > 1 {
				s.name = f[1]
			}
			if f[0] == "listen" && len(f) > 2 {
				s.lines = append(s.lines, append([]string{"bind"}, f[2:]...))
			}
			out = append(out, s)
		default:
			if len(out) > 0 {
				out[len(out)-1].lines = append(out[len(out)-1].lines, f)
			}
		}
	}
	return out
}

// HAProxy translates host-based routing (acl hdr(host) + use_backend) of frontends on 80/443.
func HAProxy(h Host, file string) ([]Route, error) {
	b, err := h.ReadFile(file)
	if err != nil {
		return nil, err
	}
	secs := haproxySections(string(b))
	backends := map[string]hapSection{}
	for _, s := range secs {
		if s.kind == "backend" {
			backends[s.name] = s
		}
	}
	var out []Route
	for _, fe := range secs {
		if fe.kind != "frontend" && fe.kind != "listen" {
			continue
		}
		http, tls := false, false
		var crt string
		mode := "http"
		acls := map[string][]string{} // name → conditions "host:x" / "path:/p"
		var uses [][]string
		forceHTTPS := false
		var issues []string
		for _, l := range fe.lines {
			switch l[0] {
			case "bind":
				if len(l) < 2 {
					continue
				}
				_, port, _ := net.SplitHostPort(strings.Replace(l[1], "*", "0.0.0.0", 1))
				hasSSL := false
				for i := 2; i < len(l); i++ {
					if l[i] == "ssl" {
						hasSSL = true
					}
					if l[i] == "crt" && i+1 < len(l) {
						crt = l[i+1]
					}
				}
				switch port {
				case "80":
					http = true
				case "443":
					tls = tls || hasSSL
					if !hasSSL {
						issues = append(issues, "443 without ssl (TCP passthrough)")
					}
				}
			case "mode":
				if len(l) > 1 {
					mode = l[1]
				}
			case "acl":
				if len(l) < 4 {
					continue
				}
				name, fetch := l[1], l[2]
				vals := l[3:]
				for len(vals) > 0 && strings.HasPrefix(vals[0], "-") {
					vals = vals[1:]
				}
				for _, v := range vals {
					switch fetch {
					case "hdr(host)", "req.hdr(host)", "hdr_dom(host)", "req.ssl_sni", "ssl_fc_sni":
						acls[name] = append(acls[name], "host:"+strings.ToLower(v))
					case "path_beg":
						acls[name] = append(acls[name], "path:"+v)
					default:
						acls[name] = append(acls[name], "?:"+fetch)
					}
				}
			case "use_backend":
				uses = append(uses, l[1:])
			case "default_backend":
				// catch-all: unknown hosts keep going to HAProxy
			case "http-request":
				joined := strings.Join(l[1:], " ")
				switch {
				case strings.HasPrefix(joined, "redirect scheme https"):
					forceHTTPS = true
				case strings.HasPrefix(joined, "set-header X-Forwarded"), strings.HasPrefix(joined, "add-header X-Forwarded"):
				default:
					issues = append(issues, "http-request "+joined)
				}
			case "option", "timeout", "maxconn", "log", "compression", "capture", "http-response", "redirect":
				if l[0] == "redirect" && len(l) > 2 && l[1] == "scheme" && l[2] == "https" {
					forceHTTPS = true
				}
				if l[0] == "http-response" {
					issues = append(issues, "http-response rules are not applied")
				}
			default:
				issues = append(issues, "frontend directive "+l[0])
			}
		}
		if !http && !tls {
			continue
		}
		ref := fmt.Sprintf("%s:%d", file, fe.line)
		for _, u := range uses {
			if len(u) < 3 || (u[1] != "if" && u[1] != "unless") {
				continue
			}
			r := Route{Source: SourceHAProxy, Ref: ref, Status: StatusOK, PathPrefix: "/"}
			if u[1] == "unless" {
				r.fallback("use_backend ... unless")
			}
			var hosts []string
			for _, cond := range u[2:] {
				name := strings.TrimPrefix(cond, "!")
				if cond == "or" || cond == "||" {
					continue
				}
				for _, c := range acls[name] {
					kind, v, _ := strings.Cut(c, ":")
					switch {
					case strings.HasPrefix(cond, "!"):
						r.fallback("negated acl %s", name)
					case kind == "host":
						hosts = append(hosts, strings.Split(v, ":")[0])
					case kind == "path":
						r.PathPrefix = v
					default:
						r.fallback("acl %s uses %s", name, v)
					}
				}
				if _, ok := acls[name]; !ok {
					r.fallback("acl %s is not a simple host/path match", name)
				}
			}
			if mode != "http" {
				r.fallback("mode %s", mode)
			}
			for _, is := range issues {
				r.fallback("%s", is)
			}
			hapBackend(&r, backends[u[0]], u[0])
			r.TLS, r.ForceHTTPS = tls, forceHTTPS && tls
			if tls && crt != "" {
				r.Cert = &CertRef{CertFile: crt, KeyFile: crt}
				if strings.HasSuffix(crt, "/") || h.isDir(crt) {
					r.Cert = nil
					r.warn("crt points to a directory: the gateway issues its own certificates")
				}
			}
			for _, host := range hosts {
				rr := r
				rr.Notes = append([]string(nil), r.Notes...)
				rr.Host = host
				out = append(out, rr)
			}
		}
	}
	return out, nil
}

func (h Host) isDir(path string) bool {
	m := h.Glob(filepath.Join(path, "*"))
	return len(m) > 0
}

func hapBackend(r *Route, be hapSection, name string) {
	if be.kind == "" {
		r.fallback("backend %s not found", name)
		return
	}
	var servers [][]string
	for _, l := range be.lines {
		switch l[0] {
		case "server":
			servers = append(servers, l)
		case "mode":
			if len(l) > 1 && l[1] != "http" {
				r.fallback("backend mode %s", l[1])
			}
		case "balance", "option", "timeout", "cookie", "http-check", "default-server", "retries", "log":
			if l[0] == "cookie" {
				r.warn("sticky cookie is not kept")
			}
			if l[0] == "timeout" && len(l) > 2 && l[1] == "server" {
				if n, err := strconv.Atoi(strings.TrimSuffix(l[2], "s")); err == nil {
					r.TimeoutSeconds = n
				}
			}
		default:
			r.fallback("backend directive %s", l[0])
		}
	}
	if len(servers) == 0 {
		r.fallback("backend %s has no server", name)
		return
	}
	if len(servers) > 1 {
		r.warn("backend %s has %d servers: only the first is used (no load balancing / failover)", name, len(servers))
	}
	s := servers[0]
	if len(s) < 3 {
		r.fallback("server line without address")
		return
	}
	host, port, err := net.SplitHostPort(s[2])
	if err != nil {
		host, port = s[2], "80"
	}
	n, _ := strconv.Atoi(port)
	for _, a := range s[3:] {
		if a == "ssl" {
			r.Target.HTTPS = true
		}
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	r.Target.Host, r.Target.Port = host, n
}

// ==================================================================== Apache ==

var apacheIgnored = map[string]bool{
	"serveradmin": true, "errorlog": true, "customlog": true, "loglevel": true, "proxypassreverse": true,
	"proxyrequests": true, "sslengine": true, "sslprotocol": true, "sslciphersuite": true, "sslhonorcipherorder": true,
	"sslcertificatechainfile": true, "protocols": true, "keepalive": true, "servername": true, "serveralias": true,
	"proxypass": true, "sslcertificatefile": true, "sslcertificatekeyfile": true, "proxypreservehost": true,
	"documentroot": true, "rewriteengine": true, "limitrequestbody": true, "proxytimeout": true, "timeout": true,
	"sslproxyengine": true, "sslproxyverify": true, "sslproxycheckpeercn": true, "sslproxychecckpeername": true,
	"sslproxycheckpeername": true, "include": true, "includeoptional": true, "sslopensslconfcmd": true,
	"sslsessiontickets": true, "sslusestapling": true, "sslcompression": true,
}

var rewriteHTTPS = regexp.MustCompile(`^\^.*\s+https://%\{(SERVER_NAME|HTTP_HOST)\}%\{REQUEST_URI\}`)

// Apache translates <VirtualHost> blocks on 80/443 that only proxy (ProxyPass).
func Apache(h Host, mainFile string) ([]Route, []string, error) {
	type vhost struct {
		ref    string
		port   string
		lines  [][]string
		nested []string
	}
	var vhosts []vhost
	var files []string
	base := filepath.Dir(mainFile)
	var load func(file string, depth int) error
	load = func(file string, depth int) error {
		if depth > 8 {
			return nil
		}
		b, err := h.ReadFile(file)
		if err != nil {
			return err
		}
		files = append(files, file)
		var cur *vhost
		depthBlk := 0
		for n, raw := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(raw)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			f := strings.Fields(l)
			key := strings.ToLower(f[0])
			switch {
			case strings.HasPrefix(key, "<virtualhost"):
				v := vhost{ref: fmt.Sprintf("%s:%d", file, n+1)}
				addr := strings.TrimSuffix(strings.Join(f[1:], " "), ">")
				if i := strings.LastIndex(addr, ":"); i >= 0 {
					v.port = addr[i+1:]
				}
				vhosts = append(vhosts, v)
				cur = &vhosts[len(vhosts)-1]
				depthBlk = 0
			case key == "</virtualhost>":
				cur = nil
			case (key == "include" || key == "includeoptional") && len(f) > 1 && cur == nil:
				pat := strings.Trim(f[1], `"`)
				if !filepath.IsAbs(pat) {
					pat = filepath.Join(base, pat)
				}
				if strings.HasSuffix(pat, "/") {
					pat += "*"
				}
				for _, m := range h.Glob(pat) {
					if err := load(m, depth+1); err != nil && key == "include" {
						return err
					}
				}
			case cur != nil && strings.HasPrefix(key, "</"):
				depthBlk--
			case cur != nil && strings.HasPrefix(key, "<"):
				depthBlk++
				cur.nested = append(cur.nested, strings.Trim(f[0], "<>"))
			case cur != nil && depthBlk == 0:
				cur.lines = append(cur.lines, f)
			}
		}
		return nil
	}
	if err := load(mainFile, 0); err != nil {
		return nil, files, err
	}
	// group by name: :80 vhosts that only redirect make the :443 vhost ForceHTTPS
	redirect := map[string]bool{}
	type content struct {
		names  []string
		routes []Route
		tls    bool
		cert   *CertRef
	}
	var contents []content
	for _, v := range vhosts {
		if v.port != "80" && v.port != "443" {
			continue
		}
		var names []string
		var cert CertRef
		var routes []Route
		redirectOnly := false
		base := Route{Source: SourceApache, Ref: v.ref, Status: StatusOK}
		preserveHost := false
		for _, n := range v.nested {
			if !strings.EqualFold(n, "Directory") {
				base.fallback("<%s> blocks are not supported", n)
			}
		}
		for _, l := range v.lines {
			key := strings.ToLower(l[0])
			switch key {
			case "servername", "serveralias":
				names = append(names, l[1:]...)
			case "sslcertificatefile":
				cert.CertFile = l[1]
			case "sslcertificatekeyfile":
				cert.KeyFile = l[1]
			case "proxypreservehost":
				preserveHost = len(l) > 1 && strings.EqualFold(l[1], "on")
			case "limitrequestbody":
				if n, err := strconv.Atoi(l[1]); err == nil && n > 0 {
					base.MaxBodyMB = (n + (1 << 20) - 1) >> 20
				}
			case "proxytimeout", "timeout":
				if n, err := strconv.Atoi(l[1]); err == nil {
					base.TimeoutSeconds = n
				}
			case "redirect", "redirectpermanent":
				args := l[1:]
				if len(args) > 0 && (strings.EqualFold(args[0], "permanent") || args[0] == "301" || args[0] == "302") {
					args = args[1:]
				}
				if len(args) == 2 && args[0] == "/" && strings.HasPrefix(args[1], "https://") {
					redirectOnly = true
				} else {
					base.fallback("Redirect %s", strings.Join(l[1:], " "))
				}
			case "rewritecond":
			case "rewriterule":
				if rewriteHTTPS.MatchString(strings.Join(l[1:], " ")) {
					redirectOnly = true
				} else {
					base.fallback("RewriteRule %s", strings.Join(l[1:], " "))
				}
			case "header", "requestheader":
				base.warn("%s %s is not applied", l[0], strings.Join(l[1:], " "))
			default:
				if !apacheIgnored[key] {
					base.fallback("directive %s is not supported", l[0])
				}
			}
		}
		for _, l := range v.lines {
			if !strings.EqualFold(l[0], "ProxyPass") || len(l) < 3 {
				continue
			}
			r := base
			r.Notes = append([]string(nil), base.Notes...)
			r.PathPrefix = normPath(l[1])
			if l[2] == "!" {
				r.fallback("ProxyPass %s ! (excluded path)", l[1])
				routes = append(routes, r)
				continue
			}
			u, err := url.Parse(l[2])
			if err != nil || u.Host == "" {
				r.fallback("ProxyPass target %s", l[2])
				routes = append(routes, r)
				continue
			}
			up := strings.TrimSuffix(u.Path, "/")
			if up != strings.TrimSuffix(r.PathPrefix, "/") {
				r.fallback("ProxyPass %s %s rewrites the path", l[1], l[2])
			}
			port := 80
			if u.Scheme == "https" {
				port = 443
				r.Target.HTTPS = true
			}
			if p := u.Port(); p != "" {
				port, _ = strconv.Atoi(p)
			}
			host := u.Hostname()
			if host == "localhost" {
				host = "127.0.0.1"
			}
			if u.Scheme == "h2c" {
				r.BackendProtocol = "h2c"
			}
			if u.Scheme == "ws" || u.Scheme == "wss" {
				r.Target.HTTPS = u.Scheme == "wss"
			}
			r.Target.Host, r.Target.Port = host, port
			if !preserveHost {
				r.warn("Apache sent the backend address as Host (ProxyPreserveHost off); the gateway forwards the client's Host")
			}
			routes = append(routes, r)
		}
		if redirectOnly && len(routes) == 0 {
			for _, n := range names {
				redirect[strings.ToLower(n)] = true
			}
			continue
		}
		if len(routes) == 0 {
			r := base
			r.PathPrefix = "/"
			r.fallback("virtual host serves files (no ProxyPass)")
			routes = append(routes, r)
		}
		c := content{names: names, routes: routes, tls: v.port == "443"}
		if cert.CertFile != "" && cert.KeyFile != "" {
			cc := cert
			c.cert = &cc
		}
		contents = append(contents, c)
	}
	// HTTPS virtual hosts first: when :80 and :443 serve the same, the TLS one (with its certificate) wins
	sort.SliceStable(contents, func(i, j int) bool { return contents[i].tls && !contents[j].tls })
	var out []Route
	for _, c := range contents {
		for _, n := range c.names {
			n = normHost(n)
			for _, r := range c.routes {
				r.Host, r.TLS = n, c.tls
				if c.tls {
					r.Cert = c.cert
					r.ForceHTTPS = redirect[n]
				}
				out = append(out, r)
			}
		}
	}
	return out, files, nil
}
