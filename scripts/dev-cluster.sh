#!/bin/sh
# Local development cluster: k3s in Docker via k3d. Installs everything it needs.
# Mirrors production (Traefik and ServiceLB disabled). Dev only — not a supported install target.
#
#   ./scripts/dev-cluster.sh up        # install prerequisites, create cluster "nectar-dev", start kubectl proxy,
#                                      # build + deploy nectar-gateway from ../nectar_gateway (if present)
#   ./scripts/dev-cluster.sh gateway   # (re)build and redeploy only the gateway
#   ./scripts/dev-cluster.sh down      # stop the proxy and delete the cluster
#   ./scripts/dev-cluster.sh status    # show what is running
#
# Prerequisites installed automatically when missing:
#   macOS: Homebrew, Go, k3d, kubectl, OrbStack (Docker runtime; an existing Docker Desktop/Colima is used if present)
#   Linux: Go (go.dev), Docker (get.docker.com), k3d (official script), kubectl (dl.k8s.io)
# Set NECTAR_NO_INSTALL=1 to only check, never install.
set -eu

NAME="${NECTAR_DEV_CLUSTER:-nectar-dev}"
PROXY_PORT="${NECTAR_PROXY_PORT:-8001}"
STATE_DIR="${NECTAR_DEV_STATE:-.data}"
PROXY_PID="$STATE_DIR/kubectl-proxy.pid"
GATEWAY_SRC="${NECTAR_GATEWAY_SRC:-../nectar_gateway}"
GW_PORT="${NECTAR_GATEWAY_ADMIN_PORT:-18081}"
GW_PF_PID="$STATE_DIR/gateway-port-forward.pid"
DEV_ENV="$STATE_DIR/dev.env"
NS=nectar-system

say()  { printf '\033[38;5;214m⬢\033[0m %s\n' "$*"; }
ok()   { printf '\033[32m✓\033[0m %s\n' "$*"; }
die()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

OS="$(uname -s)"

# a stale GOROOT (removed Homebrew Go) breaks the go command
if [ -n "${GOROOT:-}" ] && [ ! -x "$GOROOT/bin/go" ]; then unset GOROOT; fi

need_install() {
	[ "${NECTAR_NO_INSTALL:-}" = "1" ] && die "$1 is missing (NECTAR_NO_INSTALL=1 set, not installing)"
	say "Installing $1…"
}

# ------------------------------------------------------------------ macOS --
ensure_brew() {
	have brew && return
	for b in /opt/homebrew/bin/brew /usr/local/bin/brew; do
		if [ -x "$b" ]; then eval "$("$b" shellenv)"; return; fi
	done
	need_install "Homebrew (you may be asked for your password)"
	NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
	for b in /opt/homebrew/bin/brew /usr/local/bin/brew; do
		if [ -x "$b" ]; then eval "$("$b" shellenv)"; break; fi
	done
	have brew || die "Homebrew installation failed"
	ok "Homebrew installed"
}

go_works() { go version >/dev/null 2>&1; }

ensure_go() {
	go_works && return
	case "$OS" in
		Darwin)
			ensure_brew
			need_install "Go"
			brew install go
			;;
		Linux)
			need_install "Go"
			arch="$(uname -m)"; case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
			v="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)"
			curl -fsSLo /tmp/go.tgz "https://go.dev/dl/$v.linux-$arch.tar.gz"
			sudo_cmd rm -rf /usr/local/go
			sudo_cmd tar -C /usr/local -xzf /tmp/go.tgz
			rm -f /tmp/go.tgz
			PATH="/usr/local/go/bin:$PATH"; export PATH
			say "Add /usr/local/go/bin to your PATH (e.g. in ~/.profile)"
			;;
	esac
	hash -r 2>/dev/null || true
	go_works || die "Go installation failed"
	ok "$(go version)"
}

mac_tools() {
	ensure_brew
	for tool in k3d kubectl; do
		if ! have "$tool"; then
			need_install "$tool"
			brew install "$tool"
		fi
	done
}

mac_docker() {
	docker info >/dev/null 2>&1 && return
	if ! have docker && [ ! -d /Applications/OrbStack.app ] && [ ! -d /Applications/Docker.app ]; then
		need_install "OrbStack (Docker runtime)"
		brew install --cask orbstack
	fi
	if [ -d /Applications/OrbStack.app ]; then
		say "Starting OrbStack…"
		open -ga OrbStack
	elif [ -d /Applications/Docker.app ]; then
		say "Starting Docker Desktop…"
		open -ga Docker
	elif have colima; then
		say "Starting Colima…"
		colima start
	fi
	# OrbStack installs the docker CLI into ~/.orbstack/bin
	[ -d "$HOME/.orbstack/bin" ] && PATH="$HOME/.orbstack/bin:$PATH" && export PATH
}

