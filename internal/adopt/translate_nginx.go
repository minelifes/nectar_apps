package adopt

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// nginx directive with its source position.
type ngDir struct {
	Name  string
	Args  []string
	Block []ngDir // nil for "name args;"
	File  string
	Line  int
}

type ngLexer struct {
	src  []rune
	pos  int
	line int
}

func (l *ngLexer) next() (tok string, line int, ok bool) {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '#' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		if !unicode.IsSpace(c) {
			break
		}
		if c == '\n' {
			l.line++
		}
		l.pos++
	}
	if l.pos >= len(l.src) {
		return "", l.line, false
	}
	line = l.line
	c := l.src[l.pos]
	switch c {
	case '{', '}', ';':
		l.pos++
		return string(c), line, true
	case '"', '\'':
		q := c
		l.pos++
		var b strings.Builder
		for l.pos < len(l.src) && l.src[l.pos] != q {
			if l.src[l.pos] == '\\' && l.pos+1 < len(l.src) {
				l.pos++
			}
			if l.src[l.pos] == '\n' {
				l.line++
			}
			b.WriteRune(l.src[l.pos])
			l.pos++
		}
		l.pos++
		return b.String(), line, true
	}
	start := l.pos
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if unicode.IsSpace(c) || c == '{' || c == '}' || c == ';' {
			break
		}
		l.pos++
	}
	return string(l.src[start:l.pos]), line, true
}

func ngParse(l *ngLexer, file string, depth int) ([]ngDir, error) {
	var out []ngDir
	var cur []string
	curLine := 0
	for {
		tok, line, ok := l.next()
		if !ok {
			if len(cur) > 0 || depth > 0 {
				return nil, fmt.Errorf("%s: unexpected end of file", file)
			}
			return out, nil
		}
		switch tok {
		case ";":
			if len(cur) > 0 {
				out = append(out, ngDir{Name: cur[0], Args: cur[1:], File: file, Line: curLine + 1})
			}
			cur = nil
		case "{":
			if len(cur) == 0 {
				return nil, fmt.Errorf("%s:%d: unexpected '{'", file, line+1)
			}
			blk, err := ngParse(l, file, depth+1)
			if err != nil {
				return nil, err
			}
			if blk == nil {
				blk = []ngDir{}
			}
			out = append(out, ngDir{Name: cur[0], Args: cur[1:], Block: blk, File: file, Line: curLine + 1})
			cur = nil
		case "}":
			if len(cur) > 0 {
				return nil, fmt.Errorf("%s:%d: missing ';'", file, line+1)
			}
			if depth == 0 {
				return nil, fmt.Errorf("%s:%d: unexpected '}'", file, line+1)
			}
			return out, nil
		default:
			if len(cur) == 0 {
				curLine = line
			}
			cur = append(cur, tok)
		}
	}
}

// nginxLoad parses a config file and expands include directives (relative paths resolve against the
// directory of the main file, like nginx's prefix). It returns the directives and all files read.
func nginxLoad(h Host, mainFile string) ([]ngDir, []string, error) {
	base := filepath.Dir(mainFile)
	var files []string
	var load func(file string, depth int) ([]ngDir, error)
	load = func(file string, depth int) ([]ngDir, error) {
		if depth > 10 {
			return nil, fmt.Errorf("%s: include nesting too deep", file)
		}
		b, err := h.ReadFile(file)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
		dirs, err := ngParse(&ngLexer{src: []rune(string(b))}, file, 0)
		if err != nil {
			return nil, err
		}
		var expand func([]ngDir) ([]ngDir, error)
		expand = func(ds []ngDir) ([]ngDir, error) {
			var out []ngDir
			for _, d := range ds {
				if d.Name == "include" && len(d.Args) == 1 && d.Block == nil {
					pat := d.Args[0]
					if !filepath.IsAbs(pat) {
						pat = filepath.Join(base, pat)
					}
					for _, f := range h.Glob(pat) {
						if strings.HasSuffix(f, "mime.types") || strings.Contains(f, "fastcgi") || strings.HasSuffix(f, "params") {
							continue
						}
						sub, err := load(f, depth+1)
						if err != nil {
							return nil, err
						}
						out = append(out, sub...)
					}
					continue
				}
				if d.Block != nil {
					blk, err := expand(d.Block)
					if err != nil {
						return nil, err
					}
					d.Block = blk
				}
				out = append(out, d)
			}
			return out, nil
		}
		return expand(dirs)
	}
	dirs, err := load(mainFile, 0)
	return dirs, files, err
}

