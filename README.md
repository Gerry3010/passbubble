# Password Manager - Keychain Edition

> 🤖 **AI Development Notice**: This project was developed collaboratively between Gerry and Claude (Anthropic's AI assistant). The initial codebase, fixes, and documentation were created through AI assistance to ensure robust functionality and proper security practices. We believe in transparency about AI involvement in open source projects.

A comprehensive command-line password manager built for Linux systems using GNOME Keyring as the secure backend storage.

## 🌿 Branches

- **main**: Clean initial version  
- **bash**: Current working bash implementation with fixes ⭐

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

- **Secure Storage**: Uses GNOME Keyring for encrypted password storage
- **Easy CLI Interface**: Simple commands for all password operations
- **Advanced Password Generation**: Multiple password types with customizable options
- **Backup & Restore**: Encrypted backup system with multiple encryption options
- **Search & Organization**: Powerful search and listing capabilities
- **Cross-System Compatibility**: Works with any Linux distribution using GNOME Keyring
- **Configuration Management**: Customizable settings and preferences

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

1. **Initialize the Password Manager**:
   ```bash
   ./bin/pwmgr init
   ```

2. **Add your first password**:
   ```bash
   ./bin/pwmgr add gmail john.doe@gmail.com
   ```

3. **Retrieve a password**:
   ```bash
   ./bin/pwmgr get gmail
   ```

4. **List all stored passwords**:
   ```bash
   ./bin/pwmgr list
   ```

## 📖 Usage Guide

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
├── bin/
│   └── pwmgr              # Main password manager script
├── config/
│   └── pwmgr.conf.example # Configuration template
├── utils/
│   ├── pwgen-advanced     # Advanced password generator
│   └── pwmgr-backup       # Backup and restore utility
├── docs/
│   └── ...                # Additional documentation
├── tests/
│   └── ...                # Test scripts
└── README.md              # This file
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