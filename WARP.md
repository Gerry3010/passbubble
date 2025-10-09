# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

This is a command-line password manager built for Linux systems using GNOME Keyring as the secure backend storage. The project is implemented in Bash and consists of multiple modular scripts for password management, generation, and backup/restore functionality.

**Key Branch Information:**
- `main`: Clean initial version
- `bash`: Current working implementation with fixes (recommended for development)

## Development Commands

### Testing
```bash
# Make test scripts executable (first time setup)
chmod +x tests/*.sh

# Run basic functionality tests
./tests/test-basic.sh

# Individual test categories (if they exist)
./tests/test-generation.sh
./tests/test-backup.sh
```

### Core Operations
```bash
# Initialize the password manager (creates config)
./bin/pwmgr init

# Test password operations
./bin/pwmgr add test-service
./bin/pwmgr get test-service
./bin/pwmgr list
./bin/pwmgr delete test-service

# Test password generation
./bin/pwmgr generate 16
./utils/pwgen-advanced -l 20 -c 5

# Test backup functionality
./utils/pwmgr-backup backup
./utils/pwmgr-backup list
```

### Setup & Dependencies

**Required System Dependencies:**
```bash
# Manjaro/Arch Linux
sudo pacman -S gnome-keyring libsecret

# Ubuntu/Debian  
sudo apt install gnome-keyring libsecret-tools

# Fedora/RHEL
sudo dnf install gnome-keyring libsecret
```

**Verify Setup:**
```bash
# Check keyring daemon
systemctl --user status gnome-keyring-daemon

# Test secret-tool
secret-tool --version
```

## Architecture Overview

### Core Components

**Main Script (`bin/pwmgr`):**
- Primary CLI interface for password operations (add, get, update, delete, list, search)
- Uses GNOME Keyring via `secret-tool` for secure storage
- Implements password generation and configuration management
- Contains a Python-based listing mechanism using D-Bus API (`busctl`) to enumerate keyring entries

**Advanced Password Generator (`utils/pwgen-advanced`):**
- Supports multiple password types: strong, alphanum, numbers, memorable, passphrase  
- Configurable length, character sets, and complexity requirements
- Built-in password strength checking
- Excludes ambiguous characters option

**Backup/Restore Utility (`utils/pwmgr-backup`):**
- Exports passwords to JSON format with metadata
- Supports GPG encryption and password-based encryption
- Automatic backup cleanup (keeps configurable number of backups)
- Integrity verification for backup files

### Data Storage Architecture

- **Backend**: GNOME Keyring (encrypted storage)
- **Schema**: `org.freedesktop.Secret.Generic`
- **Attributes**: Each password entry uses `service` and optional `username` attributes
- **Access**: Via `secret-tool` CLI and direct D-Bus API calls for enumeration

### Configuration System

- **Config File**: `config/pwmgr.conf` (created from `pwmgr.conf.example`)
- **Key Settings**: Password generation defaults, backup locations, security policies
- **Runtime**: Configuration loaded at script startup via `load_config()`

## Important Implementation Details

### Password Listing Fix
The project includes a significant fix for password enumeration. The original `secret-tool search service ''` approach was unreliable. The current implementation uses:
- Python script with `busctl` to enumerate keyring entries via D-Bus
- Proper attribute parsing to extract service names and usernames
- Correct exit codes and sorted output

### Error Handling
- All scripts use `set -euo pipefail` for strict error handling
- Comprehensive input validation for service names, usernames, and password lengths
- User confirmation prompts for destructive operations (delete)

### Security Features
- No plaintext password storage - all data encrypted in GNOME Keyring
- Secure random password generation using OpenSSL
- Optional GPG encryption for backups
- Session integration with desktop keyring unlocking

## Testing Strategy

The test suite (`tests/test-basic.sh`) covers:
- Script executability and dependencies
- Password CRUD operations
- Password generation (both basic and advanced)
- Backup/restore functionality
- Error handling scenarios
- Help and version information

**Test Pattern:**
```bash
# Cleanup function automatically removes test data
# Tests use predictable service names like "test-service-1"
# Exit codes and output patterns are verified
```

## Configuration Management

**Default Configuration Location**: `config/pwmgr.conf`

**Key Configuration Areas:**
- Password generation (length, character sets, exclusions)
- Backup settings (location, retention, encryption)
- Security policies (confirmations, clipboard timeout)
- Display preferences (colors, verbosity)

## Development Notes

- **Shell**: Bash scripts with proper error handling
- **Dependencies**: Minimal - primarily GNOME Keyring ecosystem
- **Portability**: Designed for Linux distributions with GNOME Keyring
- **Modularity**: Clear separation between core operations, generation, and backup utilities

## Debugging

Enable verbose output for troubleshooting:
```bash
./bin/pwmgr -v <command>
./utils/pwgen-advanced --verbose
```

Check keyring status:
```bash
./bin/pwmgr config  # Shows keyring daemon status
```