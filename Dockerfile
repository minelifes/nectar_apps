# Nectar Apps panel image (multi-arch: docker buildx build --platform linux/amd64,linux/arm64 .)

# web UI (architecture-independent output, built once on the build platform) through fvm + ui/.fvmrc
FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS ui
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git unzip xz-utils zip \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY scripts/ensure-fvm.sh scripts/
COPY ui/.fvmrc ui/pubspec.yaml ui/pubspec.lock* ui/
RUN FVM="$(./scripts/ensure-fvm.sh)" && cd ui && "$FVM" flutter pub get
COPY ui/ ui/
RUN cd ui && "$HOME/.local/bin/fvm" dart run intl_utils:generate && "$HOME/.local/bin/fvm" flutter build web --release --no-web-resources-cdn

FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS TARGETARCH VERSION=dev COMMIT=none
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/ui/build/web/ ./web/dist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X nectar_apps/internal/version.Version=${VERSION} -X nectar_apps/internal/version.Commit=${COMMIT}" \
      -o /out/nectar-apps ./cmd/nectar-apps

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nectar-apps /usr/local/bin/nectar-apps
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nectar-apps"]
CMD ["serve"]
