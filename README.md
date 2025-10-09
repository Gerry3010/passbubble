# Password Manager - Go Edition

> 🤖 **AI Development Notice**: This project was developed collaboratively between Gerry and Claude (Anthropic's AI assistant). The initial codebase, fixes, and documentation were created through AI assistance to ensure robust functionality and proper security practices. We believe in transparency about AI involvement in open source projects.

A comprehensive command-line password manager built for multiple platforms using GNOME Keyring (Linux) and system keychains (macOS/Windows) as secure backend storage.

## 🌿 Project Evolution

- **Go Implementation** (v2.0+): Modern, cross-platform CLI with TOTP support ⭐
- **Legacy Bash**: Original Linux-only implementation (deprecated)

## 🔧 Recent Fixes (in bash branch)

### ✅ Fixed: "No passwords found or error accessing keyring" Error

**Problem**: The original `secret-tool search service ''` command with empty strings didn't work properly to enumerate keyring entries.

**Solution**: Implemented a robust Python-based approach that:
- Uses `busctl` to enumerate all keyring entries via D-Bus API
- Checks each entry's attributes for `service` attribute presence
- Extracts service names and usernames correctly
- Provides clean, sorted output

**Result**: 
- ✅ Now properly lists all passwords with `service` attribute
- ✅ Correct exit codes (0 for success)
- ✅ Shows usernames when available  
- ✅ Full compatibility maintained with all existing features

## 🌟 Features

- **🔐 Interactive TUI**: Beautiful terminal interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **🔑 Secure Storage**: Uses GNOME Keyring (Linux), Keychain (macOS), or Credential Manager (Windows)
- **🔓 TOTP Support**: Two-Factor Authentication compatible with Google Authenticator, Authy, etc.
- **🌐 Cross-Platform**: Linux, macOS, Windows, FreeBSD, OpenBSD support
- **💻 Dual Interface**: Both interactive TUI and traditional CLI commands
- **🎲 Advanced Password Generation**: Multiple password types with cryptographic security
- **💾 Backup & Restore**: JSON-based encrypted backup system with GPG support  
- **🔍 Search & Organization**: Fast searching and categorization of all secret types
- **🔒 Multiple Secret Types**: Passwords, TOTP keys, API keys, secure notes (planned)
- **📊 Version Information**: Built-in version tracking with build details

## 📋 Requirements

- Linux system (tested on Manjaro Linux)
- GNOME Keyring and related packages:
  - `gnome-keyring`
  - `libsecret-tools` (provides `secret-tool`)
- Standard Unix tools: `bash`, `tar`, `gzip`
- Optional dependencies:
  - `gpg` (for GPG-encrypted backups)
  - `openssl` (for password-encrypted backups)
  - `jq` (for JSON processing in backups)

### Installation of Dependencies

On Manjaro/Arch Linux:
```bash
sudo pacman -S gnome-keyring libsecret
```

On Ubuntu/Debian:
```bash
sudo apt install gnome-keyring libsecret-tools
```

On Fedora/RHEL:
```bash
sudo dnf install gnome-keyring libsecret
```

## 🚀 Quick Start

### Interactive TUI (Recommended)

1. **Launch the beautiful TUI interface**:
   ```bash
   # Build and run
   make build && ./build/pwmgr-go
   
   # Or run TUI explicitly
   ./build/pwmgr-go tui
   ```

2. **Navigate with intuitive controls**:
   - `↑/↓` or `j/k`: Navigate lists
   - `Enter`: View entry details with live TOTP codes
   - `s`: Show/hide secrets securely
   - `a`: Add new entry
   - `b`: Backup management
   - `q`: Quit

### CLI Commands (Traditional)

1. **Add your first password**:
   ```bash
   ./build/pwmgr-go add gmail john.doe@gmail.com
   ```

2. **Set up TOTP for 2FA**:
   ```bash
   ./build/pwmgr-go totp-add google user@gmail.com --generate --issuer "Google"
   ```

3. **Retrieve a password**:
   ```bash
   ./build/pwmgr-go get gmail john.doe@gmail.com
   ```

4. **Generate a TOTP code**:
   ```bash
   ./build/pwmgr-go totp-code google user@gmail.com
   ```

5. **List all stored secrets**:
   ```bash
   ./build/pwmgr-go list
   ```

## 📦 Installation

### From Releases (Recommended)

1. Download the appropriate binary for your system from [Releases](https://github.com/gerry/password-manager/releases)
2. Extract the archive and copy to your PATH:
   ```bash
   # Linux/macOS/BSD
   tar -xzf pwmgr-go-*-your-os-arch.tar.gz
   sudo cp pwmgr-go /usr/local/bin/
   
   # Windows (PowerShell)
   Expand-Archive pwmgr-go-*-windows-*.zip
   Copy-Item pwmgr-go.exe C:\Windows\System32\
   ```

### Building from Source

**Requirements:**
- Go 1.25 or later
- System keyring (GNOME Keyring on Linux, built-in on macOS/Windows)
- Make (optional, for build automation)

```bash
git clone https://github.com/gerry/password-manager.git
cd password-manager

# Using Makefile (recommended)
make build
sudo cp build/pwmgr-go /usr/local/bin/

# Or direct Go build
go build -o pwmgr-go ./cmd/pwmgr
```

### System Dependencies

The Go implementation automatically integrates with your system's keyring:

**Linux:**
```bash
# Manjaro/Arch
sudo pacman -S gnome-keyring libsecret

# Ubuntu/Debian
sudo apt install gnome-keyring libsecret-tools

# Fedora/RHEL
sudo dnf install gnome-keyring libsecret
```

**macOS:** Built-in Keychain (no installation needed)

**Windows:** Built-in Credential Manager (no installation needed)

## 📚 Usage Guide (Go Implementation)

### 🔐 Interactive TUI (Bubble Tea)

The TUI provides a beautiful, interactive interface for managing your secrets:

```bash
# Launch TUI (default when no command specified)
pwmgr-go

# Or explicitly launch TUI
pwmgr-go tui
```

**Main Screen Features:**
- 🔑 Browse all stored passwords and TOTP secrets
- 🔍 Search and filter entries
- 🎨 Color-coded entry types with icons
- ⌨️ Vim-style navigation (`j`/`k` or arrow keys)
- 🎯 Quick access to all major functions

**Detail Screen Features:**
- 🔍 View complete entry information
- 🔓 Secure secret display (press `s` to toggle)
- 🔄 Live TOTP code generation with countdown
- 📊 Progress bar showing code validity
- ⏱️ Auto-refresh TOTP codes every second

**Backup Management Screen:**
- 💾 Visual backup browser
- 📅 Backup creation dates and file sizes
- 🎢 Quick restore and delete options

**Security Features:**
- 🔐 Passwords hidden by default (use CLI for copying)
- 🗱️ Automatic sensitive data cleanup
- ⚠️ Clear security notices and guidance

### Password Management

```bash
# Add a password
pwmgr-go add github username

# Get a password  
pwmgr-go get github username

# List all passwords
pwmgr-go list

# Update a password
pwmgr-go update github username

# Delete a password
pwmgr-go delete github username

# Search passwords
pwmgr-go search gmail
```

### TOTP (Two-Factor Authentication)

```bash
# Generate new TOTP secret
pwmgr-go totp-add google user@gmail.com --generate --issuer "Google"

# Add from QR code URL
pwmgr-go totp-add github --url "otpauth://totp/GitHub:user?secret=ABC..."

# Add with manual secret
pwmgr-go totp-add vpn --secret "ABCDEF123456" --issuer "Company VPN"

# Generate TOTP code
pwmgr-go totp-code google user@gmail.com

# Watch codes (live updates)
pwmgr-go totp-code google user@gmail.com --watch

# List TOTP secrets
pwmgr-go totp-list

# Delete TOTP secret
pwmgr-go totp-delete google user@gmail.com
```

### Password Generation

```bash
# Generate strong password
pwmgr-go generate 16

# Generate multiple passwords
pwmgr-go generate -c 5

# Generate passphrase
pwmgr-go generate -t passphrase

# Generate with specific options
pwmgr-go generate -l 20 --no-ambiguous

# Check password strength
pwmgr-go generate --check
```

### Backup & Restore

```bash
# Create backup
pwmgr-go backup create

# Create encrypted backup
pwmgr-go backup create --encrypt --gpg-recipient "user@example.com"

# List backups
pwmgr-go backup list

# Restore backup
pwmgr-go backup restore backup-20241209-143022.json

# Clean old backups
pwmgr-go backup clean --keep 5
```

### Build System

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Create release archives
make release

# Run tests
make test

# Development workflow
make dev  # format + vet + test + build
```

### Version Information

```bash
# Show version information
pwmgr-go version

# JSON output for scripting
pwmgr-go version --json

# Built-in version info includes:
# - Version number
# - Build time
# - Git commit hash
# - Go version used
# - Target platform
```

## 📚 Legacy Usage Guide (Bash Implementation)

### Basic Operations

#### Adding Passwords
```bash
# Add password for a service (will prompt for password)
./bin/pwmgr add github

# Add password with username
./bin/pwmgr add gmail john.doe@gmail.com

# Let the system generate a secure password
./bin/pwmgr add banking  # Leave password empty when prompted
```

#### Retrieving Passwords
```bash
# Get password for a service
./bin/pwmgr get github

# Get password with specific username
./bin/pwmgr get gmail john.doe@gmail.com
```

#### Updating Passwords
```bash
# Update existing password
./bin/pwmgr update github

# Update password with username
./bin/pwmgr update gmail john.doe@gmail.com
```

#### Deleting Passwords
```bash
# Delete a password (will ask for confirmation)
./bin/pwmgr delete github

# Delete password with username
./bin/pwmgr delete gmail john.doe@gmail.com
```

#### Listing and Searching
```bash
# List all stored passwords
./bin/pwmgr list

# Search for passwords matching a pattern
./bin/pwmgr search bank
./bin/pwmgr search gmail
```

### Password Generation

#### Using the Main Script
```bash
# Generate a 16-character password
./bin/pwmgr generate

# Generate a 20-character password
./bin/pwmgr generate 20
```

#### Using the Advanced Generator
```bash
# Basic password generation
./utils/pwgen-advanced

# Generate 5 passwords of 20 characters each
./utils/pwgen-advanced -l 20 -c 5

# Generate alphanumeric password (no symbols)
./utils/pwgen-advanced -t alphanum -l 12

# Generate memorable password
./utils/pwgen-advanced -t memorable -l 16

# Generate passphrase
./utils/pwgen-advanced -t passphrase

# Generate password without ambiguous characters
./utils/pwgen-advanced --no-ambiguous -l 16

# Check password strength
./utils/pwgen-advanced --check -l 16
```

### Backup and Restore

#### Creating Backups
```bash
# Create a basic backup
./utils/pwmgr-backup backup

# Create GPG-encrypted backup
./utils/pwmgr-backup backup --encrypt

# Create password-protected backup
./utils/pwmgr-backup backup --password
```

#### Managing Backups
```bash
# List all backups
./utils/pwmgr-backup list

# Verify backup integrity
./utils/pwmgr-backup verify backup-2024-01-15_14-30-00.tar.gz

# Clean old backups (keep last 10)
./utils/pwmgr-backup clean
```

#### Restoring from Backup
```bash
# Restore from backup (will prompt for confirmation)
./utils/pwmgr-backup restore backup-2024-01-15_14-30-00.tar.gz
```

### Configuration

#### View Current Configuration
```bash
./bin/pwmgr config
```

#### Customize Settings
1. Copy the example configuration:
   ```bash
   cp config/pwmgr.conf.example config/pwmgr.conf
   ```

2. Edit the configuration file:
   ```bash
   nano config/pwmgr.conf
   ```

## 🔧 Configuration Options

The password manager can be customized through the `config/pwmgr.conf` file. Key settings include:

- **Password Generation**: Default length, character sets, exclusions
- **Backup Settings**: Location, retention policy, encryption defaults
- **Security**: Confirmation requirements, clipboard timeout
- **Display**: Colors, verbosity, output format
- **Integration**: Browser integration, export formats

See `config/pwmgr.conf.example` for all available options.

## 📁 Project Structure

```
Password-Manager/
├── cmd/pwmgr/             # Main application entry point
├── internal/
│   ├── cli/               # CLI commands and interface
│   └── version/           # Version information
├── pkg/
│   ├── backup/           # Backup and restore functionality
│   ├── generator/        # Password generation
│   ├── keyring/          # Keyring integration
│   └── totp/             # TOTP implementation
├── build/                # Build output directory
├── dist/                 # Release archives
├── bin/                  # Legacy bash scripts
├── utils/                # Legacy utilities
├── config/               # Configuration examples
├── tests/                # Test scripts
├── Makefile              # Build automation
├── go.mod                # Go module definition
└── README.md             # This documentation
```

## 🔐 Security Features

- **Encrypted Storage**: All passwords are stored encrypted in GNOME Keyring
- **Secure Generation**: Cryptographically secure random password generation
- **Backup Encryption**: Multiple encryption options for backups (GPG, OpenSSL)
- **No Plaintext Storage**: Passwords never stored in plaintext on disk
- **Session Integration**: Works with desktop session keyring unlocking

## 🧪 Testing

Run the test suite to verify functionality:

```bash
# Make test scripts executable
chmod +x tests/*.sh

# Run basic functionality tests
./tests/test-basic.sh

# Run password generation tests
./tests/test-generation.sh

# Run backup/restore tests
./tests/test-backup.sh
```

## 🐛 Troubleshooting

### Common Issues

1. **"secret-tool not found"**
   - Install `libsecret-tools` package
   - Verify GNOME Keyring is running: `systemctl --user status gnome-keyring-daemon`

2. **"No passwords found" when listing**
   - Ensure GNOME Keyring daemon is running
   - Check if you're logged into the correct user session
   - Verify keyring is unlocked

3. **Backup/Restore failures**
   - Check disk space for backup operations
   - Verify encryption tools (GPG/OpenSSL) are installed for encrypted backups
   - Ensure backup directory is writable

4. **Permission denied errors**
   - Make scripts executable: `chmod +x bin/* utils/*`
   - Check file permissions in the project directory

### Debug Mode

Enable verbose output for debugging:

```bash
./bin/pwmgr -v <command>
./utils/pwgen-advanced --verbose
```

## 🚧 Development

### Adding Features

1. Main script functionality: Edit `bin/pwmgr`
2. Password generation: Edit `utils/pwgen-advanced`
3. Backup system: Edit `utils/pwmgr-backup`
4. Configuration: Add options to `config/pwmgr.conf.example`

### Code Style

- Follow existing bash scripting conventions
- Use meaningful variable names
- Add comments for complex logic
- Implement proper error handling
- Test all new features

## 📄 License

This project is released under the MIT License. See LICENSE file for details.

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## 📞 Support

For issues, questions, or contributions:

- Check the troubleshooting section above
- Review existing issues in the project repository
- Create a new issue with detailed information about your problem

## 🎯 Roadmap

Future enhancements planned:

- [ ] GUI interface using Python/GTK
- [ ] Browser integration extensions
- [ ] Mobile app companion
- [ ] Cloud sync capabilities
- [ ] Multi-factor authentication
- [ ] Password sharing features
- [ ] Audit and security reports

---

**Note**: This password manager is designed for personal use and development learning. For production environments, consider established solutions like Bitwarden, KeePass, or commercial password managers.