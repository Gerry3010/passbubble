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

package generator

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Length != 20 {
		t.Errorf("Expected default length 20, got %d", opts.Length)
	}
	if opts.Type != Strong {
		t.Errorf("Expected default type Strong, got %v", opts.Type)
	}
	if opts.Count != 1 {
		t.Errorf("Expected default count 1, got %d", opts.Count)
	}
	if !opts.Lowercase || !opts.Uppercase || !opts.Digits || !opts.Symbols {
		t.Errorf("all four classes should be on by default: %+v", opts)
	}
	if opts.SymbolSet != "!@#$%^&*()_+-=[]{}|;:,.<>?" {
		t.Errorf("default symbol set = %q", opts.SymbolSet)
	}
	if opts.NoAmbiguous {
		t.Error("avoid ambiguous should be off by default")
	}
}

// strongOpts returns the default options with a custom length and count.
func strongOpts(length, count int) *Options {
	o := DefaultOptions()
	o.Length = length
	o.Count = count
	return o
}

// checkPassword asserts the shared-spec properties of one strong password:
// length in code points, only allowed characters, every class represented.
func checkPassword(t *testing.T, o *Options, pw string) {
	t.Helper()
	if !utf8.ValidString(pw) {
		t.Fatalf("password is not valid UTF-8: %q", pw)
	}
	if n := utf8.RuneCountInString(pw); n != o.Length {
		t.Errorf("length = %d code points, want %d (%q)", n, o.Length, pw)
	}

	symbols := CleanSymbolSet(o.SymbolSet)
	removed := o.ExcludeChars
	if o.NoAmbiguous {
		removed += AmbiguousChars
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range pw {
		if strings.ContainsRune(removed, r) {
			t.Errorf("password contains removed character %q: %q", r, pw)
		}
		switch {
		case o.Lowercase && strings.ContainsRune(LowercaseChars, r):
			hasLower = true
		case o.Uppercase && strings.ContainsRune(UppercaseChars, r):
			hasUpper = true
		case o.Digits && strings.ContainsRune(DigitChars, r):
			hasDigit = true
		case o.Symbols && strings.ContainsRune(symbols, r):
			hasSymbol = true
		default:
			t.Errorf("password contains a character outside the enabled classes %q: %q", r, pw)
		}
	}
	if o.Lowercase && !hasLower {
		t.Errorf("no lowercase letter in %q", pw)
	}
	if o.Uppercase && !hasUpper {
		t.Errorf("no uppercase letter in %q", pw)
	}
	if o.Digits && !hasDigit {
		t.Errorf("no digit in %q", pw)
	}
	if o.Symbols && !hasSymbol {
		t.Errorf("no symbol in %q", pw)
	}
}

// TestStrongSpec covers the generator spec shared with the app and the
// extension.
func TestStrongSpec(t *testing.T) {
	cases := []struct {
		name string
		mod  func(o *Options)
	}{
		{"defaults", func(o *Options) {}},
		{"minimum length equals class count", func(o *Options) { o.Length = 4 }},
		{"long", func(o *Options) { o.Length = 128 }},
		{"custom symbol set", func(o *Options) { o.SymbolSet = "-_" }},
		{"only custom symbols", func(o *Options) {
			o.Lowercase, o.Uppercase, o.Digits = false, false, false
			o.SymbolSet = "-_"
			o.Length = 30
		}},
		{"unicode symbols stay whole", func(o *Options) { o.SymbolSet = "€§ä🔑" }},
		{"symbol set is cleaned", func(o *Options) { o.SymbolSet = " ! !\t\n\x00!" }},
		{"no upper, no digits", func(o *Options) { o.Uppercase, o.Digits = false, false }},
		{"digits only", func(o *Options) {
			o.Lowercase, o.Uppercase, o.Symbols = false, false, false
		}},
		{"avoid ambiguous includes symbols", func(o *Options) {
			o.NoAmbiguous = true
			o.SymbolSet = "|-"
		}},
		{"avoid ambiguous and exclude", func(o *Options) {
			o.NoAmbiguous = true
			o.ExcludeChars = "aeiouAEIOU-€"
			o.SymbolSet = "-_€"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := strongOpts(20, 50)
			c.mod(o)
			passwords, err := New(o).Generate()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if len(passwords) != 50 {
				t.Fatalf("got %d passwords, want 50", len(passwords))
			}
			for _, pw := range passwords {
				checkPassword(t, o, pw)
			}
		})
	}
}

func TestStrongErrors(t *testing.T) {
	cases := []struct {
		name string
		mod  func(o *Options)
		want error
	}{
		{"no class", func(o *Options) {
			o.Lowercase, o.Uppercase, o.Digits, o.Symbols = false, false, false, false
		}, ErrNoClasses},
		{"empty symbol set", func(o *Options) { o.SymbolSet = "" }, ErrEmptyClass},
		{"whitespace-only symbol set", func(o *Options) { o.SymbolSet = " \t\n" }, ErrEmptyClass},
		{"symbol set emptied by avoid ambiguous", func(o *Options) {
			o.SymbolSet = "|"
			o.NoAmbiguous = true
		}, ErrEmptyClass},
		{"class emptied by exclude", func(o *Options) { o.ExcludeChars = DigitChars }, ErrEmptyClass},
		{"length below class count", func(o *Options) { o.Length = 3 }, ErrTooShort},
		{"zero length", func(o *Options) {
			o.Length = 0
			o.Uppercase, o.Digits, o.Symbols = false, false, false
		}, ErrTooShort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := strongOpts(20, 1)
			c.mod(o)
			if err := o.Validate(); !errors.Is(err, c.want) {
				t.Errorf("Validate() = %v, want %v", err, c.want)
			}
			if _, err := New(o).Generate(); !errors.Is(err, c.want) {
				t.Errorf("Generate() = %v, want %v", err, c.want)
			}
		})
	}

	// An empty symbol set is fine while symbols are off.
	o := strongOpts(20, 1)
	o.Symbols, o.SymbolSet = false, ""
	if err := o.Validate(); err != nil {
		t.Errorf("symbols off with empty set: %v", err)
	}
}

