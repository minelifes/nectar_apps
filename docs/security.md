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
