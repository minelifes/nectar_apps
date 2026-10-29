package adopt

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"nectar_apps/internal/kube"
)

func obj(kind, ns, name string, spec map[string]any) kube.Object {
	o := kube.Object{"kind": kind, "metadata": map[string]any{"name": name, "namespace": ns}}
	for k, v := range spec {
		o[k] = v
	}
	return o
}

func withAnn(o kube.Object, ann map[string]any) kube.Object {
	o["metadata"].(map[string]any)["annotations"] = ann
	return o
}

func svc(ns, name string, ports ...any) kube.Object {
	var ps []any
	for i := 0; i+1 < len(ports); i += 2 {
		ps = append(ps, map[string]any{"name": ports[i], "port": float64(ports[i+1].(int))})
	}
	return obj("Service", ns, name, map[string]any{"spec": map[string]any{"type": "ClusterIP", "clusterIP": "10.43.0.10", "ports": ps}})
}

func ingress(ns, name, class string, tls []any, rules ...any) kube.Object {
	spec := map[string]any{"rules": rules}
	if class != "" {
		spec["ingressClassName"] = class
	}
	if tls != nil {
		spec["tls"] = tls
	}
	return obj("Ingress", ns, name, map[string]any{"spec": spec})
}

func rule(host string, paths ...any) any {
	return map[string]any{"host": host, "http": map[string]any{"paths": paths}}
}

func path(p, pathType, svcName string, port any) any {
	pp := map[string]any{"path": p, "pathType": pathType}
	switch v := port.(type) {
	case int:
		pp["backend"] = map[string]any{"service": map[string]any{"name": svcName, "port": map[string]any{"number": float64(v)}}}
	case string:
		pp["backend"] = map[string]any{"service": map[string]any{"name": svcName, "port": map[string]any{"name": v}}}
	}
	return pp
}

