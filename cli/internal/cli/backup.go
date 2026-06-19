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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

// localBackup is a simple JSON backup of decrypted entries stored locally.
type localBackup struct {
	CreatedAt string        `json:"created_at"`
	Server    string        `json:"server"`
	Email     string        `json:"email"`
	Entries   []backupEntry `json:"entries"`
}

type backupEntry struct {
	Name     string           `json:"name"`
	Type     string           `json:"type"`
	URL      string           `json:"url,omitempty"`
	Data     *vault.EntryData `json:"data,omitempty"`
}

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
			if backupDir == "" {
				home, _ := os.UserHomeDir()
				backupDir = filepath.Join(home, "Documents", "pwmgr-backups")
			}
			if err := os.MkdirAll(backupDir, 0700); err != nil {
				return fmt.Errorf("create backup dir: %w", err)
			}
			ts := time.Now().Format("20060102-150405")
			outputPath = filepath.Join(backupDir, fmt.Sprintf("backup-%s.json", ts))
		}

		fmt.Println("Fetching entries...")
		apiEntries, err := v.ListEntries()
		if err != nil {
			return err
		}

		entries := make([]backupEntry, 0, len(apiEntries))
		for i, e := range apiEntries {
			fmt.Printf("\r  Decrypting %d/%d...", i+1, len(apiEntries))
			full, err := v.GetEntry(e.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nWarning: could not decrypt '%s': %v\n", e.Name, err)
				continue
			}
			entries = append(entries, backupEntry{
				Name: full.Name,
				Type: full.Type,
				URL:  full.URL,
				Data: full.Data,
			})
		}
		fmt.Println()

		bk := localBackup{
			CreatedAt: time.Now().Format(time.RFC3339),
			Server:    cfg.ServerURL,
			Email:     cfg.Email,
			Entries:   entries,
		}

		data, err := json.MarshalIndent(bk, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outputPath, data, 0600); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}

		fmt.Printf("✓ Backup saved to %s (%d entries)\n", outputPath, len(entries))
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

		backupFile := args[0]
		data, err := os.ReadFile(backupFile)
		if err != nil {
			return fmt.Errorf("read backup file: %w", err)
		}

		var bk localBackup
		if err := json.Unmarshal(data, &bk); err != nil {
			return fmt.Errorf("parse backup file: %w", err)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Restore %d entries from %s?\n", len(bk.Entries), bk.CreatedAt)
			if !confirm("Continue?") {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		ok, failed := 0, 0
		for i, e := range bk.Entries {
			fmt.Printf("\r  Importing %d/%d...", i+1, len(bk.Entries))
			_, err := v.CreateEntry(e.Name, e.Type, e.URL, e.Data, nil, "", "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nWarning: could not import '%s': %v\n", e.Name, err)
				failed++
			} else {
				ok++
			}
		}
		fmt.Println()
		fmt.Printf("✓ Restored %d entries (%d failed)\n", ok, failed)
		return nil
	},
}

var listBackupsCmd = &cobra.Command{
	Use:   "list-backups",
	Short: "List local backup files",
	RunE: func(cmd *cobra.Command, args []string) error {
		backupDir, _ := cmd.Flags().GetString("dir")
		if backupDir == "" {
			home, _ := os.UserHomeDir()
			backupDir = filepath.Join(home, "Documents", "pwmgr-backups")
		}

		files, err := filepath.Glob(filepath.Join(backupDir, "backup-*.json"))
		if err != nil || len(files) == 0 {
			fmt.Printf("No backups found in %s\n", backupDir)
			return nil
		}

		fmt.Printf("Backups in %s:\n\n", backupDir)
		for _, f := range files {
			info, _ := os.Stat(f)
			size := ""
			if info != nil {
				size = fmt.Sprintf("%d KB", info.Size()/1024)
			}
			fmt.Printf("  %-45s  %s\n", filepath.Base(f), size)
		}
		return nil
	},
}
