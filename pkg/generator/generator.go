package generator

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// PasswordType represents different types of passwords
type PasswordType string

const (
	Strong      PasswordType = "strong"
	Alphanumeric PasswordType = "alphanum"
	Numbers     PasswordType = "numbers"
	Memorable   PasswordType = "memorable"
	Passphrase  PasswordType = "passphrase"
)

// Options contains password generation options
type Options struct {
	Length        int
	Type          PasswordType
	Count         int
	Symbols       string
	ExcludeChars  string
	NoAmbiguous   bool
	MinUpper      int
	MinLower      int
	MinDigits     int
	MinSymbols    int
}

// DefaultOptions returns default generation options
func DefaultOptions() *Options {
	return &Options{
		Length:     16,
		Type:       Strong,
		Count:      1,
		Symbols:    "!@#$%^&*()_+-=[]{}|;:,.<>?",
		MinUpper:   1,
		MinLower:   1,
		MinDigits:  1,
		MinSymbols: 1,
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

// Generate generates passwords based on the configured options
func (g *Generator) Generate() ([]string, error) {
	var passwords []string
	
	for i := 0; i < g.options.Count; i++ {
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
	case Strong:
		return g.generateStrong()
	case Alphanumeric:
		return g.generateAlphanumeric()
	case Numbers:
		return g.generateNumbers()
	case Memorable:
		return g.generateMemorable()
	case Passphrase:
		return g.generatePassphrase()
	default:
		return g.generateStrong()
	}
}

func (g *Generator) generateStrong() (string, error) {
	lowercase := "abcdefghijklmnopqrstuvwxyz"
	uppercase := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits := "0123456789"
	symbols := g.options.Symbols
	
	if g.options.NoAmbiguous {
		lowercase = strings.ReplaceAll(lowercase, "l", "")
		uppercase = strings.ReplaceAll(uppercase, "O", "")
		uppercase = strings.ReplaceAll(uppercase, "I", "")
		digits = strings.ReplaceAll(digits, "0", "")
		digits = strings.ReplaceAll(digits, "1", "")
	}
	
	// Remove excluded characters
	if g.options.ExcludeChars != "" {
		for _, char := range g.options.ExcludeChars {
			c := string(char)
			lowercase = strings.ReplaceAll(lowercase, c, "")
			uppercase = strings.ReplaceAll(uppercase, c, "")
			digits = strings.ReplaceAll(digits, c, "")
			symbols = strings.ReplaceAll(symbols, c, "")
		}
	}
	
	var password strings.Builder
	
	// Ensure minimum requirements
	for i := 0; i < g.options.MinLower; i++ {
		char, err := randomChar(lowercase)
		if err != nil {
			return "", err
		}
		password.WriteString(char)
	}
	
	for i := 0; i < g.options.MinUpper; i++ {
		char, err := randomChar(uppercase)
		if err != nil {
			return "", err
		}
		password.WriteString(char)
	}
	
	for i := 0; i < g.options.MinDigits; i++ {
		char, err := randomChar(digits)
		if err != nil {
			return "", err
		}
		password.WriteString(char)
	}
	
	for i := 0; i < g.options.MinSymbols; i++ {
		char, err := randomChar(symbols)
		if err != nil {
			return "", err
		}
		password.WriteString(char)
	}
	
	// Fill remaining length with random characters from all sets
	allChars := lowercase + uppercase + digits + symbols
	remaining := g.options.Length - password.Len()
	
	for i := 0; i < remaining; i++ {
		char, err := randomChar(allChars)
		if err != nil {
			return "", err
		}
		password.WriteString(char)
	}
	
	// Shuffle the password
	return shuffleString(password.String())
}

func (g *Generator) generateAlphanumeric() (string, error) {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	
	if g.options.NoAmbiguous {
		chars = strings.ReplaceAll(chars, "l", "")
		chars = strings.ReplaceAll(chars, "O", "")
		chars = strings.ReplaceAll(chars, "I", "")
		chars = strings.ReplaceAll(chars, "0", "")
		chars = strings.ReplaceAll(chars, "1", "")
	}
	
	return randomString(chars, g.options.Length)
}

func (g *Generator) generateNumbers() (string, error) {
	chars := "0123456789"
	if g.options.NoAmbiguous {
		chars = "23456789"
	}
	return randomString(chars, g.options.Length)
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
func randomChar(chars string) (string, error) {
	if len(chars) == 0 {
		return "", fmt.Errorf("character set is empty")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
	if err != nil {
		return "", err
	}
	return string(chars[n.Int64()]), nil
}

func randomString(chars string, length int) (string, error) {
	var result strings.Builder
	for i := 0; i < length; i++ {
		char, err := randomChar(chars)
		if err != nil {
			return "", err
		}
		result.WriteString(char)
	}
	return result.String(), nil
}

func randomChoice(choices []string) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("choices slice is empty")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(choices))))
	if err != nil {
		return "", err
	}
	return choices[n.Int64()], nil
}

func randomRange(min, max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + min, nil
}

func randomBool() bool {
	n, _ := rand.Int(rand.Reader, big.NewInt(2))
	return n.Int64() == 1
}

func shuffleString(s string) (string, error) {
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		runes[i], runes[j.Int64()] = runes[j.Int64()], runes[i]
	}
	return string(runes), nil
}