# ------------------------------------------------------------------ Linux --
sudo_cmd() {
	if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

linux_tools() {
	have curl || die "curl is required"
	if ! have docker; then
		need_install "Docker"
		curl -fsSL https://get.docker.com | sudo_cmd sh
		sudo_cmd systemctl enable --now docker 2>/dev/null || true
		if [ "$(id -u)" -ne 0 ]; then
			sudo_cmd usermod -aG docker "$(id -un)"
			say "Added $(id -un) to the docker group (takes effect in new shells; using sudo for now)"
		fi
	fi
	if ! have k3d; then
		need_install "k3d"
		curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | sudo_cmd env USE_SUDO=false K3D_INSTALL_DIR=/usr/local/bin bash
	fi
	if ! have kubectl; then
		need_install "kubectl"
		arch="$(uname -m)"; case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
		v="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
		curl -fsSLo /tmp/kubectl "https://dl.k8s.io/release/$v/bin/linux/$arch/kubectl"
		sudo_cmd install -m 0755 /tmp/kubectl /usr/local/bin/kubectl
		rm -f /tmp/kubectl
	fi
}

wait_docker() {
	i=0
	until docker info >/dev/null 2>&1; do
		i=$((i + 1))
		[ "$i" -gt 90 ] && die "Docker is not running (waited 3 minutes). Start it and run again."
		[ "$i" -eq 1 ] && say "Waiting for Docker…"
		sleep 2
	done
	ok "Docker is running"
}

ensure_prerequisites() {
	ensure_go
	case "$OS" in
		Darwin) mac_tools; mac_docker ;;
		Linux) linux_tools ;;
		*) die "unsupported OS for the dev cluster: $OS" ;;
	esac
	wait_docker
	ok "k3d $(k3d version 2>/dev/null | head -1 | awk '{print $3}'), kubectl $(kubectl version --client 2>/dev/null | head -1 | awk '{print $3}')"
}

# ---------------------------------------------------------------- cluster --
proxy_running() { [ -f "$PROXY_PID" ] && kill -0 "$(cat "$PROXY_PID")" 2>/dev/null; }

start_proxy() {
	if proxy_running; then ok "kubectl proxy already running on :$PROXY_PORT"; return; fi
	if curl -fsS -m 2 "http://127.0.0.1:$PROXY_PORT/version" >/dev/null 2>&1; then
		ok "a kubectl proxy is already listening on :$PROXY_PORT (started outside this script) — using it"
		return
	fi
	mkdir -p "$STATE_DIR"
	nohup kubectl --context "k3d-$NAME" proxy --port "$PROXY_PORT" >"$STATE_DIR/kubectl-proxy.log" 2>&1 &
	echo $! >"$PROXY_PID"
	sleep 1
	proxy_running || die "kubectl proxy failed to start (see $STATE_DIR/kubectl-proxy.log)"
	ok "kubectl proxy on http://127.0.0.1:$PROXY_PORT"
}

stop_proxy() {
	if proxy_running; then kill "$(cat "$PROXY_PID")" 2>/dev/null || true; fi
	rm -f "$PROXY_PID"
}

up() {
	ensure_prerequisites
	if k3d cluster list "$NAME" >/dev/null 2>&1; then
		ok "cluster $NAME exists"
		k3d cluster start "$NAME" >/dev/null 2>&1 || true
	else
		say "Creating cluster $NAME (k3s in Docker, Traefik/ServiceLB disabled)…"
		k3d cluster create "$NAME" \
			--k3s-arg '--disable=traefik@server:0' \
			--k3s-arg '--disable=servicelb@server:0' \
			-p '8088:80@server:0' -p '8443:443@server:0' \
			--wait
	fi
	kubectl config use-context "k3d-$NAME" >/dev/null
	i=0
	until kubectl get nodes --no-headers 2>/dev/null | grep -q ' Ready'; do
		i=$((i + 1)); [ "$i" -gt 60 ] && die "node not Ready after 2 minutes"
		sleep 2
	done
	ok "cluster $NAME is Ready"
	start_proxy
	gateway
	cat <<EOF

Dev cluster ready (kubectl context k3d-$NAME; gateway: host 8088 → 80, 8443 → 443).
Start the panel against it:   make run      (or: make dev  — does both)
Setup token in dev mode:      dev

EOF
}

# ---------------------------------------------------------------- gateway --
gw_pf_running() { [ -f "$GW_PF_PID" ] && kill -0 "$(cat "$GW_PF_PID")" 2>/dev/null; }

