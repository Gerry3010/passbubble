# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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