// nginx directives that do not change how requests are routed (or only tune nginx itself).
var ngIgnored = map[string]bool{
	"access_log": true, "error_log": true, "log_not_found": true, "server_tokens": true, "charset": true,
	"keepalive_timeout": true, "keepalive_requests": true, "send_timeout": true, "client_body_timeout": true,
	"client_header_timeout": true, "client_body_buffer_size": true, "large_client_header_buffers": true,
	"gzip": true, "gzip_types": true, "gzip_vary": true, "gzip_proxied": true, "gzip_comp_level": true, "gzip_min_length": true,
	"gzip_disable": true, "gzip_http_version": true, "gzip_buffers": true, "etag": true, "sendfile": true, "tcp_nopush": true,
	"tcp_nodelay": true, "http2": true, "listen": true, "server_name": true, "resolver": true, "resolver_timeout": true,
	"proxy_http_version": true, "proxy_buffering": true, "proxy_buffer_size": true, "proxy_buffers": true,
	"proxy_busy_buffers_size": true, "proxy_connect_timeout": true, "proxy_redirect": true, "proxy_request_buffering": true,
	"proxy_cache_bypass": true, "proxy_ssl_server_name": true, "proxy_ssl_verify": true, "proxy_ssl_name": true,
	"proxy_max_temp_file_size": true, "proxy_temp_file_write_size": true, "proxy_ignore_client_abort": true,
	"proxy_next_upstream": true, "proxy_intercept_errors": false, "index": true, "ssl_certificate": true,
	"ssl_certificate_key": true, "ssl_trusted_certificate": true, "ssl_dhparam": true, "ssl_protocols": true,
	"ssl_ciphers": true, "ssl_prefer_server_ciphers": true, "ssl_session_cache": true, "ssl_session_timeout": true,
	"ssl_session_tickets": true, "ssl_stapling": true, "ssl_stapling_verify": true, "ssl_ecdh_curve": true,
	"ssl_early_data": true, "ssl_buffer_size": true, "ssl": true, "server_name_in_redirect": true, "port_in_redirect": true,
	"absolute_redirect": true, "types_hash_max_size": true, "server_names_hash_bucket_size": true, "underscores_in_headers": true,
	"grpc_read_timeout": true, "grpc_send_timeout": true, "grpc_set_header": true, "chunked_transfer_encoding": true,
	"open_file_cache": true, "default_type": true, "include": true, "real_ip_header": true, "set_real_ip_from": true,
}

var ngBenignProxyHeaders = map[string]bool{
	"host": true, "x-real-ip": true, "x-forwarded-for": true, "x-forwarded-proto": true, "x-forwarded-host": true,
	"x-forwarded-port": true, "x-forwarded-ssl": true, "x-forwarded-scheme": true, "x-scheme": true, "upgrade": true,
	"connection": true, "forwarded": true,
}

type ngSettings struct {
	maxBodyMB    int
	maxBodySet   bool
	timeout      int
	htpasswdFile string
	authOff      bool
}

// ngServer is one server block, reduced to what matters for migration.
type ngServer struct {
	names      []string
	http, tls  bool // listens on 80 / 443
	redirect   bool // only redirects to https
	routes     []Route
	cert       *CertRef
	ref        string
	staticOnly bool
}

// Nginx translates host nginx server blocks that listen on 80/443. mainFile is usually /etc/nginx/nginx.conf.
func Nginx(h Host, mainFile string) ([]Route, []string, error) {
	dirs, files, err := nginxLoad(h, mainFile)
	if err != nil {
		return nil, files, err
	}
	upstreams := map[string][]string{}
	// upstream blocks may come after the servers that use them: collect them first
	var pre func([]ngDir)
	pre = func(ds []ngDir) {
		for _, d := range ds {
			if d.Name == "upstream" && len(d.Args) == 1 && d.Block != nil {
				for _, s := range d.Block {
					if s.Name == "server" && len(s.Args) > 0 {
						upstreams[d.Args[0]] = append(upstreams[d.Args[0]], s.Args[0])
					}
				}
			} else if d.Block != nil && d.Name != "server" {
				pre(d.Block)
			}
		}
	}
	pre(dirs)
	var servers []ngServer
	var collect func([]ngDir)
	collect = func(ds []ngDir) {
		for _, d := range ds {
			switch {
			case d.Name == "upstream" || d.Name == "stream" || d.Name == "map":
				// upstreams are collected above; stream (TCP/UDP) blocks are not HTTP routes
			case d.Name == "server" && d.Block != nil:
				if srv, ok := ngServerBlock(h, d, upstreams); ok {
					servers = append(servers, srv)
				}
			case d.Block != nil:
				collect(d.Block)
			}
		}
	}
	collect(dirs)
	return mergeNginxServers(servers), files, nil
}

