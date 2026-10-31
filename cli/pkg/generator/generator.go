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
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PasswordType represents different types of passwords
type PasswordType string

const (
	Strong       PasswordType = "strong"
	Alphanumeric PasswordType = "alphanum"
	Numbers      PasswordType = "numbers"
	Memorable    PasswordType = "memorable"
	Passphrase   PasswordType = "passphrase"
)

// Character classes and defaults of the strong generator. They are part of
// the generator spec shared with the app and the browser extension; keep
// them identical there.
const (
	LowercaseChars = "abcdefghijklmnopqrstuvwxyz"
	UppercaseChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	DigitChars     = "0123456789"
	// DefaultSymbolSet is the symbol class unless the user picks another one.
	DefaultSymbolSet = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	// AmbiguousChars are removed from every class when NoAmbiguous is set.
	AmbiguousChars = "0Oo1lI|"

	DefaultLength = 20
	// MinLength and MaxLength bound the length users can pick in the UI. The
	// generator itself only requires one character per enabled class.
	MinLength = 8
	MaxLength = 128
)

// Errors of the strong generator. Callers show them instead of silently
// falling back to other characters.
var (
	ErrNoClasses  = errors.New("enable at least one character class")
	ErrEmptyClass = errors.New("an enabled character class has no characters left")
	ErrTooShort   = errors.New("length is shorter than the number of enabled character classes")
)

// Options contains password generation options
type Options struct {
	Length int
	Type   PasswordType
	Count  int

	// Character classes of the strong generator. Every enabled class
	// contributes at least one character.
	Lowercase bool
	Uppercase bool
	Digits    bool
	Symbols   bool
	// SymbolSet is the symbol class. It is cleaned before use (see
	// CleanSymbolSet), so it may contain any Unicode characters.
	SymbolSet string

	ExcludeChars string // removed from every class
	NoAmbiguous  bool   // removes AmbiguousChars from every class
}

// DefaultOptions returns default generation options
func DefaultOptions() *Options {
	return &Options{
		Length:    DefaultLength,
		Type:      Strong,
		Count:     1,
		Lowercase: true,
		Uppercase: true,
		Digits:    true,
		Symbols:   true,
		SymbolSet: DefaultSymbolSet,
	}
}

// Generator provides password generation functionality
type Generator struct {
	options *Options
}

// New creates a new password generator
func New(opts *Options) *Generator {
	if opts == nil {
		opts = DefaultOptions()
	}
	return &Generator{options: opts}
}

// Generate generates passwords based on the configured options. A Count
// below one yields a single password.
func (g *Generator) Generate() ([]string, error) {
	count := g.options.Count
	if count < 1 {
		count = 1
	}
	passwords := make([]string, 0, count)

	for i := 0; i < count; i++ {
		password, err := g.generateSingle()
		if err != nil {
			return nil, fmt.Errorf("failed to generate password: %w", err)
		}
		passwords = append(passwords, password)
	}

	return passwords, nil
}

func (g *Generator) generateSingle() (string, error) {
	switch g.options.Type {
	case Alphanumeric:
		opts := *g.options
		opts.Lowercase, opts.Uppercase, opts.Digits, opts.Symbols = true, true, true, false
		return generateStrong(&opts)
	case Numbers:
		opts := *g.options
		opts.Lowercase, opts.Uppercase, opts.Digits, opts.Symbols = false, false, true, false
		return generateStrong(&opts)
	case Memorable:
		return g.generateMemorable()
	case Passphrase:
		return g.generatePassphrase()
	default:
		return generateStrong(g.options)
	}
}

// Validate reports whether the strong generator can produce a password with
// these options. It returns ErrNoClasses, ErrEmptyClass or ErrTooShort.
func (o *Options) Validate() error {
	_, err := o.classes()
	return err
}

// CleanSymbolSet drops whitespace, control characters and repeated
// characters from a symbol set, keeping the first occurrence of each. It
// works on code points, so multi-byte characters stay whole.
func CleanSymbolSet(set string) string {
	seen := make(map[rune]bool)
	var b strings.Builder
	for i := 0; i < len(set); {
		r, size := utf8.DecodeRuneInString(set[i:])
		i += size
		if r == utf8.RuneError && size == 1 {
			continue // invalid UTF-8 byte
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) || seen[r] {
			continue
		}
		seen[r] = true
		b.WriteRune(r)
	}
	return b.String()
}

