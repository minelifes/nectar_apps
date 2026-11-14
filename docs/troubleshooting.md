# Troubleshooting

## Databases stay "deploying", builds fail with "permission denied" (Ubuntu 26.04)

**Symptoms:**

- MySQL or MariaDB logs stop at "Switching to dedicated user 'mysql'", or MariaDB warns *"Signal handler
  thread did not exit in a timely manner"*.
- PostgreSQL shows no logs at all.
- Git builds fail right after a successful `RUN` with *"failed to execute command: permission denied"*.
- The kernel log shows lines like this:

```sh
sudo journalctl -k | grep 'apparmor="DENIED"' | grep 'operation="signal"'
# … profile="cri-containerd.apparmor.d" comm="gosu" … peer="cri-containerd.apparmor.d//&unconfined"
```

**Cause:** on newer Ubuntu releases (26.04 with k3s 1.36), processes in a container can carry the label
`cri-containerd.apparmor.d//&unconfined`. k3s's container AppArmor profile only lets the plain label send
signals, so signals inside a container are refused. Programs that signal their own processes then wait
forever: `gosu`, which official images use to switch from root to their user, and databases stopping their
own threads.

**Fix:** rerun the installer (it keeps your settings and data). On Ubuntu 25.10 and newer, or when the
kernel log shows these denials, it installs `/etc/apparmor.d/cri-containerd.apparmor.d`. That is k3s's own
profile plus one rule that treats the stacked label like the plain one. It is loaded right away, so running
containers recover without a restart:

```sh
curl -sfL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh | sudo sh -s -- --yes
```

`--apparmor-fix` installs the profile on any distribution, and `--no-apparmor-fix` never installs it.
Databases whose first start was interrupted may be half-initialized: delete and recreate them (new ones
are empty anyway).

## An opened port works through an SSH tunnel but not from outside

"Open ports on the server" are node ports (30000–32767). If `ssh -L 30001:localhost:30001 server` works but
`server_ip:30001` does not, a firewall drops the traffic before it reaches k3s:

- **ufw / firewalld on the server**: installers before this fix only allowed 22, 80 and 443. Allow the range
  (or a single port):

  ```sh
  sudo ufw allow 30000:32767/tcp && sudo ufw allow 30000:32767/udp
  # firewalld:
  sudo firewall-cmd --permanent --add-port=30000-32767/tcp --add-port=30000-32767/udp && sudo firewall-cmd --reload
  ```

- **ufw drops forwarded traffic**: k3s forwards a node port to the pod's address (10.42.x.x), so the packet
  passes ufw's FORWARD chain, whose default policy is DROP. `tcpdump -ni <nic> tcp port 30001` then shows the
  SYN arriving and no answer. Installers since this fix add:

  ```sh
  sudo ufw route allow to 10.42.0.0/16
  sudo ufw route allow from 10.42.0.0/16
  ```

- **the cloud provider's firewall** (security group, Hetzner/DO/AWS firewall): add an inbound rule for the
  port or range there too.

## "Unable to open config file" (or permission denied) although the file is there — Ubuntu 26.04

On Ubuntu 25.10+/26.04 with recent k3s, container processes carry a stacked AppArmor label, and a host
profile that attaches to a program's path also confines that program inside containers. Ubuntu ships
profiles for programs that are usually not installed (for example `mosquitto`), which then deny the
container's own files. The kernel log shows it:

```sh
sudo journalctl -k --no-pager | grep 'apparmor="DENIED"' | tail
# … profile="mosquitto" name="/mosquitto/config/mosquitto.conf" …
```

The installer (`--apparmor-fix`, automatic on these systems) disables profiles whose program does not exist
on the host. By hand, for one profile:

```sh
sudo ln -s /etc/apparmor.d/mosquitto /etc/apparmor.d/disable/
sudo apparmor_parser -R /etc/apparmor.d/mosquitto
```

Panels before this fix also blocked opened ports in the project's NetworkPolicy (k3s checks it with the
client's address, not the node's): redeploy the app after updating, which adds the NetworkPolicy
`<app>-ports` allowing those ports from anywhere.

## An app behind the gateway loads its scripts over http:// ("blocked: mixed-content")

Typical with Keycloak (`/resources/master/admin/en/…` blocked in the admin console), but any app that builds
absolute URLs does it: the app thinks it is served over plain HTTP. The gateway terminates HTTPS and passes
the visit on with `X-Forwarded-Proto: https`, `X-Forwarded-Host` and `X-Forwarded-Port`; the app has to be
told to trust them — or simply told its public address.

**Keycloak** (Environment tab, then redeploy):

```
KC_PROXY_HEADERS=xforwarded
KC_HOSTNAME=${keycloak.public_url}     # your app's slug; becomes https://<its default domain>
KC_HTTP_ENABLED=true
```

`${<app>.public_url}` is `https://` + the app's default domain (`${<app>.public_host}` without the scheme),
filled in at every deploy; empty while the app has no domain (redeploy after adding one). The Keycloak template sets all three. Other apps have their own switch — Grafana
`GF_SERVER_ROOT_URL`, n8n `WEBHOOK_URL`, WordPress `WP_HOME`/`WP_SITEURL`, Spring
`server.forward-headers-strategy=framework`.

**A proxy domain to another server** (the gateway → that server's nginx → the app): that nginx usually sets
`proxy_set_header X-Forwarded-Proto $scheme;`, and its `$scheme` is `http` because the gateway talks plain
HTTP to it. Either tick *The target uses HTTPS* on the proxy domain (the gateway then connects over HTTPS
with the site's name), or let nginx pass the gateway's header on:
`proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;`

