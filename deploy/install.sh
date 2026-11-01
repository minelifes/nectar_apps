#!/bin/sh
# Nectar Apps installer — https://github.com/minelifes/nectar_apps
#
#   curl -sfL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh | sudo sh -s -- --base-domain apps.example.com --email you@example.com
#
# Linux only. Fresh server: installs k3s (no Traefik, no ServiceLB) + nectar-gateway + nectar-apps panel.
# Existing k3s: "adopt" mode — discovery report, backup, shadow install on 18080/18443, verification and a
# cutover with automatic rollback. Nothing that runs is broken; `--adopt --rollback` undoes it.
#
# Copyright (c) 2026 TechBrain LLC. Licensed under the Nectar Source License (LICENSE.md).

set -eu

# ---------------------------------------------------------------- defaults --
NECTAR_VERSION="${NECTAR_VERSION:-latest}"
GATEWAY_VERSION="${GATEWAY_VERSION:-latest}"
GATEWAY_IMAGE="${GATEWAY_IMAGE:-}"       # default ghcr.io/minelifes/nectar-gateway:$GATEWAY_VERSION
K3S_VERSION="${K3S_VERSION:-}"            # empty = k3s stable channel
BASE_DOMAIN=""
ACME_EMAIL=""
ADMIN_EMAIL=""
MANIFESTS_DIR=""                          # local manifests (dev); default = download for NECTAR_VERSION
REPO_RAW="https://raw.githubusercontent.com/minelifes/nectar_apps"
NAMESPACE="nectar-system"
STATE_DIR="/var/lib/nectar"
PANEL_NODEPORT=30480
HTTP_PORT=80                              # gateway ports (adopt mode starts on the shadow ports)
HTTPS_PORT=443
ADMIN_PORT=8081                           # gateway admin API on the host network (a free one is picked)
ROUTING="gateway"                         # gateway | ingress (adopt strategy "keep")
INGRESS_CLASS=""
INGRESS_SYNC="nectar"
INGRESS_CONTROLLER=""
STRATEGY=""                               # adopt: migrate | keep | adopt-gateway (default: recommended)
ADOPT_BIN="${ADOPT_BIN:-}"                # nectar-apps binary for the host-side adopt commands
RELEASES="https://github.com/minelifes/nectar_apps/releases"
GATEWAY_RELEASES="https://github.com/minelifes/nectar_gateway/releases"
FORCE=0
RESTART_K3S=0

DRY_RUN=0
ASSUME_YES=0
SKIP_FIREWALL=0
MODE="install"                            # install | discover | adopt | rollback | finalize | status | uninstall
PURGE_K3S=0
RESTORE_KIT=""                            # disaster recovery: recovery kit JSON from the old panel
RESTORE_MASTER_KEY=""                     # … and the master key printed by the first install
K3S_UPGRADES=0                            # install Rancher's system-upgrade-controller (k3s updates from the panel)
APPARMOR_FIX="auto"                       # auto | 1 | 0: allow signals inside containers (Ubuntu 25.10+, see fix_apparmor_signals)
REGISTRY="auto"                           # built-in image registry for Git builds: auto (new installs) | 1 | 0
REGISTRY_HOST="registry.nectar.local"     # the name nodes pull built images from (a k3s mirror)
REGISTRY_NODEPORT=30500
REGISTRY_PASSWORD=""
REGISTRY_MANUAL=0                          # set when your own registries.yaml has mirrors/configs (merge by hand)
CLI=1                                     # install the nectar CLI into /usr/local/bin (--no-cli skips it)
CLI_BIN="${CLI_BIN:-}"                    # --cli-binary: a local nectar binary instead of the release download
JOIN_URL=""                               # --join: add this machine to an existing Nectar server as a k3s agent
JOIN_TOKEN=""
SUC_VERSION="${SUC_VERSION:-v0.14.2}"
LONGHORN=0                                # --longhorn: replicated volumes across nodes (storage class "longhorn")
LONGHORN_VERSION="${LONGHORN_VERSION:-v1.7.2}"
NFS=""                                    # --nfs SERVER:/export: storage class "nfs" (shared, ReadWriteMany)
NFS_PROVISIONER_IMAGE="${NFS_PROVISIONER_IMAGE:-registry.k8s.io/sig-storage/nfs-subdir-external-provisioner:v4.0.2}"

# ------------------------------------------------------------------ output --
if [ -t 1 ]; then
	C_AMBER="$(printf '\033[38;5;214m')"; C_RED="$(printf '\033[31m')"; C_GREEN="$(printf '\033[32m')"
	C_DIM="$(printf '\033[2m')"; C_BOLD="$(printf '\033[1m')"; C_RESET="$(printf '\033[0m')"
else
	C_AMBER=""; C_RED=""; C_GREEN=""; C_DIM=""; C_BOLD=""; C_RESET=""
fi
info() { printf '%s⬢%s %s\n' "$C_AMBER" "$C_RESET" "$*"; }
ok()   { printf '%s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_AMBER" "$C_RESET" "$*" >&2; }
die()  { printf '%s✗ %s%s\n' "$C_RED" "$*" "$C_RESET" >&2; exit 1; }

# run CMD... — executes, or prints in --dry-run
run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '%s+ %s%s\n' "$C_DIM" "$*" "$C_RESET"
	else
		"$@"
	fi
}

have() { command -v "$1" >/dev/null 2>&1; }

