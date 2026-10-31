# Backups and recovery

**Backups** stores encrypted, deduplicated [restic](https://restic.net) snapshots on S3-compatible storage
(AWS, Backblaze B2, Cloudflare R2, MinIO), an SFTP server or a folder on the server.

A **plan** backs up the volumes of an app, a database dump or **the panel itself** (its database and uploads)
on a schedule and keeps 7 daily, 4 weekly and 6 monthly snapshots by default. Any snapshot can be restored
from the panel; a failed backup raises an alert.

## Rebuilding a lost server

Keep two things somewhere safe: the **master key** printed by the first install and the **recovery kit** of
the target that holds the panel backup (Backups → target → Recovery kit). On the new server:

```bash
curl -fsSL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh \
  | sudo bash -s -- --base-domain apps.example.com --restore-kit kit.json --master-key 'KEY'
```

The panel restores its database first, then recreates the databases (with their latest dumps) and redeploys
the apps (with their latest volume snapshots). A folder target on the lost server is gone with it — use
off-site storage for the panel backup.

## Rotating the master key

```bash
sudo k3s kubectl -n nectar-system exec deploy/nectar-apps -- nectar-apps admin rotate-master-key
```

The command stores a new key next to the current one and restarts the panel, which re-seals every secret
(environment variables, files, database credentials, two-factor secrets, alert channels, backup targets,
registry logins, Git tokens, SMTP password, sign-in provider and push secrets) in one transaction, then makes the
new key the current one. It prints the new key once: store it, download new recovery kits and take a panel
backup — older backups still need the old key. `--key K` uses your own key (base64, 32 bytes);
`--no-restart` leaves the restart to you.
