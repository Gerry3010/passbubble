package cli

import (
	"fmt"
	"strconv"

	"github.com/gerry/password-manager/pkg/generator"
	"github.com/gerry/password-manager/pkg/keyring"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(searchCmd)

	// Generate command flags
	generateCmd.Flags().IntP("length", "l", 16, "Password length")
	generateCmd.Flags().IntP("count", "c", 1, "Number of passwords to generate")
	generateCmd.Flags().StringP("type", "t", "strong", "Password type (strong, alphanum, numbers, memorable, passphrase)")
	generateCmd.Flags().StringP("symbols", "s", "!@#$%^&*()_+-=[]{}|;:,.<>?", "Custom symbol set")
	generateCmd.Flags().StringP("exclude", "e", "", "Characters to exclude from password")
	generateCmd.Flags().Bool("no-ambiguous", false, "Exclude ambiguous characters (0, O, l, 1, I)")
	generateCmd.Flags().Int("min-upper", 1, "Minimum uppercase letters")
	generateCmd.Flags().Int("min-lower", 1, "Minimum lowercase letters")
	generateCmd.Flags().Int("min-digits", 1, "Minimum digits")
	generateCmd.Flags().Int("min-symbols", 1, "Minimum symbols")
	generateCmd.Flags().Bool("check", false, "Check password strength")
}

// generateCmd generates passwords
var generateCmd = &cobra.Command{
	Use:   "generate [length]",
	Short: "Generate a random password",
	Long: `Generate secure random passwords with various options.

You can specify the password length as an argument or use the --length flag.
Multiple password types are supported with customizable complexity requirements.

Examples:
  pwmgr generate 20
  pwmgr generate -t alphanum -l 12
  pwmgr generate -c 5 --no-ambiguous
  pwmgr generate -t passphrase
  pwmgr generate --check -l 16`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := generator.DefaultOptions()

		// Get length from argument or flag
		if len(args) > 0 {
			length, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid length: %s", args[0])
			}
			if length < 8 || length > 128 {
				return fmt.Errorf("password length must be between 8 and 128")
			}
			opts.Length = length
		} else {
			length, _ := cmd.Flags().GetInt("length")
			if length < 8 || length > 128 {
				return fmt.Errorf("password length must be between 8 and 128")
			}
			opts.Length = length
		}

		// Get other flags
		count, _ := cmd.Flags().GetInt("count")
		typeStr, _ := cmd.Flags().GetString("type")
		symbols, _ := cmd.Flags().GetString("symbols")
		exclude, _ := cmd.Flags().GetString("exclude")
		noAmbiguous, _ := cmd.Flags().GetBool("no-ambiguous")
		minUpper, _ := cmd.Flags().GetInt("min-upper")
		minLower, _ := cmd.Flags().GetInt("min-lower")
		minDigits, _ := cmd.Flags().GetInt("min-digits")
		minSymbols, _ := cmd.Flags().GetInt("min-symbols")
		checkStrength, _ := cmd.Flags().GetBool("check")

		// Set options
		opts.Count = count
		opts.Symbols = symbols
		opts.ExcludeChars = exclude
		opts.NoAmbiguous = noAmbiguous
		opts.MinUpper = minUpper
		opts.MinLower = minLower
		opts.MinDigits = minDigits
		opts.MinSymbols = minSymbols

		// Parse password type
		switch typeStr {
		case "strong":
			opts.Type = generator.Strong
		case "alphanum":
			opts.Type = generator.Alphanumeric
		case "numbers":
			opts.Type = generator.Numbers
		case "memorable":
			opts.Type = generator.Memorable
		case "passphrase":
			opts.Type = generator.Passphrase
		default:
			return fmt.Errorf("invalid password type: %s", typeStr)
		}

		// Generate passwords
		gen := generator.New(opts)
		passwords, err := gen.Generate()
		if err != nil {
			return fmt.Errorf("failed to generate password: %w", err)
		}

		// Display passwords
		for _, password := range passwords {
			if checkStrength {
				strength := generator.CheckStrength(password)
				fmt.Printf("Password: %s\n", password)
				fmt.Printf("Strength: %s (Score: %d/100)\n", strength.Level, strength.Score)
				fmt.Printf("Length: %d characters\n", strength.Length)
				if len(strength.Feedback) > 0 {
					fmt.Println("Suggestions:")
					for _, feedback := range strength.Feedback {
						fmt.Printf("  - %s\n", feedback)
					}
				}
				fmt.Println()
			} else {
				fmt.Println(password)
			}
		}

		return nil
	},
}

// searchCmd searches for passwords
var searchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Search for services matching pattern",
	Long: `Search for password entries matching the given pattern.

The search is case-insensitive and matches against both service names and usernames.

Examples:
  pwmgr search bank
  pwmgr search gmail
  pwmgr search @company.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern := args[0]

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		entries, err := kr.Search(pattern)
		if err != nil {
			return fmt.Errorf("failed to search passwords: %w", err)
		}

		if len(entries) == 0 {
			fmt.Printf("No entries found matching: %s\n", pattern)
			return nil
		}

		fmt.Printf("Found %d entries matching '%s':\n", len(entries), pattern)
		fmt.Println()
		for _, entry := range entries {
			if entry.Username != "" {
				fmt.Printf("  %s (%s)\n", entry.Service, entry.Username)
			} else {
				fmt.Printf("  %s\n", entry.Service)
			}
		}
		return nil
	},
}
