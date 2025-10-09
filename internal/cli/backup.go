package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gerry/password-manager/pkg/backup"
	"github.com/gerry/password-manager/pkg/keyring"
	"github.com/spf13/cobra"
)

var backupCmds = []*cobra.Command{backupCmd, restoreCmd, listBackupsCmd, verifyCmd, cleanCmd}

func init() {
	// Add all backup commands to root
	for _, cmd := range backupCmds {
		rootCmd.AddCommand(cmd)
	}

	// Backup command flags
	backupCmd.Flags().StringP("output", "o", "", "Output file path")
	backupCmd.Flags().StringP("dir", "d", "", "Backup directory (default: ~/Documents/pwmgr-backups)")
	backupCmd.Flags().BoolP("encrypt", "e", false, "Encrypt backup with GPG")
	backupCmd.Flags().BoolP("password", "p", false, "Use password encryption instead of GPG")

	// Restore command flags
	restoreCmd.Flags().BoolP("force", "f", false, "Force restore without confirmation")

	// List command flags
	listBackupsCmd.Flags().StringP("dir", "d", "", "Backup directory (default: ~/Documents/pwmgr-backups)")

	// Clean command flags
	cleanCmd.Flags().StringP("dir", "d", "", "Backup directory (default: ~/Documents/pwmgr-backups)")
	cleanCmd.Flags().IntP("keep", "k", 10, "Number of backups to keep")
}

// backupCmd creates a new backup
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a new backup",
	Long: `Create a new backup of all stored passwords.

The backup includes all password entries with metadata and integrity verification.
Supports optional GPG or password-based encryption.

Examples:
  pwmgr backup                          # Create basic backup
  pwmgr backup --encrypt                # Create GPG-encrypted backup
  pwmgr backup --password               # Create password-protected backup
  pwmgr backup -o /path/to/backup.tar.gz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Get flags
		outputPath, _ := cmd.Flags().GetString("output")
		backupDir, _ := cmd.Flags().GetString("dir")
		useGPG, _ := cmd.Flags().GetBool("encrypt")
		usePassword, _ := cmd.Flags().GetBool("password")

		// Set default backup directory
		if backupDir == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			backupDir = filepath.Join(homeDir, "Documents", "pwmgr-backups")
		}

		// Create backup manager
		opts := &backup.BackupOptions{
			OutputPath:  outputPath,
			UseGPG:      useGPG,
			UsePassword: usePassword,
			BackupDir:   backupDir,
			MaxBackups:  10,
		}

		mgr := backup.New(kr, opts)

		// Create backup
		fmt.Println("Creating backup...")
		filename, err := mgr.CreateBackup()
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}

		fmt.Printf("✓ Backup created successfully: %s\n", filename)
		return nil
	},
}

// restoreCmd restores from a backup file
var restoreCmd = &cobra.Command{
	Use:   "restore <backup-file>",
	Short: "Restore from a backup file",
	Long: `Restore passwords from a backup file.

This will import all passwords from the backup into the keyring.
Existing passwords with the same service/username combination will be skipped.

Examples:
  pwmgr restore backup-2024-01-15.tar.gz
  pwmgr restore --force encrypted-backup.tar.gz.gpg`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupFile := args[0]

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Check if file exists
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			return fmt.Errorf("backup file not found: %s", backupFile)
		}

		// Get flags
		force, _ := cmd.Flags().GetBool("force")

		// Confirm restoration unless forced
		if !force {
			fmt.Printf("This will restore passwords from: %s\n", backupFile)
			fmt.Printf("Existing passwords will not be overwritten.\n")
			fmt.Print("Continue? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" {
				fmt.Println("Restore cancelled")
				return nil
			}
		}

		// Create backup manager
		mgr := backup.New(kr, nil)

		// Restore backup
		fmt.Println("Restoring backup...")
		if err := mgr.RestoreBackup(backupFile); err != nil {
			return fmt.Errorf("failed to restore backup: %w", err)
		}

		return nil
	},
}

// listBackupsCmd lists available backups
var listBackupsCmd = &cobra.Command{
	Use:   "list-backups",
	Short: "List available backups",
	Long: `List all available backup files in the backup directory.

Shows backup name, size, and modification time sorted by newest first.

Examples:
  pwmgr list-backups
  pwmgr list-backups -d /custom/backup/path`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		backupDir, _ := cmd.Flags().GetString("dir")

		// Set default backup directory
		if backupDir == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			backupDir = filepath.Join(homeDir, "Documents", "pwmgr-backups")
		}

		// Create backup manager
		opts := &backup.BackupOptions{
			BackupDir: backupDir,
		}

		kr := keyring.New() // We need this for the manager, but won't use keyring operations
		mgr := backup.New(kr, opts)

		// List backups
		backups, err := mgr.ListBackups()
		if err != nil {
			return fmt.Errorf("failed to list backups: %w", err)
		}

		if len(backups) == 0 {
			fmt.Printf("No backups found in: %s\n", backupDir)
			return nil
		}

		fmt.Printf("Available backups in %s:\n\n", backupDir)
		for _, b := range backups {
			sizeStr := formatSize(b.Size)
			timeStr := b.ModTime.Format("2006-01-02 15:04:05")
			fmt.Printf("  %-40s  %8s  %s\n", b.Name, sizeStr, timeStr)
		}

		return nil
	},
}

// verifyCmd verifies backup integrity
var verifyCmd = &cobra.Command{
	Use:   "verify <backup-file>",
	Short: "Verify backup integrity",
	Long: `Verify the integrity of a backup file.

Checks the backup file structure and validates checksums to ensure
the backup is not corrupted and can be restored successfully.

Examples:
  pwmgr verify backup-2024-01-15.tar.gz
  pwmgr verify encrypted-backup.tar.gz.gpg`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupFile := args[0]

		// Check if file exists
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			return fmt.Errorf("backup file not found: %s", backupFile)
		}

		// Create backup manager
		kr := keyring.New()
		mgr := backup.New(kr, nil)

		// Verify backup
		fmt.Printf("Verifying backup: %s\n", backupFile)
		if err := mgr.VerifyBackup(backupFile); err != nil {
			return fmt.Errorf("backup verification failed: %w", err)
		}

		fmt.Println("✓ Backup verification successful")
		return nil
	},
}

// cleanCmd cleans old backup files
var cleanCmd = &cobra.Command{
	Use:   "clean-backups",
	Short: "Clean old backups",
	Long: `Remove old backup files, keeping only the most recent ones.

By default, keeps the 10 most recent backups and removes older ones.
Use --keep flag to specify a different number.

Examples:
  pwmgr clean-backups              # Keep last 10 backups
  pwmgr clean-backups --keep 5     # Keep last 5 backups
  pwmgr clean-backups -d /custom/path`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		backupDir, _ := cmd.Flags().GetString("dir")
		maxBackups, _ := cmd.Flags().GetInt("keep")

		// Set default backup directory
		if backupDir == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			backupDir = filepath.Join(homeDir, "Documents", "pwmgr-backups")
		}

		// Create backup manager
		opts := &backup.BackupOptions{
			BackupDir:  backupDir,
			MaxBackups: maxBackups,
		}

		kr := keyring.New()
		mgr := backup.New(kr, opts)

		// Clean old backups
		fmt.Printf("Cleaning old backups (keeping %d most recent)...\n", maxBackups)
		if err := mgr.CleanOldBackups(); err != nil {
			return fmt.Errorf("failed to clean backups: %w", err)
		}

		return nil
	},
}

// Helper function to format file sizes
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
