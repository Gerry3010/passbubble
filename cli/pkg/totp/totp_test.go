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

package totp

import (
	"strings"
	"testing"

	"github.com/pquerna/otp"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Period != 30 {
		t.Errorf("Expected default period 30, got %d", opts.Period)
	}
	if opts.Digits != otp.DigitsSix {
		t.Errorf("Expected default digits 6, got %d", opts.Digits)
	}
	if opts.Algorithm != otp.AlgorithmSHA1 {
		t.Errorf("Expected default algorithm SHA1, got %v", opts.Algorithm)
	}
}

func TestGenerateSecret(t *testing.T) {
	opts := &GenerateOptions{
		Issuer:      "Test Issuer",
		AccountName: "test@example.com",
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	}

	secret, url, err := GenerateSecret(opts)
	if err != nil {
		t.Fatalf("Failed to generate secret: %v", err)
	}

	if !IsValidSecret(secret) {
		t.Error("Generated secret is not a valid base32 string")
	}

	expectedParts := []string{
		"otpauth://totp/",
		"Test%20Issuer:test@example.com",
		"secret=" + secret,
		"issuer=Test%20Issuer",
		"period=30",
		"digits=6",
		"algorithm=SHA1",
	}

	for _, part := range expectedParts {
		if part != "" && !contains(url, part) {
			t.Errorf("Expected URL to contain '%s', URL: %s", part, url)
		}
	}
}

func TestGenerateAndValidateCode(t *testing.T) {
	secret, err := GenerateRandomSecret()
	if err != nil {
		t.Fatalf("Failed to generate random secret: %v", err)
	}
	opts := &GenerateOptions{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	code, err := GenerateCode(secret, opts)
	if err != nil {
		t.Fatalf("Failed to generate code: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("Expected 6-digit code, got %s", code)
	}
	if !ValidateCode(code, secret, opts) {
		t.Error("Generated code did not validate")
	}
}

func TestParseTOTPURL(t *testing.T) {
	testCases := []struct {
		url           string
		expectError   bool
		expectOptions *GenerateOptions
		expectSecret  string
	}{
		{
			url: "otpauth://totp/Example:test@example.com?secret=JBSWY3DPEHPK3PXP&issuer=Example&period=30&digits=6&algorithm=SHA1",
			expectOptions: &GenerateOptions{
				Issuer:      "Example",
				AccountName: "Example:test@example.com",
				Period:      30,
				Digits:      otp.DigitsSix,
				Algorithm:   otp.AlgorithmSHA1,
			},
			expectSecret: "JBSWY3DPEHPK3PXP",
		},
		{
			url: "otpauth://totp/Test:user?secret=JBSWY3DPEHPK3PXP&digits=8&algorithm=SHA256",
			expectOptions: &GenerateOptions{
				Issuer:      "", // no issuer parameter provided
				AccountName: "Test:user",
				Period:      30, // default
				Digits:      otp.DigitsEight,
				Algorithm:   otp.AlgorithmSHA256,
			},
			expectSecret: "JBSWY3DPEHPK3PXP",
		},
		{
			url:         "invalid://url",
			expectError: true,
		},
		{
			url:         "otpauth://totp/Test?issuer=Test", // missing secret
			expectError: true,
		},
	}

	for _, tc := range testCases {
		opts, secret, err := ParseTOTPURL(tc.url)

		if tc.expectError {
			if err == nil {
				t.Errorf("Expected error parsing URL: %s", tc.url)
			}
			continue
		}

		if err != nil {
			t.Errorf("Failed to parse URL %s: %v", tc.url, err)
			continue
		}

		if secret != tc.expectSecret {
			t.Errorf("Expected secret %s, got %s", tc.expectSecret, secret)
		}

		if opts.Issuer != tc.expectOptions.Issuer {
			t.Errorf("Expected issuer %s, got %s", tc.expectOptions.Issuer, opts.Issuer)
		}
		if opts.AccountName != tc.expectOptions.AccountName {
			t.Errorf("Expected account name %s, got %s", tc.expectOptions.AccountName, opts.AccountName)
		}
		if opts.Period != tc.expectOptions.Period {
			t.Errorf("Expected period %d, got %d", tc.expectOptions.Period, opts.Period)
		}
		if opts.Digits != tc.expectOptions.Digits {
			t.Errorf("Expected digits %v, got %v", tc.expectOptions.Digits, opts.Digits)
		}
		if opts.Algorithm != tc.expectOptions.Algorithm {
			t.Errorf("Expected algorithm %v, got %v", tc.expectOptions.Algorithm, opts.Algorithm)
		}
	}
}

func TestGenerateRandomSecret(t *testing.T) {
	// Generate multiple secrets and verify they're unique and valid
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		secret, err := GenerateRandomSecret()
		if err != nil {
			t.Fatalf("Failed to generate random secret: %v", err)
		}

		if !IsValidSecret(secret) {
			t.Errorf("Generated invalid base32 secret: %s", secret)
		}

		if seen[secret] {
			t.Errorf("Generated duplicate secret: %s", secret)
		}
		seen[secret] = true
	}
}

func TestGetTimeRemaining(t *testing.T) {
	period := uint(30)
	// Time remaining should be between 0 and period-1
	remaining := GetTimeRemaining(period)
	if remaining < 0 || remaining >= int(period) {
		t.Errorf("Time remaining %d is outside valid range 0-%d", remaining, period-1)
	}
}

func TestFormatCode(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"123456", "123 456"},
		{"12345678", "1234 5678"},
		{"12345", "12345"},         // too short, no formatting
		{"123456789", "123456789"}, // too long, no formatting
	}

	for _, tc := range testCases {
		result := FormatCode(tc.input)
		if result != tc.expected {
			t.Errorf("Expected code format %s, got %s", tc.expected, result)
		}
	}
}

func TestIsValidSecret(t *testing.T) {
	testCases := []struct {
		secret string
		valid  bool
	}{
		{"JBSWY3DPEHPK3PXP", true}, // valid base32
		{"MZXW6YTBOI======", true}, // valid base32 with padding
		{"12345", false},           // not base32
		{"!@#$%", false},           // not base32
		{"", false},                // empty
		{"JBSWY3DPEHPK3PXP", true}, // spaces cleaned in function
		{"jbswy3dpehpk3pxp", true}, // case insensitive
	}

	for _, tc := range testCases {
		valid := IsValidSecret(tc.secret)
		if valid != tc.valid {
			t.Errorf("Expected IsValidSecret(%s) to be %v", tc.secret, tc.valid)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
