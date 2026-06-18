# Server Deployment Guide

Self-hosted Passbubble on a VPS or dedicated server using the published DockerHub image — no source code needed.

## Prerequisites

- Docker ≥ 24 with the `docker compose` plugin
- `openssl` (pre-installed on most Linux distros)
- A domain with an A/AAAA record pointing to your server IP
- Ports **80** and **443** open in your firewall (needed for Caddy's ACME TLS challenge)

---

## First-time setup

### 1. Create a deployment directory

```bash
mkdir -p /opt/passbubble && cd /opt/passbubble
```

### 2. Download the required files

```bash
BASE=https://raw.githubusercontent.com/Gerry3010/passbubble/main
curl -fsSL $BASE/docker-compose.server.yml -o docker-compose.yml
curl -fsSL $BASE/Caddyfile -o Caddyfile
```

### 3. Generate secrets and create `.env`

```bash
PG_PASS=$(openssl rand -base64 32 | tr -d '/+=\n')
RD_PASS=$(openssl rand -base64 32 | tr -d '/+=\n')
JWT=$(openssl rand -base64 64 | tr -d '/+=\n')

cat > .env <<EOF
POSTGRES_PASSWORD=${PG_PASS}
REDIS_PASSWORD=${RD_PASS}
JWT_SECRET=${JWT}

SERVER_PORT=8765
ENVIRONMENT=production

# Set this BEFORE first start — cannot be changed after bootstrap
ADMIN_EMAIL=you@example.com

# Caddy TLS
DOMAIN=pwmgr.example.com
CADDY_EMAIL=you@example.com
EOF
```

Then **edit `.env`** and fill in `ADMIN_EMAIL`, `DOMAIN`, and `CADDY_EMAIL`.

> **Back up `.env` somewhere safe** (password manager, secret store). See [Pitfalls](#pitfalls).

### 4. Start the stack

```bash
docker compose --profile production up -d
```

Caddy fetches a Let's Encrypt certificate automatically on first start. Check `docker compose logs caddy -f` if TLS doesn't come up within ~30 seconds.

### 5. Register your admin account

Download the CLI from [GitHub Releases](https://github.com/Gerry3010/passbubble/releases/latest):

```bash
# Linux amd64 — check the releases page for other platforms (arm64, macOS, Windows)
curl -fsSL https://github.com/Gerry3010/passbubble/releases/latest/download/pwmgr-linux-amd64 -o pwmgr
chmod +x pwmgr
./pwmgr setup --server https://pwmgr.example.com
```

The interactive wizard generates your keypairs and registers the first account. **The first account on a fresh server is automatically promoted to admin** — no invitation token needed.

After setup:
- Web UI: `https://pwmgr.example.com/web/`
- Admin panel: `https://pwmgr.example.com/admin/`

---

## Updating

```bash
cd /opt/passbubble
docker compose --profile production pull
docker compose --profile production up -d
```

Database migrations run automatically when the backend starts. The update takes ~10 seconds. Your data (Postgres, Redis, backups) lives in named volumes and is untouched by the container replacement.

Update the CLI binary separately from GitHub Releases when a new version ships.

---

## Backup & restore

```bash
# Trigger a backup (creates an encrypted archive in the backups volume)
./pwmgr backup

# List existing backups
./pwmgr list-backups

# Restore from a backup
./pwmgr restore <backup-file>
```

### Copy backups off the server

```bash
docker compose cp backend:/data/backups/. ./backups-export/
```

### Full disaster recovery

If you need to restore from scratch on a new server:

1. Follow steps 1–4 above (same domain, same `.env`).
2. Wait for the backend to be healthy.
3. Copy a backup file into the container:
   ```bash
   docker compose cp my-backup.enc backend:/data/backups/
   ```
4. Run `./pwmgr restore my-backup.enc --server https://pwmgr.example.com`.

---

## Pitfalls

### Set `ADMIN_EMAIL` before first start
The first registered user is promoted to admin only if their email matches `ADMIN_EMAIL` at registration time. If the variable is empty or wrong when the first user registers, that account won't be admin. Fix: set it correctly before starting, then register immediately.

### Never use `docker compose down -v`
The `-v` flag deletes all named volumes — that's your entire database (`postgres_data`), Redis cache, and backups. A plain `docker compose down` (no `-v`) is safe; volumes survive. When in doubt, omit `-v`.

| Command | Containers | Volumes |
|---------|-----------|---------|
| `docker compose stop` | stopped | **kept** |
| `docker compose down` | removed | **kept** |
| `docker compose down -v` | removed | **DELETED** |
| `docker rm <name>` | removed | **kept** |

### Back up your `.env`
- **`JWT_SECRET` lost** → all active sessions are invalidated (everyone gets logged out). No data loss, but everyone has to re-login.
- **`POSTGRES_PASSWORD` lost** → backend can't connect to the database. You'd need to reset the PG password directly inside the `postgres` container.
- Store `.env` in your password manager or a secrets store — not in the repo.

### Rotating `JWT_SECRET`
If you need to rotate it (leaked `.env`, security incident): update the value in `.env` and restart. All existing sessions are invalidated — users must log in again. Vault data is unaffected.

### Ports 80 and 443 must be reachable for TLS
Caddy uses the ACME HTTP-01 challenge to issue Let's Encrypt certificates. If port 80 is blocked by a firewall or another service occupies it, Caddy fails to start. Check `docker compose logs caddy`.

### Don't expose the backend port publicly
`docker-compose.server.yml` does **not** publish the backend's port to the host — only Caddy exposes 80/443. Don't add a `ports:` entry to `backend`; doing so lets requests bypass TLS and Caddy's security headers.

### First-run bootstrap window
Until the first account is registered, anyone who can reach your server can claim the admin role. Set `ADMIN_EMAIL` and register your admin account immediately after first start — don't leave a fresh server unregistered.

### Migration failures
Migrations run at startup and are idempotent. If a migration fails (disk full, transient DB issue), the container exits. Check `docker compose logs backend` and fix the root cause, then `docker compose up -d` again — safe to retry.
