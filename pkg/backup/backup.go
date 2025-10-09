package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Gerry3010/passbubble/pkg/keyring"
)

// BackupData represents the structure of a backup file
type BackupData struct {
	Passwords []keyring.Entry `json:"passwords"`
	Metadata  BackupMetadata  `json:"metadata"`
}

// BackupMetadata contains information about the backup
type BackupMetadata struct {
	BackupDate    time.Time `json:"backup_date"`
	Hostname      string    `json:"hostname"`
	User          string    `json:"user"`
	Version       string    `json:"version"`
	PasswordCount int       `json:"password_count"`
	Checksum      string    `json:"checksum"`
}

// BackupOptions contains options for backup operations
type BackupOptions struct {
	OutputPath  string
	UseGPG      bool
	UsePassword bool
	BackupDir   string
	MaxBackups  int
}

// Manager handles backup and restore operations
type Manager struct {
	keyring keyring.KeyringInterface
	options *BackupOptions
}

// New creates a new backup manager
func New(kr keyring.KeyringInterface, opts *BackupOptions) *Manager {
	if opts == nil {
		homeDir, _ := os.UserHomeDir()
		opts = &BackupOptions{
			BackupDir:  filepath.Join(homeDir, "Documents", "pwmgr-backups"),
			MaxBackups: 10,
		}
	}
	return &Manager{
		keyring: kr,
		options: opts,
	}
}

// CreateBackup creates a new backup of all passwords
func (m *Manager) CreateBackup() (string, error) {
	if err := m.ensureBackupDir(); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Get all password entries
	entries, err := m.keyring.List()
	if err != nil {
		return "", fmt.Errorf("failed to list passwords: %w", err)
	}

	// Fetch actual passwords for each entry
	for i := range entries {
		password, err := m.keyring.Get(entries[i].Service, entries[i].Username)
		if err != nil {
			continue // Skip entries that can't be retrieved
		}
		entries[i].Password = password
	}

	// Create backup data
	hostname, _ := os.Hostname()
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}

	backupData := BackupData{
		Passwords: entries,
		Metadata: BackupMetadata{
			BackupDate:    time.Now(),
			Hostname:      hostname,
			User:          user,
			Version:       "2.0.0",
			PasswordCount: len(entries),
		},
	}

	// Generate filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("pwmgr-backup-%s.tar.gz", timestamp)
	if m.options.OutputPath != "" {
		filename = m.options.OutputPath
	} else {
		filename = filepath.Join(m.options.BackupDir, filename)
	}

	// Create backup file
	if err := m.writeBackup(backupData, filename); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	// Apply encryption if requested
	if m.options.UseGPG {
		encryptedFilename, err := m.encryptWithGPG(filename)
		if err != nil {
			os.Remove(filename) // Clean up unencrypted file
			return "", fmt.Errorf("failed to encrypt backup: %w", err)
		}
		os.Remove(filename) // Remove unencrypted file
		filename = encryptedFilename
	} else if m.options.UsePassword {
		encryptedFilename, err := m.encryptWithPassword(filename)
		if err != nil {
			os.Remove(filename) // Clean up unencrypted file
			return "", fmt.Errorf("failed to encrypt backup: %w", err)
		}
		os.Remove(filename) // Remove unencrypted file
		filename = encryptedFilename
	}

	return filename, nil
}

// RestoreBackup restores passwords from a backup file
func (m *Manager) RestoreBackup(backupPath string) error {
	// Decrypt if necessary
	actualPath := backupPath
	if strings.HasSuffix(backupPath, ".gpg") {
		decryptedPath, err := m.decryptWithGPG(backupPath)
		if err != nil {
			return fmt.Errorf("failed to decrypt GPG backup: %w", err)
		}
		defer os.Remove(decryptedPath) // Clean up decrypted file
		actualPath = decryptedPath
	} else if strings.HasSuffix(backupPath, ".enc") {
		decryptedPath, err := m.decryptWithPassword(backupPath)
		if err != nil {
			return fmt.Errorf("failed to decrypt password-protected backup: %w", err)
		}
		defer os.Remove(decryptedPath) // Clean up decrypted file
		actualPath = decryptedPath
	}

	// Read backup data
	backupData, err := m.readBackup(actualPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Verify backup integrity
	if err := m.verifyBackup(backupData); err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}

	// Restore passwords
	restored := 0
	skipped := 0
	for _, entry := range backupData.Passwords {
		// Check if password already exists
		if _, err := m.keyring.Get(entry.Service, entry.Username); err == nil {
			skipped++
			continue // Skip existing passwords
		}

		// Store password
		if err := m.keyring.Store(entry.Service, entry.Username, entry.Password); err != nil {
			fmt.Printf("Warning: Failed to restore password for %s: %v\n", entry.Service, err)
			continue
		}
		restored++
	}

	fmt.Printf("Backup restored successfully: %d passwords restored, %d skipped (already exist)\n", restored, skipped)
	return nil
}

