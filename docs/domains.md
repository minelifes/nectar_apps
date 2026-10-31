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
