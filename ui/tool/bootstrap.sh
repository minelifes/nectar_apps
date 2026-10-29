#!/bin/sh
# Installs fvm and the Flutter SDK pinned in .fvmrc (if missing), generates the platform folders
# (web/ios/android/macos) around lib/ and applies the settings a self-hosted panel client needs.
# Safe to re-run: existing files are never overwritten.
#   ./tool/bootstrap.sh            (from ui/, or `make ui-setup` from the repo root)
set -eu
cd "$(dirname "$0")/.."

step() { printf '\n\033[1;33m[%s/6]\033[0m %s\n' "$1" "$2"; }

step 1 "fvm and the pinned Flutter SDK"
FVM="$(../scripts/ensure-fvm.sh)"

step 2 "Flutter first run (downloads the Dart SDK and web tools the first time: can take a few minutes)"
"$FVM" flutter --version </dev/null

step 3 "generating platform folders (web, ios, android, macos) — existing files are kept"
had_test=0; [ -f test/widget_test.dart ] && had_test=1
"$FVM" flutter create --project-name nectar_apps_ui --org dev.nectar --platforms web,ios,android,macos . </dev/null
# `flutter create` adds a counter-app test that does not match this app
[ "$had_test" -eq 0 ] && rm -f test/widget_test.dart

step 4 "platform settings (network access, plain-HTTP panels)"
# Android: network access in release builds + plain-HTTP servers (SSH tunnels, LAN IPs)
m=android/app/src/main/AndroidManifest.xml
if [ -f "$m" ] && ! grep -q 'android.permission.INTERNET' "$m"; then
	perl -0pi -e 's#<application#<uses-permission android:name="android.permission.INTERNET"/>\n    <application#' "$m"
fi
if [ -f "$m" ] && ! grep -q 'usesCleartextTraffic' "$m"; then
	perl -0pi -e 's#<application#<application android:usesCleartextTraffic="true"#' "$m"
fi

if [ "$(uname)" = Darwin ]; then
	pb=/usr/libexec/PlistBuddy
	# macOS sandbox: outgoing connections to the panel
	for f in macos/Runner/DebugProfile.entitlements macos/Runner/Release.entitlements; do
		[ -f "$f" ] && { $pb -c 'Print :com.apple.security.network.client' "$f" >/dev/null 2>&1 ||
			$pb -c 'Add :com.apple.security.network.client bool true' "$f"; }
		# choosing a JAR / zip to upload
		[ -f "$f" ] && { $pb -c 'Print :com.apple.security.files.user-selected.read-only' "$f" >/dev/null 2>&1 ||
			$pb -c 'Add :com.apple.security.files.user-selected.read-only bool true' "$f"; }
	done
	# iOS + macOS: allow plain-HTTP panels (http://localhost:8480 through an SSH tunnel, LAN IPs)
	for f in ios/Runner/Info.plist macos/Runner/Info.plist; do
		[ -f "$f" ] && { $pb -c 'Print :NSAppTransportSecurity' "$f" >/dev/null 2>&1 || {
			$pb -c 'Add :NSAppTransportSecurity dict' "$f"
			$pb -c 'Add :NSAppTransportSecurity:NSAllowsArbitraryLoads bool true' "$f"
			$pb -c 'Add :NSAppTransportSecurity:NSAllowsLocalNetworking bool true' "$f"
		}; }
	done
fi

step 5 "packages (flutter pub get)"
"$FVM" flutter pub get </dev/null

step 6 "app icons"
"$FVM" dart run flutter_launcher_icons </dev/null
echo "✓ ready: make ui-dev   (or: cd ui && fvm flutter run -d macos / ios / android)"
