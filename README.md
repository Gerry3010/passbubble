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

## Quickstart (server deployment)

Requires `docker` ≥ 24 with the `docker compose` plugin and `openssl`. No source code needed — pulls the pre-built image from DockerHub.

```bash
mkdir -p /opt/passbubble && cd /opt/passbubble

# Download compose file + Caddyfile
BASE=https://raw.githubusercontent.com/Gerry3010/passbubble/main
curl -fsSL $BASE/docker-compose.server.yml -o docker-compose.yml
curl -fsSL $BASE/Caddyfile -o Caddyfile

# Generate secrets
cat > .env <<EOF
POSTGRES_PASSWORD=$(openssl rand -base64 32 | tr -d '/+=\n')
REDIS_PASSWORD=$(openssl rand -base64 32 | tr -d '/+=\n')
JWT_SECRET=$(openssl rand -base64 64 | tr -d '/+=\n')
ADMIN_EMAIL=you@example.com
DOMAIN=pwmgr.example.com
CADDY_EMAIL=you@example.com
ENVIRONMENT=production
EOF

# Start (with TLS via Caddy)
docker compose --profile production up -d
```

Then install the CLI and register the first admin account:

```bash
# Download the CLI binary for your platform from the latest GitHub Release, e.g.:
curl -fsSL https://github.com/Gerry3010/passbubble/releases/latest/download/pwmgr-linux-amd64 -o pwmgr
chmod +x pwmgr

./pwmgr setup --server https://pwmgr.example.com
```

**The first account on a fresh server is automatically promoted to admin** — no invitation token needed.

```bash
./pwmgr add "My Service"    # add an entry
./pwmgr tui                 # interactive terminal UI
```

Web UI: `https://pwmgr.example.com/web/`
Admin panel: `https://pwmgr.example.com/admin/`

> For the full setup guide including updates, backups, and pitfalls see [`docs/server-deployment.md`](docs/server-deployment.md).

## Updating

```bash
# Quick update (image only):
cd /opt/passbubble && docker compose pull && docker compose up -d

# Full update (image + compose file + nginx config):
curl -fsSL https://raw.githubusercontent.com/Gerry3010/passbubble/main/deploy.sh | bash
```

`deploy.sh` is idempotent — it skips secret generation if `.env` already exists and reads `DOMAIN`/`ADMIN_EMAIL` from it.

## Docker Compose services

`docker-compose.server.yml` is the **nginx variant** — the backend binds to `127.0.0.1:8765` only and nginx (on the host) handles TLS. Use this if you already have nginx running on the server.

`docker-compose.server.caddy.yml` is the **Caddy variant** — Caddy runs as a container and manages TLS automatically. Use this on a fresh server with no existing reverse proxy (`--profile production`).

| Service | Purpose |
|---|---|
| `postgres` | Primary datastore (users, entries, folders, sessions, invitations) |
| `redis` | Session/rate-limit cache |
| `backend` | API server; also serves the embedded Flutter web build at `/web/*` and `/admin/*` |
| `caddy` | TLS reverse proxy — Caddy variant only, started with `--profile production` |

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

## Local development

Requires `docker` + `docker compose`, `go` 1.26+, `flutter` 3.44+, and `openssl`.

```bash
./setup.sh
```

This will:
1. Generate a `.env` with random secrets (skipped if one already exists).
2. Build the `pwmgr` CLI into `./bin/pwmgr`.
3. Run `docker compose up -d --build` — builds the Flutter web app and embeds it into the Go binary on first run (takes a few minutes).
4. Launch `pwmgr setup` to register the bootstrap admin account.

```bash
./bin/pwmgr tui                 # interactive TUI
```

Web UI: `http://localhost:8765/web/`  
Admin panel: `http://localhost:8765/admin/`

**Manual alternative** (without `setup.sh`):

```bash
cp .env.example .env   # edit secrets
docker compose up -d --build
cd cli && go build -o ../bin/pwmgr ./cmd/pwmgr && cd ..
./bin/pwmgr setup --server http://localhost:8765
```

**Running tests:**

```bash
cd backend && go build ./... && go test ./...
cd cli && go build ./... && go test ./...
cd flutter_app && flutter analyze && flutter test
```

The Docker build for `backend` builds the Flutter web app in a separate stage and copies the output into `backend/internal/static/web` before compiling the Go binary via `go:embed` — so `docker compose up --build` always ships an up-to-date web UI without manual steps.

## License

AGPL-3.0 — see [LICENSE](LICENSE).
