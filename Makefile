# Passbubble Monorepo Makefile
# Usage: make <target>

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   = -ldflags="-s -w"

.PHONY: help up down up-prod dev \
        build-backend build-cli build-all \
        test test-backend test-cli test-flutter test-all \
        lint migrate migrate-down migrate-create \
        clean

help:
	@echo "Passbubble targets:"
	@echo "  up             Start dev Docker stack (postgres + redis + backend)"
	@echo "  down           Stop Docker stack"
	@echo "  up-prod        Start production stack (with Caddy)"
	@echo "  dev            Start backend in watch mode (air required)"
	@echo ""
	@echo "  build-backend  Build backend binary"
	@echo "  build-cli      Build CLI binary"
	@echo "  build-all      Build both"
	@echo ""
	@echo "  test           Run backend + CLI tests"
	@echo "  test-backend   Run backend tests"
	@echo "  test-cli       Run CLI tests"
	@echo "  test-flutter   Run Flutter tests"
	@echo "  test-all       Run all tests"
	@echo ""
	@echo "  lint           Run golangci-lint on backend + CLI"
	@echo "  migrate        Run pending DB migrations"
	@echo "  migrate-down   Rollback last migration"
	@echo "  clean          Remove build artifacts"

# ── Docker ────────────────────────────────────────────────────────────────────

up:
	docker compose up -d

down:
	docker compose down

up-prod:
	docker compose --profile production up -d

dev:
	cd backend && air -c .air.toml

# ── Build ─────────────────────────────────────────────────────────────────────

build-backend:
	cd backend && CGO_ENABLED=0 go build $(LDFLAGS) -o ../bin/passbubble-server ./cmd/server

build-cli:
	cd cli && go build $(LDFLAGS) -o ../bin/pwmgr ./cmd/pwmgr

build-all: build-backend build-cli

# ── Test ──────────────────────────────────────────────────────────────────────

test: test-backend test-cli

test-backend:
	cd backend && go test ./... -race -count=1

test-cli:
	cd cli && go test ./... -race -count=1

test-flutter:
	cd flutter_app && flutter test --coverage

test-all: test-backend test-cli test-flutter

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	cd backend && golangci-lint run ./...
	cd cli && golangci-lint run ./...
	@if [ -d flutter_app ]; then cd flutter_app && flutter analyze; fi

# ── Database ──────────────────────────────────────────────────────────────────

migrate:
	docker compose exec backend /bin/sh -c "echo 'migrations run automatically on startup'"

migrate-down:
	@echo "Run: docker compose exec postgres psql -U pwmgr -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'"

migrate-create:
	@read -p "Migration name: " name; \
	  n=$$(ls backend/internal/db/migrations/*.sql 2>/dev/null | wc -l | xargs); \
	  n=$$(printf "%06d" $$((n/2+1))); \
	  touch backend/internal/db/migrations/$${n}_$${name}.up.sql; \
	  touch backend/internal/db/migrations/$${n}_$${name}.down.sql; \
	  echo "Created migration $${n}_$${name}"

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd backend && go clean -testcache
	cd cli && go clean -testcache
