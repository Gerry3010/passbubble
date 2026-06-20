# Passbubble Monorepo Makefile

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
_VER_PKG    = github.com/Gerry3010/passbubble/backend/internal/version
LDFLAGS     = -ldflags="-s -w \
  -X $(_VER_PKG).Version=$(VERSION) \
  -X $(_VER_PKG).Commit=$(COMMIT) \
  -X $(_VER_PKG).BuildTime=$(BUILD_TIME)"

# Optional local-only deployment targets (gitignored; see deploy.local.mk).
# The leading '-' makes this a no-op when the file is absent (CI, fresh clones).
-include deploy.local.mk

.PHONY: help up down up-prod dev \
        build-backend build-cli build-all \
        test test-backend test-cli test-flutter test-all \
        lint migrate migrate-down migrate-create sqlc \
        build-extension test-extension sync-assets icons mailer-icon launcher-icons \
        native-crypto native-crypto-android web-crypto \
        clean

help:
	@echo "Passbubble - Self-Hosted Password Manager"
	@echo ""
	@echo "Usage:"
	@echo "  make up              Start development stack (Docker Compose)"
	@echo "  make down            Stop stack"
	@echo "  make up-prod         Start production stack (with Caddy TLS)"
	@echo "  make dev             Run backend locally"
	@echo "  make build-all       Build all binaries"
	@echo "  make test            Run all tests"
	@echo "  make lint            Run linters"
	@echo "  make migrate         Run DB migrations"
	@echo "  make migrate-down    Rollback last migration"
	@echo "  make migrate-create  Create new migration"
	@echo "  make sqlc            Regenerate sqlc models"
	@echo "  make build-extension Build Chrome + Firefox extension"
	@echo "  make test-extension  Run extension tests"
	@echo "  make sync-assets     Distribute SVGs from assets/svg/ → flutter_app + extension"
	@echo "  make icons           Rasterize SVG → extension PNGs (needs rsvg-convert)"
	@echo "  make clean           Remove build artifacts"

# ── Docker ────────────────────────────────────────────────────────────────────

up:
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env - please edit passwords before continuing!"; exit 1; fi
	docker compose up -d postgres redis mailpit backend

down:
	docker compose down

up-prod:
	docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production up -d

dev: mailer-icon
	cd backend && go run ./cmd/server

# ── Build ─────────────────────────────────────────────────────────────────────

build-backend: mailer-icon
	mkdir -p build
	cd backend && CGO_ENABLED=0 go build $(LDFLAGS) -o ../build/passbubble-server ./cmd/server

build-cli:
	mkdir -p build
	cd cli && go build $(LDFLAGS) -o ../build/pwmgr ./cmd/pwmgr

build-all: build-backend build-cli

# ── Test ──────────────────────────────────────────────────────────────────────

test: test-backend test-cli
	@echo "All Go tests passed"

test-backend: mailer-icon
	cd backend && go test ./... -race -count=1

test-cli:
	cd cli && go test ./... -race -count=1

test-flutter: sync-assets
	cd flutter_app && flutter pub get && flutter test --coverage

test-all: test test-flutter

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	cd backend && golangci-lint run ./...
	cd cli && golangci-lint run ./...
	@if [ -d flutter_app ]; then cd flutter_app && flutter analyze; fi

# ── Database ──────────────────────────────────────────────────────────────────

migrate:
	@if [ -z "$(DB_URL)" ]; then \
		export DB_URL=$$(grep DATABASE_URL .env 2>/dev/null | cut -d= -f2-); \
	fi; \
	migrate -path backend/internal/db/migrations -database "$${DB_URL:-$(DB_URL)}" up

migrate-down:
	migrate -path backend/internal/db/migrations -database "$(DB_URL)" down 1

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir backend/internal/db/migrations -seq "$${name}"

sqlc:
	cd backend && sqlc generate

# ── Browser Extension ─────────────────────────────────────────────────────────

build-extension: icons
	cd packages/shared-ts && npm run build
	cd extension && npm run build:chrome && npm run build:firefox

test-extension:
	cd packages/shared-ts && npm ci && npm test
	cd extension && npm ci && npm test

# ── SVG Assets ────────────────────────────────────────────────────────────────

