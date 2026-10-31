# Nectar Apps

**Deploy any app to your own server in three clicks.** Pick a type — Java JAR, native binary, static site,
Node.js, Python, PHP or a Docker image — pick a runtime version and upload. The app runs in its own
Kubernetes namespace and is live at `myapp.yourdomain.com` behind
[nectar-gateway](https://github.com/minelifes/nectar_gateway), with HTTPS from Let's Encrypt.

!!! warning "Early development"
    Deploys, Git builds, domains, alerts, databases, backups, updates and the web terminal work, but they have
    not yet been tested on many real servers. Expect rough edges.

## What you get

| | |
|---|---|
| **One install** | `install.sh` sets up k3s, nectar-gateway (ports 80/443) and the panel on a Linux server — or moves into a k3s server you already run ([adopt mode](adopt.md)). |
| **Apps without Dockerfiles** | Upload a JAR, a binary, a zip of your site — the panel runs it on an official runtime image. Or deploy any image, or build from Git. |
| **Domains and HTTPS** | Every app gets `<app>-<project>.<base domain>`; add your own domains, redirects, IP allowlists, CORS, HSTS. Wildcard certificates and automatic Cloudflare DNS records. |
| **Databases** | PostgreSQL, MySQL, MariaDB, Redis per project, linked to apps with `DATABASE_URL`. |
| **Operations** | Logs (live + searchable history), metrics, a web terminal, volume file browser, alerts (ntfy, Telegram, email, webhooks, native push), restic backups, one-click updates with rollback. |
| **Teams** | Users, roles per project, API tokens, two-factor authentication, single sign-on (GitHub, Google, OIDC). |
| **Automation** | `nectar` CLI, a GitHub Action, `nectar.yaml` blueprints, docker-compose import, a full [HTTP API](api.md). |
| **Apps on your phone** | The Flutter app (Android, iOS, macOS) is the same UI as the web panel. |

## How it fits together

```text
             internet ──► nectar-gateway (80/443, Let's Encrypt, rate limits, health checks)
                                │   routes + certificates pushed by the panel
                                ▼
   nectar-apps panel ──► k3s: one namespace per project ──► your apps, databases, volumes
   (Go + SQLite, web UI)          Deployments · Services · PVCs · CronJobs · Jobs
```

- The **panel** (`nectar-apps`, Go, SQLite) is the source of truth: it renders Kubernetes objects with
  server-side apply and labels everything it owns. It never touches objects it did not create unless you adopt them.
- The **gateway** (`nectar-gateway`) is a reverse proxy with its own certificate store. It runs on the edge
  node(s) with host networking, so it sees real client IPs.
- The **UI** is Flutter (web build embedded into the panel binary; the same code is the mobile/desktop app).

## License

Source-available under the [Nectar Source License](https://github.com/minelifes/nectar_apps/blob/main/LICENSE.md):
free for personal use and home labs, education and nonprofits, and small businesses (fewer than 100 people and
less than $1M revenue). Commercial licenses: support@techbrain.llc.