usage() {
	cat <<EOF
Nectar Apps installer

Usage: install.sh [options]

  --base-domain DOMAIN   base domain for apps, e.g. apps.example.com
  --email EMAIL          email for Let's Encrypt
  --admin-email EMAIL    email of the first Owner account
  --version VERSION      nectar-apps version (default: latest)
  --gateway-version V    nectar-gateway version (default: latest)
  --k3s-version V        k3s version, e.g. v1.34.1+k3s1 (default: stable channel)
  --manifests DIR        use local manifests instead of downloading (development)
  --skip-firewall        do not configure ufw/firewalld
  --k3s-upgrades         install Rancher's system-upgrade-controller so the panel can update k3s
  --apparmor-fix         always install the container AppArmor profile that allows signals inside containers
                         (default: only on Ubuntu 25.10+ or when the kernel log shows those denials)
  --no-apparmor-fix      never install it
                         (Settings → Updates → k3s; off until you choose a policy there)
  --registry             set up the built-in image registry for Git builds (default on new installs;
                         on an existing server this restarts k3s once to load the registry mirror)
  --no-registry          do not set up the registry
  --longhorn             install Longhorn: volumes replicated across nodes (storage class "longhorn"; needs
                         open-iscsi on every node — installed here, and by --join with --longhorn)
  --nfs SERVER:/EXPORT   storage class "nfs" on an NFS server (shared volumes, ReadWriteMany; the NFS
                         client is installed here, and by --join with --nfs)

Add a node (run on the NEW machine; the panel shows this command under Server → Add node):
  --join URL --token T   join the cluster at URL (https://<server>:6443) as a k3s agent
  --registry-password P  pull built images from the built-in registry (shown by the panel)

Disaster recovery (a fresh server, the old one is gone):
  --restore-kit FILE     recovery kit downloaded from the old panel (Backups → target → Recovery kit)
  --master-key KEY       master key printed by the old server's first install
                         The panel restores its database and files from the newest panel backup, then
                         recreates every database and app and restores their newest backups.

Existing k3s (adopt mode):
  --discover             print the read-only migration report and exit
  --adopt                discover → backup → shadow install → verify → cutover (asks before each change)
  --strategy S           migrate (nectar-gateway takes 80/443, old proxy = fallback), keep (keep your
                         ingress controller, the panel writes Ingress objects), adopt-gateway (import a
                         standalone nectar_gateway). Default: the recommended one from the report
  --status               adoption phase and hosts still served by the old proxy
  --rollback             undo the cutover: the previous proxy serves 80/443 again
  --finalize             stop depending on the old proxy (disables bundled Traefik via a k3s drop-in)
  --restart-k3s          with --finalize: restart k3s now so Traefik is removed
  --force                with --adopt/--finalize: skip the safety checks
  --binary PATH          use a local nectar-apps binary for the adopt commands (development)

  --no-cli               do not install the nectar CLI (default: /usr/local/bin/nectar, from the same release)
  --cli-binary PATH      install this nectar binary instead of downloading it (development)

  --uninstall            remove nectar-apps and nectar-gateway (keeps k3s; adopt mode: restores the old proxy)
  --purge-k3s            with --uninstall: also uninstall k3s (DESTROYS ALL WORKLOADS)
  --dry-run              print what would be done, change nothing
  -y, --yes              do not ask for confirmation
  -h, --help             show this help
EOF
}

# ------------------------------------------------------------------- args --
while [ $# -gt 0 ]; do
	case "$1" in
		--base-domain) BASE_DOMAIN="${2:?}"; shift 2 ;;
		--email) ACME_EMAIL="${2:?}"; shift 2 ;;
		--admin-email) ADMIN_EMAIL="${2:?}"; shift 2 ;;
		--version) NECTAR_VERSION="${2:?}"; shift 2 ;;
		--gateway-version) GATEWAY_VERSION="${2:?}"; shift 2 ;;
		--k3s-version) K3S_VERSION="${2:?}"; shift 2 ;;
		--manifests) MANIFESTS_DIR="${2:?}"; shift 2 ;;
		--skip-firewall) SKIP_FIREWALL=1; shift ;;
		--k3s-upgrades) K3S_UPGRADES=1; shift ;;
		--apparmor-fix) APPARMOR_FIX=1; shift ;;
		--no-apparmor-fix) APPARMOR_FIX=0; shift ;;
		--registry) REGISTRY=1; shift ;;
		--no-registry) REGISTRY=0; shift ;;
		--registry-password) REGISTRY_PASSWORD="${2:?}"; shift 2 ;;
		--longhorn) LONGHORN=1; shift ;;
		--nfs) NFS="${2:?}"; shift 2 ;;
		--join) MODE="join"; JOIN_URL="${2:?}"; shift 2 ;;
		--token) JOIN_TOKEN="${2:?}"; shift 2 ;;
		--restore-kit) RESTORE_KIT="${2:?}"; shift 2 ;;
		--master-key) RESTORE_MASTER_KEY="${2:?}"; shift 2 ;;
		--discover) MODE="discover"; shift ;;
		--adopt) MODE="adopt"; shift ;;
		--strategy) STRATEGY="${2:?}"; shift 2 ;;
		--strategy=*) STRATEGY="${1#--strategy=}"; shift ;;
		--status) MODE="status"; shift ;;
		--rollback) MODE="rollback"; shift ;;
		--finalize) MODE="finalize"; shift ;;
		--restart-k3s) RESTART_K3S=1; shift ;;
		--force) FORCE=1; shift ;;
		--binary) ADOPT_BIN="${2:?}"; shift 2 ;;
		--no-cli) CLI=0; shift ;;
		--cli-binary) CLI_BIN="${2:?}"; shift 2 ;;
		--uninstall) MODE="uninstall"; shift ;;
		--purge-k3s) PURGE_K3S=1; shift ;;
		--dry-run) DRY_RUN=1; shift ;;
		-y|--yes) ASSUME_YES=1; shift ;;
		-h|--help) usage; exit 0 ;;
		*) usage >&2; die "unknown option: $1" ;;
	esac
done

confirm() {
	[ "$ASSUME_YES" -eq 1 ] && return 0
	[ "$DRY_RUN" -eq 1 ] && return 0
	printf '%s [y/N] ' "$1"
	# read from the terminal even when piped through `curl | sh`
	if [ -r /dev/tty ]; then read -r answer </dev/tty; else answer=""; fi
	case "$answer" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

# ---------------------------------------------------------------- checks --
check_platform() {
	[ "$(uname -s)" = "Linux" ] || die "Nectar Apps supports Linux only (k3s runs natively only on Linux). Detected: $(uname -s)"
	case "$(uname -m)" in
		x86_64|amd64) ARCH="amd64" ;;
		aarch64|arm64) ARCH="arm64" ;;
		*) die "unsupported CPU architecture: $(uname -m) (supported: amd64, arm64)" ;;
	esac
	if [ "$DRY_RUN" -eq 0 ] && [ "$(id -u)" -ne 0 ]; then
		die "please run as root (e.g. curl ... | sudo sh -s -- ...)"
	fi
	[ -r /etc/os-release ] || die "/etc/os-release not found — cannot detect the distribution"
	# shellcheck disable=SC1091
	. /etc/os-release
	DISTRO_ID="${ID:-unknown}"
	DISTRO_LIKE="${ID_LIKE:-}"
	DISTRO_NAME="${PRETTY_NAME:-$DISTRO_ID}"

	case " $DISTRO_ID $DISTRO_LIKE " in
		*" debian "*|*" ubuntu "*) FAMILY="debian" ;;
		*" rhel "*|*" fedora "*|*" centos "*) FAMILY="rhel" ;;
		*" alpine "*) FAMILY="alpine" ;;
		*" arch "*) FAMILY="arch" ;;
		*" suse "*|*" opensuse "*|*" sles "*) FAMILY="suse" ;;
		*) FAMILY="other" ;;
	esac

	if have systemctl && [ -d /run/systemd/system ]; then INIT="systemd"
	elif have rc-service; then INIT="openrc"
	elif [ "$DRY_RUN" -eq 1 ]; then INIT="none"; warn "neither systemd nor OpenRC found (ignored in --dry-run)"
	else die "neither systemd nor OpenRC found — k3s needs one of them"
	fi
	info "Platform: $DISTRO_NAME ($FAMILY), $ARCH, $INIT"
}

# ------------------------------------------------------- package manager --
pkg_install() {
	# pkg_install <generic names...> — maps generic names to distro packages
	pkgs=""
	for p in "$@"; do
		case "$FAMILY:$p" in
			*:curl) pkgs="$pkgs curl" ;;
			*:ca-certificates) pkgs="$pkgs ca-certificates" ;;
			*:openssl) pkgs="$pkgs openssl" ;;
			debian:iproute) pkgs="$pkgs iproute2" ;;
			rhel:iproute|suse:iproute) pkgs="$pkgs iproute" ;;
			alpine:iproute|arch:iproute) pkgs="$pkgs iproute2" ;;
			*) pkgs="$pkgs $p" ;;
		esac
	done
	# shellcheck disable=SC2086
	case "$FAMILY" in
		debian) run env DEBIAN_FRONTEND=noninteractive apt-get update -qq
		        run env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $pkgs ;;
		rhel)   if have dnf; then run dnf install -y -q $pkgs; else run yum install -y -q $pkgs; fi ;;
		alpine) run apk add --no-cache -q $pkgs ;;
		arch)   run pacman -Sy --noconfirm --needed $pkgs ;;
		suse)   run zypper --non-interactive -q install $pkgs ;;
		other)  warn "unknown distribution: please make sure these are installed:$pkgs" ;;
	esac
}

install_prerequisites() {
	missing=""
	have curl || missing="$missing curl"
	have openssl || missing="$missing openssl"
	have ss || missing="$missing iproute"
	[ -e /etc/ssl/certs/ca-certificates.crt ] || [ -e /etc/pki/tls/certs/ca-bundle.crt ] || missing="$missing ca-certificates"
	if [ -n "$missing" ]; then
		info "Installing prerequisites:$missing"
		# shellcheck disable=SC2086
		pkg_install $missing
	fi
	if [ "$DRY_RUN" -eq 0 ]; then
		have curl || die "curl is required"
		have openssl || die "openssl is required"
	fi
	ok "Prerequisites present"
	resolve_versions
}

# --------------------------------------------------------------- helpers --
port_holder() {
	# port_holder <port> — prints the process listening on TCP <port>, empty if free
	have ss || return 0
	ss -Htlnp "sport = :$1" 2>/dev/null | sed -n 's/.*users:((\("[^"]*"\).*/\1/p' | head -1
}

k3s_present() { have k3s || [ -f /etc/rancher/k3s/k3s.yaml ]; }

kc() { k3s kubectl "$@"; }

random_secret() { openssl rand -base64 "${1:-32}" | tr -d '\n'; }

public_ip() {
	curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}'
}

