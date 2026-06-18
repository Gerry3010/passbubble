# ─── Flutter web build stage ─────────────────────────────────────────────────
# Builds the Flutter vault web app, served at /web/*. The admin panel at
# /admin/* is a standalone hand-written HTML/CSS/JS page (backend/internal/
# static/admin/) — no E2E crypto needed there, so no Flutter build for it.
#
# --platform=$BUILDPLATFORM: run Flutter on the native build host (amd64) even
# when the final image targets arm64 — Flutter web output is arch-independent.
FROM --platform=$BUILDPLATFORM ghcr.io/cirruslabs/flutter:stable AS flutter-builder

WORKDIR /build
COPY flutter_app/pubspec.yaml flutter_app/pubspec.lock ./
RUN flutter pub get
COPY flutter_app/ .
RUN flutter build web --release --base-href "/web/"

# ─── Go build stage ───────────────────────────────────────────────────────────
# --platform=$BUILDPLATFORM: compile on the native host using Go's built-in
# cross-compilation (GOARCH=$TARGETARCH) instead of QEMU emulation, cutting
# arm64 build time from ~10 min down to ~1 min.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /build

# Download dependencies first (better caching)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source (includes the static admin/ HTML) and embed the Flutter web
# build before compiling
COPY backend/ .
COPY --from=flutter-builder /build/build/web ./internal/static/web

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w \
      -X github.com/Gerry3010/passbubble/backend/internal/version.Version=${VERSION} \
      -X github.com/Gerry3010/passbubble/backend/internal/version.Commit=${COMMIT} \
      -X github.com/Gerry3010/passbubble/backend/internal/version.BuildTime=${BUILD_TIME}" \
    -o /passbubble-server ./cmd/server

# ─── Runtime image ───────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /passbubble-server /usr/local/bin/passbubble-server

# Migrations are embedded via go:embed in the binary — no extra COPY needed
EXPOSE 8765

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8765/health || exit 1

USER nobody
ENTRYPOINT ["/usr/local/bin/passbubble-server"]