func TestEveryCharacterEventuallyAppears(t *testing.T) {
	check := func(t *testing.T, o *Options, want string) {
		t.Helper()
		seen := map[rune]bool{}
		passwords, err := New(o).Generate()
		if err != nil {
			t.Fatal(err)
		}
		for _, pw := range passwords {
			for _, r := range pw {
				seen[r] = true
			}
		}
		for _, r := range want {
			if !seen[r] {
				t.Errorf("character %q never appeared", r)
			}
		}
	}

	t.Run("custom unicode set", func(t *testing.T) {
		o := strongOpts(8, 300)
		o.Lowercase, o.Uppercase, o.Digits = false, false, false
		o.SymbolSet = "-_€🔑"
		check(t, o, "-_€🔑")
	})
	t.Run("defaults", func(t *testing.T) {
		o := strongOpts(20, 1000)
		check(t, o, LowercaseChars+UppercaseChars+DigitChars+DefaultSymbolSet)
	})
}

func TestCleanSymbolSet(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"-_":                   "-_",
		" !!a\t€€\x00b\u00a0 ": "!a€b",
		"!@\xff#":              "!@#", // invalid UTF-8 byte
		"🔑🔑ä":                  "🔑ä",
	}
	for in, want := range cases {
		if got := CleanSymbolSet(in); got != want {
			t.Errorf("CleanSymbolSet(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateStrong(t *testing.T) {
	opts := &Options{
		Length:    16,
		Type:      Strong,
		Count:     1,
		Lowercase: true,
		Uppercase: true,
		Digits:    true,
		Symbols:   true,
		SymbolSet: "!@#$%^&*()_+-=[]{}|;:,.<>?",
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	if len(passwords) != 1 {
		t.Errorf("Expected 1 password, got %d", len(passwords))
	}
	checkPassword(t, opts, passwords[0])
}

// TestStrongNoAmbiguousAndExclude checks that both filters apply to every
// class of the strong generator, the symbol set included.
func TestStrongNoAmbiguousAndExclude(t *testing.T) {
	opts := strongOpts(40, 50)
	opts.NoAmbiguous = true
	opts.ExcludeChars = "xyzXYZ789#$"

	passwords, err := New(opts).Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, pw := range passwords {
		if strings.ContainsAny(pw, AmbiguousChars+opts.ExcludeChars) {
			t.Errorf("password contains an ambiguous or excluded character: %q", pw)
		}
		checkPassword(t, opts, pw)
	}
}

func TestGenerateAlphanumeric(t *testing.T) {
	opts := &Options{
		Length: 12,
		Type:   Alphanumeric,
		Count:  1,
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	password := passwords[0]
	if len(password) != 12 {
		t.Errorf("Expected password length 12, got %d", len(password))
	}

	// Should only contain letters and numbers
	for _, char := range password {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			t.Errorf("Alphanumeric password contains invalid character: %c", char)
		}
	}
}

func TestGenerateNumbers(t *testing.T) {
	opts := &Options{
		Length: 8,
		Type:   Numbers,
		Count:  1,
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	password := passwords[0]
	if len(password) != 8 {
		t.Errorf("Expected password length 8, got %d", len(password))
	}

	// Should only contain digits
	for _, char := range password {
		if char < '0' || char > '9' {
			t.Errorf("Numeric password contains invalid character: %c", char)
		}
	}
}

func TestGeneratePassphrase(t *testing.T) {
	opts := &Options{
		Length: 16,
		Type:   Passphrase,
		Count:  1,
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	password := passwords[0]
	if len(password) < 10 {
		t.Errorf("Passphrase too short: %s", password)
	}

	// Should contain separators and end with digits
	hasSeparator := strings.Contains(password, "-") || strings.Contains(password, "_") ||
		strings.Contains(password, ".") || strings.Contains(password, "+")

	if !hasSeparator {
		t.Error("Passphrase should contain separator characters")
	}
}

func TestGenerateMultiple(t *testing.T) {
	opts := strongOpts(10, 5)

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate passwords: %v", err)
	}

	if len(passwords) != 5 {
		t.Errorf("Expected 5 passwords, got %d", len(passwords))
	}

	// All passwords should be different
	seen := make(map[string]bool)
	for _, password := range passwords {
		if seen[password] {
			t.Errorf("Duplicate password generated: %s", password)
		}
		seen[password] = true

		if len(password) != 10 {
			t.Errorf("Expected password length 10, got %d for password: %s", len(password), password)
		}
	}
}

func TestCheckStrength(t *testing.T) {
	testCases := []struct {
		password string
		minScore int
		maxScore int
	}{
		{"12345678", 0, 35},        // Very weak
		{"password", 15, 50},       // Weak to moderate
		{"Password1", 35, 65},      // Weak to strong
		{"Password1!", 65, 85},     // Strong to very strong
		{"P@ssw0rd!2023", 80, 100}, // Very strong
	}

	for _, tc := range testCases {
		result := CheckStrength(tc.password)

		if result.Password != tc.password {
			t.Errorf("Expected password %s, got %s", tc.password, result.Password)
		}

		if result.Score < tc.minScore || result.Score > tc.maxScore {
			t.Errorf("Password %s: expected score between %d-%d, got %d",
				tc.password, tc.minScore, tc.maxScore, result.Score)
		}

		if result.Length != len(tc.password) {
			t.Errorf("Expected length %d, got %d", len(tc.password), result.Length)
		}
	}
}

func TestNoAmbiguous(t *testing.T) {
	opts := &Options{
		Length:      20,
		Type:        Alphanumeric,
		Count:       10,
		NoAmbiguous: true,
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate passwords: %v", err)
	}

	ambiguous := "0Oo1lI"
	for _, password := range passwords {
		for _, char := range ambiguous {
			if strings.ContainsRune(password, char) {
				t.Errorf("Password contains ambiguous character '%c': %s", char, password)
			}
		}
	}
}

func TestExcludeChars(t *testing.T) {
	excluded := "aeiou"
	opts := &Options{
		Length:       16,
		Type:         Alphanumeric,
		Count:        5,
		ExcludeChars: excluded,
	}

	gen := New(opts)
	passwords, err := gen.Generate()

	if err != nil {
		t.Fatalf("Failed to generate passwords: %v", err)
	}

	for _, password := range passwords {
		for _, char := range excluded {
			if strings.ContainsRune(password, char) {
				t.Errorf("Password contains excluded character '%c': %s", char, password)
			}
		}
	}
}
