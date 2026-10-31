# Nectar Apps — developer tasks.  `make help` lists them.

# A stale GOROOT (e.g. left over from a removed Homebrew Go) breaks every go command: ignore it.
ifneq ($(GOROOT),)
ifeq ($(wildcard $(GOROOT)/bin/go),)
unexport GOROOT
endif
endif

GO        ?= go
BIN       := bin/nectar-apps
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -s -w -X nectar_apps/internal/version.Version=$(VERSION) -X nectar_apps/internal/version.Commit=$(COMMIT)
IMAGE     ?= ghcr.io/minelifes/nectar-apps
DATA_DIR  ?= .data
# local dev: the panel runs on your machine and talks to the k3d cluster through kubectl proxy;
# app pods download artifacts from the panel via host.k3d.internal
NECTAR_KUBE_API     ?= http://127.0.0.1:8001
NECTAR_INTERNAL_URL ?= http://host.k3d.internal:8080

.PHONY: help build cli run dev test lint fmt tidy image dev-cluster dev-cluster-down shellcheck clean ui ui-setup ui-dev ui-l10n ui-test

help: ## show this help
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[38;5;214m%-18s\033[0m %s\n", $$1, $$2}'

build: ## build the server binary into bin/
	@$(GO) version >/dev/null 2>&1 || ./scripts/dev-cluster.sh install-go
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/nectar-apps

cli: ## build the nectar CLI into bin/nectar
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/nectar ./cmd/nectar

run: build ## run the panel locally against the dev cluster (setup token: dev)
	NECTAR_DATA_DIR=$(DATA_DIR) NECTAR_DEV=1 NECTAR_SETUP_TOKEN=dev \
	NECTAR_KUBE_API=$(NECTAR_KUBE_API) NECTAR_INTERNAL_URL=$(NECTAR_INTERNAL_URL) $(BIN) serve

dev: dev-cluster run ## install prerequisites, start the dev cluster and run the panel — one command

test: ## run all Go tests
	$(GO) test -race -count=1 ./...

lint: ## go vet + gofmt check
	$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run: make fmt" && exit 1)

fmt: ## format Go code
	gofmt -w .

tidy: ## go mod tidy
	$(GO) mod tidy

shellcheck: ## lint the installer
	shellcheck -s sh deploy/install.sh scripts/*.sh

image: ## build the container image (multi-arch via buildx: make image PLATFORMS=linux/amd64,linux/arm64)
	docker buildx build --platform $${PLATFORMS:-linux/amd64} --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(IMAGE):$(VERSION) .

# Flutter always runs through FVM with the SDK pinned in ui/.fvmrc; ensure-fvm.sh installs both if missing
ENSURE_FVM := $(CURDIR)/scripts/ensure-fvm.sh

ui-setup: ## install fvm + the pinned Flutter, generate the platform folders (web/ios/android/macos), icons
	cd ui && ./tool/bootstrap.sh

ui: ## build the Flutter web UI into web/dist (embedded by the next `make build`)
	cd ui && fvm="$$($(ENSURE_FVM))" && "$$fvm" dart run intl_utils:generate && "$$fvm" flutter build web --release --no-web-resources-cdn
	find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} +
	cp -R ui/build/web/. web/dist/

ui-dev: ## run the UI in Chrome against the local panel (make run in another terminal)
	cd ui && fvm="$$($(ENSURE_FVM))" && "$$fvm" flutter run -d chrome --dart-define=NECTAR_API=http://localhost:8080

ui-l10n: ## regenerate the translations (lib/l10n/intl_*.arb → lib/generated, Flutter Intl)
	cd ui && fvm="$$($(ENSURE_FVM))" && "$$fvm" dart run intl_utils:generate

ui-test: ## analyze + test the Flutter UI
	cd ui && fvm="$$($(ENSURE_FVM))" && "$$fvm" dart run intl_utils:generate && "$$fvm" flutter analyze && "$$fvm" flutter test

dev-cluster: ## install k3d/kubectl/Docker if missing and start a local k3s cluster (k3d)
	./scripts/dev-cluster.sh up

dev-cluster-down: ## stop kubectl proxy and delete the local dev cluster
	./scripts/dev-cluster.sh down

clean: ## remove build output and local data
	rm -rf bin $(DATA_DIR) ui/build
	find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} +
