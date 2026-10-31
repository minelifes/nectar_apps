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