ssh_port() {
	p="$(ss -Htlnp 2>/dev/null | awk '/"sshd"/ {sub(/.*:/, "", $4); print $4; exit}')"
	echo "${p:-22}"
}

# ---------------------------------------------------------------- adopt --
# The host-side work (discovery, backup, verification, cutover, rollback) is done by the nectar-apps
# binary: `nectar-apps adopt …`. It is downloaded from the GitHub release (checksum verified).
# latest_release RELEASES_URL prints the newest release version (X.Y.Z). GitHub answers
# /releases/latest/download/<file> with a redirect to /releases/download/vX.Y.Z/<file>.
latest_release() {
	loc="$(curl -fsSI --max-time 15 "$1/latest/download/release.json" 2>/dev/null | tr -d '\r' | sed -n 's/^[Ll]ocation: *//p' | head -n 1)"
	v="$(printf '%s' "$loc" | sed -n 's|.*/releases/download/v\{0,1\}\([^/]*\)/.*|\1|p')"
	[ -n "$v" ] && printf '%s\n' "$v"
}

# resolve_versions turns "latest" into the newest release and drops a leading "v" (1.0.0 and v1.0.0 both
# work). Images are tagged X.Y.Z, X.Y and stable/beta (never "latest"), and manifests and binaries come
# from the same release, so everything must use one concrete version.
resolve_versions() {
	[ -n "${VERSIONS_RESOLVED:-}" ] && return 0
	have curl || return 0 # called again after the prerequisites are installed
	NECTAR_VERSION="${NECTAR_VERSION#v}"
	GATEWAY_VERSION="${GATEWAY_VERSION#v}"
	if [ "$NECTAR_VERSION" = latest ]; then
		if v="$(latest_release "$RELEASES")"; then
			NECTAR_VERSION="$v"
		elif [ -n "$MANIFESTS_DIR" ] || [ "$DRY_RUN" -eq 1 ]; then
			warn "could not look up the latest Nectar Apps release; using \"latest\""
		else
			die "could not look up the latest Nectar Apps release at $RELEASES — pass --version X.Y.Z"
		fi
	fi
	if [ "$GATEWAY_VERSION" = latest ]; then
		if v="$(latest_release "$GATEWAY_RELEASES")"; then
			GATEWAY_VERSION="$v"
		else
			GATEWAY_VERSION="stable" # the stable channel tag always points at the newest stable gateway
			warn "could not look up the latest nectar-gateway release; using the :stable image"
		fi
	fi
	VERSIONS_RESOLVED=1
	[ "$NECTAR_VERSION" = latest ] || info "Nectar Apps $NECTAR_VERSION, nectar-gateway $GATEWAY_VERSION"
}

# download_verified ASSET DEST: a release asset for NECTAR_VERSION, checked against its .sha256.
download_verified() {
	if [ "$NECTAR_VERSION" = latest ]; then base="$RELEASES/latest/download"; else base="$RELEASES/download/v$NECTAR_VERSION"; fi
	if [ "$DRY_RUN" -eq 1 ]; then run curl -fsSL "$base/$1" -o "$2"; return 0; fi
	tmp="$(mktemp)"
	if ! curl -fsSL "$base/$1" -o "$tmp"; then rm -f "$tmp"; warn "download failed: $base/$1"; return 1; fi
	if ! want="$(curl -fsSL "$base/$1.sha256" | awk '{print $1}')"; then rm -f "$tmp"; warn "download failed: $base/$1.sha256"; return 1; fi
	got="$(sha256sum "$tmp" 2>/dev/null | awk '{print $1}' || openssl dgst -sha256 -r "$tmp" | awk '{print $1}')"
	if [ -z "$want" ] || [ "$want" != "$got" ]; then
		rm -f "$tmp"
		warn "checksum mismatch for $1"
		return 1
	fi
	mkdir -p "$(dirname "$2")"
	install -m 0755 "$tmp" "$2"
	rm -f "$tmp"
}

fetch_adopt_bin() {
	if [ -n "$ADOPT_BIN" ]; then
		[ -x "$ADOPT_BIN" ] || die "--binary $ADOPT_BIN is not executable"
		return
	fi
	ADOPT_BIN="$STATE_DIR/bin/nectar-apps"
	info "Downloading nectar-apps_linux_$ARCH ($NECTAR_VERSION)"
	download_verified "nectar-apps_linux_$ARCH" "$ADOPT_BIN" || die "could not download the nectar-apps binary"
}

# install_cli puts the nectar CLI into /usr/local/bin and saves this panel's local address for root (and
# the sudo user), so `nectar login` on the server only asks for an API token. A missing release asset
# is not fatal: the panel works without the CLI.
CLI_DEST=/usr/local/bin/nectar
install_cli() {
	[ "$CLI" -eq 1 ] || return 0
	if [ -n "$CLI_BIN" ]; then
		[ -x "$CLI_BIN" ] || die "--cli-binary $CLI_BIN is not executable"
		run install -m 0755 "$CLI_BIN" "$CLI_DEST"
	else
		info "Installing the nectar CLI ($NECTAR_VERSION) to $CLI_DEST"
		download_verified "nectar_linux_$ARCH" "$CLI_DEST" || { warn "the nectar CLI was not installed (install it later from $RELEASES)"; return 0; }
	fi
	run mkdir -p "$STATE_DIR"
	run touch "$STATE_DIR/cli-installed" # uninstall removes only a CLI it installed
	cli_config /root
	if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != root ]; then
		h="$(getent passwd "$SUDO_USER" 2>/dev/null | cut -d: -f6)"
		[ -n "$h" ] && [ -d "$h" ] && cli_config "$h" "$SUDO_USER"
	fi
	CLI_DONE=1
}

# cli_config HOME [USER]: write ~/.config/nectar/config.json with the panel address unless one exists.
cli_config() {
	f="$1/.config/nectar/config.json"
	[ -f "$f" ] && return 0
	if [ "$DRY_RUN" -eq 1 ]; then run sh -c "write $f (url http://127.0.0.1:$PANEL_NODEPORT)"; return 0; fi
	mkdir -p "$1/.config/nectar"
	printf '{"url":"http://127.0.0.1:%s","token":""}\n' "$PANEL_NODEPORT" >"$f"
	chmod 600 "$f"
	if [ -n "${2:-}" ]; then chown -R "$2" "$1/.config/nectar" 2>/dev/null || true; fi
}

adopt_cli() { "$ADOPT_BIN" adopt "$@" --namespace "$NAMESPACE" --state-dir "$STATE_DIR"; }

discover() {
	info "Existing k3s detected — read-only discovery (nothing is changed)"
	fetch_adopt_bin
	[ "$DRY_RUN" -eq 1 ] && { run "$ADOPT_BIN" adopt discover; return; }
	adopt_cli discover
}