// classes returns the enabled character classes after cleaning and
// filtering, in the order lowercase, uppercase, digits, symbols.
func (o *Options) classes() ([][]rune, error) {
	type class struct {
		name    string
		enabled bool
		chars   string
	}
	all := []class{
		{"lowercase", o.Lowercase, LowercaseChars},
		{"uppercase", o.Uppercase, UppercaseChars},
		{"digits", o.Digits, DigitChars},
		{"symbols (the symbol set has no usable characters)", o.Symbols, CleanSymbolSet(o.SymbolSet)},
	}

	removed := o.ExcludeChars
	if o.NoAmbiguous {
		removed += AmbiguousChars
	}

	var out [][]rune
	for _, c := range all {
		if !c.enabled {
			continue
		}
		var chars []rune
		for _, r := range c.chars {
			if !strings.ContainsRune(removed, r) {
				chars = append(chars, r)
			}
		}
		if len(chars) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrEmptyClass, c.name)
		}
		out = append(out, chars)
	}

	if len(out) == 0 {
		return nil, ErrNoClasses
	}
	if o.Length < len(out) {
		return nil, fmt.Errorf("%w (length %d, %d classes)", ErrTooShort, o.Length, len(out))
	}
	return out, nil
}

// generateStrong draws one character from every enabled class, fills up to
// the length from the union of the classes, and shuffles the result.
func generateStrong(o *Options) (string, error) {
	classes, err := o.classes()
	if err != nil {
		return "", err
	}

	password := make([]rune, 0, o.Length)
	var union []rune
	seen := make(map[rune]bool)
	for _, class := range classes {
		r, err := randomRune(class)
		if err != nil {
			return "", err
		}
		password = append(password, r)

		// A custom symbol set may repeat letters or digits; count each
		// character once so the fill stays uniform over distinct characters.
		for _, c := range class {
			if !seen[c] {
				seen[c] = true
				union = append(union, c)
			}
		}
	}

	for len(password) < o.Length {
		r, err := randomRune(union)
		if err != nil {
			return "", err
		}
		password = append(password, r)
	}

	if err := shuffleRunes(password); err != nil {
		return "", err
	}
	return string(password), nil
}

func (g *Generator) generateMemorable() (string, error) {
	vowels := "aeiou"
	consonants := "bcdfghjklmnpqrstvwxyz"
	var password strings.Builder
	useConsonant := true

	for i := 0; i < g.options.Length; i++ {
		var char string
		var err error

		if useConsonant {
			char, err = randomChar(consonants)
		} else {
			char, err = randomChar(vowels)
		}

		if err != nil {
			return "", err
		}

		// Randomly capitalize some letters
		if randomBool() {
			char = strings.ToUpper(char)
		}

		password.WriteString(char)
		useConsonant = !useConsonant

		// Randomly insert digits
		if i > 0 && i < g.options.Length-1 && randomBool() {
			digit, err := randomChar("23456789")
			if err != nil {
				return "", err
			}
			password.WriteString(digit)
		}
	}

	result := password.String()
	if len(result) > g.options.Length {
		result = result[:g.options.Length]
	}

	return result, nil
}

func (g *Generator) generatePassphrase() (string, error) {
	words := []string{
		"correct", "horse", "battery", "staple", "dragon", "monkey", "pizza",
		"guitar", "rainbow", "thunder", "mountain", "river", "ocean", "forest",
		"castle", "wizard", "knight", "princess", "treasure", "adventure",
		"mystery", "journey", "courage", "freedom", "wisdom", "strength",
		"harmony", "balance", "energy", "spirit", "magic", "wonder",
	}

	wordCount := 4
	if g.options.Length >= 20 {
		wordCount = 5
	} else if g.options.Length <= 12 {
		wordCount = 3
	}

	var selectedWords []string
	for i := 0; i < wordCount; i++ {
		word, err := randomChoice(words)
		if err != nil {
			return "", err
		}

		// Randomly capitalize
		if randomBool() {
			if len(word) > 0 {
				word = strings.ToUpper(word[:1]) + word[1:]
			}
		}

		selectedWords = append(selectedWords, word)
	}

	// Join with random separators
	separators := []string{"-", "_", ".", "+"}
	separator, err := randomChoice(separators)
	if err != nil {
		return "", err
	}

	result := strings.Join(selectedWords, separator)

	// Add a random number at the end
	num, err := randomRange(10, 999)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%d", result, num), nil
}

