# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.13] - 2026-06-18

### Added
- Email verification on registration: set `SMTP_HOST` (+ optional `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `APP_BASE_URL`) to require users to click a one-time link before their account is activated. Omitting `SMTP_HOST` preserves the previous auto-activate behaviour.
- New endpoint `GET /api/v1/auth/verify-email?token=…` — validates token, activates account, returns a browser-friendly HTML confirmation page.
- Login now returns HTTP 403 with `"email not verified — check your inbox"` when the account is still in `pending` state.
- DB migration `000002_email_verification` adds the `email_verification_tokens` table.
- [Mailpit](https://github.com/axllent/mailpit) added to the dev Docker Compose stack (`make up`) — pre-wired as SMTP backend so email verification works out of the box. Web UI at `http://localhost:8025`.

## [2.0.12] - 2026-06-18

### Fixed
- Flutter: `deriveMasterKey` tests timed out on CI (30s limit) — pass minimal Argon2id params (`memory=1024, iterations=1`) in unit tests; production hardness unchanged

## [2.0.11] - 2026-06-18

### Fixed
- CLI: `GetTimeRemaining` off-by-one — returned `period` (30) instead of 0 when `now % period == 0`, causing `TestGetTimeRemaining` to fail

### Changed
- `CLAUDE.md`: add mandatory pre-commit/pre-tag checklist (backend + CLI `-race` + flutter)

## [2.0.10] - 2026-06-18

### Added
- `deploy.sh` — fully automated server deployment script: pulls compose file, generates secrets, starts Docker stack, configures nginx vhost, obtains Let's Encrypt cert via certbot, downloads CLI binary
- `nginx/passbubble.conf` — nginx vhost template (TLS, HSTS, proxy_pass, SSE support)
- `docker-compose.server.caddy.yml` — preserved Caddy variant for fresh servers without existing reverse proxy

### Changed
- `docker-compose.server.yml`: remove Caddy service, bind backend to `127.0.0.1:8765` only (nginx handles TLS on the host)

### Fixed
- `deploy.sh`: use `maybe_sudo` helper — works as both root and sudoer
- `deploy.sh`: gracefully skip CLI download if GitHub release binary not yet available
- `deploy.sh`: read `DOMAIN`/`ADMIN_EMAIL` from existing `.env` on re-runs (no re-prompt)

## [2.0.9] - 2026-06-18

### Fixed
- CI: remove Flutter Windows build — `local_auth_windows` incompatible with MSVC 14.51 (VS 18)
- CI: fix DockerHub image name `gerry3010/passbubble-server` → `gerre01/passbubble-server` in all workflows and docs

## [2.0.8] - 2026-06-18

### Fixed
- Flutter: upgrade `local_auth` 2.3 → 3.0 to fix Windows MSVC 14.51 C++ experimental-coroutine build error in `local_auth_windows`
- Flutter: migrate `AuthenticationOptions(biometricOnly: true)` → `biometricOnly: true` (new `local_auth` 3.0 API)

## [2.0.7] - 2026-06-18

### Performance
- Docker: use `--platform=$BUILDPLATFORM` + `GOARCH=$TARGETARCH` for Go and Flutter builder stages — replaces QEMU emulation with native cross-compilation, cutting arm64 build time from ~10 min to ~1 min

## [2.0.6] - 2026-06-18

### Fixed
- CI: fix Docker build context (`context: .` + `file: backend/Dockerfile`) so `flutter_app/` is available during the image build

## [2.0.5] - 2026-06-18

### Fixed
- CLI: resolve 17 golangci-lint issues (errcheck: defer Close/Remove/ReadFull, staticcheck: De Morgan's law)
- Flutter: fix deprecated API usage in export/import tabs (`value`→`initialValue`, `activeColor`→`activeThumbColor`, `withOpacity`→`withValues`)

## [2.0.4] - 2026-06-18

### Fixed
- CI: upgrade `golangci-lint-action` v6 → v7 (v6 rejects golangci-lint v2.x)
- CI: Go version 1.24 → 1.26 to match `go.mod` (backend + CLI)
- Flutter: add missing `PbBottomNav` widget (`shared/widgets/bottom_nav.dart`)
- Flutter: add missing `job_polling_service.dart` (`runningJobsProvider`)
- Flutter: add `JobResponse`, `MySharesResponse`, `ShareLinkResponse`, `DirectShareResponse` to `models.dart`
- Flutter: add `listJobs`, `listMyShares`, `revokeShareLink`, `revokeEntryShare`, `revokeFolderShare` to `ApiClient`

## [2.0.3] - 2026-06-18

### Fixed
- CI: pin Flutter to `3.44.0` (Dart 3.10) — wildcard `3.44.x` fell back to cached `3.32.8`
- CI: restore `sdk: ^3.10.0` Dart constraint in `pubspec.yaml` (correct for Flutter 3.44.0)

## [2.0.2] - 2026-06-18

### Fixed
- CI: update GitHub Actions to Node.js 24 compatible versions (checkout v6, docker/* actions v4/v4/v4/v6/v7)
- CI: add `libsecret-1-dev` to Linux Flutter build dependencies (required by `flutter_secure_storage_linux`)
- CI: remove unused `assets:` declaration from `pubspec.yaml` (caused build warning on Windows and Docker)

## [2.0.1] - 2026-06-18

### Fixed
- CI: upgrade Flutter from 3.32.x to 3.44.x to satisfy the `sdk: ^3.10.4` Dart constraint declared in `pubspec.yaml`

### Added
- `docker-compose.server.yml` — standalone production compose file using the DockerHub image (no source or Go needed on the server)
- `docs/server-deployment.md` — full server setup, update, and pitfall guide

## [2.0.0] - 2026-06-16

### Changed
- **Breaking: full architecture rewrite to a self-hosted client/server monorepo.** The single-binary, system-keyring-backed CLI is replaced by:
  - `backend/` — Go REST API server (PostgreSQL + Redis), with end-to-end encryption (X25519 + ML-KEM-768 hybrid KEM, AES-256-GCM, Argon2id) so the server never sees plaintext
  - `cli/` — Go CLI/TUI (`pwmgr`) acting as an API client, with all crypto performed client-side
  - `flutter_app/` — Flutter app serving the web UI (`/web/*`) and admin panel (`/admin/*`), embedded into the backend binary at build time
- Added multi-user support: invitations, sharing, folders, admin roles
- Added Docker Compose deployment (`./setup.sh`) with first-run bootstrap admin registration
- Removed the old GNOME-Keyring/system-keychain storage backend, the standalone single-binary CLI source (`cmd/`, `internal/`, `pkg/`), and associated build artifacts (`build/`, `dist/`) — superseded by the new monorepo layout

## [1.0.0] - 2025-10-09

### Added
- Initial release of the Password Manager Go Edition
- Complete rewrite from Bash to Go with modern architecture
- Interactive Bubble Tea TUI with beautiful interface
- Secure password storage using system keyring (GNOME Keyring, Keychain, Credential Manager)
- TOTP 2FA support with live code generation and visual countdown
- Advanced password generation with multiple types (strong, memorable, passphrase)
- Encrypted backup and restore functionality with GPG support
- Cross-platform compatibility (Linux, macOS, Windows)
- Comprehensive CLI interface with all password management operations
- Search and organization capabilities
- Real-time TOTP code refresh with visual progress indicators

### Fixed
- **Critical**: TOTP progress bar auto-refresh functionality
  - Fixed timer continuation issue where progress bar would stop updating
  - Progress bar now properly counts down from 30 to 0 automatically
  - TOTP codes refresh seamlessly when countdown expires
  - Implemented proper Bubble Tea command batching for smooth UI updates

### Technical Details
- Built with Go 1.21+ and modern dependencies
- Uses Cobra for CLI framework and Bubble Tea for TUI
- Secure keyring integration via zalando/go-keyring
- TOTP implementation using pquerna/otp library
- Comprehensive test coverage for all core functionality
- Cross-platform build system with release automation

### Migration from Bash Version
- All Bash functionality preserved and enhanced
- Improved performance and reliability
- Better error handling and user experience
- Backward compatible with existing password entries
- Enhanced security with proper secret handling

### Documentation
- Comprehensive README with installation and usage instructions
- Inline help and examples for all commands
- Architecture documentation and development guide
- Security best practices and recommendations

### Dependencies
- github.com/charmbracelet/bubbletea (TUI framework)
- github.com/charmbracelet/lipgloss (TUI styling)
- github.com/spf13/cobra (CLI framework)
- github.com/zalando/go-keyring (secure storage)
- github.com/pquerna/otp (TOTP implementation)
- github.com/spf13/viper (configuration management)