package generator

import (
	"strings"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Length != 16 {
		t.Errorf("Expected default length 16, got %d", opts.Length)
	}
	if opts.Type != Strong {
		t.Errorf("Expected default type Strong, got %v", opts.Type)
	}
	if opts.Count != 1 {
		t.Errorf("Expected default count 1, got %d", opts.Count)
	}
}

func TestGenerateStrong(t *testing.T) {
	opts := &Options{
		Length:     16,
		Type:       Strong,
		Count:      1,
		Symbols:    "!@#$%^&*()_+-=[]{}|;:,.<>?",
		MinUpper:   1,
		MinLower:   1,
		MinDigits:  1,
		MinSymbols: 1,
	}
	
	gen := New(opts)
	passwords, err := gen.Generate()
	
	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}
	
	if len(passwords) != 1 {
		t.Errorf("Expected 1 password, got %d", len(passwords))
	}
	
	password := passwords[0]
	if len(password) != 16 {
		t.Errorf("Expected password length 16, got %d", len(password))
	}
	
	// Check character requirements
	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSymbol := false
	
	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case strings.ContainsRune(opts.Symbols, char):
			hasSymbol = true
		}
	}
	
	if !hasUpper {
		t.Error("Password should contain uppercase letters")
	}
	if !hasLower {
		t.Error("Password should contain lowercase letters")
	}
	if !hasDigit {
		t.Error("Password should contain digits")
	}
	if !hasSymbol {
		t.Error("Password should contain symbols")
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
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
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
	opts := &Options{
		Length: 10,
		Type:   Strong,
		Count:  5,
	}
	
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
		{"12345678", 0, 35},         // Very weak
		{"password", 15, 50},        // Weak to moderate
		{"Password1", 35, 65},       // Weak to strong
		{"Password1!", 65, 85},      // Strong to very strong
		{"P@ssw0rd!2023", 80, 100},  // Very strong
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
	
	ambiguous := "0O1lI"
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