// CheckStrength analyzes password strength
func CheckStrength(password string) *StrengthResult {
	result := &StrengthResult{
		Password: password,
		Length:   len(password),
	}

	score := 0

	// Length scoring
	if result.Length >= 12 {
		score += 25
	} else if result.Length >= 8 {
		score += 15
	} else {
		result.Feedback = append(result.Feedback, "Password is too short (minimum 8 characters)")
	}

	// Character variety
	if regexp.MustCompile(`[a-z]`).MatchString(password) {
		score += 15
		result.HasLower = true
	} else {
		result.Feedback = append(result.Feedback, "Add lowercase letters")
	}

	if regexp.MustCompile(`[A-Z]`).MatchString(password) {
		score += 15
		result.HasUpper = true
	} else {
		result.Feedback = append(result.Feedback, "Add uppercase letters")
	}

	if regexp.MustCompile(`[0-9]`).MatchString(password) {
		score += 15
		result.HasDigits = true
	} else {
		result.Feedback = append(result.Feedback, "Add numbers")
	}

	if regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) {
		score += 20
		result.HasSymbols = true
	} else {
		result.Feedback = append(result.Feedback, "Add symbols")
	}

	// Pattern penalties - check for repeated characters
	hasRepeated := false
	for i := 0; i < len(password)-2; i++ {
		if password[i] == password[i+1] && password[i+1] == password[i+2] {
			hasRepeated = true
			break
		}
	}
	if hasRepeated {
		score -= 10
		result.Feedback = append(result.Feedback, "Avoid repeated characters")
	}

	if regexp.MustCompile(`(abc|bcd|cde|def|efg|fgh|ghi|hij|ijk|jkl|klm|lmn|mno|nop|opq|pqr|qrs|rst|stu|tuv|uvw|vwx|wxy|xyz)`).MatchString(strings.ToLower(password)) ||
		regexp.MustCompile(`(012|123|234|345|456|567|678|789)`).MatchString(password) {
		score -= 15
		result.Feedback = append(result.Feedback, "Avoid sequential characters")
	}

	result.Score = score

	// Determine strength level
	if score >= 80 {
		result.Level = "Very Strong"
	} else if score >= 65 {
		result.Level = "Strong"
	} else if score >= 50 {
		result.Level = "Moderate"
	} else if score >= 35 {
		result.Level = "Weak"
	} else {
		result.Level = "Very Weak"
	}

	return result
}

// StrengthResult contains password strength analysis
type StrengthResult struct {
	Password   string   `json:"password"`
	Score      int      `json:"score"`
	Level      string   `json:"level"`
	Length     int      `json:"length"`
	HasLower   bool     `json:"has_lower"`
	HasUpper   bool     `json:"has_upper"`
	HasDigits  bool     `json:"has_digits"`
	HasSymbols bool     `json:"has_symbols"`
	Feedback   []string `json:"feedback"`
}

// Helper functions

// randomIndex returns a uniformly distributed index in [0, n) from
// crypto/rand. rand.Int rejection-samples, so there is no modulo bias.
func randomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("character set is empty")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func randomRune(chars []rune) (rune, error) {
	i, err := randomIndex(len(chars))
	if err != nil {
		return 0, err
	}
	return chars[i], nil
}

// randomChar picks one character (code point) of chars.
func randomChar(chars string) (string, error) {
	r, err := randomRune([]rune(chars))
	if err != nil {
		return "", err
	}
	return string(r), nil
}

func randomChoice(choices []string) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("choices slice is empty")
	}
	i, err := randomIndex(len(choices))
	if err != nil {
		return "", err
	}
	return choices[i], nil
}

func randomRange(min, max int) (int, error) {
	n, err := randomIndex(max - min + 1)
	if err != nil {
		return 0, err
	}
	return n + min, nil
}

func randomBool() bool {
	n, _ := rand.Int(rand.Reader, big.NewInt(2))
	return n.Int64() == 1
}

// shuffleRunes shuffles in place (Fisher-Yates).
func shuffleRunes(runes []rune) error {
	for i := len(runes) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return err
		}
		runes[i], runes[j] = runes[j], runes[i]
	}
	return nil
}