func secret(ns, name, typ string, data map[string]string) kube.Object {
	d := map[string]any{}
	for k, v := range data {
		d[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return obj("Secret", ns, name, map[string]any{"type": typ, "data": d})
}

func bcryptLine(t *testing.T, user string) string {
	h, err := bcrypt.GenerateFromPassword([]byte("secret-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return user + ":" + string(h)
}

func byKey(rs []Route) map[string]Route {
	m := map[string]Route{}
	for _, r := range rs {
		m[r.Key()] = r
	}
	return m
}

func TestIngressTraefik(t *testing.T) {
	c := Cluster{
		IngressClasses: []kube.Object{withAnn(obj("IngressClass", "", "traefik", map[string]any{"spec": map[string]any{"controller": "traefik.io/ingress-controller"}}),
			map[string]any{"ingressclass.kubernetes.io/is-default-class": "true"})},
		Services: []kube.Object{svc("shop", "web", "http", 8080), svc("shop", "api", "grpc", 9000), svc("other", "x", "http", 80)},
		Secrets:  []kube.Object{secret("shop", "shop-tls", "kubernetes.io/tls", map[string]string{"tls.crt": "x", "tls.key": "y"})},
		Ingresses: []kube.Object{
			withAnn(ingress("shop", "web", "", []any{map[string]any{"hosts": []any{"shop.example.com"}, "secretName": "shop-tls"}},
				rule("shop.example.com", path("/", "Prefix", "web", "http"), path("/api", "Prefix", "api", 9000))),
				map[string]any{"cert-manager.io/cluster-issuer": "le", "traefik.ingress.kubernetes.io/router.entrypoints": "websecure"}),
			ingress("other", "nginx-only", "nginx", nil, rule("other.example.com", path("/", "Prefix", "x", 80))),
			withAnn(ingress("shop", "mw", "traefik", nil, rule("mw.example.com", path("/", "Prefix", "web", 8080))),
				map[string]any{"traefik.ingress.kubernetes.io/router.middlewares": "shop-strip@kubernetescrd"}),
		},
		Middlewares: []kube.Object{obj("Middleware", "shop", "strip", map[string]any{"spec": map[string]any{"stripPrefix": map[string]any{"prefixes": []any{"/x"}}}})},
	}
	rs := Finalize(Ingresses(c, K8sOptions{Controller: "traefik", Classes: []string{"traefik"}}))
	m := byKey(rs)
	if len(rs) != 3 {
		t.Fatalf("routes: %+v", rs)
	}
	root := m["shop.example.com/"]
	if root.Status != StatusOK || root.Target.Host != "web.shop.svc.cluster.local" || root.Target.Port != 8080 || !root.TLS ||
		root.Cert == nil || root.Cert.Secret != "shop/shop-tls" || root.ForceHTTPS {
		t.Fatalf("root: %+v", root)
	}
	if api := m["shop.example.com/api"]; api.Target.Port != 9000 || api.Status != StatusOK {
		t.Fatalf("api: %+v", api)
	}
	if mw := m["mw.example.com/"]; mw.Status != StatusFallback || !strings.Contains(strings.Join(mw.Notes, ";"), "stripPrefix") {
		t.Fatalf("middleware: %+v", mw)
	}
	if _, ok := m["other.example.com/"]; ok {
		t.Fatal("ingress of another class must be skipped")
	}
}

func TestIngressNginxAnnotations(t *testing.T) {
	good := bcryptLine(t, "alice")
	c := Cluster{
		Services: []kube.Object{svc("a", "web", "http", 80), svc("a", "grpc", "g", 50051)},
		Secrets: []kube.Object{
			secret("a", "auth", "Opaque", map[string]string{"auth": good + "\n"}),
			secret("a", "md5", "Opaque", map[string]string{"auth": "bob:$apr1$abc$def"}),
		},
		Ingresses: []kube.Object{
			withAnn(ingress("a", "main", "nginx", []any{map[string]any{"hosts": []any{"a.example.com"}, "secretName": "missing-tls"}},
				rule("a.example.com", path("/", "Prefix", "web", 80))),
				map[string]any{"nginx.ingress.kubernetes.io/proxy-body-size": "20m", "nginx.ingress.kubernetes.io/proxy-read-timeout": "120",
					"nginx.ingress.kubernetes.io/auth-type": "basic", "nginx.ingress.kubernetes.io/auth-secret": "auth"}),
			withAnn(ingress("a", "grpc", "nginx", nil, rule("g.example.com", path("/", "Prefix", "grpc", 50051))),
				map[string]any{"nginx.ingress.kubernetes.io/backend-protocol": "GRPC", "nginx.ingress.kubernetes.io/ssl-redirect": "false"}),
			withAnn(ingress("a", "md5", "nginx", nil, rule("m.example.com", path("/", "Prefix", "web", 80))),
				map[string]any{"nginx.ingress.kubernetes.io/auth-type": "basic", "nginx.ingress.kubernetes.io/auth-secret": "a/md5"}),
			// one rule of r.example.com rewrites → the whole host stays on the old controller
			withAnn(ingress("a", "rewrite", "nginx", nil, rule("r.example.com", path("/old", "Prefix", "web", 80))),
				map[string]any{"nginx.ingress.kubernetes.io/rewrite-target": "/"}),
			ingress("a", "rest", "nginx", nil, rule("r.example.com", path("/", "Prefix", "web", 80)), rule("", path("/", "Prefix", "web", 80))),
			ingress("a", "redir", "nginx", nil),
		},
	}
	c.Ingresses[5] = withAnn(ingress("a", "redir", "nginx", nil, rule("old.example.com", path("/", "Prefix", "web", 80))),
		map[string]any{"nginx.ingress.kubernetes.io/permanent-redirect": "https://new.example.com"})
	rs := Finalize(Ingresses(c, K8sOptions{Controller: "ingress-nginx", Classes: []string{"nginx"}}))
	m := byKey(rs)
	a := m["a.example.com/"]
	if a.Status != StatusWarn || a.MaxBodyMB != 20 || a.TimeoutSeconds != 120 || a.Htpasswd != good || !a.ForceHTTPS || a.Cert == nil {
		t.Fatalf("a: %+v", a)
	}
	if g := m["g.example.com/"]; g.BackendProtocol != "h2c" || g.ForceHTTPS || g.Status != StatusOK {
		t.Fatalf("grpc: %+v", g)
	}
	if md := m["m.example.com/"]; md.Status != StatusFallback || !strings.Contains(md.Notes[0], "non-bcrypt") {
		t.Fatalf("md5: %+v", md)
	}
	if r1, r2 := m["r.example.com/old"], m["r.example.com/"]; r1.Status != StatusFallback || r2.Status != StatusFallback {
		t.Fatalf("per-host fallback: %+v %+v", r1, r2)
	}
	if o := m["old.example.com/"]; o.RedirectURL != "https://new.example.com" || o.RedirectCode != 301 || o.Status != StatusOK {
		t.Fatalf("redirect: %+v", o)
	}
	if catch := m["/"]; catch.Status != StatusFallback {
		t.Fatalf("catch-all: %+v", catch)
	}
	g := a.GatewayRoute("x", "o")
	if g.ACME || !g.ForceSSL || g.MaxBodyMB != 20 || *g.BasicAuthHtpasswd != good {
		t.Fatalf("gateway route: %+v", g)
	}
}

func TestIngressRouteAndHTTPRoute(t *testing.T) {
	c := Cluster{
		Services: []kube.Object{svc("app", "web", "http", 80), svc("app", "v2", "http", 8080)},
		Secrets:  []kube.Object{secret("app", "users", "kubernetes.io/basic-auth", map[string]string{"username": "bob", "password": "pw-123456"})},
		Middlewares: []kube.Object{
			obj("Middleware", "app", "https", map[string]any{"spec": map[string]any{"redirectScheme": map[string]any{"scheme": "https", "permanent": true}}}),
			obj("Middleware", "app", "auth", map[string]any{"spec": map[string]any{"basicAuth": map[string]any{"secret": "users"}}}),
			obj("Middleware", "app", "hdr", map[string]any{"spec": map[string]any{"headers": map[string]any{"customResponseHeaders": map[string]any{"X": "1"}}}}),
		},
		IngressRoutes: []kube.Object{
			obj("IngressRoute", "app", "web", map[string]any{"spec": map[string]any{
				"tls": map[string]any{"certResolver": "le"},
				"routes": []any{
					map[string]any{"match": "(Host(`a.io`) || Host(`www.a.io`)) && PathPrefix(`/`)", "kind": "Rule",
						"middlewares": []any{map[string]any{"name": "https"}, map[string]any{"name": "auth"}},
						"services":    []any{map[string]any{"name": "web", "port": float64(80)}}},
					map[string]any{"match": "Host(`h.io`)", "middlewares": []any{map[string]any{"name": "hdr"}},
						"services": []any{map[string]any{"name": "web", "port": float64(80)}}},
					map[string]any{"match": "Host(`w.io`)", "services": []any{map[string]any{"name": "web", "port": float64(80)}, map[string]any{"name": "v2", "port": float64(8080)}}},
					map[string]any{"match": "HostRegexp(`{x:.+}.io`)", "services": []any{map[string]any{"name": "web", "port": float64(80)}}},
				},
			}}),
		},
		GatewayClasses: []kube.Object{obj("GatewayClass", "", "traefik", map[string]any{"spec": map[string]any{"controllerName": "traefik.io/gateway-controller"}})},
		Gateways: []kube.Object{obj("Gateway", "app", "gw", map[string]any{"spec": map[string]any{"gatewayClassName": "traefik",
			"listeners": []any{map[string]any{"name": "https", "protocol": "HTTPS", "hostname": "*.g.io",
				"tls": map[string]any{"certificateRefs": []any{map[string]any{"name": "g-cert"}}}}}}})},
		HTTPRoutes: []kube.Object{
			obj("HTTPRoute", "app", "r", map[string]any{"spec": map[string]any{"parentRefs": []any{map[string]any{"name": "gw"}},
				"hostnames": []any{"x.g.io"},
				"rules": []any{map[string]any{"matches": []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/v2"}}},
					"backendRefs": []any{map[string]any{"name": "v2", "port": float64(8080)}}}}}}),
			obj("HTTPRoute", "app", "redirect", map[string]any{"spec": map[string]any{"parentRefs": []any{map[string]any{"name": "gw"}},
				"hostnames": []any{"x.g.io"},
				"rules": []any{map[string]any{"matches": []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/v2"}}},
					"filters": []any{map[string]any{"type": "RequestRedirect", "requestRedirect": map[string]any{"scheme": "https"}}}}}}}),
		},
	}
	rs := Finalize(Kubernetes(c, K8sOptions{Controller: "traefik", Classes: []string{"traefik"}}))
	m := byKey(rs)
	for _, h := range []string{"a.io/", "www.a.io/"} {
		r := m[h]
		if r.Status != StatusWarn || !r.ForceHTTPS || !strings.HasPrefix(r.Htpasswd, "bob:$2") || !r.TLS || r.Cert != nil {
			t.Fatalf("%s: %+v", h, r)
		}
	}
	if m["h.io/"].Status != StatusFallback || m["w.io/"].Status != StatusFallback {
		t.Fatalf("headers / weighted: %+v %+v", m["h.io/"], m["w.io/"])
	}
	hr := m["x.g.io/v2"]
	if hr.Status != StatusOK || !hr.ForceHTTPS || hr.Cert == nil || hr.Cert.Secret != "app/g-cert" || hr.Target.Port != 8080 {
		t.Fatalf("httproute: %+v", hr)
	}
	if _, _, _, err := parseTraefikMatch("Host(`a`) && Headers(`X`, `1`)"); err == nil {
		t.Fatal("Headers matcher must be rejected")
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNginx(t *testing.T) {
	root := t.TempDir()
	good := bcryptLine(t, "ops")
	writeFiles(t, root, map[string]string{
		"/etc/nginx/nginx.conf": `user www-data;
events { worker_connections 768; }
http {
    include /etc/nginx/mime.types;
    upstream app { server 127.0.0.1:3000; server 127.0.0.1:3001 backup; }
    include /etc/nginx/sites-enabled/*;
}`,
		"/etc/nginx/mime.types": "types { text/html html; }",
		"/etc/nginx/htpasswd":   good + "\n",
		"/etc/nginx/sites-enabled/app.conf": `
server {
    listen 80;
    listen [::]:80;
    server_name app.example.com www.app.example.com;
    return 301 https://$host$request_uri;
}
server {
    listen 443 ssl http2;
    listen [::]:443 ssl;
    server_name app.example.com www.app.example.com;
    ssl_certificate /etc/letsencrypt/live/app/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app/privkey.pem;
    client_max_body_size 50m;
    location / {
        proxy_pass http://app;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 300s;
    }
    location /admin {
        proxy_pass http://127.0.0.1:4000/admin;
        proxy_set_header Host $host;
        auth_basic "admin";
        auth_basic_user_file /etc/nginx/htpasswd;
    }
}`,
		"/etc/nginx/sites-enabled/static.conf": `
server { listen 80; server_name static.example.com; root /var/www/html; location / { try_files $uri $uri/ =404; } }
server { listen 80; server_name regex.example.com; location / { proxy_pass http://localhost:5000; } location ~ \.php$ { fastcgi_pass unix:/run/php.sock; } }
server { listen 80; server_name rw.example.com; location /api/ { proxy_pass http://127.0.0.1:6000/; proxy_set_header Host $host; client_max_body_size 1m; } }
server { listen 8080; server_name internal.example.com; location / { proxy_pass http://127.0.0.1:7000; } }
server { listen 80 default_server; server_name _; return 444; }
`,
	})
	rs, files, err := Nginx(Host{Root: root}, "/etc/nginx/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files: %v", files)
	}
	m := byKey(Finalize(rs))
	app := m["app.example.com/"]
	if !app.Migratable() || app.Target != (Target{Host: "127.0.0.1", Port: 3000}) || !app.TLS || !app.ForceHTTPS ||
		app.Cert == nil || app.Cert.CertFile != "/etc/letsencrypt/live/app/fullchain.pem" || app.MaxBodyMB != 50 || app.TimeoutSeconds != 300 {
		t.Fatalf("app: %+v", app)
	}
	if !strings.Contains(strings.Join(app.Notes, ";"), "2 servers") {
		t.Fatalf("upstream warning missing: %v", app.Notes)
	}
	adm := m["app.example.com/admin"]
	if adm.Htpasswd != good || adm.Target.Port != 4000 || !adm.Migratable() {
		t.Fatalf("admin: %+v", adm)
	}
	if _, ok := m["www.app.example.com/"]; !ok {
		t.Fatal("second server_name missing")
	}
	for _, k := range []string{"static.example.com/", "regex.example.com/", "rw.example.com/api/"} {
		if m[k].Status != StatusFallback {
			t.Fatalf("%s should stay on nginx: %+v", k, m[k])
		}
	}
	if _, ok := m["internal.example.com/"]; ok {
		t.Fatal("server on port 8080 is not an edge server")
	}
}

func TestCaddyHAProxyApache(t *testing.T) {
	root := t.TempDir()
	hash := strings.SplitN(bcryptLine(t, "x"), ":", 2)[1]
	writeFiles(t, root, map[string]string{
		"/etc/caddy/Caddyfile": `{
	email ops@example.com
}
(common) {
	encode gzip
}
example.org, www.example.org {
	encode zstd gzip
	reverse_proxy localhost:8080
	handle /api/* {
		reverse_proxy 127.0.0.1:9000
	}
}
http://plain.example.org {
	basicauth {
		admin ` + base64.StdEncoding.EncodeToString([]byte(hash)) + `
	}
	reverse_proxy 10.0.0.5:80
}
files.example.org {
	root * /srv
	file_server
}
`,
		"/etc/haproxy/haproxy.cfg": `
global
    log /dev/log local0
defaults
    mode http
    timeout client 30s
frontend web
    bind *:80
    bind *:443 ssl crt /etc/haproxy/certs/site.pem
    http-request redirect scheme https unless { ssl_fc }
    acl host_shop hdr(host) -i shop.example.net
    acl is_api path_beg /api
    use_backend shop_api if host_shop is_api
    use_backend shop if host_shop
    default_backend fallback
backend shop
    server s1 127.0.0.1:8000 check
backend shop_api
    server a1 127.0.0.1:8001 check
    server a2 127.0.0.1:8002 check
backend fallback
    server f 127.0.0.1:9999
`,
		"/etc/apache2/apache2.conf":                "ServerRoot /etc/apache2\nIncludeOptional sites-enabled/*.conf\n",
		"/etc/apache2/sites-enabled/000-site.conf": "<VirtualHost *:80>\n  ServerName blog.example.com\n  RewriteEngine on\n  RewriteCond %{SERVER_NAME} =blog.example.com\n  RewriteRule ^ https://%{SERVER_NAME}%{REQUEST_URI} [END,NE,R=permanent]\n</VirtualHost>\n",
		"/etc/apache2/sites-enabled/001-ssl.conf":  "<VirtualHost *:443>\n  ServerName blog.example.com\n  ProxyPreserveHost On\n  ProxyPass / http://127.0.0.1:2368/\n  ProxyPassReverse / http://127.0.0.1:2368/\n  SSLEngine on\n  SSLCertificateFile /etc/ssl/blog.crt\n  SSLCertificateKeyFile /etc/ssl/blog.key\n</VirtualHost>\n<VirtualHost *:80>\n  ServerName php.example.com\n  DocumentRoot /var/www/php\n</VirtualHost>\n",
	})
	h := Host{Root: root}
	cr, err := Caddy(h, "/etc/caddy/Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	m := byKey(Finalize(cr))
	if r := m["example.org/"]; !r.Migratable() || r.Target.Port != 8080 || !r.TLS || !r.ForceHTTPS {
		t.Fatalf("caddy root: %+v", r)
	}
	if r := m["www.example.org/api/"]; r.Target.Port != 9000 {
		t.Fatalf("caddy api: %+v", r)
	}
	if r := m["plain.example.org/"]; r.TLS || !strings.HasPrefix(r.Htpasswd, "admin:$2") || r.Target.Host != "10.0.0.5" {
		t.Fatalf("caddy plain: %+v", r)
	}
	if r := m["files.example.org/"]; r.Status != StatusFallback {
		t.Fatalf("caddy file_server: %+v", r)
	}

	hr, err := HAProxy(h, "/etc/haproxy/haproxy.cfg")
	if err != nil {
		t.Fatal(err)
	}
	m = byKey(Finalize(hr))
	if r := m["shop.example.net/"]; !r.Migratable() || r.Target.Port != 8000 || !r.ForceHTTPS || r.Cert == nil || r.Cert.CertFile != "/etc/haproxy/certs/site.pem" {
		t.Fatalf("haproxy shop: %+v", r)
	}
	if r := m["shop.example.net/api"]; r.Target.Port != 8001 || r.Status != StatusWarn {
		t.Fatalf("haproxy api: %+v", r)
	}

	ar, files, err := Apache(h, "/etc/apache2/apache2.conf")
	if err != nil || len(files) != 3 {
		t.Fatalf("apache: %v %v", err, files)
	}
	m = byKey(Finalize(ar))
	if r := m["blog.example.com/"]; !r.Migratable() || r.Target.Port != 2368 || !r.TLS || !r.ForceHTTPS || r.Cert == nil || r.Status != StatusOK {
		t.Fatalf("apache blog: %+v", r)
	}
	if r := m["php.example.com/"]; r.Status != StatusFallback {
		t.Fatalf("apache static: %+v", r)
	}
}

func TestRebindConfig(t *testing.T) {
	cases := []struct{ kind, in, want string }{
		{"nginx", "server { listen 80; server_name a; }\n", "server { listen 127.0.0.1:10080; server_name a; }\n"},
		{"nginx", "server {\n    listen 80 default_server;\n    listen [::]:80;\n    listen 443 ssl http2;\n    listen 127.0.0.1:8080;\n}",
			"server {\n    listen 127.0.0.1:10080 default_server;\n    listen [::1]:10080;\n    listen 127.0.0.1:10443 ssl http2;\n    listen 127.0.0.1:8080;\n}"},
		{"apache", "Listen 80\n<IfModule ssl_module>\n\tListen 443\n</IfModule>\n<VirtualHost *:80>\n<VirtualHost *:443 [::]:443>",
			"Listen 127.0.0.1:10080\n<IfModule ssl_module>\n\tListen 127.0.0.1:10443\n</IfModule>\n<VirtualHost *:10080>\n<VirtualHost *:10443 [::]:10443>"},
		{"haproxy", "frontend web\n    bind *:80\n    bind :::443 v4v6 ssl crt /x.pem\n    bind 127.0.0.1:9000",
			"frontend web\n    bind 127.0.0.1:10080\n    bind 127.0.0.1:10443 ssl crt /x.pem\n    bind 127.0.0.1:9000"},
		{"haproxy", "    bind [::]:80\n", "    bind ::1:10080\n"},
		{"caddy", "example.org {\n\treverse_proxy :8080\n}\n", "{\n\thttp_port 10080\n\thttps_port 10443\n\tdefault_bind 127.0.0.1 [::1]\n}\n\nexample.org {\n\treverse_proxy :8080\n}\n"},
		{"caddy", "{\n\temail a@b.c\n}\nhttp://x.org:80 {\n}\n", "{\n\thttp_port 10080\n\thttps_port 10443\n\tdefault_bind 127.0.0.1 [::1]\n\n\temail a@b.c\n}\nhttp://x.org:10080 {\n}\n"},
	}
	for _, c := range cases {
		got, changed := RebindConfig(c.kind, c.in)
		if got != c.want || !changed {
			t.Errorf("%s:\n got %q\nwant %q", c.kind, got, c.want)
		}
		if again, ch := RebindConfig(c.kind, got); ch {
			t.Errorf("%s: not idempotent: %q", c.kind, again)
		}
	}
}

func TestListenersFromProc(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"/proc/net/tcp": "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
			"   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0000000000000000 100 0 0 10 0\n" +
			"   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 2222 1 0000000000000000 100 0 0 10 0\n" +
			"   2: 00000000:01BB 0100007F:9C40 01 00000000:00000000 00:00000000 00000000     0        0 3333 1 0000000000000000 100 0 0 10 0\n",
		"/proc/net/tcp6": "  sl  local_address remote_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode\n" +
			"   0: 00000000000000000000000000000000:01BB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 4444 1\n",
		"/proc/812/comm":    "nginx\n",
		"/proc/812/cmdline": "nginx: master process /usr/sbin/nginx -c /etc/nginx/nginx.conf\x00",
		"/proc/812/cgroup":  "0::/system.slice/nginx.service\n",
	})
	fd := filepath.Join(root, "proc/812/fd")
	os.MkdirAll(fd, 0o755)
	os.Symlink("socket:[1111]", filepath.Join(fd, "6"))
	os.Symlink("socket:[4444]", filepath.Join(fd, "7"))
	ls := Host{Root: root}.Listeners(80, 443)
	if len(ls) != 2 || ls[0].Port != 80 || ls[0].Address != "0.0.0.0" || ls[0].Process != "nginx" || ls[0].Unit != "nginx.service" ||
		ls[1].Port != 443 || ls[1].Address != "::" || ls[1].PID != 812 {
		t.Fatalf("listeners: %+v", ls)
	}
	if hostProxyConfig(Host{Root: root}, HostProxy{Kind: "nginx", Command: ls[0].Command}) != "/etc/nginx/nginx.conf" {
		t.Fatal("config from -c flag")
	}
}

func TestFinalizeDuplicates(t *testing.T) {
	rs := Finalize([]Route{
		{Source: "a", Ref: "1", Host: "X.io.", PathPrefix: "", Target: Target{Host: "h", Port: 1}},
		{Source: "b", Ref: "2", Host: "x.io", PathPrefix: "/", Target: Target{Host: "h", Port: 1}}, // identical → merged
		{Source: "c", Ref: "3", Host: "x.io", PathPrefix: "/", Target: Target{Host: "other", Port: 2}},
	})
	if len(rs) != 2 || rs[1].Status != StatusFallback || rs[0].Status != StatusFallback {
		t.Fatalf("duplicates: %+v", rs)
	}
}
