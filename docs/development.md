# Development

Requirements: Go 1.26+. Everything else (Docker runtime, k3d, kubectl) is installed by the dev script;
Flutter (via [fvm](https://fvm.app)) for the UI.

```bash
make dev-cluster                  # macOS: k3s in Docker (k3d)
make dev                          # starts the dev cluster + kubectl proxy and runs the panel
open http://localhost:8080        # setup token in dev mode: "dev"
make dev-cluster-down             # stop and delete the dev cluster
```

```bash
make ui-setup    # once: generate the Flutter platform folders
make ui-dev      # the UI in Chrome against `make run`
make ui build    # embed the web UI into the binary
make test        # unit + API tests
make lint        # go vet + gofmt
make shellcheck  # installer lint
make help        # everything else
```

Layout: `cmd/nectar-apps` (panel binary) · `cmd/nectar` (CLI) · `internal/` (api, auth, authz, store, kube,
deploy, routing, gateway client, alerts, backups, blueprint, sso, push, …) · `api/openapi.yaml` (API
contract) · `deploy/` (installer + manifests) · `web/` (embedded UI) · `ui/` (Flutter) · `docs/` (this site,
MkDocs Material: `pip install mkdocs-material && mkdocs serve`).

Contributions need the [CLA](https://github.com/minelifes/nectar_apps/blob/main/CLA.md).
