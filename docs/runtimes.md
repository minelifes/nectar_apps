# Runtimes

Artifact apps need no Dockerfile: the pod's init container downloads your upload (a short-lived signed URL)
into the pod, and the main container is an official runtime image. Deploys take seconds and there is no
registry involved. A small optional pre-pull DaemonSet keeps the runtime images your apps use warm on every
node (Server → Disk → Retention).

| Type | Upload | Versions | Image |
|---|---|---|---|
| **Java (JAR)** | an executable `.jar` | 25 LTS, 21, 17, 11, 8 | Eclipse Temurin JRE |
| **Native binary** | one executable (Go, Rust, C/C++, Zig …) | Debian, Alpine, distroless, static | matching base image |
| **Static website / SPA** | zip of `dist/` or `build/` | nginx | nginx (SPA fallback to `index.html`) |
| **Node.js** | zip with `package.json` | 24, 22, 20 | official `node` |
| **Python** | zip with `requirements.txt` | 3.13, 3.12, 3.11 | official `python` |
| **PHP** | zip of the site | 8.4, 8.3 | `php` with Apache |
| **Docker image** | an image reference, or [build from Git](git-builds.md) | — | yours |

The panel detects the type of an upload (markers such as `package.json`, `requirements.txt`, `index.php`,
`index.html`, `Dockerfile`) and suggests it. The command, port and health check have sensible defaults per
runtime and can be changed in the app's settings (e.g. JVM options as arguments, a start script for Node).

## Image updates (Docker image apps)

An image app follows a tag — `postgres:17`, `grafana/grafana:11`, `ghcr.io/acme/api:stable`. Publishers move
such tags to new images (security patches, rebuilds), but a running app keeps the image it pulled. App →
Settings → **Image updates** watches the tag:

- **Off** (default) — nothing is checked automatically; *Check now* still tells you.
- **Notify** — when the tag points to a newer image than the app runs, the app shows it and the alert rule
  **Newer image available** notifies your channels (once per new image). *Deploy the newer image* deploys it.
- **Deploy automatically** — the newer image is deployed like any deploy (health checks; a failed one keeps
  the old release serving and raises *Deploy failed*). It is deployed pinned to its digest
  (`postgres:17@sha256:…`), so every node runs the same image and you can roll back to the previous one. An
  app with undeployed settings or variables is not updated automatically — only announced — so a security
  patch never ships half-finished changes.

Apps are checked every 6 hours (`NECTAR_IMAGE_WATCH_HOURS` on the panel, `0` = only by hand). Private
images use the logins from Settings → Registries. Images pinned to a digest and images built by Nectar
(Git builds) are not watched. The check asks the registry for the tag's digest only (a `HEAD` request), which
does not count against Docker Hub's pull limit.

## Settings that apply to every app

- **Port** and **health check** (HTTP path or TCP), replicas, CPU and memory limits.
- **Release command**: runs before the new version starts (e.g. database migrations); a failure stops the deploy.
- **Environment variables** and secrets, env groups, linked databases.
- **Volumes** (persistent disks) and **files** (config files mounted into the container).
- **Extra ports** (TCP/UDP opened on the server) and **internal ports** (reachable by the project's apps only).
- A **web terminal** into any running pod.
