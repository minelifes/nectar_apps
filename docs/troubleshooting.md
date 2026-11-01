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
