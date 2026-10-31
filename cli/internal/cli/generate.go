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
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	rootCmd.AddCommand(generateCmd)

	f := generateCmd.Flags()
	f.IntP("length", "l", 0, "Password length, 8-128 (default: your saved default, initially 20)")
	f.IntP("count", "c", 1, "Number of passwords to generate")
	f.StringP("type", "t", "strong", "Preset: strong (all classes), alphanum (no symbols), numbers (digits only), lower (lowercase only)")
	f.String("symbol-set", "", "Symbols to draw from; also turns symbols on unless --no-symbols is given")
	f.Bool("no-symbols", false, "Leave out symbols")
	f.Bool("no-upper", false, "Leave out uppercase letters")
	f.Bool("no-lower", false, "Leave out lowercase letters")
	f.Bool("no-digits", false, "Leave out digits")
	f.Bool("no-ambiguous", false, "Avoid ambiguous characters (0 O o 1 l I |)")
	f.StringP("exclude", "e", "", "Characters to leave out (this run only, never saved)")
	f.Bool("save-defaults", false, "Save the resulting options as your generator defaults")
	f.Bool("reset-defaults", false, "Reset your generator defaults to the factory settings")
}

var generateCmd = &cobra.Command{
	Use:   "generate [length]",
	Short: "Generate secure passwords",
	Long: `Generate cryptographically secure passwords on this device. Nothing is
sent to the server, so no login is needed.

The options start from your saved generator defaults (factory settings: 20
characters with lowercase, uppercase, digits and the symbols
` + generator.DefaultSymbolSet + `). Flags change them for this run only;
--save-defaults keeps the result as your new defaults. Setting a --no-* flag
to false turns its class back on, e.g. --no-symbols=false.

Examples:
  pwmgr generate
  pwmgr generate 32
  pwmgr generate --symbol-set "-_" --length 30
  pwmgr generate -t alphanum -l 16 -c 5
  pwmgr generate --no-ambiguous --no-symbols
  pwmgr generate --length 24 --symbol-set "-_.!" --save-defaults
  pwmgr generate --reset-defaults`,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmd.Flags()
		save, _ := f.GetBool("save-defaults")
		reset, _ := f.GetBool("reset-defaults")
		if save && reset {
			return fmt.Errorf("--save-defaults and --reset-defaults cannot be combined")
		}
		path := config.ConfigPath(cfgFile)
		stderr := cmd.ErrOrStderr()

		if reset {
			cfg.Generator = nil
			if err := cfg.Save(path); err != nil {
				return fmt.Errorf("reset defaults: %w", err)
			}
			_, _ = fmt.Fprintln(stderr, "✓ Generator defaults reset to the factory settings")
		}

		settings, err := applyGenerateFlags(cfg.GeneratorDefaults(), f, args)
		if err != nil {
			return err
		}
		count, _ := f.GetInt("count")
		if count < 1 {
			return fmt.Errorf("--count must be at least 1")
		}
		exclude, _ := f.GetString("exclude")

		opts := settings.Options()
		opts.Count = count
		opts.ExcludeChars = exclude
		if err := opts.Validate(); err != nil {
			return err
		}

		if save {
			saved := settings
			cfg.Generator = &saved
			if err := cfg.Save(path); err != nil {
				return fmt.Errorf("save defaults: %w", err)
			}
			_, _ = fmt.Fprintf(stderr, "✓ Generator defaults saved: %s\n", settings.Summary())
			if settings.Symbols {
				_, _ = fmt.Fprintf(stderr, "  Symbol set: %s\n", generator.CleanSymbolSet(settings.SymbolSet))
			}
		}

		passwords, err := generator.New(opts).Generate()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, p := range passwords {
			_, _ = fmt.Fprintf(out, "%s  (strength: %d/100)\n", p, generator.CheckStrength(p).Score)
		}
		return nil
	},
}

