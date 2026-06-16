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
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	localtotp "github.com/Gerry3010/passbubble/cli/pkg/totp"
	"github.com/pquerna/otp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(totpAddCmd)
	rootCmd.AddCommand(totpCodeCmd)
	rootCmd.AddCommand(totpListCmd)
	rootCmd.AddCommand(totpDeleteCmd)

	totpAddCmd.Flags().StringP("issuer", "i", "", "Issuer name")
	totpAddCmd.Flags().StringP("secret", "s", "", "TOTP secret (base32)")
	totpAddCmd.Flags().StringP("url", "u", "", "TOTP URL (otpauth://...)")
	totpAddCmd.Flags().Bool("generate", false, "Generate a new TOTP secret")
	totpAddCmd.Flags().Uint("period", 30, "TOTP period in seconds")
	totpAddCmd.Flags().Uint("digits", 6, "Number of digits (6 or 8)")
	totpAddCmd.Flags().String("algorithm", "SHA1", "Hash algorithm (SHA1, SHA256, SHA512)")

	totpCodeCmd.Flags().BoolP("watch", "w", false, "Watch mode — continuously display codes")
}

var totpAddCmd = &cobra.Command{
	Use:   "totp-add <name> [username]",
	Short: "Add a TOTP secret",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		name := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		issuer, _ := cmd.Flags().GetString("issuer")
		secret, _ := cmd.Flags().GetString("secret")
		totpURL, _ := cmd.Flags().GetString("url")
		generate, _ := cmd.Flags().GetBool("generate")
		period, _ := cmd.Flags().GetUint("period")
		digits, _ := cmd.Flags().GetUint("digits")
		algorithm, _ := cmd.Flags().GetString("algorithm")

		var totpData vault.EntryData

		switch {
		case totpURL != "":
			opts, parsedSecret, err := localtotp.ParseTOTPURL(totpURL)
			if err != nil {
				return fmt.Errorf("parse TOTP URL: %w", err)
			}
			totpData = vault.EntryData{
				Username:   username,
				TOTPSecret: parsedSecret,
				Issuer:     opts.Issuer,
				Period:     int(opts.Period),
				Digits:     int(opts.Digits),
				Algorithm:  strings.ToUpper(opts.Algorithm.String()),
			}

		case generate:
			opts := &localtotp.GenerateOptions{
				Issuer:      issuer,
				AccountName: name,
				Period:      period,
				Digits:      convertDigits(digits),
				Algorithm:   convertAlgorithm(algorithm),
			}
			genSecret, url, err := localtotp.GenerateSecret(opts)
			if err != nil {
				return fmt.Errorf("generate TOTP secret: %w", err)
			}
			fmt.Printf("Generated TOTP secret: %s\n", genSecret)
			fmt.Printf("URL: %s\n", url)
			totpData = vault.EntryData{
				Username:   username,
				TOTPSecret: genSecret,
				Issuer:     issuer,
				Period:     int(period),
				Digits:     int(digits),
				Algorithm:  strings.ToUpper(algorithm),
			}

		case secret != "":
			if !localtotp.IsValidSecret(secret) {
				return fmt.Errorf("invalid TOTP secret (must be base32)")
			}
			totpData = vault.EntryData{
				Username:   username,
				TOTPSecret: secret,
				Issuer:     issuer,
				Period:     int(period),
				Digits:     int(digits),
				Algorithm:  strings.ToUpper(algorithm),
			}

		default:
			return fmt.Errorf("provide --secret, --url, or --generate")
		}

		entry, err := v.CreateEntry(name, "totp", "", &totpData, nil)
		if err != nil {
			return err
		}
		fmt.Printf("✓ TOTP secret stored for '%s' (id: %s)\n", entry.Name, entry.ID)

		// Show first code
		code, cerr := localtotp.GenerateCode(totpData.TOTPSecret, nil)
		if cerr == nil {
			remaining := localtotp.GetTimeRemaining(30)
			fmt.Printf("Current code: %s (valid for %ds)\n", localtotp.FormatCode(code), remaining)
		}
		return nil
	},
}

var totpCodeCmd = &cobra.Command{
	Use:   "totp-code <name>",
	Short: "Generate TOTP codes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		watch, _ := cmd.Flags().GetBool("watch")

		matches, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no TOTP entry found for '%s'", args[0])
		}

		id := matches[0].ID
		for _, m := range matches {
			if m.Name == args[0] && m.Type == "totp" {
				id = m.ID
				break
			}
		}

		entry, err := v.GetEntry(id)
		if err != nil {
			return err
		}
		if entry.Type != "totp" {
			return fmt.Errorf("entry '%s' is not a TOTP entry", entry.Name)
		}
		if entry.Data == nil || entry.Data.TOTPSecret == "" {
			return fmt.Errorf("no TOTP secret found in entry")
		}

		period := uint(30)
		if entry.Data.Period > 0 {
			period = uint(entry.Data.Period)
		}
		opts := &localtotp.GenerateOptions{
			Period:    period,
			Digits:    convertDigits(uint(entry.Data.Digits)),
			Algorithm: convertAlgorithm(entry.Data.Algorithm),
		}

		if watch {
			fmt.Printf("TOTP codes for '%s' (Ctrl+C to stop)\n\n", entry.Name)
			for {
				code, err := localtotp.GenerateCode(entry.Data.TOTPSecret, opts)
				if err != nil {
					return err
				}
				remaining := localtotp.GetTimeRemaining(period)
				fmt.Printf("\r%s | Code: %s | Valid: %2ds",
					time.Now().Format("15:04:05"),
					localtotp.FormatCode(code),
					remaining)
				time.Sleep(time.Second)
			}
		}

		code, err := localtotp.GenerateCode(entry.Data.TOTPSecret, opts)
		if err != nil {
			return err
		}
		remaining := localtotp.GetTimeRemaining(period)
		fmt.Printf("Code: %s (valid for %ds)\n", localtotp.FormatCode(code), remaining)
		return nil
	},
}

var totpListCmd = &cobra.Command{
	Use:   "totp-list",
	Short: "List all TOTP entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		entries, err := v.ListEntries()
		if err != nil {
			return err
		}

		var totpEntries []vault.Entry
		for _, e := range entries {
			if e.Type == "totp" {
				totpEntries = append(totpEntries, e)
			}
		}

		if len(totpEntries) == 0 {
			fmt.Println("No TOTP entries found. Use 'pwmgr totp-add' to add one.")
			return nil
		}

		fmt.Printf("Found %d TOTP entries:\n\n", len(totpEntries))
		for _, e := range totpEntries {
			fmt.Printf("  • %s\n", e.Name)
		}
		return nil
	},
}

var totpDeleteCmd = &cobra.Command{
	Use:   "totp-delete <name>",
	Short: "Delete a TOTP entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		matches, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no entry found for '%s'", args[0])
		}

		entry := matches[0]
		if !confirm(fmt.Sprintf("Delete TOTP entry '%s'?", entry.Name)) {
			fmt.Println("Cancelled.")
			return nil
		}

		if err := v.DeleteEntry(entry.ID); err != nil {
			return err
		}
		fmt.Printf("✓ TOTP entry '%s' deleted\n", entry.Name)
		return nil
	},
}

func convertDigits(d uint) otp.Digits {
	if d == 8 {
		return otp.DigitsEight
	}
	return otp.DigitsSix
}

func convertAlgorithm(a string) otp.Algorithm {
	switch strings.ToUpper(a) {
	case "SHA256":
		return otp.AlgorithmSHA256
	case "SHA512":
		return otp.AlgorithmSHA512
	default:
		return otp.AlgorithmSHA1
	}
}
