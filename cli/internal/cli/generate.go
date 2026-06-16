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
	"strconv"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().IntP("length", "l", 20, "Password length")
	generateCmd.Flags().IntP("count", "c", 1, "Number of passwords to generate")
	generateCmd.Flags().StringP("type", "t", "strong", "Type (strong, alphanum, numbers, lower)")
	generateCmd.Flags().StringP("exclude", "e", "", "Characters to exclude")
	generateCmd.Flags().Bool("no-ambiguous", false, "Exclude ambiguous characters (0, O, l, 1, I)")
}

var generateCmd = &cobra.Command{
	Use:   "generate [length]",
	Short: "Generate secure passwords",
	Long: `Generate cryptographically secure passwords.

Examples:
  pwmgr generate
  pwmgr generate 32
  pwmgr generate -t alphanum -l 16 -c 5
  pwmgr generate --no-ambiguous`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		length, _ := cmd.Flags().GetInt("length")
		if len(args) > 0 {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid length: %s", args[0])
			}
			length = n
		}
		count, _ := cmd.Flags().GetInt("count")
		genType, _ := cmd.Flags().GetString("type")
		exclude, _ := cmd.Flags().GetString("exclude")
		noAmbiguous, _ := cmd.Flags().GetBool("no-ambiguous")

		resp, err := v.Client().Generate(apiclient.GenerateRequest{
			Length:       length,
			Count:        count,
			Type:         genType,
			ExcludeChars: exclude,
			NoAmbiguous:  noAmbiguous,
		})
		if err != nil {
			return fmt.Errorf("generate: %w", err)
		}

		for _, p := range resp.Passwords {
			fmt.Printf("%s  (strength: %d/100)\n", p.Password, p.Strength)
		}
		return nil
	},
}

// generateDefaults returns default GenerateRequest parameters, used when
// the user wants an auto-generated password in other commands.
func generateDefaults() apiclient.GenerateRequest {
	return apiclient.GenerateRequest{Length: 20, Count: 1, Type: "strong"}
}
