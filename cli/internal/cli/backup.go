// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cli

import (
	"fmt"
	"os"

	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(listBackupsCmd)

	backupCmd.Flags().StringP("output", "o", "", "Output file path")
	backupCmd.Flags().StringP("dir", "d", "", "Backup directory")
	restoreCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	listBackupsCmd.Flags().StringP("dir", "d", "", "Backup directory")
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Export all entries to a local JSON file",
	Long: `Download and decrypt all entries, then save them to a local JSON file.

The backup contains decrypted passwords — keep it secure.

Examples:
  pwmgr backup
  pwmgr backup -o /path/to/backup.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		outputPath, _ := cmd.Flags().GetString("output")
		backupDir, _ := cmd.Flags().GetString("dir")
		if outputPath == "" {
			var err error
			if backupDir == "" {
				if backupDir, err = backup.DefaultDir(); err != nil {
					return err
				}
			}
			if outputPath, err = backup.NewPath(backupDir, ""); err != nil {
				return err
			}
		}

		fmt.Println("Fetching entries...")
		result, err := backup.Create(v, outputPath, backup.Options{
			Server: cfg.ServerURL,
			Email:  cfg.Email,
			Progress: func(done, total int) {
				fmt.Printf("\r  Decrypting %d/%d...", done, total)
			},
		})
		fmt.Println()
		if result.Path == "" {
			return err
		}
		if err != nil {
			fmt.Printf("Backup saved to %s (%d of %d entries)\n", result.Path, result.Saved, result.Total)
			return err
		}
		fmt.Printf("✓ Backup saved to %s (%d entries)\n", result.Path, result.Saved)
		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore <backup-file>",
	Short: "Import entries from a local JSON backup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		file, err := backup.Read(args[0])
		if err != nil {
			return err
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Restore %d entries from %s?\n", len(file.Entries), file.CreatedAt)
			if !confirm("Continue?") {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		result := backup.Restore(v, file, func(done, total int) {
			fmt.Printf("\r  Importing %d/%d...", done, total)
		})
		fmt.Println()
		for _, failure := range result.Failed {
			fmt.Fprintf(os.Stderr, "Warning: could not import %s\n", failure)
		}
		fmt.Printf("✓ Restored %d entries (%d failed)\n", result.Restored, len(result.Failed))
		return nil
	},
}

var listBackupsCmd = &cobra.Command{
	Use:   "list-backups",
	Short: "List local backup files",
	RunE: func(cmd *cobra.Command, args []string) error {
		backupDir, _ := cmd.Flags().GetString("dir")
		if backupDir == "" {
			var err error
			if backupDir, err = backup.DefaultDir(); err != nil {
				return err
			}
		}

		backups, err := backup.List(backupDir)
		if err != nil {
			return err
		}
		if len(backups) == 0 {
			fmt.Printf("No backups found in %s\n", backupDir)
			return nil
		}

		fmt.Printf("Backups in %s:\n\n", backupDir)
		for _, b := range backups {
			fmt.Printf("  %-45s  %d KB\n", b.Name, b.Size/1024)
		}
		return nil
	},
}