// ListBackups lists available backup files
func (m *Manager) ListBackups() ([]BackupInfo, error) {
	if err := m.ensureBackupDir(); err != nil {
		return nil, fmt.Errorf("backup directory not accessible: %w", err)
	}

	files, err := os.ReadDir(m.options.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasPrefix(name, "pwmgr-backup-") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:    name,
			Path:    filepath.Join(m.options.BackupDir, name),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})

	return backups, nil
}

// VerifyBackup verifies the integrity of a backup file
func (m *Manager) VerifyBackup(backupPath string) error {
	backupData, err := m.readBackup(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	return m.verifyBackup(backupData)
}

// CleanOldBackups removes old backup files, keeping only the most recent ones
func (m *Manager) CleanOldBackups() error {
	backups, err := m.ListBackups()
	if err != nil {
		return err
	}

	if len(backups) <= m.options.MaxBackups {
		return nil // Nothing to clean
	}

	// Remove old backups
	toRemove := backups[m.options.MaxBackups:]
	for _, backup := range toRemove {
		if err := os.Remove(backup.Path); err != nil {
			fmt.Printf("Warning: Failed to remove old backup %s: %v\n", backup.Name, err)
		}
	}

	fmt.Printf("Cleaned %d old backup(s)\n", len(toRemove))
	return nil
}

// BackupInfo contains information about a backup file
type BackupInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// Helper methods

func (m *Manager) ensureBackupDir() error {
	return os.MkdirAll(m.options.BackupDir, 0755)
}

func (m *Manager) writeBackup(data BackupData, filename string) error {
	// Create temporary directory for backup contents
	tempDir, err := os.MkdirTemp("", "pwmgr-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Write JSON data
	jsonPath := filepath.Join(tempDir, "passwords.json")
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Calculate checksum
	hash := sha256.Sum256(jsonData)
	data.Metadata.Checksum = hex.EncodeToString(hash[:])

	// Update JSON with checksum
	jsonData, err = json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(jsonPath, jsonData, 0600); err != nil {
		return err
	}

	// Create tar.gz archive
	return m.createTarGz(filename, tempDir)
}

func (m *Manager) readBackup(filename string) (BackupData, error) {
	var data BackupData

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "pwmgr-restore-")
	if err != nil {
		return data, err
	}
	defer os.RemoveAll(tempDir)

	// Extract tar.gz
	if err := m.extractTarGz(filename, tempDir); err != nil {
		return data, err
	}

	// Read JSON data
	jsonPath := filepath.Join(tempDir, "passwords.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return data, err
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return data, err
	}

	return data, nil
}

func (m *Manager) verifyBackup(data BackupData) error {
	// Recalculate checksum without the checksum field
	originalChecksum := data.Metadata.Checksum
	data.Metadata.Checksum = ""

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	hash := sha256.Sum256(jsonData)
	calculatedChecksum := hex.EncodeToString(hash[:])

	if originalChecksum != calculatedChecksum {
		return fmt.Errorf("backup checksum verification failed")
	}

	return nil
}

func (m *Manager) createTarGz(filename, sourceDir string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		_, err = io.Copy(tarWriter, srcFile)
		return err
	})
}

func (m *Manager) extractTarGz(filename, destDir string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeReg:
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Manager) encryptWithGPG(filename string) (string, error) {
	encryptedFilename := filename + ".gpg"
	cmd := exec.Command("gpg", "--symmetric", "--cipher-algo", "AES256", "--output", encryptedFilename, filename)
	return encryptedFilename, cmd.Run()
}

func (m *Manager) decryptWithGPG(filename string) (string, error) {
	decryptedFilename := strings.TrimSuffix(filename, ".gpg")
	cmd := exec.Command("gpg", "--decrypt", "--output", decryptedFilename, filename)
	return decryptedFilename, cmd.Run()
}

func (m *Manager) encryptWithPassword(filename string) (string, error) {
	encryptedFilename := filename + ".enc"
	cmd := exec.Command("openssl", "enc", "-aes-256-cbc", "-salt", "-in", filename, "-out", encryptedFilename)
	return encryptedFilename, cmd.Run()
}

func (m *Manager) decryptWithPassword(filename string) (string, error) {
	decryptedFilename := strings.TrimSuffix(filename, ".enc")
	cmd := exec.Command("openssl", "enc", "-aes-256-cbc", "-d", "-in", filename, "-out", decryptedFilename)
	return decryptedFilename, cmd.Run()
}
