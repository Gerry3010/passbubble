# Passbubble

> 🤖 **AI Development Notice**: This project was developed collaboratively between Gerry and Claude (Anthropic's AI assistant). The codebase, fixes, and documentation were created through AI assistance to ensure robust functionality and proper security practices. We believe in transparency about AI involvement in open source projects.

Passbubble is a self-hosted, end-to-end encrypted password manager. You run the server (one `docker compose up`), and access your vault from a Go CLI/TUI, a web app, or an admin panel — all backed by the same encrypted store. The server never sees plaintext: every entry is encrypted on the client before it's sent.

## Architecture

```
[Flutter Web App (/web/*)]   [Admin Panel (/admin/*)]   [Go CLI / TUI (pwmgr)]
                  \                  |                    /
                   \                 |                   /
                    \                v                  /
                     ──────►  Go REST API Server  ◄──────
                                     |
                       ┌─────────────┼─────────────┐
                       ▼             ▼             ▼
                  [PostgreSQL]   [Redis]    [Volume: Backups]
```

- `backend/` — Go REST API server (chi router, PostgreSQL, Redis). Also embeds and serves the built Flutter web app at `/web/*` (all users) and `/admin/*` (JWT-gated, admin role only — same app, route-guarded).
- `cli/` — Go CLI (`pwmgr`) and Bubble Tea TUI. Talks to the server over HTTPS; all crypto happens client-side.
- `flutter_app/` — Flutter app for web, iOS, Android, Linux, macOS, Windows.

## End-to-end encryption

- **Key exchange**: hybrid X25519 (classical) + ML-KEM-768 (post-quantum) KEM.
- **Symmetric encryption**: AES-256-GCM for entry payloads and for encrypting your private keys at rest.
- **Key derivation**: Argon2id derives a master key from your master password — the password itself is never sent to the server.
- The server only ever stores/serves encrypted blobs (`encrypted_data`) plus per-recipient wrapped data keys (`entry_keys`), which is what makes sharing entries between users possible without the server being able to read them.

## Quickstart

Requires `docker` + the `docker compose` plugin, `go` 1.22+, and `openssl`.

```bash
./setup.sh
```

This will:
1. Check prerequisites.
2. Generate a `.env` with random `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `JWT_SECRET` (skipped if `.env` already exists).
3. Build the `pwmgr` CLI into `./bin/pwmgr`.
4. `docker compose up -d --build` (this also builds the Flutter web app and embeds it into the server binary — first build takes a few minutes).
5. Wait for the backend's `/health` endpoint.
6. Launch `pwmgr setup`, an interactive wizard that generates your keypairs and registers the first account. **The first account on a fresh server is automatically promoted to admin** — no invitation token needed.

After setup:

```bash
./bin/pwmgr add "My Service"     # add an entry
./bin/pwmgr tui                  # interactive terminal UI
./bin/pwmgr register --server …  # invite additional users (requires an invitation)
```

Web UI: `http://localhost:8765/web/`
Admin panel: `http://localhost:8765/admin/` (sign in with the admin account created during setup)

## Manual setup

If you don't want to run `setup.sh`:

```bash
cp .env.example .env        # then edit secrets
docker compose up -d --build
cd cli && go build -o ../bin/pwmgr ./cmd/pwmgr && cd ..
./bin/pwmgr setup --server http://localhost:8765
```

## Docker Compose services

| Service | Purpose |
|---|---|
| `postgres` | Primary datastore (users, entries, folders, sessions, invitations) |
| `redis` | Session/rate-limit cache |
| `backend` | API server; also serves the embedded Flutter web build at `/web/*` and `/admin/*` |
| `caddy` | TLS reverse proxy — only started with `--profile production` |

Production deployment with TLS:

```bash
DOMAIN=pwmgr.example.com CADDY_EMAIL=you@example.com \
  docker compose --profile production up -d --build
```

## CLI reference (`pwmgr`)

| Command | Description |
|---|---|
| `pwmgr setup` | First-run wizard: generate keys, register the bootstrap admin account |
| `pwmgr login` | Authenticate against a server |
| `pwmgr logout` / `pwmgr whoami` | Session management |
| `pwmgr register` | Register a new account using an invitation token |
| `pwmgr add <name> [username]` | Add a password entry |
| `pwmgr get <name>` | Retrieve and decrypt an entry |
| `pwmgr update <name>` / `pwmgr delete <name>` | Edit/remove an entry |
| `pwmgr list` / `pwmgr search <pattern>` | List or search entries (no master password required — metadata only) |
| `pwmgr totp-add <name> [username]` / `pwmgr totp-code <name>` / `pwmgr totp-list` / `pwmgr totp-delete <name>` | Manage TOTP/2FA entries |
| `pwmgr generate [length]` | Generate a secure password |
| `pwmgr backup` / `pwmgr restore <backup-file>` / `pwmgr list-backups` | Encrypted backup/restore |
| `pwmgr tui` | Interactive terminal UI (also the default with no subcommand) |
| `pwmgr version` | Build/version info |

Commands that decrypt entry data (`get`, `tui`, `add`, etc.) prompt for your master password to unlock the vault in memory for the session; metadata-only commands (`list`, `search`) don't need it.

## Development

```bash
# Backend
cd backend && go build ./... && go test ./...

# CLI
cd cli && go build ./... && go test ./...

# Flutter app
cd flutter_app && flutter pub get && flutter analyze && flutter test
flutter build web --release   # only needed for manual (non-Docker) embedding
```

The Docker build for `backend` builds the Flutter web app in a separate stage (`ghcr.io/cirruslabs/flutter:stable`) and copies the output into `backend/internal/static/web` before compiling the Go binary via `go:embed` — so a plain `docker compose up --build` always ships an up-to-date web/admin UI without any manual steps.

## License

AGPL-3.0 — see [LICENSE](LICENSE).
