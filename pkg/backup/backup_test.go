package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerry/password-manager/pkg/keyring"
)

// MockKeyring provides a mock implementation for testing
type MockKeyring struct {
	entries map[string]keyring.Entry
}

func NewMockKeyring() *MockKeyring {
	return &MockKeyring{
		entries: make(map[string]keyring.Entry),
	}
}

func (mk *MockKeyring) Store(service, username, password string) error {
	key := service
	if username != "" {
		key = service + ":" + username
	}
	mk.entries[key] = keyring.Entry{
		Service:    service,
		Username:   username,
		Password:   password,
		SecretType: keyring.SecretTypePassword,
	}
	return nil
}

func (mk *MockKeyring) StoreEntry(entry keyring.Entry) error {
	key := entry.Service
	if entry.Username != "" {
		key = entry.Service + ":" + entry.Username
	}
	mk.entries[key] = entry
	return nil
}

func (mk *MockKeyring) Get(service, username string) (string, error) {
	key := service
	if username != "" {
		key = service + ":" + username
	}
	if entry, exists := mk.entries[key]; exists {
		return entry.Password, nil
	}
	return "", os.ErrNotExist
}

func (mk *MockKeyring) GetEntry(service, username string) (*keyring.Entry, error) {
	key := service
	if username != "" {
		key = service + ":" + username
	}
	if entry, exists := mk.entries[key]; exists {
		return &entry, nil
	}
	return nil, os.ErrNotExist
}

func (mk *MockKeyring) Delete(service, username string) error {
	key := service
	if username != "" {
		key = service + ":" + username
	}
	delete(mk.entries, key)
	return nil
}

func (mk *MockKeyring) List() ([]keyring.Entry, error) {
	var entries []keyring.Entry
	for _, entry := range mk.entries {
		entries = append(entries, entry)
	}
	return entries, nil
}

func (mk *MockKeyring) Search(pattern string) ([]keyring.Entry, error) {
	var matches []keyring.Entry
	for _, entry := range mk.entries {
		if entry.Service == pattern || entry.Username == pattern {
			matches = append(matches, entry)
		}
	}
	return matches, nil
}

func (mk *MockKeyring) IsAvailable() bool {
	return true
}

func TestBackupOptions(t *testing.T) {
	kr := NewMockKeyring()

	// Test with nil options (should use defaults)
	mgr := New(kr, nil)
	if mgr.options.MaxBackups != 10 {
		t.Errorf("Expected default MaxBackups 10, got %d", mgr.options.MaxBackups)
	}

	homeDir, _ := os.UserHomeDir()
	expectedDir := filepath.Join(homeDir, "Documents", "pwmgr-backups")
	if mgr.options.BackupDir != expectedDir {
		t.Errorf("Expected default BackupDir %s, got %s", expectedDir, mgr.options.BackupDir)
	}

	// Test with custom options
	customOpts := &BackupOptions{
		BackupDir:  "/tmp/test-backups",
		MaxBackups: 5,
		UseGPG:     true,
	}

	mgr2 := New(kr, customOpts)
	if mgr2.options.BackupDir != "/tmp/test-backups" {
		t.Errorf("Expected custom BackupDir /tmp/test-backups, got %s", mgr2.options.BackupDir)
	}
	if mgr2.options.MaxBackups != 5 {
		t.Errorf("Expected custom MaxBackups 5, got %d", mgr2.options.MaxBackups)
	}
	if !mgr2.options.UseGPG {
		t.Error("Expected UseGPG to be true")
	}
}

func TestCreateAndRestoreBackup(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "pwmgr-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Setup mock keyring with test data
	kr := NewMockKeyring()
	kr.Store("gmail", "user@example.com", "password123")
	kr.Store("github", "", "github_token_456")
	kr.Store("bank", "john.doe", "secure_banking_789")

	// Create backup manager
	opts := &BackupOptions{
		BackupDir:  tempDir,
		MaxBackups: 10,
	}
	mgr := New(kr, opts)

	// Create backup
	backupPath, err := mgr.CreateBackup()
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatalf("Backup file not created: %s", backupPath)
	}

	// Verify backup
	if err := mgr.VerifyBackup(backupPath); err != nil {
		t.Fatalf("Backup verification failed: %v", err)
	}

	// Clear mock keyring
	kr.entries = make(map[string]keyring.Entry)

	// Restore backup
	if err := mgr.RestoreBackup(backupPath); err != nil {
		t.Fatalf("Failed to restore backup: %v", err)
	}

	// Verify restored data
	entries, err := kr.List()
	if err != nil {
		t.Fatalf("Failed to list entries after restore: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries after restore, got %d", len(entries))
	}

	// Check specific entries
	password, err := kr.Get("gmail", "user@example.com")
	if err != nil || password != "password123" {
		t.Errorf("Gmail password not restored correctly: got %s, err: %v", password, err)
	}

	password, err = kr.Get("github", "")
	if err != nil || password != "github_token_456" {
		t.Errorf("GitHub password not restored correctly: got %s, err: %v", password, err)
	}

	password, err = kr.Get("bank", "john.doe")
	if err != nil || password != "secure_banking_789" {
		t.Errorf("Bank password not restored correctly: got %s, err: %v", password, err)
	}
}

