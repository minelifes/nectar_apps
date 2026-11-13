# Security

- **Secrets** (variables, files, database passwords, tokens, provider secrets) are sealed with the
  **master key** (AES-256-GCM, a key per purpose); see [rotating it](backups.md#rotating-the-master-key).
- **Two-factor authentication** with recovery codes; Settings lists your sessions and signs out the ones you
  don't recognize.
- **IP allowlist** (Settings → Access): only the listed addresses or networks reach the panel. The panel refuses
  a list that would lock you out, and connections from the server itself or a private network (e.g. an SSH
  tunnel) always pass.
- **Email** (Settings → Email): with an SMTP server the panel sends invitations, password resets and alert emails.
- **Isolation:** each project is a namespace with network policies (only the gateway and the project's own apps
  reach its pods), resource quotas, and pods without privileges.
- **Gateway:** rate limits per domain, automatic blocking of clients that keep exceeding them, IP allowlists per
  domain, basic auth, HSTS. The gateway's admin port is closed to the internet by the installer's firewall rules
  and needs the service token.

## Recovery

```bash
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin reset-password --email you@example.com
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin make-owner --email you@example.com
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin reset-mfa --email you@example.com   # lost phone
k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin clear-ip-allowlist                  # locked out
```

Report vulnerabilities privately to support@techbrain.llc.

## Image vulnerability scans

App → **Security** → *Scan* checks the image the app runs for known vulnerabilities with
[Trivy](https://trivy.dev): the image's system packages and the app's dependencies (npm, pip, Maven, Go, …).
You get the counts by severity, how many are fixed in a newer version (update the base image or the
dependency and redeploy), and the worst 300 findings with the package, the installed version and the version
with the fix. Viewing needs `app.view`, scanning `app.deploy`.

- The scan is a short Job in the `nectar-scans` namespace that reads the image from its registry — the
  running app is not touched. Private registries use the saved logins; images built by Nectar are read from
  the built-in registry. One scan runs at a time; it takes a few minutes and up to 1.5 GB of memory for
  large images.
- Each scan downloads Trivy's vulnerability database (from `mirror.gcr.io` / `ghcr.io`), so the server needs
  outbound HTTPS.
- After a deploy of another image the result is marked as out of date. To re-scan every deployed app
  regularly — new vulnerabilities are found in unchanged images too — set `NECTAR_IMAGE_SCAN_HOURS` (e.g.
  `24`) on the panel; `0` (default) scans only on request. `NECTAR_TRIVY_IMAGE` replaces the pinned Trivy
  image (e.g. a copy in your own registry).
- Apps with critical findings get a shield badge with their number in the project's app list. When a scan
  finds critical vulnerabilities that the last one did not (another image, or newly published ones), the
  alert rule **Critical vulnerabilities** notifies your channels once (Alerts → Rules; on by default).
- A finding is a known vulnerability in an installed package, not proof the app can be attacked through it.
