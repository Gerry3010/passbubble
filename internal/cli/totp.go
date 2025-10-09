package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Gerry3010/passbubble/pkg/keyring"
	"github.com/Gerry3010/passbubble/pkg/totp"
	"github.com/pquerna/otp"
	"github.com/spf13/cobra"
)

var totpCmds = []*cobra.Command{totpAddCmd, totpCodeCmd, totpListCmd, totpDeleteCmd}

func init() {
	// Add all TOTP commands to root
	for _, cmd := range totpCmds {
		rootCmd.AddCommand(cmd)
	}

	// TOTP add command flags
	totpAddCmd.Flags().StringP("issuer", "i", "", "Issuer name (e.g., 'Google', 'GitHub')")
	totpAddCmd.Flags().StringP("account", "a", "", "Account name (e.g., 'john@example.com')")
	totpAddCmd.Flags().StringP("secret", "s", "", "TOTP secret (base32 encoded)")
	totpAddCmd.Flags().StringP("url", "u", "", "TOTP URL (otpauth://...)")
	totpAddCmd.Flags().Uint("period", 30, "TOTP period in seconds")
	totpAddCmd.Flags().Uint("digits", 6, "Number of digits (6 or 8)")
	totpAddCmd.Flags().StringP("algorithm", "g", "SHA1", "Hash algorithm (SHA1, SHA256, SHA512)")
	totpAddCmd.Flags().Bool("generate", false, "Generate a new TOTP secret")

	// TOTP code command flags
	totpCodeCmd.Flags().BoolP("watch", "w", false, "Watch mode - continuously display codes")
	totpCodeCmd.Flags().IntP("refresh", "r", 1, "Refresh interval in seconds (watch mode)")
}

