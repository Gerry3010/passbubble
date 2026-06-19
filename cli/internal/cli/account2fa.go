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

	"github.com/spf13/cobra"
)

func init() {
	account2faCmd.AddCommand(account2faEnableCmd)
	account2faCmd.AddCommand(account2faDisableCmd)
	rootCmd.AddCommand(account2faCmd)
}

var account2faCmd = &cobra.Command{
	Use:   "account-2fa",
	Short: "Manage two-factor authentication (TOTP) for your account login",
}

var account2faEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable login 2FA: shows a secret to add to your authenticator app",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		setup, err := v.Client().SetupTOTP()
		if err != nil {
			return fmt.Errorf("could not start 2FA setup: %w", err)
		}

		fmt.Println("Add this account to your authenticator app:")
		fmt.Printf("  Secret:      %s\n", setup.Secret)
		fmt.Printf("  otpauth URL: %s\n\n", setup.OTPAuthURL)

		code, err := promptInput("Enter the 6-digit code shown in your app to confirm: ")
		if err != nil {
			return err
		}
		if err := v.Client().ConfirmTOTP(setup.Secret, strings.TrimSpace(code)); err != nil {
			return fmt.Errorf("could not enable 2FA: %w", err)
		}
		fmt.Println("Two-factor authentication is now enabled.")
		return nil
	},
}

var account2faDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable login 2FA (requires a current code or your password)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		code, err := promptInput("Current 6-digit code (leave blank to use your password): ")
		if err != nil {
			return err
		}
		code = strings.TrimSpace(code)

		var password string
		if code == "" {
			password, err = promptPassword("Account password: ")
			if err != nil {
				return err
			}
		}
		if err := v.Client().DisableTOTP(code, password); err != nil {
			return fmt.Errorf("could not disable 2FA: %w", err)
		}
		fmt.Println("Two-factor authentication is now disabled.")
		return nil
	},
}
