# Passbubble — Claude Code Guidelines

## Repository Layout

This is a Go + Flutter + TypeScript monorepo:

```
backend/        Go REST API server (own go.mod)
cli/            Go TUI/CLI client (own go.mod, depends on backend/ via replace)
flutter_app/    Flutter mobile/web app
packages/shared-ts/   TypeScript E2E crypto library (used by browser extension)
extension/      Chrome + Firefox MV3 browser extension
```

Run tests from the right module directory:
```bash
cd backend && go test ./...
cd cli && go test ./...
```

## License Headers

**Every source file must start with the AGPL v3 header.** Use this exact block:

```go
// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
```

Applies to: `.go`, `.ts`, `.tsx` files. The `//` comment style works for both Go and TypeScript.

## Workflow: No Worktrees

**Do not use git worktrees for feature work.** Merging multiple worktrees back into main requires manual conflict resolution for every shared file (Makefile, docker-compose.yml, go.mod, etc.) and is error-prone.

Instead:
- Work directly on `main` or short-lived feature branches
- Create a normal branch: `git checkout -b feature/my-feature`
- Merge via PR or `git merge` when done

## Make Targets

```bash
make up              # Start dev stack (postgres + redis + backend via Docker)
make dev             # Run backend locally (go run)
make test            # backend + CLI tests
make build-all       # Build backend + CLI binaries → build/
make lint            # golangci-lint + flutter analyze
make migrate         # Run DB migrations (needs DB_URL or .env)
make sqlc            # Regenerate sqlc type-safe query code
make build-extension # Build Chrome + Firefox extension
make test-extension  # Run extension + shared-ts tests
```

## Key Conventions

- Backend and CLI are separate Go modules — the CLI references backend via a `replace` directive in `cli/go.mod`
- DB migrations live in `backend/internal/db/migrations/` (golang-migrate format)
- The backend binary embeds the Flutter web app (`backend/internal/static/web/`) and admin panel at build time via `//go:embed`
- Browser extension CSP boundary: all crypto runs in the background service worker, never in content scripts
