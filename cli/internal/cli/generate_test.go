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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	"github.com/spf13/pflag"
)

// runGenerate runs `pwmgr --config <cfgPath> generate <args>` and returns
// the generated passwords (stdout, strength suffix stripped) and stderr.
func runGenerate(t *testing.T, cfgPath string, args ...string) ([]string, string, error) {
	t.Helper()
	// Cobra keeps flag values between Execute calls; start every run clean.
	generateCmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(append([]string{"--config", cfgPath, "generate"}, args...))
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	err := rootCmd.Execute()

	var passwords []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if pw, _, ok := strings.Cut(line, "  (strength: "); ok {
			passwords = append(passwords, pw)
		}
	}
	return passwords, stderr.String(), err
}

// onlyFrom reports whether every character of pw is in allowed.
func onlyFrom(pw, allowed string) bool {
	for _, r := range pw {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

// writeLoggedOutConfig writes a config that points at an unreachable server
// with no session, so any server call would fail.
func writeLoggedOutConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.yaml")
	c := &config.Config{ServerURL: "http://127.0.0.1:9"}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGenerateWorksWithoutLogin(t *testing.T) {
	path := writeLoggedOutConfig(t)
	passwords, stderr, err := runGenerate(t, path, "--symbol-set", "-_", "--length", "30")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, stderr)
	}
	if len(passwords) != 1 {
		t.Fatalf("want 1 password, got %q", passwords)
	}
	pw := passwords[0]
	allowed := generator.LowercaseChars + generator.UppercaseChars + generator.DigitChars + "-_"
	if utf8.RuneCountInString(pw) != 30 || !onlyFrom(pw, allowed) || !strings.ContainsAny(pw, "-_") {
		t.Errorf("unexpected password %q", pw)
	}

	// Without any config file it works too and writes nothing.
	missing := filepath.Join(t.TempDir(), "none.yaml")
	if _, stderr, err := runGenerate(t, missing); err != nil {
		t.Fatalf("generate without config: %v\n%s", err, stderr)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("a plain generate must not create the config file (stat: %v)", err)
	}
}

func TestGenerateFlags(t *testing.T) {
	path := writeLoggedOutConfig(t)
	letters := generator.LowercaseChars + generator.UppercaseChars
	cases := []struct {
		name   string
		args   []string
		count  int
		length int
		check  func(pw string) bool
	}{
		{"defaults", nil, 1, 20, func(pw string) bool {
			return onlyFrom(pw, letters+generator.DigitChars+generator.DefaultSymbolSet)
		}},
		{"positional length", []string{"32"}, 1, 32, func(string) bool { return true }},
		{"count", []string{"-c", "3"}, 3, 20, func(string) bool { return true }},
		{"type numbers", []string{"-t", "numbers", "-l", "12"}, 1, 12, func(pw string) bool {
			return onlyFrom(pw, generator.DigitChars)
		}},
		{"type alphanum", []string{"-t", "alphanum"}, 1, 20, func(pw string) bool {
			return onlyFrom(pw, letters+generator.DigitChars)
		}},
		{"type lower", []string{"-t", "lower"}, 1, 20, func(pw string) bool {
			return onlyFrom(pw, generator.LowercaseChars)
		}},
		{"no-* flags", []string{"--no-upper", "--no-digits", "--no-symbols"}, 1, 20, func(pw string) bool {
			return onlyFrom(pw, generator.LowercaseChars)
		}},
		{"symbol set beats a preset", []string{"-t", "numbers", "--symbol-set", "€"}, 1, 20, func(pw string) bool {
			return onlyFrom(pw, generator.DigitChars+"€") && strings.Contains(pw, "€")
		}},
		{"no ambiguous", []string{"--no-ambiguous", "--symbol-set", "|-", "-l", "128"}, 1, 128, func(pw string) bool {
			return !strings.ContainsAny(pw, generator.AmbiguousChars)
		}},
		{"exclude", []string{"--exclude", "abcABC", "-l", "128"}, 1, 128, func(pw string) bool {
			return !strings.ContainsAny(pw, "abcABC")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			passwords, stderr, err := runGenerate(t, path, c.args...)
			if err != nil {
				t.Fatalf("generate %v: %v\n%s", c.args, err, stderr)
			}
			if len(passwords) != c.count {
				t.Fatalf("got %d passwords, want %d", len(passwords), c.count)
			}
			for _, pw := range passwords {
				if n := utf8.RuneCountInString(pw); n != c.length {
					t.Errorf("length %d, want %d: %q", n, c.length, pw)
				}
				if !c.check(pw) {
					t.Errorf("unexpected characters in %q", pw)
				}
			}
		})
	}
}

