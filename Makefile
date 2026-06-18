# Passbubble Monorepo Makefile

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
_VER_PKG    = github.com/Gerry3010/passbubble/backend/internal/version
LDFLAGS     = -ldflags="-s -w \
  -X $(_VER_PKG).Version=$(VERSION) \
  -X $(_VER_PKG).Commit=$(COMMIT) \
  -X $(_VER_PKG).BuildTime=$(BUILD_TIME)"

.PHONY: help up down up-prod dev \
        build-backend build-cli build-all \
        test test-backend test-cli test-flutter test-all \
        lint migrate migrate-down migrate-create sqlc \
        build-extension test-extension icons \
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
	@echo "  make icons           Rasterize SVG → extension PNGs (needs rsvg-convert)"
	@echo "  make clean           Remove build artifacts"

# ── Docker ────────────────────────────────────────────────────────────────────

up:
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env - please edit passwords before continuing!"; exit 1; fi
	docker compose up -d postgres redis backend

down:
	docker compose down

up-prod:
	docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production up -d

dev:
	cd backend && go run ./cmd/server

# ── Build ─────────────────────────────────────────────────────────────────────

build-backend:
	mkdir -p build
	cd backend && CGO_ENABLED=0 go build $(LDFLAGS) -o ../build/passbubble-server ./cmd/server

build-cli:
	mkdir -p build
	cd cli && go build $(LDFLAGS) -o ../build/pwmgr ./cmd/pwmgr

build-all: build-backend build-cli

# ── Test ──────────────────────────────────────────────────────────────────────

test: test-backend test-cli
	@echo "All Go tests passed"

test-backend:
	cd backend && go test ./... -race -count=1

test-cli:
	cd cli && go test ./... -race -count=1

test-flutter:
	cd flutter_app && flutter test --coverage

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

build-extension:
	cd packages/shared-ts && npm run build
	cd extension && npm run build:chrome && npm run build:firefox

test-extension:
	cd packages/shared-ts && npm ci && npm test
	cd extension && npm ci && npm test

# ── SVG Icons ─────────────────────────────────────────────────────────────────

icons: ## Rasterize SVG → extension PNG icons (requires rsvg-convert or inkscape)
	rsvg-convert -w 16  -h 16  flutter_app/assets/svg/icon.svg -o extension/icons/icon16.png
	rsvg-convert -w 48  -h 48  flutter_app/assets/svg/icon.svg -o extension/icons/icon48.png
	rsvg-convert -w 128 -h 128 flutter_app/assets/svg/icon.svg -o extension/icons/icon128.png

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf build/
	rm -rf flutter_app/build/
	rm -rf backend/embed/web/*
