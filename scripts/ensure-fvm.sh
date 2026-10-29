#!/bin/sh
# Makes sure FVM (Flutter Version Management, https://fvm.app) is installed and the Flutter SDK pinned in
# ui/.fvmrc is installed, then prints the path of the fvm binary on stdout (everything else goes to stderr):
#
#   FVM="$(./scripts/ensure-fvm.sh)" && (cd ui && "$FVM" flutter test)
#
# Install order: fvm already on PATH (or in a known location) → Homebrew on macOS → the release binary from
# GitHub (no sudo, works as root in containers) → the official installer.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="${FVM_BIN_DIR:-$HOME/.local/bin}"

log() { printf '%s\n' "⬢ $*" >&2; }

find_fvm() {
	if command -v fvm >/dev/null 2>&1; then command -v fvm; return 0; fi
	for p in "$BIN_DIR/fvm" "$HOME/fvm/bin/fvm" "$HOME/.pub-cache/bin/fvm" /opt/homebrew/bin/fvm /usr/local/bin/fvm; do
		[ -x "$p" ] && { echo "$p"; return 0; }
	done
	return 1
}

install_release() {
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	case "$os" in darwin) os=macos ;; linux) ;; *) return 1 ;; esac
	case "$(uname -m)" in x86_64|amd64) arch=x64 ;; arm64|aarch64) arch=arm64 ;; *) return 1 ;; esac
	command -v curl >/dev/null 2>&1 || return 1
	tag="${FVM_VERSION:-}"
	if [ -z "$tag" ]; then # github.com/…/releases/latest redirects to …/tag/<version>
		tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/leoafarias/fvm/releases/latest 2>/dev/null | sed -n 's#.*/tag/##p')"
	fi
	if [ -z "$tag" ]; then
		tag="$(curl -fsSL https://api.github.com/repos/leoafarias/fvm/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
	fi
	[ -n "$tag" ] || return 1
	tmp="$(mktemp -d)"
	log "downloading fvm $tag ($os-$arch)"
	curl -fsSL "https://github.com/leoafarias/fvm/releases/download/$tag/fvm-$tag-$os-$arch.tar.gz" -o "$tmp/fvm.tgz" || return 1
	tar -xzf "$tmp/fvm.tgz" -C "$tmp" || return 1
	dest="${FVM_HOME_DIR:-$HOME/.local/share/fvm-cli}"
	rm -rf "$dest" && mkdir -p "$dest" "$BIN_DIR"
	cp -R "$tmp/fvm/." "$dest/"
	ln -sf "$dest/fvm" "$BIN_DIR/fvm"
	rm -rf "$tmp"
}

log "checking fvm…"
if FVM="$(find_fvm)"; then
	log "fvm found: $FVM ($("$FVM" --version 2>/dev/null | tail -n1))"
else
	log "fvm is not installed, installing it"
	if [ "$(uname -s)" = Darwin ] && command -v brew >/dev/null 2>&1; then
		log "→ brew install fvm (output below)"
		if ! { HOMEBREW_NO_AUTO_UPDATE=1 brew tap leoafarias/fvm >&2 && HOMEBREW_NO_AUTO_UPDATE=1 brew install fvm >&2; }; then
			log "  Homebrew failed, trying the release binary"
		fi
	fi
	FVM="$(find_fvm)" || { install_release || true; FVM="$(find_fvm)"; } || {
		log "→ official installer (https://fvm.app/install.sh)"
		curl -fsSL https://fvm.app/install.sh | bash >&2 || true
		FVM="$(find_fvm)"
	} || { log "could not install fvm — see https://fvm.app/documentation/getting-started/installation"; exit 1; }
	log "installed $FVM"
	case ":$PATH:" in *":$(dirname "$FVM"):"*) ;; *) log "add $(dirname "$FVM") to your PATH to use fvm directly" ;; esac
fi

# the Flutter SDK pinned for this project (ui/.fvmrc); a no-op when it is already installed
cd "$ROOT/ui"
want="$(sed -n 's/.*"flutter": *"\([^"]*\)".*/\1/p' .fvmrc 2>/dev/null)"
log "Flutter SDK from ui/.fvmrc: ${want:-?}"
log "→ fvm install (the first time this clones Flutter, ~1 GB: a few minutes; progress below)"
"$FVM" install >&2 </dev/null || { log "fvm install failed (output above)"; exit 1; }
log "Flutter SDK ready"

echo "$FVM"