func ngServerBlock(h Host, d ngDir, upstreams map[string][]string) (ngServer, bool) {
	srv := ngServer{ref: fmt.Sprintf("%s:%d", d.File, d.Line)}
	listens := false
	var set ngSettings
	var srvNotes []string
	srvFallback := ""
	for _, x := range d.Block {
		switch x.Name {
		case "listen":
			listens = true
			if len(x.Args) == 0 {
				continue
			}
			addr := x.Args[0]
			port := addr
			if hp, p, err := net.SplitHostPort(addr); err == nil {
				port = p
				if hp == "127.0.0.1" || hp == "::1" || hp == "localhost" {
					continue // loopback only: not the public edge
				}
			}
			ssl := false
			for _, a := range x.Args[1:] {
				if a == "ssl" {
					ssl = true
				}
			}
			switch port {
			case "80":
				srv.http = true
			case "443":
				srv.tls = true
				if !ssl {
					srvFallback = "listens on 443 without ssl"
				}
			}
		case "server_name":
			srv.names = append(srv.names, x.Args...)
		case "ssl_certificate":
			if len(x.Args) > 0 {
				if srv.cert == nil {
					srv.cert = &CertRef{}
				}
				srv.cert.CertFile = x.Args[0]
			}
		case "ssl_certificate_key":
			if len(x.Args) > 0 {
				if srv.cert == nil {
					srv.cert = &CertRef{}
				}
				srv.cert.KeyFile = x.Args[0]
			}
		}
	}
	if !listens {
		srv.http = true // nginx default: listen *:80
	}
	if !srv.http && !srv.tls {
		return srv, false
	}
	if srv.cert != nil && (strings.Contains(srv.cert.CertFile, "$") || strings.Contains(srv.cert.KeyFile, "$")) {
		srvFallback = "certificate path uses variables"
	}
	// server-level settings and returns
	var srvReturn []string
	for _, x := range d.Block {
		switch x.Name {
		case "location", "listen", "server_name", "ssl_certificate", "ssl_certificate_key":
		case "return":
			srvReturn = x.Args
		case "rewrite":
			if len(x.Args) >= 2 && isHTTPSSelfRedirect(x.Args[1]) {
				srv.redirect = true
			} else {
				srvFallback = "rewrite directive"
			}
		default:
			if note, fb := ngSetting(h, x, &set); fb != "" {
				srvFallback = fb
			} else if note != "" {
				srvNotes = append(srvNotes, note)
			}
		}
	}
	base := Route{Source: SourceNginx, Ref: srv.ref, Status: StatusOK}
	for _, n := range srvNotes {
		base.warn("%s", n)
	}
	if srvFallback != "" {
		base.fallback("%s", srvFallback)
	}
	if len(srvReturn) > 0 {
		r := base
		if ngReturn(&r, srvReturn) {
			srv.redirect = true
			return srv, true
		}
		r.PathPrefix = "/"
		srv.routes = append(srv.routes, r)
		return srv, true
	}
	for _, x := range d.Block {
		if x.Name != "location" || x.Block == nil {
			continue
		}
		r := base
		r.Notes = append([]string(nil), base.Notes...)
		r.Ref = fmt.Sprintf("%s:%d", x.File, x.Line)
		ls := set
		ngLocation(h, &r, x, upstreams, &ls)
		if r.RedirectURL == "" && r.Target.Host == "" && r.Status != StatusFallback && !r.ForceHTTPS {
			r.fallback("location %s serves files or has no proxy_pass", r.PathPrefix)
		}
		if r.ForceHTTPS && r.Target.Host == "" && r.PathPrefix == "/" {
			srv.redirect = true
			continue
		}
		srv.routes = append(srv.routes, r)
	}
	if len(srv.routes) == 0 && !srv.redirect {
		r := base
		r.PathPrefix = "/"
		r.fallback("server serves static files only")
		srv.routes = append(srv.routes, r)
		srv.staticOnly = true
	}
	return srv, true
}