do_adopt() {
	k3s_present || die "k3s is not installed: run the installer without --adopt for a fresh install"
	install_prerequisites
	fetch_adopt_bin
	if [ "$DRY_RUN" -eq 1 ]; then
		for step in discover backup shadow verify cutover; do run "$ADOPT_BIN" adopt "$step"; done
		return
	fi
	adopt_cli discover
	if [ -n "$STRATEGY" ]; then plan="$(adopt_cli plan --strategy "$STRATEGY")" || exit 1; else plan="$(adopt_cli plan)" || exit 1; fi
	eval "$plan"
	[ -n "$STRATEGY" ] || die "no adoption strategy is possible on this server yet — see the report above"
	if [ "$POSSIBLE" != 1 ] && [ "$FORCE" -ne 1 ]; then
		die "strategy $STRATEGY is not possible: $REASON"
	fi
	echo
	info "Strategy: $STRATEGY — $REASON"
	case "$STRATEGY" in
		keep) info "The panel will write Ingress objects (class ${INGRESS_CLASS:-default}) for your existing controller. Ports 80/443 are not touched." ;;
		*) info "Plan: back up → install Nectar on shadow ports 18080/18443 → compare every host → move 80/443 (auto-rollback)."
		   info "$MIGRATED_HOSTS host(s) move to nectar-gateway; $FALLBACK_ROUTES route(s) stay on the old proxy (fallback)." ;;
	esac
	confirm "Back up and install Nectar in shadow mode now?" || die "aborted — nothing was changed"

	adopt_cli backup
	if [ "$STRATEGY" = keep ]; then
		ROUTING="ingress"; INGRESS_SYNC="off"
	else
		ROUTING="gateway"; HTTP_PORT=18080; HTTPS_PORT=18443
	fi
	keep_existing_settings
	[ "$ROUTING" = gateway ] && pick_admin_port
	warn "Firewall rules are not changed on an existing server. Keep the gateway admin port $ADMIN_PORT closed to the internet."
	# an existing cluster gets the built-in registry (and the k3s restart its mirror needs) only with --registry
	[ "$REGISTRY" = auto ] && REGISTRY=0
	setup_registry
	deploy_stack
	install_cli
	if [ "$STRATEGY" = keep ]; then
		print_summary
		return
	fi

	adopt_cli shadow --strategy "$STRATEGY"
	echo
	info "Comparing the old proxy with nectar-gateway for every host"
	if ! adopt_cli verify; then
		warn "Some hosts answer differently through nectar-gateway (see above and $STATE_DIR/adopt/nectar-migration-report.md)."
		[ "$FORCE" -eq 1 ] || { warn "Fix them and re-run with --adopt, or use --force. Nothing on 80/443 was changed."; print_summary; return; }
	fi
	confirm "Move ports 80/443 to nectar-gateway now? (a few seconds of switching, automatic rollback on problems)" || {
		info "Not switched. Nectar runs in shadow mode; re-run with --adopt when you are ready."; print_summary; return; }
	if [ "$FORCE" -eq 1 ]; then adopt_cli cutover --force; else adopt_cli cutover; fi
	echo
	if [ "${EDGE:-}" = none ]; then
		# ports 80/443 were free: there is no old proxy to fall back to or to finalize away
		ok "nectar-gateway serves 80/443."
		info "Undo:  sh install.sh --rollback"
	else
		ok "nectar-gateway serves 80/443. The previous proxy keeps running behind it as the fallback."
		info "Check later which hosts still use it:  sh install.sh --status     undo:  sh install.sh --rollback"
		info "When nothing uses it any more:         sh install.sh --finalize"
	fi
	setup_storage
	print_summary
}

# ---------------------------------------------------------------- install --
configure_firewall() {
	[ "$SKIP_FIREWALL" -eq 1 ] && { warn "Skipping firewall configuration (--skip-firewall)"; return; }
	sp="$(ssh_port)"
	if have ufw; then
		info "Configuring ufw (allow SSH $sp, 80, 443; everything else incoming denied)"
		run ufw allow "$sp/tcp"
		run ufw allow 80/tcp
		run ufw allow 443/tcp
		run ufw allow 443/udp   # HTTP/3
		# pod and service networks must reach the host (k3s defaults)
		run ufw allow from 10.42.0.0/16
		run ufw allow from 10.43.0.0/16
		run ufw default deny incoming
		run ufw --force enable
	elif have firewall-cmd && firewall-cmd --state >/dev/null 2>&1; then
		info "Configuring firewalld (allow SSH, 80, 443; trust k3s pod/service networks)"
		run firewall-cmd --permanent --add-service=ssh
		run firewall-cmd --permanent --add-port="$sp/tcp"
		run firewall-cmd --permanent --add-service=http
		run firewall-cmd --permanent --add-service=https
		run firewall-cmd --permanent --add-port=443/udp
		run firewall-cmd --permanent --zone=trusted --add-source=10.42.0.0/16
		run firewall-cmd --permanent --zone=trusted --add-source=10.43.0.0/16
		run firewall-cmd --reload
	else
		warn "No ufw/firewalld found. Make sure only SSH ($sp), 80 and 443 are reachable from the internet;"
		warn "the Kubernetes API (6443) and kubelet (10250) must NOT be exposed."
	fi
}