func TestListBackups(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "pwmgr-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	kr := NewMockKeyring()
	opts := &BackupOptions{
		BackupDir:  tempDir,
		MaxBackups: 10,
	}
	mgr := New(kr, opts)

	// Initially should be empty
	backups, err := mgr.ListBackups()
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("Expected 0 backups, got %d", len(backups))
	}

	// Add some test data and create backups
	kr.Store("test1", "", "password1")

	backup1, err := mgr.CreateBackup()
	if err != nil {
		t.Fatalf("Failed to create first backup: %v", err)
	}

	// Wait a bit and create another backup
	time.Sleep(time.Millisecond * 100) // Small delay for different timestamp
	kr.Store("test2", "", "password2")
	backup2, err := mgr.CreateBackup()
	if err != nil {
		t.Fatalf("Failed to create second backup: %v", err)
	}

	// List backups
	backups, err = mgr.ListBackups()
	if err != nil {
		t.Fatalf("Failed to list backups: %v", err)
	}

	if len(backups) < 1 {
		t.Errorf("Expected at least 1 backup, got %d", len(backups))
		return
	}

	// If we have more than one backup, verify sorting
	if len(backups) >= 2 {
		if !backups[0].ModTime.After(backups[1].ModTime) {
			t.Error("Backups are not sorted by newest first")
		}
	}

	// Verify backup paths
	found1, found2 := false, false
	for _, backup := range backups {
		if backup.Path == backup1 {
			found1 = true
		}
		if backup.Path == backup2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Not all created backups were found in the list")
	}
}

func TestCleanOldBackups(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "pwmgr-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	kr := NewMockKeyring()
	kr.Store("test", "", "password")

	opts := &BackupOptions{
		BackupDir:  tempDir,
		MaxBackups: 2, // Keep only 2 backups
	}
	mgr := New(kr, opts)

	// Create 3 backups (we'll keep 2) with unique filenames
	var backups []string
	for i := 0; i < 3; i++ {
		time.Sleep(time.Millisecond * 10) // Small delay
		filename := filepath.Join(tempDir, fmt.Sprintf("pwmgr-backup-test-%d.tar.gz", i))
		opts.OutputPath = filename
		backup, err := mgr.CreateBackup()
		if err != nil {
			t.Fatalf("Failed to create backup %d: %v", i, err)
		}
		backups = append(backups, backup)

		// Reset OutputPath for next iteration
		opts.OutputPath = ""

		// Also create a file with proper modification time
		if i > 0 {
			// Set different modification times
			modTime := time.Now().Add(-time.Duration(3-i) * time.Hour)
			os.Chtimes(backup, modTime, modTime)
		}
	}

	// Verify all 3 backups exist
	for _, backup := range backups {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			t.Errorf("Backup should exist before cleanup: %s", backup)
		}
	}

	// Clean old backups
	if err := mgr.CleanOldBackups(); err != nil {
		t.Fatalf("Failed to clean backups: %v", err)
	}

	// Verify only 2 backups remain
	remaining := 0
	for _, backup := range backups {
		if _, err := os.Stat(backup); err == nil {
			remaining++
		}
	}

	if remaining != 2 {
		t.Errorf("Expected 2 backups to remain after cleanup, got %d", remaining)
	}

	// List remaining backups to verify we kept the right ones
	remainingBackups, err := mgr.ListBackups()
	if err != nil {
		t.Fatalf("Failed to list remaining backups: %v", err)
	}

	if len(remainingBackups) != 2 {
		t.Errorf("Expected 2 remaining backups in list, got %d", len(remainingBackups))
	}
}

func TestBackupMetadata(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "pwmgr-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	kr := NewMockKeyring()
	kr.Store("test1", "user1", "password1")
	kr.Store("test2", "", "password2")

	opts := &BackupOptions{
		BackupDir: tempDir,
	}
	mgr := New(kr, opts)

	backupPath, err := mgr.CreateBackup()
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Read backup to check metadata
	backupData, err := mgr.readBackup(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	// Verify metadata
	if backupData.Metadata.Version != "2.0.0" {
		t.Errorf("Expected version 2.0.0, got %s", backupData.Metadata.Version)
	}

	if backupData.Metadata.PasswordCount != 2 {
		t.Errorf("Expected password count 2, got %d", backupData.Metadata.PasswordCount)
	}

	if backupData.Metadata.Checksum == "" {
		t.Error("Checksum should not be empty")
	}

	if len(backupData.Passwords) != 2 {
		t.Errorf("Expected 2 passwords in backup, got %d", len(backupData.Passwords))
	}

	// Verify backup time is recent (within last minute)
	if time.Since(backupData.Metadata.BackupDate) > time.Minute {
		t.Error("Backup date seems too old")
	}
}
