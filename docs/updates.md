# Updates

**Settings → Updates** shows the versions of the panel, nectar-gateway and k3s and what is available. Per
component choose **off**, **notify only** (default), **auto: patch releases** or **auto: patch + minor**, the
channel (stable or beta), a maintenance window and how often to check. Major and breaking releases always wait
for you.

An update runs in a short-lived Job, so the panel can update itself: it backs up the database, switches the
image digest, waits until the new version is healthy (for the gateway also a canary and every route) and rolls
back on its own when a check fails; a release that failed is not retried automatically. Panel and gateway are
updated in an order that keeps them compatible, and **Roll back** in the history returns to the previous
versions. Gateway replicas on extra edge nodes follow the main gateway within a few minutes. The web UI updates
with the panel ("new version, reload").

k3s updates need Rancher's system-upgrade-controller (`install.sh --k3s-upgrades`); the panel then manages the
upgrade plans (follow a channel in the maintenance window, or upgrade to a version now).

The panel reads a small `updates.json` feed published on GitHub Pages by each repository's release workflow and
falls back to the GitHub Releases API.

## Signed updates

When the panel knows a signing key, it only applies panel and gateway updates whose image is signed with it
([cosign](https://docs.sigstore.dev/cosign/), key based). Releases are pinned by digest, the release workflow
signs that digest, and before an update starts the panel reads the signature from the registry
(`sha256-<digest>.sig` next to the image) and checks it with the public key — an image that is unsigned, signed
with another key, or whose signature belongs to another image is not installed (manual and automatic updates
alike; the reason is shown, and announced once for automatic ones). Rolling back to the previous version is
never blocked. Settings → Updates shows whether updates are verified.

- The keys compiled into the build are in `internal/update/release_keys.pem`. `NECTAR_UPDATE_PUBLIC_KEYS`
  (PEM, one or more `cosign.pub` keys) replaces them — needed when you build and sign your own images or use
  your own feed; `off` turns verification off.
- To sign your releases: `cosign generate-key-pair`, put `cosign.key` and its password into the repository
  secrets `COSIGN_PRIVATE_KEY` and `COSIGN_PASSWORD` of **both** repositories (the workflows then sign every
  release), and paste `cosign.pub` into `release_keys.pem`. Releases made before the key existed are not
  signed: ship the public key in a release first, sign from the next one on. To rotate, keep both keys in the
  file for a while.
- The panel needs to reach the registry (ghcr.io) anonymously; only public images can be verified.