// ngSetting applies a server/location level directive. It returns a note (⚠) or a fallback reason (✗).
func ngSetting(h Host, x ngDir, set *ngSettings) (note, fallback string) {
	arg := ""
	if len(x.Args) > 0 {
		arg = x.Args[0]
	}
	switch x.Name {
	case "client_max_body_size":
		if mb, ok := parseSizeMB(arg); ok {
			set.maxBodyMB, set.maxBodySet = mb, true
		}
	case "proxy_read_timeout", "proxy_send_timeout":
		if n, err := strconv.Atoi(strings.TrimSuffix(arg, "s")); err == nil && n > set.timeout {
			set.timeout = n
		} else if strings.HasSuffix(arg, "m") {
			if n, err := strconv.Atoi(strings.TrimSuffix(arg, "m")); err == nil && n*60 > set.timeout {
				set.timeout = n * 60
			}
		}
	case "auth_basic":
		set.authOff = arg == "off"
	case "auth_basic_user_file":
		set.htpasswdFile = arg
	case "proxy_set_header":
		if !ngBenignProxyHeaders[strings.ToLower(arg)] {
			return fmt.Sprintf("proxy_set_header %s is not sent to the backend", arg), ""
		}
		if strings.EqualFold(arg, "host") && len(x.Args) > 1 && x.Args[1] != "$host" && x.Args[1] != "$http_host" {
			return "", "proxy_set_header Host " + x.Args[1]
		}
	case "add_header":
		return fmt.Sprintf("response header %s is not added by the gateway", arg), ""
	case "proxy_hide_header", "more_set_headers", "more_clear_headers", "expires":
		return fmt.Sprintf("%s is not applied by the gateway", x.Name), ""
	case "limit_req", "limit_conn":
		return fmt.Sprintf("%s is not migrated (use the gateway rate limit)", x.Name), ""
	case "error_page":
		return "custom error pages are not used", ""
	case "proxy_cache", "proxy_cache_valid", "proxy_cache_key", "proxy_cache_use_stale", "proxy_cache_lock":
		return "no response caching in the gateway", ""
	case "root", "alias", "try_files", "autoindex":
		// only matters when a location has no proxy_pass (checked per location)
	case "allow", "deny", "satisfy", "auth_request":
		return "", x.Name + " (access control) is not supported"
	case "if", "rewrite", "set", "map", "sub_filter", "fastcgi_pass", "uwsgi_pass", "scgi_pass", "memcached_pass", "lua_need_request_body":
		return "", x.Name + " is not supported"
	default:
		if ngIgnored[x.Name] || strings.HasPrefix(x.Name, "ssl_") {
			return "", ""
		}
		return "", "directive " + x.Name + " is not supported"
	}
	return "", ""
}