# fix_apparmor_signals — on Ubuntu 26.04 with k3s 1.36, processes in a container can carry the stacked
# AppArmor label "cri-containerd.apparmor.d//&unconfined", and k3s's container profile only lets the plain
# label signal. Every signal inside a container is then denied: gosu (root → user switch in official
# database images) spins forever, databases hang when stopping their own threads, Kaniko cannot clean up a
# RUN step. The profile below is containerd's own, plus one rule that treats the stacked label like the
# plain one. Installed in /etc/apparmor.d (loaded at boot before k3s; containerd keeps a loaded profile)
# and loaded now: running containers get it at once.
APPARMOR_PROFILE=/etc/apparmor.d/cri-containerd.apparmor.d
fix_apparmor_signals() {
	[ "$APPARMOR_FIX" = 0 ] && return 0
	grep -qs Y /sys/module/apparmor/parameters/enabled || return 0
	if ! have apparmor_parser; then
		[ "$APPARMOR_FIX" = 1 ] && warn "apparmor_parser not found: cannot install the container AppArmor profile"
		return 0
	fi
	if [ "$APPARMOR_FIX" = auto ]; then
		need=0
		if [ "${DISTRO_ID:-}" = ubuntu ] && [ "$(printf '%s\n%s\n' "${VERSION_ID:-0}" 25.10 | sort -V | head -1)" = 25.10 ]; then
			need=1
		fi
		if have journalctl && journalctl -k -q --since "-7 days" 2>/dev/null | grep -q 'operation="signal".*peer="cri-containerd.apparmor.d//&'; then
			need=1
		fi
		[ "$need" -eq 1 ] || return 0
	fi
	info "Installing the container AppArmor profile that allows signals inside containers ($APPARMOR_PROFILE)"
	if [ "$DRY_RUN" -eq 1 ]; then
		run apparmor_parser -r "$APPARMOR_PROFILE"
		return 0
	fi
	cat >"$APPARMOR_PROFILE" <<'APPARMOR'
#include <tunables/global>

profile cri-containerd.apparmor.d flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  network,
  capability,
  file,
  umount,
  # Host (privileged) processes may send signals to container processes.
  signal (receive) peer=unconfined,
  # Manager may send signals to container processes.
  signal (receive) peer=cri-containerd.apparmor.d,
  # Container processes may send signals amongst themselves.
  signal (send,receive) peer=cri-containerd.apparmor.d,
  # Nectar: on newer Ubuntu (26.04) container processes can carry the stacked label
  # "cri-containerd.apparmor.d//&unconfined"; signals to and from them were denied (gosu, database
  # shutdown, Kaniko). Allow them like the plain label above.
  signal (send,receive) peer="cri-containerd.apparmor.d//&unconfined",

  deny @{PROC}/* w,   # deny write for all files directly in /proc (not in a subdir)
  # deny write to files not in /proc/<number>/** or /proc/sys/**
  deny @{PROC}/{[^1-9],[^1-9][^0-9],[^1-9s][^0-9y][^0-9s],[^1-9][^0-9][^0-9][^0-9/]*}/** w,
  deny @{PROC}/sys/[^k]** w,  # deny /proc/sys except /proc/sys/k* (effectively /proc/sys/kernel)
  deny @{PROC}/sys/kernel/{?,??,[^s][^h][^m]**} w,  # deny everything except shm* in /proc/sys/kernel/
  deny @{PROC}/sysrq-trigger rwklx,
  deny @{PROC}/kcore rwklx,

  deny mount,

  deny /sys/[^f]*/** wklx,
  deny /sys/f[^s]*/** wklx,
  deny /sys/fs/[^c]*/** wklx,
  deny /sys/fs/c[^g]*/** wklx,
  deny /sys/fs/cg[^r]*/** wklx,
  deny /sys/firmware/** rwklx,
  deny /sys/kernel/security/** rwklx,

  # suppress ptrace denials when using 'docker ps' or using 'ps' inside a container
  ptrace (trace,read,tracedby,readby) peer=cri-containerd.apparmor.d,
}
APPARMOR
	if apparmor_parser -r "$APPARMOR_PROFILE"; then
		ok "Container AppArmor profile loaded (signals inside containers allowed)"
	else
		warn "could not load $APPARMOR_PROFILE — databases may hang on start (see docs/troubleshooting)"
	fi
}

install_k3s() {
	if k3s_present; then
		ok "k3s already installed: $(k3s --version | head -1)"
		return
	fi
	info "Installing k3s ${K3S_VERSION:-(stable channel)} without Traefik and ServiceLB"
	exec_flags="server --disable traefik --disable servicelb --write-kubeconfig-mode 600"
	if [ "$DRY_RUN" -eq 1 ]; then
		run sh -c "curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION='$K3S_VERSION' INSTALL_K3S_EXEC='$exec_flags' sh -"
		return
	fi
	curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="$K3S_VERSION" INSTALL_K3S_EXEC="$exec_flags" sh -
	info "Waiting for the node to become Ready"
	i=0
	until kc get nodes --no-headers 2>/dev/null | grep -q ' Ready'; do
		i=$((i + 1)); [ "$i" -gt 90 ] && die "k3s node did not become Ready within 3 minutes (journalctl -u k3s)"
		sleep 2
	done
	ok "k3s is running"
}

fetch_manifests() {
	WORK_DIR="$(mktemp -d)"
	trap 'rm -rf "$WORK_DIR"' EXIT
	if [ -n "$MANIFESTS_DIR" ]; then
		cp "$MANIFESTS_DIR"/*.yaml "$WORK_DIR"/
	else
		ref="main"; [ "$NECTAR_VERSION" != "latest" ] && ref="v$NECTAR_VERSION"
		for f in 00-namespace.yaml 10-gateway.yaml 20-panel.yaml; do
			run curl -fsSL "$REPO_RAW/$ref/deploy/manifests/$f" -o "$WORK_DIR/$f"
		done
	fi
}

# render FILE — replaces @@PLACEHOLDERS@@ in a manifest
render() {
	gw_image="${GATEWAY_IMAGE:-ghcr.io/minelifes/nectar-gateway:$GATEWAY_VERSION}"
	sed -e "s|@@NAMESPACE@@|$NAMESPACE|g" \
	    -e "s|@@GATEWAY_IMAGE@@|$gw_image|g" \
	    -e "s|@@NECTAR_VERSION@@|$NECTAR_VERSION|g" \
	    -e "s|@@GATEWAY_VERSION@@|$GATEWAY_VERSION|g" \
	    -e "s|@@BASE_DOMAIN@@|$BASE_DOMAIN|g" \
	    -e "s|@@ACME_EMAIL@@|$ACME_EMAIL|g" -e "s|@@ACME@@|true|g" \
	    -e "s|@@PANEL_NODEPORT@@|$PANEL_NODEPORT|g" \
	    -e "s|@@HTTP_PORT@@|$HTTP_PORT|g" -e "s|@@HTTPS_PORT@@|$HTTPS_PORT|g" -e "s|@@ADMIN_PORT@@|$ADMIN_PORT|g" \
	    -e "s|@@ROUTING@@|$ROUTING|g" -e "s|@@INGRESS_CLASS@@|$INGRESS_CLASS|g" \
	    -e "s|@@INGRESS_SYNC@@|$INGRESS_SYNC|g" -e "s|@@INGRESS_CONTROLLER@@|$INGRESS_CONTROLLER|g" \
	    -e "s|@@REGISTRY_HOST@@|$REGISTRY_HOST|g" -e "s|@@REGISTRY_NODEPORT@@|$REGISTRY_NODEPORT|g" "$1"
}

ensure_secrets() {
	# Secrets are created once and never overwritten on re-runs.
	if [ "$DRY_RUN" -eq 1 ]; then
		run kc -n "$NAMESPACE" create secret generic nectar-secrets "--from-literal=master-key=<random>" "--from-literal=gateway-api-token=<random>" "--from-literal=setup-token=<random>"
		MASTER_KEY="<random>"; SETUP_TOKEN="<random>"; NEW_SECRETS=1
		return
	fi
	if kc -n "$NAMESPACE" get secret nectar-secrets >/dev/null 2>&1; then
		NEW_SECRETS=0
		SETUP_TOKEN="$(kc -n "$NAMESPACE" get secret nectar-secrets -o jsonpath='{.data.setup-token}' | base64 -d)"
		ok "Existing secrets kept"
		return
	fi
	MASTER_KEY="$(random_secret 32)"
	if [ -n "$RESTORE_MASTER_KEY" ]; then
		MASTER_KEY="$RESTORE_MASTER_KEY" # the restored database is encrypted with the old key
	fi
	SETUP_TOKEN="$(openssl rand -hex 16)"
	kc -n "$NAMESPACE" create secret generic nectar-secrets \
		--from-literal=master-key="$MASTER_KEY" \
		--from-literal=gateway-api-token="$(random_secret 32)" \
		--from-literal=setup-token="$SETUP_TOKEN" \
		--from-literal=admin-email="$ADMIN_EMAIL" >/dev/null
	NEW_SECRETS=1
	ok "Generated master key, gateway API token and setup token"
}

# keep_existing_settings — on re-runs, keep the ports and routing of the installed stack (a re-run during an
# adoption must not move the shadow gateway to 80/443, and must not turn a "keep" install into a gateway one)
keep_existing_settings() {
	[ "$DRY_RUN" -eq 1 ] && return 0
	kc -n "$NAMESPACE" get deploy nectar-apps >/dev/null 2>&1 || return 0
	env_of() { kc -n "$NAMESPACE" get deploy "$1" -o jsonpath="{.spec.template.spec.containers[0].env[?(@.name==\"$2\")].value}" 2>/dev/null; }
	v="$(env_of nectar-gateway PROXY_ADDR)"; [ -n "$v" ] && HTTP_PORT="${v##*:}"
	v="$(env_of nectar-gateway HTTPS_ADDR)"; [ -n "$v" ] && HTTPS_PORT="${v##*:}"
	v="$(env_of nectar-gateway ADMIN_ADDR)"; [ -n "$v" ] && ADMIN_PORT="${v##*:}"
	v="$(env_of nectar-apps NECTAR_ROUTING)"; [ -n "$v" ] && ROUTING="$v"
	v="$(env_of nectar-apps NECTAR_INGRESS_CLASS)"; [ -n "$v" ] && INGRESS_CLASS="$v"
	v="$(env_of nectar-apps NECTAR_INGRESS_SYNC)"; [ -n "$v" ] && INGRESS_SYNC="$v"
	v="$(env_of nectar-apps NECTAR_INGRESS_CONTROLLER)"; [ -n "$v" ] && INGRESS_CONTROLLER="$v"
	return 0
}

# pick_admin_port — the gateway admin API binds on the host network: use 8081 or the next free port
pick_admin_port() {
	kc -n "$NAMESPACE" get deploy nectar-gateway >/dev/null 2>&1 && return 0   # installed: keep it
	for p in 8081 18081 18082 18083 18084 18085; do
		if [ -z "$(port_holder "$p")" ]; then ADMIN_PORT="$p"; return 0; fi
	done
	die "no free port for the gateway admin API (tried 8081, 18081–18085)"
}

# label_edge_node — nectar-gateway runs on the node labelled nectar.dev/edge=true (this server)
label_edge_node() {
	if [ "$DRY_RUN" -eq 1 ]; then run kc label node "<this node>" nectar.dev/edge=true --overwrite; return; fi
	[ -n "$(kc get nodes -l nectar.dev/edge=true -o name 2>/dev/null)" ] && return 0
	node="$(kc get nodes -o name 2>/dev/null | sed 's|node/||' | grep -ix "$(hostname)" | head -1)"
	[ -z "$node" ] && [ "$(kc get nodes -o name 2>/dev/null | wc -l)" -eq 1 ] && node="$(kc get nodes -o name | sed 's|node/||')"
	[ -n "$node" ] || die "cannot tell which node this is: label it with  k3s kubectl label node <name> nectar.dev/edge=true"
	kc label node "$node" nectar.dev/edge=true --overwrite >/dev/null
	ok "Node $node labelled nectar.dev/edge=true (nectar-gateway runs here)"
}

deploy_stack() {
	fetch_manifests
	info "Deploying nectar-system (gateway + panel)"
	if [ "$DRY_RUN" -eq 1 ]; then
		for f in "$WORK_DIR"/*.yaml; do run kc apply -f "$f"; done
		ensure_secrets
		store_cluster_secrets
		install_upgrade_controller
		return
	fi
	render "$WORK_DIR/00-namespace.yaml" | kc apply -f - >/dev/null
	ensure_secrets
	store_cluster_secrets
	if [ -n "$RESTORE_KIT" ]; then
		# the panel finds this Secret on its first start, restores its data from the backup and deletes it
		kc -n "$NAMESPACE" create secret generic nectar-restore --from-file=kit="$RESTORE_KIT" \
			--dry-run=client -o yaml | kc apply -f - >/dev/null
		ok "Recovery kit handed to the panel"
	fi
	if [ "$ROUTING" = gateway ]; then
		label_edge_node
		render "$WORK_DIR/10-gateway.yaml" | kc apply -f - >/dev/null
	fi
	render "$WORK_DIR/20-panel.yaml" | kc apply -f - >/dev/null
	[ "${REGISTRY_NEW:-0}" -eq 1 ] && kc -n "$NAMESPACE" rollout restart deploy/nectar-apps >/dev/null 2>&1
	info "Waiting for the rollout (pulling images may take a minute)"
	if [ "$ROUTING" = gateway ]; then
		kc -n "$NAMESPACE" rollout status deploy/nectar-gateway --timeout=300s || warn "gateway not ready yet: kubectl -n $NAMESPACE describe pod -l app.kubernetes.io/name=nectar-gateway"
	fi
	kc -n "$NAMESPACE" rollout status deploy/nectar-apps --timeout=300s || warn "panel not ready yet: kubectl -n $NAMESPACE logs deploy/nectar-apps"
	install_upgrade_controller
	mkdir -p "$STATE_DIR"
	echo "$NECTAR_VERSION" >"$STATE_DIR/installed-version"
}

# ---------------------------------------------------------------- storage --
# Volumes use k3s's local-path storage by default (a folder on the node, no size limit, tied to that node).
# --longhorn and --nfs add storage classes that apps can pick per volume (app → Settings → Volumes).

storage_packages() {
	pk=""
	if [ "$LONGHORN" -eq 1 ]; then
		case "$FAMILY" in
			rhel) pk="$pk iscsi-initiator-utils" ;;
			*) pk="$pk open-iscsi" ;;
		esac
	fi
	if [ "$LONGHORN" -eq 1 ] || [ -n "$NFS" ]; then
		case "$FAMILY" in
			debian) pk="$pk nfs-common" ;;
			*) pk="$pk nfs-utils" ;;
		esac
	fi
	[ -n "$pk" ] || return 0
	info "Installing storage clients:$pk"
	# shellcheck disable=SC2086
	pkg_install $pk
	if [ "$LONGHORN" -eq 1 ]; then
		case "$INIT" in
			openrc) run rc-update add iscsid default; run rc-service iscsid start ;;
			*) run systemctl enable --now iscsid ;;
		esac
	fi
}

setup_storage() {
	[ "$LONGHORN" -eq 1 ] || [ -n "$NFS" ] || return 0
	storage_packages
	if [ "$LONGHORN" -eq 1 ]; then
		info "Installing Longhorn $LONGHORN_VERSION (replicated volumes)"
		run kc apply -f "https://raw.githubusercontent.com/longhorn/longhorn/$LONGHORN_VERSION/deploy/longhorn.yaml"
		ok "Storage class \"longhorn\" — pick it per volume (app → Settings → Volumes); Longhorn's own UI: kubectl -n longhorn-system port-forward svc/longhorn-frontend 8000:80"
	fi
	if [ -n "$NFS" ]; then
		nfs_server="${NFS%%:*}"
		nfs_path="${NFS#*:}"
		case "$NFS" in *:/*) ;; *) die "--nfs needs SERVER:/export, e.g. 10.0.0.5:/srv/nectar" ;; esac
		info "Setting up the NFS storage class ($nfs_server:$nfs_path)"
		if [ "$DRY_RUN" -eq 1 ]; then
			run kc apply -f "<nfs provisioner manifests>"
		else
			nfs_manifest "$nfs_server" "$nfs_path" | kc apply -f - >/dev/null
		fi
		ok "Storage class \"nfs\" (ReadWriteMany) — pick it per volume; data of deleted volumes is kept as archived-* folders on the server"
	fi
}

# nfs_manifest SERVER PATH: nfs-subdir-external-provisioner (one folder per volume on the export)
nfs_manifest() {
	cat <<EOF2
apiVersion: v1
kind: Namespace
metadata: { name: nectar-storage, labels: { pod-security.kubernetes.io/enforce: privileged } }
---
apiVersion: v1
kind: ServiceAccount
metadata: { name: nfs-provisioner, namespace: nectar-storage }
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: { name: nectar-nfs-provisioner }
rules:
  - { apiGroups: [""], resources: [persistentvolumes], verbs: [get, list, watch, create, delete] }
  - { apiGroups: [""], resources: [persistentvolumeclaims], verbs: [get, list, watch, update] }
  - { apiGroups: [storage.k8s.io], resources: [storageclasses], verbs: [get, list, watch] }
  - { apiGroups: [""], resources: [events], verbs: [create, update, patch] }
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: { name: nectar-nfs-provisioner }
subjects: [{ kind: ServiceAccount, name: nfs-provisioner, namespace: nectar-storage }]
roleRef: { kind: ClusterRole, name: nectar-nfs-provisioner, apiGroup: rbac.authorization.k8s.io }
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: { name: nfs-provisioner-leader, namespace: nectar-storage }
rules:
  - { apiGroups: [""], resources: [endpoints], verbs: [get, list, watch, create, update, patch] }
  - { apiGroups: [coordination.k8s.io], resources: [leases], verbs: [get, list, watch, create, update, patch] }
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: { name: nfs-provisioner-leader, namespace: nectar-storage }
subjects: [{ kind: ServiceAccount, name: nfs-provisioner, namespace: nectar-storage }]
roleRef: { kind: Role, name: nfs-provisioner-leader, apiGroup: rbac.authorization.k8s.io }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: nfs-provisioner, namespace: nectar-storage, labels: { app.kubernetes.io/name: nfs-provisioner } }
spec:
  replicas: 1
  strategy: { type: Recreate }
  selector: { matchLabels: { app.kubernetes.io/name: nfs-provisioner } }
  template:
    metadata: { labels: { app.kubernetes.io/name: nfs-provisioner } }
    spec:
      serviceAccountName: nfs-provisioner
      containers:
        - name: provisioner
          image: $NFS_PROVISIONER_IMAGE
          env:
            - { name: PROVISIONER_NAME, value: nectar.dev/nfs }
            - { name: NFS_SERVER, value: "$1" }
            - { name: NFS_PATH, value: "$2" }
          resources: { requests: { cpu: 10m, memory: 32Mi }, limits: { memory: 128Mi } }
          volumeMounts: [{ name: root, mountPath: /persistentvolumes }]
      volumes:
        - name: root
          nfs: { server: "$1", path: "$2" }
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: { name: nfs, labels: { app.kubernetes.io/managed-by: nectar-apps } }
provisioner: nectar.dev/nfs
reclaimPolicy: Delete
allowVolumeExpansion: true
parameters:
  archiveOnDelete: "true"
  pathPattern: "\${.PVC.namespace}-\${.PVC.name}"
EOF2
}

# k3s updates from the panel: the upgrade controller only acts on Plans, which the panel creates
# when Settings → Updates → k3s is set to "auto" or "update now" is pressed.
install_upgrade_controller() {
	[ "$K3S_UPGRADES" -eq 1 ] || return 0
	base="https://github.com/rancher/system-upgrade-controller/releases/download/$SUC_VERSION"
	info "Installing system-upgrade-controller $SUC_VERSION (k3s updates)"
	run kc apply -f "$base/crd.yaml"
	run kc apply -f "$base/system-upgrade-controller.yaml"
	ok "k3s updates can be configured in Settings → Updates"
}

# ------------------------------------------------------------- registry --
# Built images are pulled as registry.nectar.local/<project>/<app>:build-N. k3s's containerd maps that name to
# the registry's NodePort on 127.0.0.1 (every node), with the registry login; nothing else can reach it.
# restart_k3s restarts the k3s server with the host's init system (running containers keep running).
restart_k3s() {
	case "$INIT" in
		openrc) rc-service k3s restart || warn "restart k3s yourself: rc-service k3s restart" ;;
		*) systemctl restart k3s || warn "restart k3s yourself: systemctl restart k3s" ;;
	esac
}

# The installer owns only the marked block of /etc/rancher/k3s/registries.yaml; entries of your own stay.
# (Files written by 0.13 and earlier carry a single "# nectar-registry:" header and are entirely ours.)
registries_block() {
	cat <<EOF
# nectar-registry begin: managed by the Nectar Apps installer (built-in image registry), edits here are replaced
mirrors:
  "$REGISTRY_HOST":
    endpoint:
      - "http://127.0.0.1:$REGISTRY_NODEPORT"
configs:
  "127.0.0.1:$REGISTRY_NODEPORT":
    auth:
      username: nectar
      password: "$REGISTRY_PASSWORD"
# nectar-registry end
EOF
}

# registries_rest prints registries.yaml without our block.
registries_rest() {
	f=/etc/rancher/k3s/registries.yaml
	[ -f "$f" ] || return 0
	grep -q '^# nectar-registry: ' "$f" && return 0
	sed '/^# nectar-registry begin/,/^# nectar-registry end/d' "$f"
}

write_registries_yaml() {
	f=/etc/rancher/k3s/registries.yaml
	if [ "$DRY_RUN" -eq 1 ]; then
		run sh -c "write $f (mirror $REGISTRY_HOST → http://127.0.0.1:$REGISTRY_NODEPORT, user nectar)"
		return
	fi
	mkdir -p /etc/rancher/k3s
	rest="$(registries_rest)"
	if printf '%s\n' "$rest" | grep -qE '^(mirrors|configs):'; then
		# mirrors/configs of your own: YAML allows each key once, so the two cannot simply be joined
		warn "$f has mirrors/configs of your own: it is not changed. Merge these entries into it and restart k3s,"
		warn "otherwise the nodes cannot pull images built by Nectar (registry $REGISTRY_HOST):"
		registries_block | sed '/^# nectar-registry/d; s/^/    /' >&2
		REGISTRY_MANUAL=1
		return
	fi
	umask 077
	{
		if [ -n "$(printf '%s' "$rest" | tr -d '[:space:]')" ]; then printf '%s\n\n' "$rest"; fi
		registries_block
	} >"$f.new"
	mv "$f.new" "$f"
	chmod 600 "$f"
	umask 022
}

# remove_registries_yaml drops our block (and the file when nothing else is left).
remove_registries_yaml() {
	f=/etc/rancher/k3s/registries.yaml
	grep -qs '^# nectar-registry' "$f" || return 0
	rest="$(registries_rest)"
	if [ "$DRY_RUN" -eq 1 ]; then
		run sh -c "remove the Nectar block from $f"
	elif [ -n "$(printf '%s' "$rest" | tr -d '[:space:]')" ]; then
		printf '%s\n' "$rest" >"$f"
	else
		rm -f "$f"
	fi
	info "Removed the registry mirror from $f (k3s drops it at its next restart)"
}

# setup_registry decides about the registry before k3s starts (new installs need no k3s restart)
setup_registry() {
	if [ "$REGISTRY" = auto ]; then
		if k3s_present && kc -n "$NAMESPACE" get secret nectar-secrets >/dev/null 2>&1; then
			REGISTRY=0
			existing="$(kc -n "$NAMESPACE" get secret nectar-secrets -o jsonpath='{.data.registry-password}' 2>/dev/null | base64 -d 2>/dev/null || true)"
			if [ -n "$existing" ]; then REGISTRY=1; REGISTRY_PASSWORD="$existing"; fi
		else
			REGISTRY=1
		fi
	fi
	[ "$REGISTRY" -eq 1 ] || return 0
	if [ -z "$REGISTRY_PASSWORD" ] && k3s_present && kc -n "$NAMESPACE" get secret nectar-secrets >/dev/null 2>&1; then
		REGISTRY_PASSWORD="$(kc -n "$NAMESPACE" get secret nectar-secrets -o jsonpath='{.data.registry-password}' 2>/dev/null | base64 -d 2>/dev/null || true)"
	fi
	if [ -z "$REGISTRY_PASSWORD" ]; then
		REGISTRY_PASSWORD="$(openssl rand -hex 24)"
		REGISTRY_NEW=1 # a running panel must restart to see it
	fi
	had=0; [ -f /etc/rancher/k3s/registries.yaml ] && grep -q "$REGISTRY_PASSWORD" /etc/rancher/k3s/registries.yaml 2>/dev/null && had=1
	write_registries_yaml
	if [ "$had" -eq 0 ] && [ "$REGISTRY_MANUAL" -eq 0 ] && k3s_present && [ "$DRY_RUN" -eq 0 ]; then
		info "Restarting k3s once to load the registry mirror (running containers keep running)"
		restart_k3s
		i=0
		until kc get nodes >/dev/null 2>&1; do i=$((i + 1)); [ "$i" -gt 60 ] && break; sleep 2; done
	fi
}

# store_registry_password puts the registry password and the join details where the panel reads them
store_cluster_secrets() {
	[ "$DRY_RUN" -eq 1 ] && { run kc -n "$NAMESPACE" create secret generic nectar-join "--from-literal=url=https://<ip>:6443" "--from-file=token=<node-token>"; return; }
	if [ "${REGISTRY:-0}" = 1 ] && [ -n "$REGISTRY_PASSWORD" ]; then
		kc -n "$NAMESPACE" patch secret nectar-secrets --type merge \
			-p "{\"stringData\":{\"registry-password\":\"$REGISTRY_PASSWORD\"}}" >/dev/null
	fi
	if [ -r /var/lib/rancher/k3s/server/node-token ]; then
		ip="$(kc get node "$(hostname)" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null)"
		[ -n "$ip" ] || ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
		kc -n "$NAMESPACE" create secret generic nectar-join --from-literal=url="https://$ip:6443" \
			--from-file=token=/var/lib/rancher/k3s/server/node-token --dry-run=client -o yaml | kc apply -f - >/dev/null
	fi
}

# ----------------------------------------------------------------- join --
do_join() {
	if [ -z "$JOIN_URL" ] || [ -z "$JOIN_TOKEN" ]; then
		die "--join needs the server URL and --token (shown under Server → Add node)"
	fi
	k3s_present && die "k3s is already installed on this machine"
	server_host="$(printf '%s' "$JOIN_URL" | sed -E 's#^https?://##; s#[:/].*$##')"
	info "This machine joins the cluster at $JOIN_URL as a k3s agent (worker node)."
	confirm "Continue?" || die "aborted"
	install_prerequisites
	if [ "$SKIP_FIREWALL" -eq 0 ]; then
		if have ufw; then
			run ufw allow "$(ssh_port)/tcp"
			run ufw allow from "$server_host"
			run ufw allow from 10.42.0.0/16
			run ufw allow from 10.43.0.0/16
			run ufw default deny incoming
			run ufw --force enable
		elif have firewall-cmd && firewall-cmd --state >/dev/null 2>&1; then
			run firewall-cmd --permanent --zone=trusted --add-source="$server_host"
			run firewall-cmd --permanent --zone=trusted --add-source=10.42.0.0/16
			run firewall-cmd --permanent --zone=trusted --add-source=10.43.0.0/16
			run firewall-cmd --reload
		fi
	fi
	if [ -n "$REGISTRY_PASSWORD" ]; then
		write_registries_yaml
	fi
	storage_packages # --longhorn / --nfs: the node needs the storage clients too
	if [ "$DRY_RUN" -eq 1 ]; then
		run sh -c "curl -sfL https://get.k3s.io | K3S_URL='$JOIN_URL' K3S_TOKEN=<token> INSTALL_K3S_VERSION='$K3S_VERSION' sh -"
		return
	fi
	curl -sfL https://get.k3s.io | K3S_URL="$JOIN_URL" K3S_TOKEN="$JOIN_TOKEN" INSTALL_K3S_VERSION="$K3S_VERSION" sh -
	ok "Joined. The node shows up under Server in the panel within a minute."
	me="$(hostname -I 2>/dev/null | awk '{print $1}')"
	echo
	echo "  If the server has a firewall, allow this machine there (on the server):"
	echo "    sudo ufw allow from ${me:-<this-ip>}        # or: firewall-cmd --permanent --zone=trusted --add-source=${me:-<this-ip>}"
	echo
}

# setup_done: the panel already has its owner (a re-run). The NodePort may need a moment after a rollout.
setup_done() {
	i=0
	while [ "$i" -lt 5 ]; do
		if r="$(curl -fsS --max-time 5 "http://127.0.0.1:$PANEL_NODEPORT/api/setup" 2>/dev/null)"; then
			case "$r" in *'"required":false'*) return 0 ;; *) return 1 ;; esac
		fi
		i=$((i + 1)); sleep 2
	done
	return 1
}

print_summary() {
	ip="$(public_ip)"; sp="$(ssh_port)"
	echo
	printf '%s⬢ Nectar Apps is installed%s\n\n' "$C_BOLD$C_AMBER" "$C_RESET"
	if [ "$DRY_RUN" -eq 0 ] && setup_done; then
		# a re-run (update): the owner exists, the one-time setup link no longer applies
		echo "  Sign in as before. Through an SSH tunnel:  ssh -p $sp -L 8480:127.0.0.1:$PANEL_NODEPORT root@${ip:-<server-ip>}"
		echo "  then browse to: http://localhost:8480 (or your panel domain)"
		echo
	else
		echo "  The panel is not exposed to the internet yet. Open it through an SSH tunnel:"
		echo
		echo "    ssh -p $sp -L 8480:127.0.0.1:$PANEL_NODEPORT root@${ip:-<server-ip>}"
		echo "    then browse to: http://localhost:8480/setup?token=${SETUP_TOKEN:-<see secret>}"
		echo
		[ -n "$BASE_DOMAIN" ] && echo "  Point *.$BASE_DOMAIN (A record) to ${ip:-<server-ip>} — the first-run wizard publishes the panel at panel.$BASE_DOMAIN."
	fi
	if [ -n "$RESTORE_KIT" ]; then
		echo "  Restoring from the backup: follow it with"
		echo "    k3s kubectl -n $NAMESPACE logs -f deploy/nectar-apps"
		echo "  Sign in with your old account once the panel is up; apps and databases come back one by one."
		echo
	fi
	if [ "${NEW_SECRETS:-0}" -eq 1 ] && [ -n "${MASTER_KEY:-}" ] && [ -z "$RESTORE_MASTER_KEY" ]; then
		echo
		printf '  %sMASTER KEY — store it somewhere safe (password manager). It is needed to restore backups:%s\n' "$C_BOLD" "$C_RESET"
		echo "    $MASTER_KEY"
		echo "  It is shown only once."
	fi
	echo
	if [ "${CLI_DONE:-0}" -eq 1 ]; then
		echo "  CLI on this server:  nectar login   (asks for an API token: Settings → API tokens), then e.g. nectar apps"
	fi
	echo "  Forgot your password?  k3s kubectl -n $NAMESPACE exec deploy/nectar-apps -- nectar-apps admin reset-password --email you@example.com"
	echo
}

do_install() {
	if [ -n "$RESTORE_KIT" ] || [ -n "$RESTORE_MASTER_KEY" ]; then
		if [ -z "$RESTORE_KIT" ] || [ -z "$RESTORE_MASTER_KEY" ]; then
			die "--restore-kit and --master-key go together"
		fi
		[ -r "$RESTORE_KIT" ] || die "cannot read $RESTORE_KIT"
		grep -q '"repository"' "$RESTORE_KIT" || die "$RESTORE_KIT is not a recovery kit"
		! k3s_present || ! kc get ns "$NAMESPACE" >/dev/null 2>&1 || die "Nectar is already installed here: restore on a fresh server"
	fi
	if k3s_present && ! kc get ns "$NAMESPACE" >/dev/null 2>&1; then
		# k3s exists but Nectar is not installed → adopt mode (asks before every change)
		do_adopt
		exit 0
	fi
	if k3s_present && [ -f "$STATE_DIR/adopt/state.json" ]; then
		# re-run on an adopted server: update the stack, keep its ports/routing
		install_prerequisites
		fix_apparmor_signals
		keep_existing_settings
		setup_registry
		deploy_stack
		setup_storage
		install_cli
		print_summary
		exit 0
	fi
	if ! k3s_present; then
		for p in 80 443; do
			h="$(port_holder "$p")"
			[ -n "$h" ] && die "port $p is already in use by $h — stop it first (or use adopt mode once available)"
		done
	fi
	info "This will install k3s (if missing), nectar-gateway (ports 80/443) and the Nectar Apps panel."
	confirm "Continue?" || die "aborted"
	install_prerequisites
	configure_firewall
	setup_registry
	install_k3s
	fix_apparmor_signals
	keep_existing_settings
	deploy_stack
	setup_storage
	install_cli
	print_summary
}

do_uninstall() {
	k3s_present || die "k3s not found — nothing to uninstall"
	warn "This removes nectar-apps, nectar-gateway and their data (namespace $NAMESPACE)."
	warn "Projects created by Nectar (namespaces prj-*) are NOT removed."
	[ "$PURGE_K3S" -eq 1 ] && warn "--purge-k3s: k3s and ALL workloads on this server will be destroyed."
	confirm "Continue?" || die "aborted"
	if [ -f "$STATE_DIR/adopt/state.json" ]; then
		info "Adopted server: restoring the previous proxy first"
		fetch_adopt_bin
		if [ "$DRY_RUN" -eq 1 ]; then run "$ADOPT_BIN" adopt rollback --purge; else adopt_cli rollback --purge || warn "rollback reported problems (backups: $STATE_DIR/backup)"; fi
		# keep the finished adoption's state and report with the backups; a later install starts afresh
		if [ -d "$STATE_DIR/adopt" ]; then
			run mkdir -p "$STATE_DIR/backup"
			run mv "$STATE_DIR/adopt" "$STATE_DIR/backup/adopt-$(date +%Y%m%d-%H%M%S)"
		fi
	fi
	run k3s kubectl delete namespace "$NAMESPACE" --ignore-not-found
	run k3s kubectl delete clusterrolebinding,clusterrole nectar-apps --ignore-not-found
	run k3s kubectl delete ingressclass nectar --ignore-not-found
	run k3s kubectl label nodes --all nectar.dev/edge- >/dev/null 2>&1 || true
	# the built-in registry is gone: drop its k3s mirror (it also holds the registry password)
	remove_registries_yaml
	if [ -f "$STATE_DIR/cli-installed" ]; then
		run rm -f "$CLI_DEST" "$STATE_DIR/cli-installed"
		info "Removed the nectar CLI ($CLI_DEST); ~/.config/nectar stays"
	fi
	if [ -d "$STATE_DIR/backup" ]; then
		run rm -rf "$STATE_DIR/bin" "$STATE_DIR/installed-version"
		warn "Backups kept in $STATE_DIR/backup (delete them yourself when no longer needed)"
	else
		run rm -rf "$STATE_DIR"
	fi
	if [ "$PURGE_K3S" -eq 1 ]; then
		if [ -x /usr/local/bin/k3s-uninstall.sh ]; then run /usr/local/bin/k3s-uninstall.sh; else warn "k3s-uninstall.sh not found"; fi
	fi
	ok "Uninstalled"
}

# ------------------------------------------------------------------- main --
check_platform
resolve_versions
case "$MODE" in
	install) do_install ;;
	discover) k3s_present || die "k3s is not installed — nothing to discover"; discover ;;
	adopt) do_adopt ;;
	status) fetch_adopt_bin; adopt_cli status ;;
	rollback)
		fetch_adopt_bin
		confirm "Undo the adoption cutover (the previous proxy gets 80/443 back)?" || die "aborted"
		adopt_cli rollback ;;
	finalize)
		fetch_adopt_bin
		set --
		[ "$FORCE" -eq 1 ] && set -- "$@" --force
		[ "$RESTART_K3S" -eq 1 ] && set -- "$@" --restart-k3s
		confirm "Stop depending on the previous proxy (finalize)?" || die "aborted"
		adopt_cli finalize "$@" ;;
	uninstall) do_uninstall ;;
	join) do_join ;;
esac
