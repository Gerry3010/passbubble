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

	"github.com/spf13/cobra"
)

var upgradeKeysCmd = &cobra.Command{
	Use:   "upgrade-keys",
	Short: "Retrofit post-quantum (ML-KEM-768) encryption onto your account",
	Long: `Generates a real ML-KEM-768 keypair for your account and re-wraps every
entry's data key to the hybrid (X25519 + ML-KEM-768) format.

Accounts created by the Flutter app are X25519-only (classical) — this adds the
post-quantum layer that protects against "harvest now, decrypt later" attacks.
Your X25519 keypair is kept, so entries shared to you stay readable, and entries
that fail to re-wrap remain decryptable (they stay classical). Safe to re-run.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		if !v.NeedsKeyUpgrade() {
			fmt.Println("✓ Your account already uses post-quantum (hybrid) encryption.")
			return nil
		}

		password, err := promptPassword("Master password: ")
		if err != nil {
			return err
		}

		fmt.Println("Upgrading keys and re-wrapping entries…")
		res, err := v.UpgradeToHybrid(password)
		if err != nil {
			return err
		}

		fmt.Printf("✓ Post-quantum upgrade complete — %d entr%s re-wrapped to hybrid.\n",
			res.Rewrapped, plural(res.Rewrapped))
		if len(res.Failed) > 0 {
			fmt.Printf("⚠ %d entr%s could not be re-wrapped (left classical, still readable): %v\n",
				len(res.Failed), plural(len(res.Failed)), res.Failed)
		}
		return nil
	},
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func init() {
	rootCmd.AddCommand(upgradeKeysCmd)
}
