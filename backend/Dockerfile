# ─── Flutter web build stage ─────────────────────────────────────────────────
# Builds the Flutter web app (serves both the user UI at /web/* and the
# admin panel at /admin/* — same app, gated by JWT role on the server side).
FROM ghcr.io/cirruslabs/flutter:stable AS flutter-builder

WORKDIR /build
COPY flutter_app/pubspec.yaml flutter_app/pubspec.lock ./
RUN flutter pub get
COPY flutter_app/ .
RUN flutter build web --release

# ─── Go build stage ───────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Download dependencies first (better caching)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source and embed the Flutter web build before compiling
COPY backend/ .
COPY --from=flutter-builder /build/build/web ./internal/static/web
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /passbubble-server ./cmd/server

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