gateway() {
	if [ ! -f "$GATEWAY_SRC/Dockerfile" ]; then
		say "nectar_gateway not found at $GATEWAY_SRC (set NECTAR_GATEWAY_SRC) — skipping the gateway; domains will not work locally"
		return 0
	fi
	# the gateway's go.sum must list golang.org/x/crypto (built-in Let's Encrypt)
	if ! grep -q 'golang.org/x/crypto' "$GATEWAY_SRC/go.sum" 2>/dev/null; then
		say "Updating $GATEWAY_SRC/go.sum (go mod tidy)…"
		(cd "$GATEWAY_SRC" && go mod tidy)
	fi
	say "Building the nectar-gateway image…"
	docker build -q -t nectar-gateway:dev "$GATEWAY_SRC" >/dev/null
	k3d image import nectar-gateway:dev -c "$NAME" >/dev/null 2>&1 || k3d image import nectar-gateway:dev -c "$NAME"
	ok "image nectar-gateway:dev imported into $NAME"

	kubectl --context "k3d-$NAME" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "k3d-$NAME" apply -f - >/dev/null
	if ! kubectl --context "k3d-$NAME" -n "$NS" get secret nectar-secrets >/dev/null 2>&1; then
		token="$(openssl rand -hex 24)"
		kubectl --context "k3d-$NAME" -n "$NS" create secret generic nectar-secrets --from-literal=gateway-api-token="$token" >/dev/null
	fi
	token="$(kubectl --context "k3d-$NAME" -n "$NS" get secret nectar-secrets -o jsonpath='{.data.gateway-api-token}' | base64 -d)"
	# the gateway runs on the edge node (all nodes of the dev cluster)
	kubectl --context "k3d-$NAME" label nodes --all nectar.dev/edge=true --overwrite >/dev/null
	sed -e "s|@@NAMESPACE@@|$NS|g" -e "s|@@GATEWAY_IMAGE@@|nectar-gateway:dev|g" -e "s|@@ACME_EMAIL@@||g" -e "s|@@ACME@@|false|g" \
		-e "s|@@HTTP_PORT@@|80|g" -e "s|@@HTTPS_PORT@@|443|g" -e "s|@@ADMIN_PORT@@|8081|g" \
		deploy/manifests/10-gateway.yaml | kubectl --context "k3d-$NAME" apply -f - >/dev/null
	# a new image with the same tag needs a restart
	kubectl --context "k3d-$NAME" -n "$NS" rollout restart deploy/nectar-gateway >/dev/null
	kubectl --context "k3d-$NAME" -n "$NS" rollout status deploy/nectar-gateway --timeout=120s >/dev/null || die "gateway did not start: kubectl -n $NS logs deploy/nectar-gateway"
	ok "nectar-gateway running (apps: http://<app>-<project>.localtest.me:8088)"

	if gw_pf_running; then kill "$(cat "$GW_PF_PID")" 2>/dev/null || true; sleep 1; fi
	nohup kubectl --context "k3d-$NAME" -n "$NS" port-forward svc/nectar-gateway-admin "$GW_PORT:8081" >"$STATE_DIR/gateway-port-forward.log" 2>&1 &
	echo $! >"$GW_PF_PID"
	sleep 1
	gw_pf_running || die "port-forward to the gateway admin API failed (see $STATE_DIR/gateway-port-forward.log)"
	cat >"$DEV_ENV" <<ENV
NECTAR_GATEWAY_URL=http://127.0.0.1:$GW_PORT
NECTAR_GATEWAY_TOKEN=$token
NECTAR_BASE_DOMAIN=localtest.me
ENV
	ok "gateway admin API on http://127.0.0.1:$GW_PORT (written to $DEV_ENV for make run)"
}

down() {
	if gw_pf_running; then kill "$(cat "$GW_PF_PID")" 2>/dev/null || true; fi
	rm -f "$GW_PF_PID" "$DEV_ENV"
	stop_proxy
	if have k3d && k3d cluster list "$NAME" >/dev/null 2>&1; then
		k3d cluster delete "$NAME"
	fi
	ok "dev cluster removed"
}

status() {
	if have k3d && k3d cluster list "$NAME" >/dev/null 2>&1; then ok "cluster $NAME exists"; else say "cluster $NAME not created"; fi
	if proxy_running; then ok "kubectl proxy running (pid $(cat "$PROXY_PID"))"; else say "kubectl proxy not running"; fi
	if gw_pf_running; then ok "gateway admin port-forward running on :$GW_PORT"; else say "gateway port-forward not running"; fi
}

case "${1:-up}" in
	up) up ;;
	gateway) gateway ;;
	down) down ;;
	status) status ;;
	install) ensure_prerequisites ;;
	install-go) ensure_go ;;
	*) echo "usage: $0 up|down|status|gateway|install|install-go"; exit 2 ;;
esac