// applyGenerateFlags applies the generate flags to the saved defaults. The
// --type preset goes first, so explicit class flags override it.
func applyGenerateFlags(s config.GeneratorSettings, f *pflag.FlagSet, args []string) (config.GeneratorSettings, error) {
	if f.Changed("length") {
		s.Length, _ = f.GetInt("length")
	}
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return s, fmt.Errorf("invalid length: %s", args[0])
		}
		s.Length = n
	}
	if (f.Changed("length") || len(args) > 0) && (s.Length < generator.MinLength || s.Length > generator.MaxLength) {
		return s, fmt.Errorf("length must be between %d and %d", generator.MinLength, generator.MaxLength)
	}

	if f.Changed("type") {
		typ, _ := f.GetString("type")
		switch strings.ToLower(typ) {
		case "strong":
			s.Lowercase, s.Uppercase, s.Digits, s.Symbols = true, true, true, true
		case "alphanum":
			s.Lowercase, s.Uppercase, s.Digits, s.Symbols = true, true, true, false
		case "numbers":
			s.Lowercase, s.Uppercase, s.Digits, s.Symbols = false, false, true, false
		case "lower":
			s.Lowercase, s.Uppercase, s.Digits, s.Symbols = true, false, false, false
		default:
			return s, fmt.Errorf("unknown type %q (use strong, alphanum, numbers or lower)", typ)
		}
	}

	if f.Changed("symbol-set") {
		s.SymbolSet, _ = f.GetString("symbol-set")
		s.Symbols = true
	}
	for _, flag := range []struct {
		name  string
		class *bool
	}{
		{"no-symbols", &s.Symbols},
		{"no-upper", &s.Uppercase},
		{"no-lower", &s.Lowercase},
		{"no-digits", &s.Digits},
	} {
		if f.Changed(flag.name) {
			off, _ := f.GetBool(flag.name)
			*flag.class = !off
		}
	}
	if f.Changed("no-ambiguous") {
		s.AvoidAmbiguous, _ = f.GetBool("no-ambiguous")
	}
	return s, nil
}

// errCancelled reports that the user backed out of an interactive prompt.
var errCancelled = errors.New("cancelled")

// pickGeneratedPassword shows a generated password and lets the user take
// it, roll a new one, or change length and symbol set for this one password.
// Only an empty line (Enter) takes it; q or the end of input cancels.
func pickGeneratedPassword(in *bufio.Reader, out io.Writer, s config.GeneratorSettings) (string, error) {
	pw, err := generateOne(s)
	if err != nil {
		return "", fmt.Errorf("generate password: %w (fix your defaults with 'pwmgr generate --reset-defaults')", err)
	}
	for {
		_, _ = fmt.Fprintf(out, "Generated: %s  (strength: %d/100)\n", pw, generator.CheckStrength(pw).Score)
		_, _ = fmt.Fprint(out, "[Enter] use · r regenerate · o options · q cancel: ")
		line, err := readLine(in)
		if err != nil {
			_, _ = fmt.Fprintln(out)
			return "", errCancelled
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return pw, nil
		case "r":
			if pw, err = generateOne(s); err != nil {
				return "", err
			}
		case "o":
			next, err := promptGeneratorOptions(in, out, s)
			if err != nil {
				if errors.Is(err, io.EOF) {
					_, _ = fmt.Fprintln(out)
					return "", errCancelled
				}
				_, _ = fmt.Fprintf(out, "  %v — keeping the previous options\n", err)
				continue
			}
			s = next
			if pw, err = generateOne(s); err != nil {
				return "", err
			}
		case "q":
			return "", errCancelled
		default:
			_, _ = fmt.Fprintln(out, "  Press Enter to use this password, or type r, o or q.")
		}
	}
}

// promptGeneratorOptions asks for length and symbol set, prefilled with s.
// An empty answer keeps the current value.
func promptGeneratorOptions(in *bufio.Reader, out io.Writer, s config.GeneratorSettings) (config.GeneratorSettings, error) {
	_, _ = fmt.Fprintf(out, "  Length [%d]: ", s.Length)
	line, err := readLine(in)
	if err != nil {
		return s, err
	}
	if line = strings.TrimSpace(line); line != "" {
		n, err := strconv.Atoi(line)
		if err != nil || n < generator.MinLength || n > generator.MaxLength {
			return s, fmt.Errorf("length must be a number between %d and %d", generator.MinLength, generator.MaxLength)
		}
		s.Length = n
	}

	_, _ = fmt.Fprintf(out, "  Symbol set [%s] (%q turns symbols off): ", s.SymbolSetInput(), config.SymbolsOffKeyword)
	line, err = readLine(in)
	if err != nil {
		return s, err
	}
	if strings.TrimSpace(line) != "" {
		s.ApplySymbolSetInput(line)
	}
	return s, s.Validate()
}

// readLine reads one line without its line ending. A last line without a
// newline still counts; io.EOF comes only once nothing is left.
func readLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// generateOne returns a single strong password for the given settings.
func generateOne(s config.GeneratorSettings) (string, error) {
	passwords, err := generator.New(s.Options()).Generate()
	if err != nil {
		return "", err
	}
	return passwords[0], nil
}
