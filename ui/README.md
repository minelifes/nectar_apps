# Nectar Apps UI (Flutter)

One codebase for the web panel (embedded into the `nectar-apps` binary), iOS, Android and macOS.

Flutter always runs through [FVM](https://fvm.app): the SDK version is pinned in `.fvmrc`, and
`scripts/ensure-fvm.sh` installs fvm (Homebrew on macOS, otherwise the release binary) and that SDK when
they are missing. Every `make ui*` target does this for you. To change the SDK: `cd ui && fvm use <version>`.

## First time

```bash
make ui-setup      # fvm + pinned Flutter, flutter create for web/ios/android/macos, app icons (safe to re-run)
```

The platform folders are generated, not hand-written: `tool/bootstrap.sh` also turns on what a self-hosted
panel client needs (network access in the macOS sandbox, Android INTERNET + plain HTTP, iOS/macOS ATS
exceptions for `http://` panels such as an SSH tunnel). Commit the generated folders.

## Develop

```bash
make run           # terminal 1: the panel on :8080 (dev mode, setup token "dev")
make ui-dev        # terminal 2: Chrome, talking to http://localhost:8080 with bearer tokens
cd ui && fvm flutter run -d macos   # native: asks for the server address (http://localhost:8080)
make ui-test       # fvm flutter analyze + fvm flutter test
```

`--dart-define=NECTAR_API=<url>` points any build at a panel. Without it, the web build talks to the origin
that served it (the embedded panel, cookie session) and native apps ask for the address once.

## Ship

```bash
make ui build      # builds web → web/dist, then the Go binary embeds it
```

Release images build the UI in the Dockerfile (also through fvm and `.fvmrc`), so `docker build` needs no local Flutter.

## Layout

```
lib/
  api/        client (dio, cookie or token auth, refresh), models, session state
  core/       server address, formatting, theme mode
  data/       Riverpod providers for API data
  theme/      brand tokens + Material 3 theme (dark-first)
  widgets/    hex shapes, mark, loader, status pills, gauges, cards, app tiles
  shell/      sidebar (wide) / bottom navigation (phones)
  features/   auth (connect, login, first-run setup), dashboard, projects, server, settings
```

Fonts (Inter, Space Grotesk, JetBrains Mono, SIL OFL) are bundled, so the panel works without internet
access, and the web build uses `--no-web-resources-cdn` for the same reason.