func TestGenerateFlagErrors(t *testing.T) {
	path := writeLoggedOutConfig(t)
	cases := []struct {
		name string
		args []string
		want error  // checked with errors.Is when set
		msg  string // otherwise the error must contain this
	}{
		{"no class", []string{"--no-lower", "--no-upper", "--no-digits", "--no-symbols"}, generator.ErrNoClasses, ""},
		{"empty symbol set", []string{"--symbol-set", " "}, generator.ErrEmptyClass, ""},
		{"too short", []string{"--length", "4"}, nil, "between 8 and 128"},
		{"too long", []string{"200"}, nil, "between 8 and 128"},
		{"unknown type", []string{"-t", "bogus"}, nil, "unknown type"},
		{"zero count", []string{"-c", "0"}, nil, "--count"},
		{"save and reset", []string{"--save-defaults", "--reset-defaults"}, nil, "cannot be combined"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := runGenerate(t, path, c.args...)
			switch {
			case err == nil:
				t.Fatalf("generate %v should fail", c.args)
			case c.want != nil && !errors.Is(err, c.want):
				t.Errorf("error = %v, want %v", err, c.want)
			case c.want == nil && !strings.Contains(err.Error(), c.msg):
				t.Errorf("error = %v, want it to mention %q", err, c.msg)
			}
		})
	}
}

