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
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(tagsCmd)
}

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List all tags with their entry counts",
	Long: `List every tag in your vault with the number of entries carrying it.

Tags are end-to-end encrypted, so this decrypts every entry and asks for the
master password.

Examples:
  pwmgr tags
  pwmgr list --tag work            # entries with a tag
  pwmgr update github --add-tag work`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}
		records, err := v.ListAllDecrypted()
		if err != nil {
			return err
		}

		perEntry := make([][]string, len(records))
		for i, r := range records {
			perEntry[i] = r.Data.Tags
		}
		counts := vault.CountTags(perEntry)
		if len(counts) == 0 {
			fmt.Println("No tags yet. Use 'pwmgr add <name> --tag <tag>' or 'pwmgr update <name> --add-tag <tag>'.")
			return nil
		}

		fmt.Printf("%-*s  %s\n", vault.MaxTagLength, "TAG", "ENTRIES")
		fmt.Printf("%-*s  %s\n", vault.MaxTagLength, "---", "-------")
		for _, c := range counts {
			fmt.Printf("%-*s  %d\n", vault.MaxTagLength, c.Name, c.Count)
		}
		fmt.Printf("\n%d tags total\n", len(counts))
		return nil
	},
}

// listTagged prints the entries carrying every tag in filter. Tags sit inside
// the encrypted data, so this decrypts the whole vault.
func listTagged(filter []string) error {
	if err := ensureUnlocked(); err != nil {
		return err
	}
	records, err := v.ListAllDecrypted()
	if err != nil {
		return err
	}

	var matches []vault.EntryRecord
	for _, r := range records {
		if vault.HasTags(r.Data.Tags, filter) {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		fmt.Printf("No entries tagged %s.\n", describeFilter(filter))
		return nil
	}

	fmt.Printf("%-40s  %-12s  %-40s  %s\n", "NAME", "TYPE", "URL", "TAGS")
	fmt.Printf("%-40s  %-12s  %-40s  %s\n", "----", "----", "---", "----")
	for _, e := range matches {
		url := e.URL
		if len(url) > 40 {
			url = url[:37] + "..."
		}
		fmt.Printf("%-40s  %-12s  %-40s  %s\n", e.Name, e.Type, url, formatTags(e.Data.Tags))
	}
	fmt.Printf("\n%d of %d entries tagged %s\n", len(matches), len(records), describeFilter(filter))
	return nil
}

// splitTagArgs flattens repeated tag flag values and splits each on commas,
// so "--tag a --tag b" and "--tag a,b" mean the same. Empty pieces are left
// for vault.NormalizeTags to drop.
func splitTagArgs(vals []string) []string {
	var out []string
	for _, val := range vals {
		out = append(out, strings.Split(val, ",")...)
	}
	return out
}

// describeFilter renders a tag filter for messages: 'work' and 'finance'.
func describeFilter(filter []string) string {
	return "'" + strings.Join(filter, "' and '") + "'"
}

// formatTags renders a tag list for terminal output.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "(none)"
	}
	return strings.Join(tags, ", ")
}