func ngLocation(h Host, r *Route, loc ngDir, upstreams map[string][]string, set *ngSettings) {
	args := loc.Args
	switch {
	case len(args) == 1 && strings.HasPrefix(args[0], "@"):
		r.fallback("named location %s", args[0])
		return
	case len(args) == 1:
		r.PathPrefix = args[0]
	case len(args) == 2 && args[0] == "^~":
		r.PathPrefix = args[1]
	case len(args) == 2 && args[0] == "=":
		r.PathPrefix = args[1]
		if args[1] != "/" {
			r.warn("exact location = %s is served as a prefix", args[1])
		}
	default:
		r.PathPrefix = strings.Join(args, " ")
		r.fallback("regular-expression location %s", strings.Join(args, " "))
		return
	}
	r.PathPrefix = normPath(r.PathPrefix)
	hostSet := false
	proxied := false
	for _, x := range loc.Block {
		switch x.Name {
		case "location":
			r.fallback("nested location %s", strings.Join(x.Args, " "))
		case "proxy_pass", "grpc_pass":
			proxied = true
			ngProxyPass(r, x, upstreams)
		case "return":
			if !ngReturn(r, x.Args) && r.Status != StatusFallback && r.RedirectURL == "" {
				r.fallback("return %s", strings.Join(x.Args, " "))
			}
			proxied = true
		case "proxy_set_header":
			if strings.EqualFold(firstArg(x.Args), "host") {
				hostSet = true
			}
			fallthrough
		default:
			if note, fb := ngSetting(h, x, set); fb != "" {
				r.fallback("%s", fb)
			} else if note != "" {
				r.warn("%s", note)
			}
		}
	}
	if proxied && r.Target.Host != "" && !hostSet {
		r.warn("nginx sent the backend address as Host; the gateway forwards the client's Host header")
	}
	if set.maxBodySet {
		r.MaxBodyMB = set.maxBodyMB
	} else if proxied && r.Target.Host != "" {
		r.warn("nginx limited request bodies to 1 MB (default); the gateway does not limit them")
	}
	if set.timeout > 0 {
		r.TimeoutSeconds = set.timeout
	}
	if set.htpasswdFile != "" && !set.authOff {
		b, err := h.ReadFile(set.htpasswdFile)
		if err != nil {
			r.fallback("basic auth file %s: %v", set.htpasswdFile, err)
		} else {
			var lines []string
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
					lines = append(lines, l)
				}
			}
			if ht, err := checkHtpasswd(lines); err != nil {
				r.fallback("basic auth: %v", err)
			} else {
				r.Htpasswd, r.BasicAuth = ht, set.htpasswdFile
			}
		}
	}
}

