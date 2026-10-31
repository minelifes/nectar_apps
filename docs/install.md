# Install

## Requirements

- A Linux server (amd64 or arm64): Debian/Ubuntu, RHEL/Rocky/Alma/Fedora, Alpine, Arch or openSUSE.
- 2 CPU cores and 2 GB RAM are enough for the panel and a few small apps.
- Ports 80 and 443 reachable from the internet, and a DNS name for your apps (e.g. `*.apps.example.com` →
  the server's IP). The panel can create the records for you on Cloudflare later.

## One command

```bash
curl -sfL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh \
  | sudo sh -s -- --base-domain apps.example.com --email you@example.com
```

On a fresh server this installs k3s (without Traefik and ServiceLB), nectar-gateway on ports 80/443, the panel,
the built-in image registry and the `nectar` CLI, and configures the firewall (ufw or firewalld).

At the end it prints:

- the **master key** — store it somewhere safe; it decrypts every secret and is needed to restore backups;
- an **SSH tunnel command** and a **one-time setup link**: the panel is not public until you create the first
  Owner account.

`--dry-run` prints every command without changing anything.

## Options

| Option | |
|---|---|
| `--base-domain DOMAIN` | base domain for apps (`<app>-<project>.<base domain>`) |
| `--email EMAIL` | Let's Encrypt account email |
| `--admin-email EMAIL` | email of the first Owner |
| `--version V`, `--gateway-version V`, `--k3s-version V` | pin versions (default: latest stable) |
| `--skip-firewall` | leave the firewall alone |
| `--k3s-upgrades` | install Rancher's system-upgrade-controller so the panel can upgrade k3s |
| `--registry` / `--no-registry` | built-in image registry for Git builds (default on new installs) |
| `--longhorn` | Longhorn storage: volumes replicated across nodes (storage class `longhorn`) |
| `--nfs SERVER:/EXPORT` | NFS storage class `nfs` (volumes shared by several replicas) |
| `--join URL --token T` | add this machine to an existing cluster (the panel shows the full command) |
| `--restore-kit FILE --master-key KEY` | rebuild a lost server from its panel backup ([Backups](backups.md)) |
| `--discover`, `--adopt`, `--status`, `--rollback`, `--finalize` | [adopt an existing k3s server](adopt.md) |
| `--no-cli` | do not install the `nectar` CLI |
| `--uninstall` (`--purge-k3s`) | remove Nectar (and k3s — destroys all workloads) |
| `-y`, `--yes` | do not ask for confirmation |

## First run

Open the setup link (through the SSH tunnel the installer printed), create the Owner account, then the
**setup wizard** asks for the base domain, the HTTPS email and offers a sample app to check that DNS, HTTPS and
deploys work end to end. After that the panel is reachable at `https://panel.<base domain>`.

If something goes wrong: `sudo k3s kubectl -n nectar-system get pods` and
`sudo k3s kubectl -n nectar-system logs deploy/nectar-apps`. Recovery commands (reset a password, clear the IP
allowlist, …) are listed under [Security](security.md#recovery).
