#!/usr/bin/env bash
# Passbubble server deployment script
# Run on the target server as a user with sudo + docker access.
# Usage: bash deploy.sh

set -euo pipefail

DOMAIN="${DOMAIN:-}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"
INSTALL_DIR="${INSTALL_DIR:-/opt/passbubble}"
NGINX_AVAILABLE="/etc/nginx/sites-available/passbubble"
NGINX_ENABLED="/etc/nginx/sites-enabled/passbubble"

RAW="https://raw.githubusercontent.com/Gerry3010/passbubble/main"

# ── helpers ───────────────────────────────────────────────────────────────────
info()  { echo -e "\033[1;32m[passbubble]\033[0m $*"; }
error() { echo -e "\033[1;31m[error]\033[0m $*" >&2; exit 1; }

require() { command -v "$1" &>/dev/null || error "'$1' not found — install it first."; }

ask() {
  local var="$1" prompt="$2" default="${3:-}"
  local val
  if [[ -n "${!var:-}" ]]; then
    info "$prompt: ${!var} (from env)"
    return
  fi
  read -rp "  $prompt${default:+ [$default]}: " val
  val="${val:-$default}"
  [[ -n "$val" ]] || error "$prompt is required."
  printf -v "$var" '%s' "$val"
}

# ── preflight ─────────────────────────────────────────────────────────────────
require docker
require openssl
require curl
docker compose version &>/dev/null || error "docker compose plugin not found."

info "=== Passbubble Deployment ==="
echo ""

# ── install dir ───────────────────────────────────────────────────────────────
info "Creating install directory $INSTALL_DIR …"
sudo mkdir -p "$INSTALL_DIR"
sudo chown "$USER":"$USER" "$INSTALL_DIR"
cd "$INSTALL_DIR"

# Read DOMAIN/ADMIN_EMAIL from existing .env before prompting
if [[ -f .env ]]; then
  [[ -z "$DOMAIN" ]]      && DOMAIN=$(grep -E '^DOMAIN='      .env | cut -d= -f2-)
  [[ -z "$ADMIN_EMAIL" ]] && ADMIN_EMAIL=$(grep -E '^ADMIN_EMAIL=' .env | cut -d= -f2-)
fi

ask DOMAIN      "Domain (e.g. pwmgr.example.com)"
ask ADMIN_EMAIL "Admin e-mail"

# ── compose file ──────────────────────────────────────────────────────────────
info "Downloading docker-compose.server.yml …"
curl -fsSL "$RAW/docker-compose.server.yml" -o docker-compose.yml

# ── .env ──────────────────────────────────────────────────────────────────────
if [[ -f .env ]]; then
  info ".env already exists — skipping secret generation."
else
  info "Generating secrets …"
  cat > .env <<EOF
POSTGRES_PASSWORD=$(openssl rand -base64 32 | tr -d '/+=\n')
REDIS_PASSWORD=$(openssl rand -base64 32 | tr -d '/+=\n')
JWT_SECRET=$(openssl rand -base64 64 | tr -d '/+=\n')
ADMIN_EMAIL=${ADMIN_EMAIL}
DOMAIN=${DOMAIN}
SERVER_PORT=8765
ENVIRONMENT=production
EOF
  chmod 600 .env
  info ".env written with random secrets."
fi

# ── start stack ───────────────────────────────────────────────────────────────
info "Pulling images and starting stack …"
docker compose pull
docker compose up -d

info "Waiting for backend to become healthy (up to 90s) …"
HEALTHY=0
for i in $(seq 1 45); do
  if curl -sf "http://127.0.0.1:${SERVER_PORT:-8765}/health" &>/dev/null; then
    HEALTHY=1
    echo ""
    break
  fi
  printf "."
  sleep 2
done
if [[ $HEALTHY -eq 0 ]]; then
  echo ""
  docker compose logs --tail=30 backend || true
  error "Backend did not start in time. Logs above. Check: docker compose logs backend"
fi

# ── nginx ─────────────────────────────────────────────────────────────────────
if command -v nginx &>/dev/null; then
  info "Installing nginx vhost for $DOMAIN …"

  sudo tee "$NGINX_AVAILABLE" > /dev/null <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ${DOMAIN};

    ssl_certificate     /etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    include             /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam         /etc/letsencrypt/ssl-dhparams.pem;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    client_max_body_size 50M;

    location / {
        proxy_pass         http://127.0.0.1:8765;
        proxy_http_version 1.1;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_set_header   Connection        '';
        proxy_buffering    off;
        proxy_read_timeout 120s;
    }
}
NGINX

  [[ -L "$NGINX_ENABLED" ]] || sudo ln -s "$NGINX_AVAILABLE" "$NGINX_ENABLED"

  # ── TLS via certbot ─────────────────────────────────────────────────────────
  if command -v certbot &>/dev/null; then
    info "Running certbot for $DOMAIN (email: $ADMIN_EMAIL) …"
    sudo certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$ADMIN_EMAIL"
    sudo nginx -t && sudo systemctl reload nginx
    info "TLS configured."
  else
    info "certbot not found — installing …"
    if command -v apt-get &>/dev/null; then
      sudo apt-get install -y certbot python3-certbot-nginx
    elif command -v dnf &>/dev/null; then
      sudo dnf install -y certbot python3-certbot-nginx
    else
      error "Cannot install certbot automatically. Install it manually and run: sudo certbot --nginx -d $DOMAIN"
    fi
    sudo certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$ADMIN_EMAIL"
    sudo nginx -t && sudo systemctl reload nginx
    info "TLS configured."
  fi
else
  info "nginx not detected — skipping vhost setup."
  info "Use nginx/passbubble.conf from the repo as a template."
fi

# ── CLI download ──────────────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  CLI_BIN="pwmgr-linux-amd64" ;;
  aarch64) CLI_BIN="pwmgr-linux-arm64" ;;
  *)        CLI_BIN="" ;;
esac

if [[ -n "$CLI_BIN" ]]; then
  info "Downloading CLI ($CLI_BIN) …"
  curl -fsSL "https://github.com/Gerry3010/passbubble/releases/latest/download/$CLI_BIN" \
    -o "$INSTALL_DIR/pwmgr"
  chmod +x "$INSTALL_DIR/pwmgr"
  info "CLI saved to $INSTALL_DIR/pwmgr"
fi

# ── done ──────────────────────────────────────────────────────────────────────
echo ""
info "=== Done! ==="
echo ""
echo "  Stack:     docker compose -f $INSTALL_DIR/docker-compose.yml ps"
echo "  Logs:      docker compose -f $INSTALL_DIR/docker-compose.yml logs -f"
echo "  Web UI:    https://${DOMAIN}/web/"
echo "  Admin:     https://${DOMAIN}/admin/"
echo ""
echo "  Register the first admin account:"
echo "    $INSTALL_DIR/pwmgr setup --server https://${DOMAIN}"
echo ""