// totpAddCmd adds a new TOTP secret
var totpAddCmd = &cobra.Command{
	Use:   "totp-add <service> [username]",
	Short: "Add a new TOTP secret",
	Long: `Add a new TOTP (Time-based One-Time Password) secret to the keyring.

You can provide the secret manually, paste a TOTP URL, or generate a new one.
TOTP secrets are used for two-factor authentication.

Examples:
  pwmgr totp-add gmail user@example.com --secret JBSWY3DPEHPK3PXP
  pwmgr totp-add github --url "otpauth://totp/GitHub:user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"
  pwmgr totp-add company-vpn --generate --issuer "Company VPN"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Get flags
		issuer, _ := cmd.Flags().GetString("issuer")
		account, _ := cmd.Flags().GetString("account")
		secret, _ := cmd.Flags().GetString("secret")
		totpURL, _ := cmd.Flags().GetString("url")
		period, _ := cmd.Flags().GetUint("period")
		digits, _ := cmd.Flags().GetUint("digits")
		algorithm, _ := cmd.Flags().GetString("algorithm")
		generate, _ := cmd.Flags().GetBool("generate")

		var entry keyring.Entry
		entry.Service = service
		entry.Username = username
		entry.SecretType = keyring.SecretTypeTOTP

		// Default account name if not provided
		if account == "" {
			if username != "" {
				account = username
			} else {
				account = service
			}
		}

		// Handle different input methods
		if totpURL != "" {
			// Parse TOTP URL
			opts, parsedSecret, err := totp.ParseTOTPURL(totpURL)
			if err != nil {
				return fmt.Errorf("failed to parse TOTP URL: %w", err)
			}

			entry.Password = parsedSecret
			entry.Issuer = opts.Issuer
			entry.Period = int(opts.Period)
			entry.Digits = int(opts.Digits)
			entry.Algorithm = strings.ToUpper(opts.Algorithm.String())

		} else if generate {
			// Generate new TOTP secret
			opts := &totp.GenerateOptions{
				Issuer:      issuer,
				AccountName: account,
				Period:      period,
				Digits:      convertToOTPDigits(digits),
				Algorithm:   convertToOTPAlgorithm(algorithm),
			}

			generatedSecret, url, err := totp.GenerateSecret(opts)
			if err != nil {
				return fmt.Errorf("failed to generate TOTP secret: %w", err)
			}

			entry.Password = generatedSecret
			entry.Issuer = issuer
			entry.Period = int(period)
			entry.Digits = int(digits)
			entry.Algorithm = strings.ToUpper(algorithm)

			fmt.Printf("Generated TOTP secret for %s\n", service)
			fmt.Printf("Secret: %s\n", generatedSecret)
			fmt.Printf("URL: %s\n", url)
			fmt.Println()

		} else if secret != "" {
			// Use provided secret
			if !totp.IsValidSecret(secret) {
				return fmt.Errorf("invalid TOTP secret format (must be base32)")
			}

			entry.Password = secret
			entry.Issuer = issuer
			entry.Period = int(period)
			entry.Digits = int(digits)
			entry.Algorithm = strings.ToUpper(algorithm)

		} else {
			return fmt.Errorf("must provide either --secret, --url, or --generate")
		}

		// Set defaults for TOTP parameters
		if entry.Period == 0 {
			entry.Period = 30
		}
		if entry.Digits == 0 {
			entry.Digits = 6
		}
		if entry.Algorithm == "" {
			entry.Algorithm = "SHA1"
		}

		// Store the TOTP entry
		if err := kr.StoreEntry(entry); err != nil {
			return fmt.Errorf("failed to store TOTP secret: %w", err)
		}

		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		fmt.Printf("✓ TOTP secret stored for %s\n", displayName)

		// Generate and display first code
		opts := &totp.GenerateOptions{
			Period:    uint(entry.Period),
			Digits:    convertToOTPDigits(uint(entry.Digits)),
			Algorithm: convertToOTPAlgorithm(entry.Algorithm),
		}

		code, err := totp.GenerateCode(entry.Password, opts)
		if err == nil {
			remaining := totp.GetTimeRemaining(uint(entry.Period))
			fmt.Printf("Current code: %s (valid for %ds)\n", totp.FormatCode(code), remaining)
		}

		return nil
	},
}

// totpCodeCmd generates TOTP codes
var totpCodeCmd = &cobra.Command{
	Use:   "totp-code <service> [username]",
	Short: "Generate TOTP codes",
	Long: `Generate TOTP (Time-based One-Time Password) codes for stored secrets.

Displays the current code and remaining validity time. 
Use --watch to continuously display codes as they refresh.

Examples:
  pwmgr totp-code gmail user@example.com
  pwmgr totp-code github --watch
  pwmgr totp-code company-vpn --watch --refresh 2`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Get the TOTP entry
		entry, err := kr.GetEntry(service, username)
		if err != nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("TOTP secret not found for %s", displayName)
		}

		if entry.SecretType != keyring.SecretTypeTOTP {
			return fmt.Errorf("entry is not a TOTP secret")
		}

		// Get flags
		watch, _ := cmd.Flags().GetBool("watch")
		refresh, _ := cmd.Flags().GetInt("refresh")

		// Set up TOTP options
		opts := &totp.GenerateOptions{
			Period:    uint(entry.Period),
			Digits:    convertToOTPDigits(uint(entry.Digits)),
			Algorithm: convertToOTPAlgorithm(entry.Algorithm),
		}

		if entry.Period == 0 {
			opts.Period = 30
		}
		if entry.Digits == 0 {
			opts.Digits = otp.DigitsSix
		}
		if entry.Algorithm == "" {
			opts.Algorithm = otp.AlgorithmSHA1
		}

		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		if entry.Issuer != "" {
			displayName = fmt.Sprintf("%s (%s)", entry.Issuer, displayName)
		}

		if watch {
			// Watch mode - continuously display codes
			fmt.Printf("TOTP codes for %s (Press Ctrl+C to stop)\n", displayName)
			fmt.Printf("Period: %ds, Digits: %d, Algorithm: %s\n\n", opts.Period, opts.Digits, opts.Algorithm)

			for {
				code, err := totp.GenerateCode(entry.Password, opts)
				if err != nil {
					return fmt.Errorf("failed to generate TOTP code: %w", err)
				}

				remaining := totp.GetTimeRemaining(opts.Period)
				timestamp := time.Now().Format("15:04:05")

				fmt.Printf("\r%s | Code: %s | Valid for: %2ds", timestamp, totp.FormatCode(code), remaining)

				time.Sleep(time.Duration(refresh) * time.Second)
			}
		} else {
			// Single code generation
			code, err := totp.GenerateCode(entry.Password, opts)
			if err != nil {
				return fmt.Errorf("failed to generate TOTP code: %w", err)
			}

			remaining := totp.GetTimeRemaining(opts.Period)
			fmt.Printf("TOTP code for %s: %s\n", displayName, totp.FormatCode(code))
			fmt.Printf("Valid for %d seconds\n", remaining)
		}

		return nil
	},
}

// totpListCmd lists all TOTP secrets
var totpListCmd = &cobra.Command{
	Use:   "totp-list",
	Short: "List all TOTP secrets",
	Long: `List all stored TOTP (Time-based One-Time Password) secrets.

Shows service names, usernames, issuers, and TOTP parameters.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		entries, err := kr.List()
		if err != nil {
			return fmt.Errorf("failed to list entries: %w", err)
		}

		// Filter TOTP entries
		var totpEntries []keyring.Entry
		for _, entry := range entries {
			if entry.SecretType == keyring.SecretTypeTOTP {
				totpEntries = append(totpEntries, entry)
			}
		}

		if len(totpEntries) == 0 {
			fmt.Println("No TOTP secrets found. Use 'totp-add' command to add your first TOTP secret.")
			return nil
		}

		fmt.Printf("Found %d TOTP secrets:\n\n", len(totpEntries))

		for _, entry := range totpEntries {
			displayName := entry.Service
			if entry.Username != "" {
				displayName = fmt.Sprintf("%s (%s)", entry.Service, entry.Username)
			}

			fmt.Printf("• %s", displayName)

			if entry.Issuer != "" {
				fmt.Printf(" [%s]", entry.Issuer)
			}

			// Show TOTP parameters
			period := entry.Period
			if period == 0 {
				period = 30
			}
			digits := entry.Digits
			if digits == 0 {
				digits = 6
			}
			algorithm := entry.Algorithm
			if algorithm == "" {
				algorithm = "SHA1"
			}

			fmt.Printf(" - %ds/%dd/%s", period, digits, algorithm)

			if entry.Notes != "" {
				fmt.Printf(" (%s)", entry.Notes)
			}

			fmt.Println()
		}

		return nil
	},
}

