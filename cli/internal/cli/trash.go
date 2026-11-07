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

// Trash, favorites, and entry version history commands.
//
//	pwmgr trash list | restore <name|id> | purge <name|id>
//	pwmgr favorite <name|id> [--off]
//	pwmgr history <name|id> [--restore <version-id>]

package cli

import (
	"fmt"
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	trashCmd.AddCommand(trashListCmd)
	trashCmd.AddCommand(trashRestoreCmd)
	trashCmd.AddCommand(trashPurgeCmd)
	rootCmd.AddCommand(trashCmd)

	favoriteCmd.Flags().Bool("off", false, "Remove from favorites instead of adding")
	rootCmd.AddCommand(favoriteCmd)

	historyCmd.Flags().String("restore", "", "Restore this version id")
	rootCmd.AddCommand(historyCmd)
}

var trashCmd = &cobra.Command{
	Use:   "trash",
	Short: "Manage deleted entries (restorable for 30 days)",
}

var trashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List entries in the trash",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		entries, err := v.Client().ListTrash()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("Trash is empty.")
			return nil
		}
		fmt.Printf("%-36s  %-30s  %s\n", "ID", "NAME", "DELETED")
		fmt.Printf("%-36s  %-30s  %s\n", "--", "----", "-------")
		for _, e := range entries {
			deleted := ""
			if e.DeletedAt != nil {
				deleted = *e.DeletedAt
				if len(deleted) > 10 {
					deleted = deleted[:10]
				}
			}
			name := e.Name
			if len(name) > 30 {
				name = name[:27] + "..."
			}
			fmt.Printf("%-36s  %-30s  %s\n", e.ID, name, deleted)
		}
		fmt.Println("\nEntries are removed permanently 30 days after deletion.")
		return nil
	},
}

// trashEntryByArg resolves a trash entry by id or unique exact name. Usernames
// are not listed for several matches: the server does not hand out trashed blobs.
func trashEntryByArg(arg string) (*apiclient.EntryResponse, error) {
	entries, err := v.Client().ListTrash()
	if err != nil {
		return nil, err
	}
	var byName []apiclient.EntryResponse
	for _, e := range entries {
		if e.ID == arg {
			return &e, nil
		}
		if e.Name == arg {
			byName = append(byName, e)
		}
	}
	switch len(byName) {
	case 0:
		return nil, fmt.Errorf("no trashed entry named exactly '%s' (see 'pwmgr trash list')", arg)
	case 1:
		return &byName[0], nil
	default:
		rows := make([]ambiguousRow, len(byName))
		for i, e := range byName {
			rows[i] = ambiguousRow{id: e.ID, username: "(in trash)", host: vault.URLHost(e.URL)}
		}
		return nil, ambiguousEntryError(arg, rows)
	}
}

var trashRestoreCmd = &cobra.Command{
	Use:   "restore <name|id>",
	Short: "Restore an entry from the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		entry, err := trashEntryByArg(args[0])
		if err != nil {
			return err
		}
		if err := v.Client().RestoreEntry(entry.ID); err != nil {
			return fmt.Errorf("restore entry: %w", err)
		}
		fmt.Printf("✓ Entry '%s' restored\n", entry.Name)
		return nil
	},
}

var trashPurgeCmd = &cobra.Command{
	Use:   "purge <name|id>",
	Short: "Delete an entry from the trash permanently (irreversible)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		entry, err := trashEntryByArg(args[0])
		if err != nil {
			return err
		}
		if !confirm(fmt.Sprintf("Permanently delete '%s'? This cannot be undone.", entry.Name)) {
			fmt.Println("Cancelled.")
			return nil
		}
		if err := v.Client().PurgeEntry(entry.ID); err != nil {
			return fmt.Errorf("purge entry: %w", err)
		}
		fmt.Printf("✓ Entry '%s' permanently deleted\n", entry.Name)
		return nil
	},
}

var favoriteCmd = &cobra.Command{
	Use:   "favorite <name|id>",
	Short: "Mark an entry as favorite (favorites sort first)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		off, _ := cmd.Flags().GetBool("off")
		entry, err := resolveEntry(args[0], resolveOptions{})
		if err != nil {
			return err
		}
		name := entry.Name
		if err := v.Client().SetFavorite(entry.ID, !off); err != nil {
			return fmt.Errorf("set favorite: %w", err)
		}
		if off {
			fmt.Printf("✓ Entry '%s' removed from favorites\n", name)
		} else {
			fmt.Printf("★ Entry '%s' marked as favorite\n", name)
		}
		return nil
	},
}

var historyCmd = &cobra.Command{
	Use:   "history <name|id>",
	Short: "Show an entry's version history (or restore a version)",
	Long: `Show the version history of an entry. Every change keeps the previous
state (up to 20 versions). Restore resets the entry to a version — the current
state is versioned first, so nothing is lost.

Examples:
  pwmgr history gmail
  pwmgr history gmail --restore 3f6c2b1a-...`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		entry, err := resolveEntry(args[0], resolveOptions{})
		if err != nil {
			return err
		}
		id, name := entry.ID, entry.Name

		restoreID, _ := cmd.Flags().GetString("restore")
		if restoreID != "" {
			if !confirm(fmt.Sprintf("Restore '%s' to version %s?", name, restoreID)) {
				fmt.Println("Cancelled.")
				return nil
			}
			if err := v.Client().RestoreVersion(id, restoreID); err != nil {
				return fmt.Errorf("restore version: %w", err)
			}
			fmt.Printf("✓ Entry '%s' restored to version %s\n", name, restoreID)
			return nil
		}

		versions, err := v.Client().ListVersions(id)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			fmt.Printf("No previous versions of '%s' yet.\n", name)
			return nil
		}
		fmt.Printf("%-36s  %-20s  %s\n", "VERSION ID", "SAVED AT", "NAME")
		fmt.Printf("%-36s  %-20s  %s\n", "----------", "--------", "----")
		for _, ver := range versions {
			saved := ver.CreatedAt
			if len(saved) > 19 {
				saved = strings.Replace(saved[:19], "T", " ", 1)
			}
			fmt.Printf("%-36s  %-20s  %s\n", ver.ID, saved, ver.Name)
		}
		fmt.Println("\nRestore with: pwmgr history <name> --restore <version-id>")
		return nil
	},
}