func TestGenerateSaveAndResetDefaults(t *testing.T) {
	path := writeLoggedOutConfig(t)

	_, stderr, err := runGenerate(t, path, "--save-defaults", "--length", "30", "--symbol-set", "-_", "--no-upper")
	if err != nil {
		t.Fatalf("save defaults: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "defaults saved") {
		t.Errorf("stderr should confirm the save, got %q", stderr)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := config.DefaultGeneratorSettings()
	want.Length, want.SymbolSet, want.Uppercase = 30, "-_", false
	if saved.Generator == nil || *saved.Generator != want {
		t.Fatalf("saved generator = %+v, want %+v", saved.Generator, want)
	}
	if saved.ServerURL != "http://127.0.0.1:9" {
		t.Errorf("saving defaults lost the server URL: %q", saved.ServerURL)
	}

	// A plain run now uses the saved defaults.
	passwords, _, err := runGenerate(t, path)
	if err != nil {
		t.Fatal(err)
	}
	pw := passwords[0]
	if utf8.RuneCountInString(pw) != 30 || !onlyFrom(pw, generator.LowercaseChars+generator.DigitChars+"-_") {
		t.Errorf("password %q does not follow the saved defaults", pw)
	}

	// --no-upper=false turns a class that the defaults switched off back on,
	// for this run only.
	passwords, _, err = runGenerate(t, path, "--no-upper=false")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsAny(passwords[0], generator.UppercaseChars) {
		t.Errorf("--no-upper=false should bring uppercase back: %q", passwords[0])
	}
	if reloaded, _ := config.Load(path); *reloaded.Generator != want {
		t.Errorf("a run without --save-defaults changed the defaults: %+v", *reloaded.Generator)
	}

	// Invalid options are never saved.
	if _, _, err := runGenerate(t, path, "--save-defaults", "--symbol-set", ""); err == nil {
		t.Fatal("saving an empty symbol set should fail")
	}
	if reloaded, _ := config.Load(path); *reloaded.Generator != want {
		t.Errorf("a failed save changed the defaults: %+v", *reloaded.Generator)
	}

	_, stderr, err = runGenerate(t, path, "--reset-defaults")
	if err != nil {
		t.Fatalf("reset defaults: %v\n%s", err, stderr)
	}
	reset, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Generator != nil {
		t.Errorf("reset should drop the generator block, got %+v", *reset.Generator)
	}
	passwords, _, err = runGenerate(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(passwords[0]); n != generator.DefaultLength {
		t.Errorf("after reset the length should be %d, got %d", generator.DefaultLength, n)
	}
}

// pick runs the add confirm loop on simulated input and returns the chosen
// password, every password it displayed, and the full output.
func pick(t *testing.T, s config.GeneratorSettings, input string) (string, []string, string, error) {
	t.Helper()
	var out bytes.Buffer
	pw, err := pickGeneratedPassword(bufio.NewReader(strings.NewReader(input)), &out, s)
	var shown []string
	for _, line := range strings.Split(out.String(), "\n") {
		if i := strings.Index(line, "Generated: "); i >= 0 {
			rest, _, _ := strings.Cut(line[i+len("Generated: "):], "  (strength: ")
			shown = append(shown, rest)
		}
	}
	return pw, shown, out.String(), err
}

func TestPickGeneratedPassword(t *testing.T) {
	defaults := config.DefaultGeneratorSettings()

	t.Run("enter takes the shown password", func(t *testing.T) {
		pw, shown, out, err := pick(t, defaults, "\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(shown) != 1 || pw != shown[0] {
			t.Fatalf("returned %q, shown %q", pw, shown)
		}
		if !strings.Contains(out, "[Enter] use · r regenerate · o options · q cancel") {
			t.Errorf("prompt missing:\n%s", out)
		}
	})

	t.Run("r regenerates", func(t *testing.T) {
		pw, shown, _, err := pick(t, defaults, "r\nR\n\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(shown) != 3 || pw != shown[2] {
			t.Fatalf("returned %q, shown %q", pw, shown)
		}
		if shown[0] == shown[1] && shown[1] == shown[2] {
			t.Errorf("regenerate showed the same password three times: %q", shown)
		}
	})

	t.Run("o changes length and symbol set once", func(t *testing.T) {
		pw, _, out, err := pick(t, defaults, "o\n30\n-_\n\n")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Length [20]") || !strings.Contains(out, "Symbol set ["+generator.DefaultSymbolSet+"]") {
			t.Errorf("options prompt should show the defaults:\n%s", out)
		}
		allowed := generator.LowercaseChars + generator.UppercaseChars + generator.DigitChars + "-_"
		if utf8.RuneCountInString(pw) != 30 || !onlyFrom(pw, allowed) {
			t.Errorf("password %q ignores the options", pw)
		}
	})

	t.Run("none turns symbols off, empty keeps the length", func(t *testing.T) {
		pw, _, _, err := pick(t, defaults, "o\n\nnone\n\n")
		if err != nil {
			t.Fatal(err)
		}
		allowed := generator.LowercaseChars + generator.UppercaseChars + generator.DigitChars
		if utf8.RuneCountInString(pw) != 20 || !onlyFrom(pw, allowed) {
			t.Errorf("unexpected password %q", pw)
		}
	})

	t.Run("invalid options keep the previous ones", func(t *testing.T) {
		s := defaults
		s.AvoidAmbiguous = true
		// Length 5 is out of range; "|" is empty once ambiguous characters go.
		pw, shown, out, err := pick(t, s, "o\n5\no\n\n|\n\n")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "between 8 and 128") || !strings.Contains(out, "keeping the previous options") {
			t.Errorf("errors should be reported:\n%s", out)
		}
		if len(shown) != 3 || shown[0] != shown[1] || shown[1] != shown[2] || pw != shown[0] {
			t.Errorf("a rejected change must keep the password: returned %q, shown %q", pw, shown)
		}
	})

	t.Run("other input only shows a hint", func(t *testing.T) {
		pw, shown, out, err := pick(t, defaults, "x\n\n")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Press Enter to use") || len(shown) != 2 || pw != shown[0] {
			t.Errorf("returned %q, shown %q\n%s", pw, shown, out)
		}
	})

	for name, input := range map[string]string{"q": "q\n", "end of input": "", "end of input in options": "o\n30\n"} {
		t.Run(name+" cancels", func(t *testing.T) {
			if pw, _, _, err := pick(t, defaults, input); !errors.Is(err, errCancelled) || pw != "" {
				t.Errorf("got %q, %v; want errCancelled", pw, err)
			}
		})
	}
}