func firstArg(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

func ngProxyPass(r *Route, x ngDir, upstreams map[string][]string) {
	raw := firstArg(x.Args)
	if strings.Contains(raw, "$") {
		r.fallback("%s uses variables", x.Name)
		return
	}
	if x.Name == "grpc_pass" {
		switch {
		case strings.HasPrefix(raw, "grpcs://"):
			r.fallback("grpc_pass over TLS")
			return
		case strings.HasPrefix(raw, "grpc://"):
			raw = "http://" + strings.TrimPrefix(raw, "grpc://")
		default:
			raw = "http://" + raw
		}
		r.BackendProtocol = "h2c"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		r.fallback("cannot parse %s %q", x.Name, raw)
		return
	}
	if strings.HasPrefix(u.Host, "unix:") {
		r.fallback("unix socket backend")
		return
	}
	if u.Path != "" && u.Path != r.PathPrefix && !(u.Path == "/" && r.PathPrefix == "/") {
		r.fallback("proxy_pass %s rewrites the path of location %s", raw, r.PathPrefix)
		return
	}
	host, port := u.Hostname(), u.Port()
	https := u.Scheme == "https"
	if ups, ok := upstreams[host]; ok {
		if len(ups) == 0 || strings.HasPrefix(ups[0], "unix:") {
			r.fallback("upstream %s has no TCP server", host)
			return
		}
		if len(ups) > 1 {
			r.warn("upstream %s has %d servers: only %s is used (no load balancing / failover)", host, len(ups), ups[0])
		}
		host, port = ups[0], ""
		if hp, p, err := net.SplitHostPort(ups[0]); err == nil {
			host, port = hp, p
		}
	}
	n := 80
	if https {
		n = 443
	}
	if port != "" {
		n, _ = strconv.Atoi(port)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	r.Target = Target{Host: host, Port: n, HTTPS: https}
	if https {
		r.warn("HTTPS backend: the gateway does not verify the backend certificate")
	}
}

func isHTTPSSelfRedirect(u string) bool {
	u = strings.TrimSuffix(u, "?")
	return u == "https://$host$request_uri" || u == "https://$server_name$request_uri" || u == "https://$http_host$request_uri"
}

// ngReturn handles "return 301 https://$host$request_uri" (force HTTPS) and redirects to another URL.
func ngReturn(r *Route, args []string) bool {
	if len(args) < 2 {
		return false
	}
	code, err := strconv.Atoi(args[0])
	if err != nil || (code != 301 && code != 302 && code != 307 && code != 308) {
		return false
	}
	target := args[1]
	if isHTTPSSelfRedirect(target) {
		r.ForceHTTPS = true
		return true
	}
	if strings.HasSuffix(target, "$request_uri") && !strings.Contains(strings.TrimSuffix(target, "$request_uri"), "$") {
		r.RedirectURL, r.RedirectCode = strings.TrimSuffix(target, "$request_uri"), code
		return false
	}
	if !strings.Contains(target, "$") && (strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")) {
		r.RedirectURL, r.RedirectCode = target, code
		r.warn("redirect to %s: the gateway appends the request path", target)
		return false
	}
	r.fallback("return %s %s", args[0], target)
	return false
}

// mergeNginxServers turns server blocks into per-host routes: a redirect-only :80 server becomes
// ForceHTTPS on the :443 server of the same name; separate :80 and :443 servers with the same routes merge.
func mergeNginxServers(servers []ngServer) []Route {
	type hostInfo struct {
		redirect      bool
		httpServers   []ngServer
		tlsServers    []ngServer
		bothServers   []ngServer
		firstRedirect string
	}
	hosts := map[string]*hostInfo{}
	var order []string
	var out []Route
	for _, s := range servers {
		var names []string
		for _, n := range s.names {
			n = strings.ToLower(n)
			switch {
			case n == "" || n == "_" || n == `""`:
			case strings.HasPrefix(n, "~"):
				out = append(out, Route{Source: SourceNginx, Ref: s.ref, Host: n, Status: StatusFallback, Notes: []string{"regular-expression server_name"}})
			case strings.HasPrefix(n, "."):
				names = append(names, n[1:], "*"+n)
			default:
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			continue // default/catch-all server: unknown hosts keep going to nginx through the fallback
		}
		for _, n := range names {
			hi, ok := hosts[n]
			if !ok {
				hi = &hostInfo{}
				hosts[n] = hi
				order = append(order, n)
			}
			switch {
			case s.redirect && len(s.routes) == 0:
				hi.redirect = true
			case s.http && s.tls:
				hi.bothServers = append(hi.bothServers, s)
			case s.tls:
				hi.tlsServers = append(hi.tlsServers, s)
			default:
				hi.httpServers = append(hi.httpServers, s)
			}
			if s.redirect {
				hi.redirect = true
			}
		}
	}
	for _, name := range order {
		hi := hosts[name]
		content := append(append([]ngServer{}, hi.bothServers...), hi.tlsServers...)
		var httpOnly []ngServer
		if len(content) == 0 {
			httpOnly = hi.httpServers
			content = hi.httpServers
		} else if len(hi.httpServers) > 0 {
			// separate HTTP server with content: must serve the same as the HTTPS one
			if !sameRoutes(hi.httpServers[0].routes, content[0].routes) {
				for _, r := range content[0].routes {
					r.Host = name
					r.fallback("HTTP (%s) and HTTPS servers serve different content", hi.httpServers[0].ref)
					out = append(out, r)
				}
				continue
			}
		}
		if len(content) == 0 {
			if hi.redirect {
				out = append(out, Route{Source: SourceNginx, Host: name, PathPrefix: "/", Status: StatusFallback,
					Notes: []string{"redirects to HTTPS, but no HTTPS server block was found"}})
			}
			continue
		}
		if len(content) > 1 {
			for _, r := range content[1].routes {
				r.Host = name
				r.fallback("several server blocks for the same name (also %s)", content[0].ref)
				out = append(out, r)
			}
		}
		s := content[0]
		for _, r := range s.routes {
			r.Host = name
			r.TLS = s.tls && len(httpOnly) == 0
			if r.TLS {
				if s.cert != nil && s.cert.CertFile != "" && s.cert.KeyFile != "" {
					c := *s.cert
					r.Cert = &c
				} else {
					r.warn("HTTPS without ssl_certificate: the gateway issues its own certificate")
				}
			}
			if hi.redirect && r.TLS {
				r.ForceHTTPS = true
			}
			if !s.http && len(hi.httpServers) == 0 && !hi.redirect && r.TLS {
				r.warn("nginx answered plain HTTP for this host with its default server; the gateway serves the route on HTTP too")
			}
			out = append(out, r)
		}
	}
	return out
}

func sameRoutes(a, b []Route) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].PathPrefix != b[i].PathPrefix || a[i].Target != b[i].Target || a[i].RedirectURL != b[i].RedirectURL {
			return false
		}
	}
	return true
}
