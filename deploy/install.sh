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
REGISTRY="auto"                           # built-in image registry for Git builds: auto (new installs) | 1 | 0
REGISTRY_HOST="registry.nectar.local"     # the name nodes pull built images from (a k3s mirror)
REGISTRY_NODEPORT=30500
REGISTRY_PASSWORD=""
REGISTRY_MANUAL=0                          # set when your own registries.yaml has mirrors/configs (merge by hand)
JOIN_URL=""                               # --join: add this machine to an existing Nectar server as a k3s agent
JOIN_TOKEN=""
SUC_VERSION="${SUC_VERSION:-v0.14.2}"

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
                         (Settings → Updates → k3s; off until you choose a policy there)
  --registry             set up the built-in image registry for Git builds (default on new installs;
                         on an existing server this restarts k3s once to load the registry mirror)
  --no-registry          do not set up the registry

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
		--registry) REGISTRY=1; shift ;;
		--no-registry) REGISTRY=0; shift ;;
		--registry-password) REGISTRY_PASSWORD="${2:?}"; shift 2 ;;
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
fetch_adopt_bin() {
	if [ -n "$ADOPT_BIN" ]; then
		[ -x "$ADOPT_BIN" ] || die "--binary $ADOPT_BIN is not executable"
		return
	fi
	ADOPT_BIN="$STATE_DIR/bin/nectar-apps"
	if [ "$NECTAR_VERSION" = latest ]; then base="$RELEASES/latest/download"; else base="$RELEASES/download/v$NECTAR_VERSION"; fi
	asset="nectar-apps_linux_$ARCH"
	info "Downloading $asset ($NECTAR_VERSION)"
	if [ "$DRY_RUN" -eq 1 ]; then run curl -fsSL "$base/$asset" -o "$ADOPT_BIN"; return; fi
	mkdir -p "$STATE_DIR/bin"
	tmp="$(mktemp)"
	curl -fsSL "$base/$asset" -o "$tmp" || die "download failed: $base/$asset"
	want="$(curl -fsSL "$base/$asset.sha256" | awk '{print $1}')" || die "download failed: $base/$asset.sha256"
	got="$(sha256sum "$tmp" 2>/dev/null | awk '{print $1}' || openssl dgst -sha256 -r "$tmp" | awk '{print $1}')"
	[ -n "$want" ] && [ "$want" = "$got" ] || { rm -f "$tmp"; die "checksum mismatch for $asset"; }
	install -m 0755 "$tmp" "$ADOPT_BIN"
	rm -f "$tmp"
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
	[ -n "$JOIN_URL" ] && [ -n "$JOIN_TOKEN" ] || die "--join needs the server URL and --token (shown under Server → Add node)"
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
	echo "  Forgot your password?  k3s kubectl -n $NAMESPACE exec deploy/nectar-apps -- nectar-apps admin reset-password --email you@example.com"
	echo
}

do_install() {
	if [ -n "$RESTORE_KIT" ] || [ -n "$RESTORE_MASTER_KEY" ]; then
		[ -n "$RESTORE_KIT" ] && [ -n "$RESTORE_MASTER_KEY" ] || die "--restore-kit and --master-key go together"
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
		keep_existing_settings
		setup_registry
		deploy_stack
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
	keep_existing_settings
	deploy_stack
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