// totpDeleteCmd deletes a TOTP secret
var totpDeleteCmd = &cobra.Command{
	Use:   "totp-delete <service> [username]",
	Short: "Delete a TOTP secret",
	Long: `Delete a TOTP (Time-based One-Time Password) secret from the keyring.

This action requires confirmation and cannot be undone.

Examples:
  pwmgr totp-delete gmail user@example.com
  pwmgr totp-delete github`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Check if TOTP entry exists
		entry, err := kr.GetEntry(service, username)
		if err != nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("TOTP secret not found for %s", displayName)
		}

		if entry.SecretType != keyring.SecretTypeTOTP {
			return fmt.Errorf("entry is not a TOTP secret")
		}

		// Confirm deletion
		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		if entry.Issuer != "" {
			displayName = fmt.Sprintf("%s [%s]", displayName, entry.Issuer)
		}

		fmt.Printf("Are you sure you want to delete the TOTP secret for %s? [y/N]: ", displayName)

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" && response != "yes" {
			fmt.Println("Operation cancelled")
			return nil
		}

		// Delete the TOTP secret
		if err := kr.Delete(service, username); err != nil {
			return fmt.Errorf("failed to delete TOTP secret: %w", err)
		}

		fmt.Printf("✓ TOTP secret deleted for %s\n", displayName)
		return nil
	},
}

// Helper functions to convert between different type systems
func convertToOTPDigits(digits uint) otp.Digits {
	if digits == 8 {
		return otp.DigitsEight
	}
	return otp.DigitsSix
}

func convertToOTPAlgorithm(algorithm string) otp.Algorithm {
	switch strings.ToUpper(algorithm) {
	case "SHA256":
		return otp.AlgorithmSHA256
	case "SHA512":
		return otp.AlgorithmSHA512
	case "MD5":
		return otp.AlgorithmMD5
	default:
		return otp.AlgorithmSHA1
	}
}
