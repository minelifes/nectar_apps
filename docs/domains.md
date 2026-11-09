# Domains and HTTPS

Every web app gets a default domain, `<app>-<project>.<base domain>`, with a Let's Encrypt certificate. Add
your own domains under app → Domains; point them at the server (or let the panel create the DNS records).

## Domains page

Sidebar → **Domains** lists every domain of every app you can see with its project, HTTPS state, web options
and DNS state; filter by "Custom" or "Problems", check DNS, and edit each row. The **DNS setup** card lists
the records to create at your DNS provider (`*.<base domain>` and your own domains → the server's IPs).

## Certificates

- Per domain: the gateway obtains and renews a certificate over HTTP-01 (port 80 must reach the server).
- **Wildcard** (Gateway → Certificates): add a Cloudflare API token (Zone:Read + DNS:Edit) and click
  **Get \*.&lt;base domain&gt;**. One certificate proved over DNS-01 then serves every app subdomain: new apps
  have HTTPS at once, without per-app certificates or rate limits. Other wildcards work the same.
- Upload your own certificate for a domain, or keep cert-manager (adopt mode syncs its Secrets).

## Automatic DNS records

With a DNS provider in the gateway, switch on **Create DNS records automatically**: one `*.<base domain>`
record covers every default domain, and each custom domain gets an A/AAAA record. Records are re-pointed when
the server's IPs change (or when you add an [edge node](nodes.md#several-edge-nodes)) and removed with their
domain. Records someone else created are never changed (the domain shows "existing records").

## Web options (per domain)

In the domain dialog:

- **Redirect rules** in order (`/old → /new`, `/blog/… → https://blog.example.com/…`, 301/302/307/308), and
  redirect domains (www ↔ apex).
- **IP allowlist**: other clients get 403.
- **Headers**: response and request headers to set or remove.
- **CORS**: allowed origins, methods and headers; preflights are answered by the gateway.
- **Compression** (gzip for text responses) and **HSTS**.
- **Basic auth**, **rate limit** (requests/s per client; over the limit → 429, a client that keeps hammering
  is blocked for a while), **upload size**, **timeouts**, **maintenance page**, backend protocol (HTTP/1.1 or
  h2c for gRPC).

## Gateway page

Server → Gateway shows every route with its health check, certificate and rate limit, the latest requests over
all domains (filter by domain, status class or client IP; block an IP from its row), certificates and blocked IPs.

## Your own domain, editing, and paths to other apps

- **New app:** the *Name & domain* step takes an optional own domain; it is added next to the generated
  default domain (`<app>-<project>.<base domain>`), which always stays.
- **Edit a domain:** on the app page, Domains → the settings icon. Host and path can be changed (the domain is
  replaced by one with the same settings and gets a fresh certificate); the generated default keeps its name.
- **Route a path to another app:** on app1's Domains card, *Route a path to another app*: e.g.
  `app1.com/myCustomRoute` → `app2` (a web app, or a **worker with a port**), optionally on another of its
  ports. *Remove the path before forwarding* sends `/myCustomRoute/users` to app2 as `/users`; off, app2 gets
  the full path. The longest matching path wins, so the rest of `app1.com` still goes to app1. The route is a
  domain of app2 (host app1.com, path /myCustomRoute) and is listed on both apps.
- Removing the path needs nectar-gateway with the `strip-prefix` feature; an older gateway leaves such a
  route inactive with a message instead of sending app2 paths it does not expect.

## Proxies (a domain without an app)

Domains page → **Proxies** → *Add proxy*: a domain (and optional path) that the gateway forwards straight to
an address and port — no app needed:

- `127.0.0.1:9000` — a service running directly on the server (the gateway uses the host network; with
  several gateway nodes each forwards to its own `127.0.0.1`),
- `10.0.0.5:8080` — another machine the server reaches,
- `grafana.monitoring.svc.cluster.local:3000` — a Kubernetes Service Nectar does not manage.

Tick *The target uses HTTPS* when the target only speaks HTTPS (its certificate is not checked). Everything
else works as for app domains: HTTPS certificates, DNS records, rate limits, body size, web options, password,
removing the path. Choose **Redirect** instead to send a domain elsewhere (301/302/307/308; without a path of
its own the request's path and query are kept). Changing proxies needs `settings.manage`; the CLI has
`nectar proxy list|add|set|rm`.

## Sleeping when idle (scale to zero)

App → Settings → *Network and health check* → **Sleep when idle**: a web app that gets no requests through
its domains for the chosen time (5–1440 minutes) scales to zero and shows *Sleeping*. It frees its CPU and
memory; its volumes stay. The next visitor wakes it: nectar-gateway holds the request (up to 90 s) while the
app starts and then passes it on; a browser that gives up first sees a short "This app is starting" page that
retries. **Wake now** on the app page, a deploy, a start, or turning the setting off wake it too.

- Needs nectar-gateway with the `wake` feature (update the gateway); not with the Ingress provider.
- Only requests through the gateway count and wake the app. Other apps calling it inside the cluster
  (`http://<app>.<namespace>.svc`) neither keep it awake nor wake it — don't use it for internal APIs.
- While it sleeps the gateway's health checks for its domains are off, so it doesn't raise "down" alerts.
- The setting takes effect without a deploy.