sync-assets: ## Copy SVG sources from assets/svg/ into sub-projects
	mkdir -p flutter_app/assets/svg extension/icons
	cp assets/svg/icon.svg flutter_app/assets/svg/icon.svg
	cp assets/svg/icon.svg flutter_app/web/favicon.svg
	cp assets/svg/icon-extension.svg extension/icons/icon.svg

icons: sync-assets mailer-icon launcher-icons ## Rasterize SVG → extension + email PNG icons (requires rsvg-convert)
	mkdir -p extension/public/icons
	rsvg-convert -w 16  -h 16  assets/svg/icon-extension.svg -o extension/public/icons/icon16.png
	rsvg-convert -w 48  -h 48  assets/svg/icon-extension.svg -o extension/public/icons/icon48.png
	rsvg-convert -w 128 -h 128 assets/svg/icon-extension.svg -o extension/public/icons/icon128.png

# Android launcher icons (legacy square mipmaps) rendered from the opaque app
# SVG SSOT (assets/svg/icon.svg). Like the extension/mail PNGs these are
# generated build artifacts (gitignored) — run this before any local Android
# (APK/AAB) build so the mipmap resources exist. CI builds no Android target, so
# nothing else regenerates them. Densities: mdpi 48, hdpi 72, xhdpi 96, xxhdpi 144, xxxhdpi 192.
launcher-icons: ## Render Android launcher mipmaps from the app SVG SSOT (gitignored)
	rsvg-convert -w 48  -h 48  assets/svg/icon.svg -o flutter_app/android/app/src/main/res/mipmap-mdpi/ic_launcher.png
	rsvg-convert -w 72  -h 72  assets/svg/icon.svg -o flutter_app/android/app/src/main/res/mipmap-hdpi/ic_launcher.png
	rsvg-convert -w 96  -h 96  assets/svg/icon.svg -o flutter_app/android/app/src/main/res/mipmap-xhdpi/ic_launcher.png
	rsvg-convert -w 144 -h 144 assets/svg/icon.svg -o flutter_app/android/app/src/main/res/mipmap-xxhdpi/ic_launcher.png
	rsvg-convert -w 192 -h 192 assets/svg/icon.svg -o flutter_app/android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png

# ── Native / Web crypto (hybrid KEM) ────────────────────────────────────────────
# The Flutter app's hybrid KEM (X25519 + ML-KEM-768) ships two ways, both
# GENERATED from sources (flutter_app/native = Go c-shared wrapper around
# backend/pkg/crypto; flutter_app/web_crypto = esbuild bundle of shared-ts):
#
#   native-crypto          → host .so/.dylib/.dll  (desktop + `flutter test` FFI)   [gitignored]
#   native-crypto-android  → per-ABI jniLibs .so   (local APK/AAB; needs NDK)       [gitignored]
#   web-crypto             → flutter_app/web/passbubble_crypto.js (Flutter web)     [committed]
#
# CI builds no Android/desktop target, so nothing regenerates the .so libs there —
# run native-crypto before FFI tests/desktop runs and native-crypto-android before
# a local Android build. The web bundle stays committed because `flutter build web`
# (Dockerfile + release) consumes it and the build context lacks the node/shared-ts
# toolchain; rebuild + commit it with `make web-crypto` whenever the crypto sources
# change. The FFI test skips gracefully when the host .so is absent.

native-crypto: ## Build the host hybrid-KEM c-shared lib (.so/.dylib/.dll) for FFI (gitignored)
	cd flutter_app/native && ./build.sh

native-crypto-android: ## Build per-ABI Android jniLibs (.so) — needs ANDROID_NDK_HOME (gitignored)
	cd flutter_app/native && ./build.sh android

web-crypto: ## Bundle shared-ts hybrid-KEM → flutter_app/web/passbubble_crypto.js (committed)
	cd packages/shared-ts && npm ci
	cd flutter_app/web_crypto && npm install && npm run build

# Transparent brand icon embedded into transactional emails (//go:embed). It is a
# generated build artifact (gitignored): assets/svg/icon-extension.svg is the
# single source of truth. Every target that compiles the backend depends on this
# so the go:embed asset always exists; CI and the Dockerfile regenerate it too.
mailer-icon: ## Generate the email brand icon (go:embed) from the SVG SSOT
	rsvg-convert -w 192 -h 192 assets/svg/icon-extension.svg -o backend/internal/mailer/passbubble-icon.png

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf build/
	rm -rf flutter_app/build/
	rm -rf backend/embed/web/*
