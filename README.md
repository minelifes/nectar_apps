# ⬢ Nectar Apps

**Deploy any app to your own server in three clicks.** Pick a type (Java JAR, native binary, static site,
Node, Python, PHP, Docker image), pick a runtime version, upload — it runs in its own Kubernetes space
and is live at `myapp.yourdomain.com` behind [nectar_gateway](https://github.com/minelifes/nectar_gateway).

📖 **Documentation:** [minelifes.github.io/nectar_apps](https://minelifes.github.io/nectar_apps/) (source in [`docs/`](docs)).

> 🚧 **Early development.** Deploys, Git builds, domains, alerts, databases, backups, updates and the web terminal work; they have
> not yet been tested on many real servers. Expect rough edges.

## Install (Linux server)

```bash
curl -sfL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh \
  | sudo sh -s -- --base-domain apps.example.com --email you@example.com
```

- Fresh server: installs k3s (without Traefik/ServiceLB), nectar-gateway on ports 80/443 and the panel.
- Server that already runs k3s: see [Existing k3s server](#existing-k3s-server-adopt-mode) below.
- `--dry-run` shows every command without changing anything. `--help` lists all options.
- Supported: Debian/Ubuntu, RHEL/Rocky/Alma/Fedora, Alpine, Arch, openSUSE — amd64 and arm64.

The panel is not exposed publicly until you finish the first-run setup; the installer prints the SSH tunnel
command and the one-time setup link, plus the **master key** (store it safely — needed to restore backups).

## Existing k3s server (adopt mode)

Nectar can move into a k3s server that already runs apps, with no broken host along the way.

```bash
sudo sh install.sh --discover          # read-only report: proxies, routes, TLS, and what migrates (✓ ⚠ ✗)
sudo sh install.sh --adopt             # backup → shadow install → verify → cutover (asks before each change)
sudo sh install.sh --status            # phase, plus hosts that still reach the old proxy
sudo sh install.sh --rollback          # the old proxy serves 80/443 again
sudo sh install.sh --finalize          # once nothing uses the old proxy any more
```

- **Discovery** finds in-cluster controllers (Traefik, ingress-nginx, HAProxy, Kong, Contour, Istio, Cilium)
  and proxies on the host (nginx, Caddy, Apache, HAProxy, a standalone nectar_gateway). It also finds who
  holds ports 80/443, and every route with its TLS setup.
- **Strategies:**
  - `migrate` (default): nectar-gateway takes 80/443, and the old proxy stays behind it as a fallback for
    anything it could not translate.
  - `keep`: your ingress controller stays, and the panel writes standard Ingress objects.
  - `adopt-gateway`: imports a standalone nectar_gateway.
- **Safety:** the backup covers the k3s datastore, the routing objects and the proxy configs. The new
  gateway first runs on shadow ports 18080/18443 and is compared with the old proxy host by host (status,
  redirects, headers, body). The cutover has a health gate that rolls back automatically. Hosts that still
  use the fallback are counted, and finalize refuses to run while any host depends on the old proxy.
- **Afterwards,** Ingress, IngressRoute and HTTPRoute objects keep working: the panel syncs them to gateway
  routes, and cert-manager Secrets become gateway certificates. Synced Ingresses get the edge node's address
  in their status, as with any ingress controller (`kubectl get ingress`, Argo CD health). Existing
  namespaces show up read-only, and you can adopt any of them as a project.
- **The built-in registry** is not added to an existing server unless you pass `--registry` (its k3s mirror
  needs one k3s restart).

## Alerts and disk housekeeping

- **Alerts** are checked every 30 seconds: app or database down, crash loop, out-of-memory kills, node not ready, high
  CPU/memory, disk almost full, expiring certificates, failed backups and an unreachable gateway. Thresholds and durations
  are set under **Alerts → Rules**.
- **Notifications** go to ntfy, Telegram, email (SMTP) or a signed webhook when an alert fires and when it
  clears. Optional reminders can repeat while an alert keeps firing.
- **Disk:** every hour the panel keeps the last 10 uploads per app (and any upload a live release uses), plus
  365 days of audit log and 90 days of resolved alerts. **Server → Disk** shows usage and can remove container
  images that no container uses.
- **Image pre-pull** (optional, Server → Disk → Retention): a DaemonSet keeps busybox, the runtime images your
  apps use and any extra images on every node, so a new node or a moved pod starts without downloading them, and
  image cleanup never removes them. Each image costs one sleeping container (1m CPU, 4 MiB).

## Databases

A project can run **PostgreSQL, MySQL, MariaDB or Redis** next to its apps (project page → Databases). Each
one gets its own volume and generated credentials. Link it to an app and the app gets `DATABASE_URL` (or
`REDIS_URL`) plus the usual `PG*` / `MYSQL_*` variables with its next deploy; a prefix such as `CACHE_` lets
one app use several databases. Databases are only reachable from inside the project.

## Backups and disaster recovery

**Backups** stores encrypted, deduplicated [restic](https://restic.net) snapshots on S3-compatible storage
(AWS, Backblaze B2, Cloudflare R2, MinIO), an SFTP server or a folder on the server. A plan backs up the
volumes of an app, a database dump or **the panel itself** (its database and uploads) on a schedule and keeps
7 daily, 4 weekly and 6 monthly snapshots by default. Any snapshot can be restored from the panel; a failed
backup raises an alert.

To rebuild a lost server, keep two things somewhere safe: the **master key** printed by the first install and
the **recovery kit** of the target that holds the panel backup (Backups → target → Recovery kit). Then, on the
new server:

```bash
curl -fsSL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh \
  | sudo bash -s -- --base-domain apps.example.com --restore-kit kit.json --master-key 'KEY'
```

The panel restores its database first, then recreates the databases (with their latest dumps) and redeploys
the apps (with their latest volume snapshots). A folder target on the lost server is gone with it — use
off-site storage for the panel backup.

### Rotating the master key

```bash
sudo k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin rotate-master-key
```

The command stores a new key next to the current one (`master-key-next` in Secret `nectar-secrets`) and
restarts the panel. On start the panel re-seals every secret it keeps (environment variables, files, database
credentials, two-factor secrets, alert channels, backup targets, registry logins, Git tokens, SMTP password) in
one transaction, then makes the new key the current one. A start that fails half-way finishes the switch on the
next start. It prints the new key once: store it, download new recovery kits and take a panel backup — backups
made before the rotation still need the old key, so keep that too until the new backup exists.
`--key K` uses your own key (base64, 32 bytes); `--no-restart` leaves the restart to you.

## Security

- **Two-factor authentication** (any authenticator app) under Settings → Security, with 10 one-time recovery
  codes. Settings also lists your sessions and signs out the ones you don't recognize.
- **Passkeys** (Settings → Passkeys, web panel): sign in with Touch ID, Face ID, Windows Hello, a phone or a
  security key — no email, password or code. A passkey counts as both factors and belongs to the panel's
  address (one made at `panel.example.com` does not work at another address of the panel). Resetting a user's
  two-factor authentication also removes their passkeys.
- **IP allowlist** (Settings → Access): only the listed addresses or networks reach the panel. The panel
  refuses a list that would lock you out, and connections from the server itself or a private network
  (e.g. an SSH tunnel to the panel) always pass.
- **Email** (Settings → Email): with an SMTP server the panel sends invitations, password reset links and,
  if you want, alert emails.

## Updates

**Settings → Updates** shows the versions of the panel, nectar-gateway and k3s and what is available. The panel
reads a small `updates.json` feed (GitHub Pages, published by each repo's release workflow) and falls back to
the GitHub Releases API. Per component you choose **off**, **notify only** (the default), **auto: patch
releases** or **auto: patch + minor**, the channel (stable or beta), a maintenance window and how often to check.
Major and breaking releases always wait for you.

An update runs in a short-lived Job, so the panel can update itself: it backs up the database, switches the image
digest, waits until the new version is healthy (for the gateway also its canary and every route) and rolls back on
its own when a check fails; a release that failed is not tried again automatically. Panel and gateway are
updated in an order that keeps them compatible, and **Roll back** in the history returns to the previous
versions. The web UI updates with the panel ("new version, reload").

k3s updates need Rancher's system-upgrade-controller (`install.sh --k3s-upgrades`); the panel then manages its
upgrade plans (follow a channel in the maintenance window, or upgrade to a version now).

## Git builds and registries

An app of the **Docker image** type can be built from a Git repository instead (New app → Docker image → Build
from Git, or the app's Settings): give the repository URL, the branch and, for private repositories, an access
token. The repository needs a `Dockerfile`. Each build runs as a [Kaniko](https://github.com/GoogleContainerTools/kaniko)
Job in its own namespace, pushes `registry.nectar.local/<project>/<app>:build-N` to the **built-in registry** and
deploys it; the Builds tab shows every build with its log. Add the push webhook shown there to GitHub, GitLab or
Gitea and every push to the branch builds and deploys. The newest 5 images per app are kept.
Builds use Kaniko v1.23.2 (the last upstream release); to use a maintained fork set `NECTAR_BUILDER_IMAGE`
on the panel, e.g. `kubectl -n nectar-system set env deploy/nectar-apps NECTAR_BUILDER_IMAGE=<image>`.

The built-in registry is set up by `install.sh` on new servers (`--registry` adds it to an existing one, which
restarts k3s once; `--no-registry` skips it). It is reachable only from the cluster and with a password: nodes
pull through a k3s mirror (`/etc/rancher/k3s/registries.yaml`). The installer manages only its own block of
that file; if the file already has `mirrors:` or `configs:` of yours, it leaves it alone and prints the entries
to merge.

**Preview environments:** switch on **A preview for every pull request** in the app's Git source (Builds tab →
Edit) and let the webhook also send pull request events (GitLab: merge request events). Each pull request into
the app's branch then gets its own app in the same project — `<app>-pr<N>`, a copy with the app's settings,
variables and config files, at `<app>-pr<N>-<project>.<base domain>` — built from the pull request's head,
rebuilt on new commits and deleted when the pull request is closed or merged (or after 14 days without activity).
Previews start without database links unless you allow them to use the app's databases, and pull requests from
forks are skipped unless allowed (their code would run with the app's secrets). With an access token that may
write commit statuses, the pull request shows the result as `nectar/preview` with a link to the preview.

Images from private registries (GHCR, Docker Hub, GitLab, your own) work with a login under **Settings →
Registries**; every project gets it as an image pull secret, and builds use it for base images.

## More servers

**Server → Add node** shows a one-line command for another Linux machine: it installs k3s as an agent, joins
the cluster and sets up the registry mirror. Apps then spread over the nodes; nectar-gateway keeps serving
80/443 from the edge node your DNS points to. Each node card can stop new pods (cordon), move its pods away
(drain, respecting disruption budgets) and remove a drained worker from the cluster. Draining the edge node,
or the last node that could take the pods, asks again first: it would take the gateway or the panel offline.

**Several edge nodes.** On a node card, **Serve traffic too (edge node)** starts a nectar-gateway replica on that
node (DaemonSet `nectar-gateway-edge`, label `nectar.dev/edge-replica=true`): it copies routes, certificates and
IP blocks from the main gateway every few seconds, so the node answers on 80/443 like the main edge node, and
Let's Encrypt challenges work whichever node they reach. DNS records the panel manages then list every edge
node (round-robin; a node that goes down should also be taken out of DNS: **Stop serving traffic**). Open ports
80 and 443 on that machine. Needs the latest nectar-gateway.

## One-click apps, docker-compose and nectar.yaml

- **One-click apps** (project → One-click app): WordPress, Grafana, n8n, MinIO, MongoDB, RabbitMQ, Uptime Kuma,
  Gitea. Pick one, name it, install: the apps, a managed database where needed, volumes and generated
  passwords (secret variables) are created and deployed. The catalog is data (`internal/blueprint/templates.yaml`).
- **docker-compose** (project → ⋮ → Import docker-compose.yml, or `nectar import-compose -f docker-compose.yml
  --project shop`): every service becomes an app with the same name, so the services still find each other;
  services with published ports get a domain, databases and brokers stay internal, named volumes become volumes.
  "Check" shows the converted file and what could not be imported (bind mounts, `build:`, `env_file`).
- **nectar.yaml** (project → ⋮ → Export / Apply, or `nectar export --project shop > nectar.yaml` and
  `nectar apply -f nectar.yaml --project shop`): the project's apps, databases, variables, volumes, links and
  domains as one file for Git. Applying creates what is missing and updates existing apps; it never deletes.
  Secret values are not exported — a secret without a value keeps the current one.
- **References** in variables are filled in at every deploy: `${db.DATABASE_URL}` (a database's connection
  settings, without linking it), `${api.host}`, `${api.port}`, `${api.url}` (another app of the project) and
  `${api.SOME_VAR}` (one of its variables). Workers with a port (e.g. MongoDB) and `internal_ports` are reachable
  by the project's apps at `<app>:<port>`, never from outside.

## Log history

App logs are kept on the panel's disk (7 days, up to 512 MB per app by default — Server → Disk → Retention):
the Logs tab has **Live** (the stream) and **History**, where you search every pod the app ever had — also after
crashes, restarts and deploys — by text or regular expression over the last hour up to everything kept. The
panel follows the pods itself; nothing is installed in the cluster. History is stored per hour and compressed
once the hour is over; deleting an app deletes its logs. Switch it off there as well.

## Volumes and storage

- **Files** (app → Settings → Volumes → Files): browse a volume, upload and download files (up to 64 MB),
  edit text files, create folders, rename and delete — also while the app is stopped and for images without a
  shell. A small helper pod mounts the volume while the dialog is open (it stops after an hour at the latest).
  New files get the owner of their folder. Needs the `volume.manage` permission; every change is audited.
- **Usage:** Server → Volumes lists every app and database volume with its usage, and the **Volume almost
  full** alert (90 % by default, Alerts → Rules) warns before one runs out. On the default local-path storage a
  size is not enforced: a volume grows until the node's disk is full, which the "Disk almost full" alert covers.
- **Storage classes:** volumes live on one node by default (k3s local-path). `install.sh --longhorn` adds
  Longhorn (volumes replicated across nodes, storage class `longhorn`), `install.sh --nfs 10.0.0.5:/srv/nectar`
  an NFS storage class `nfs` (volumes that several replicas on several nodes can share). Pick the class per
  volume when you add it (it is fixed afterwards); `--join` with the same flag installs the storage client on a
  new node.

## Deploy from the command line / CI

Create a token under **Settings → API tokens** (for CI: role **Deployer**, limited to one project), then use the
`nectar` CLI. `install.sh` puts it on the server (`/usr/local/bin/nectar`, already pointed at the local panel, so
`nectar login` only asks for the token; `--no-cli` skips it). For your own machine, binaries for Linux, macOS and
Windows are attached to every release, and `make cli` builds it:

```bash
nectar login --url https://panel.apps.example.com          # asks for the token, saves it in ~/.config/nectar
nectar apps
nectar deploy shop/api --file target/app.jar                # uploads, deploys, waits until it is live
nectar deploy shop/web --image ghcr.io/acme/web:1.4.2       # apps of the Docker image type
nectar status shop/api ; nectar logs shop/api --tail 100 ; nectar rollback shop/api
```

A rollback brings back the code and settings of that release; the app keeps its current number of replicas and
its volumes.

In CI, set `NECTAR_URL` and `NECTAR_TOKEN` instead of logging in. For GitHub Actions there is an action:

```yaml
- uses: minelifes/nectar_apps@v0
  with:
    url: ${{ secrets.NECTAR_URL }}
    token: ${{ secrets.NECTAR_TOKEN }}
    app: shop/api
    file: target/app.jar          # or: image: ghcr.io/acme/api:${{ github.sha }}
```

Complete workflows are in [`examples/github-actions`](examples/github-actions). Everything the panel does is
also available over HTTP with `Authorization: Bearer nat_…` ([`api/openapi.yaml`](api/openapi.yaml)); large files
can use the resumable [tus](https://tus.io) endpoint `POST /api/apps/{id}/uploads`.

## Access

- **Users and roles:** Settings → Users invites people (by email, or a link to share) and gives them roles
  everywhere or per project; Settings → Roles shows the permission matrix, and built-in roles can be cloned
  into your own. A project's **Members** card lets people with `project.members` add colleagues to it.
- **Single sign-on** (Settings → Sign-in providers): GitHub, Google or any OpenID Connect provider (Authentik,
  Keycloak, Zitadel, Okta, Entra ID …). Register the callback URL the card shows at the provider, paste the
  client ID and secret, and the login page gets a button. Without **allowed domains** only existing users can
  sign in with it (matched by verified email); with them, **create users** adds people on their first sign-in
  with a default role, and OIDC **group → role** mappings are applied at every sign-in.
- **API tokens** act as their user, optionally limited to a role and a project, and can expire.
- **Native push (optional):** Settings → Push notifications takes your own Firebase project (service account key,
  sender id, Android/Apple app ids; iPhone and Mac also need your APNs key uploaded to Firebase and the Push
  Notifications capability in Xcode). Signed-in Nectar apps then register themselves, and an Alerts channel of
  kind **Mobile app** delivers there (tapping a notification opens its page). Without Firebase, use ntfy or Telegram.
- **On your phone:** add an ntfy or Telegram channel under Alerts. Besides incidents (app down, crash loop,
  out of memory, …) the panel can tell you when a deploy fails or finishes (Alerts → Rules → Deploy failed /
  Deploy finished).
- **Wildcard certificates** (Gateway → Certificates): add a Cloudflare API token (Zone:Read + DNS:Edit) and click
  **Get \*.\<base domain\>**. One Let's Encrypt certificate, proved over DNS-01, then serves every app subdomain:
  new apps have HTTPS at once and need no certificate of their own (no port 80 check, no per-app rate limits).
  Other wildcards (`*.example.com`) work the same. Needs nectar-gateway 1.2.2 or newer.
- **Gateway** (Server → Gateway): every route with its health check, certificate and rate limit; the latest
  requests over all domains (filter by domain, status class or client IP, block an IP from its row); wildcard
  certificates; blocked IPs (block or unblock by hand). Rate limits, upload size and timeouts are set per
  domain on the app page. With gateway 1.2.2 or newer, requests over a limit get 429 and only a client that
  keeps hammering is blocked (Blocked IPs → block length); older gateways block for 24 hours on every
  domain, so the panel then accepts limits of 10 requests/s or more.

## Domains

- **Domains page** (sidebar → Domains): every domain of every app you can see, with its project, HTTPS state,
  web options and DNS state; search, "Custom" and "Problems" filters, a DNS check and the edit dialog per row.
  The **DNS setup** card lists the records to create at your DNS provider (`*.<base domain>` and your own
  domains → the server's IPs).
- **Automatic DNS records:** with a DNS provider in the gateway (Gateway → Certificates → DNS provider,
  Cloudflare) switch on **Create DNS records automatically**: the gateway creates one `*.<base domain>` record
  for every default domain and an A/AAAA record for each custom domain you add, re-points them when the
  server's IP changes and removes a record with its domain. Records someone else created are never changed
  (the domain shows "existing records"). Needs the latest nectar-gateway.
- **Redirects, access, headers** (domain dialog): ordered redirect rules (`/old → /new`, or `/blog/… →
  https://blog.example.com/…`), an IP allowlist (others get 403), response and request headers, CORS (preflights
  answered by the gateway), gzip for text responses and HSTS. Needs the latest nectar-gateway; with an older
  gateway the domain keeps working without them and says so.

## Development

Requirements: Go 1.26+. Everything else (Homebrew, Docker runtime, k3d, kubectl) is installed by the dev script.

### Macos
Install and launch docker. Then this, this will create a k3d cluster with a k3s server and a single node. Then i can call next OS commands.
```bash
make dev-cluster
```
### Next Step (or Linux first step)

```bash
make dev                          # installs what is missing, starts k3s in Docker (k3d) + kubectl proxy, runs the panel
open http://localhost:8080        # setup token in dev mode: "dev"
make dev-cluster-down             # stop and delete the dev cluster
```

Web and mobile UI (needs Flutter): see [ui/README.md](ui/README.md).

```bash
make ui-setup    # once: generate the Flutter platform folders
make ui-dev      # the UI in Chrome against `make run`
make ui build    # embed the web UI into the binary
```

```bash
make test        # unit + API tests
make lint        # go vet + gofmt
make shellcheck  # installer lint
make help        # everything else
```

Layout: `cmd/nectar-apps` (binary) · `internal/` (api, auth, authz, store, kube, config, secretbox) ·
`api/openapi.yaml` (API contract) · `deploy/` (installer + manifests) · `web/` (embedded UI) · `ui/` (Flutter, M3).

### Recovery

```bash
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin reset-password --email you@example.com
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin make-owner --email you@example.com
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin reset-mfa --email you@example.com   # lost phone
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin clear-ip-allowlist                  # locked out
```

## License

Source-available under the [Nectar Source License](LICENSE.md): free for personal use and home labs,
education and nonprofits, and small businesses (<100 people and <$1M revenue). Commercial licenses:
support@techbrain.llc. Contributions require the [CLA](CLA.md). "Nectar" is a trademark of TechBrain LLC —
see [TRADEMARK.md](TRADEMARK.md).
