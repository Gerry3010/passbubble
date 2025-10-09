package totp

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateOptions contains options for generating TOTP secrets
type GenerateOptions struct {
	Issuer    string
	AccountName string
	Period    uint
	Digits    otp.Digits
	Algorithm otp.Algorithm
}

// DefaultOptions returns default TOTP generation options
func DefaultOptions() *GenerateOptions {
	return &GenerateOptions{
		Period:    30,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
}

// GenerateSecret generates a new TOTP secret
func GenerateSecret(opts *GenerateOptions) (string, string, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      opts.Issuer,
		AccountName: opts.AccountName,
		Period:      opts.Period,
		Digits:      opts.Digits,
		Algorithm:   opts.Algorithm,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// GenerateCode generates a TOTP code for the given secret
func GenerateCode(secret string, opts *GenerateOptions) (string, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Validate and clean the secret
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	
	// Generate code with custom options
	code, err := totp.GenerateCodeCustom(secret, time.Now(), totp.ValidateOpts{
		Period:    opts.Period,
		Skew:      1,
		Digits:    opts.Digits,
		Algorithm: opts.Algorithm,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP code: %w", err)
	}

	return code, nil
}

// ValidateCode validates a TOTP code against the secret
func ValidateCode(code, secret string, opts *GenerateOptions) bool {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Validate and clean the secret
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	
	valid, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    opts.Period,
		Skew:      1,
		Digits:    opts.Digits,
		Algorithm: opts.Algorithm,
	})
	return valid
}

// ParseTOTPURL parses a TOTP URL (otpauth://) and returns the components
func ParseTOTPURL(otpURL string) (*GenerateOptions, string, error) {
	u, err := url.Parse(otpURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid TOTP URL: %w", err)
	}

	if u.Scheme != "otpauth" || u.Host != "totp" {
		return nil, "", fmt.Errorf("not a valid TOTP URL")
	}

	// Extract account name from path
	accountName := strings.TrimPrefix(u.Path, "/")
	
	// Parse query parameters
	params := u.Query()
	secret := params.Get("secret")
	if secret == "" {
		return nil, "", fmt.Errorf("TOTP URL missing secret")
	}

	opts := DefaultOptions()
	opts.AccountName = accountName
	opts.Issuer = params.Get("issuer")

	if period := params.Get("period"); period != "" {
		if p, err := strconv.ParseUint(period, 10, 32); err == nil {
			opts.Period = uint(p)
		}
	}

	if digits := params.Get("digits"); digits != "" {
		switch digits {
		case "6":
			opts.Digits = otp.DigitsSix
		case "8":
			opts.Digits = otp.DigitsEight
		}
	}

	if algorithm := params.Get("algorithm"); algorithm != "" {
		switch strings.ToUpper(algorithm) {
		case "SHA1":
			opts.Algorithm = otp.AlgorithmSHA1
		case "SHA256":
			opts.Algorithm = otp.AlgorithmSHA256
		case "SHA512":
			opts.Algorithm = otp.AlgorithmSHA512
		case "MD5":
			opts.Algorithm = otp.AlgorithmMD5
		}
	}

	return opts, secret, nil
}

// GenerateRandomSecret generates a random base32 secret
func GenerateRandomSecret() (string, error) {
	bytes := make([]byte, 20) // 160 bits
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}
	
	return base32.StdEncoding.EncodeToString(bytes), nil
}

// GetTimeRemaining returns the seconds remaining until the next TOTP period
func GetTimeRemaining(period uint) int {
	if period == 0 {
		period = 30
	}
	now := time.Now().Unix()
	return int(period - (uint(now) % period))
}

// FormatCode formats a TOTP code with spaces for better readability
func FormatCode(code string) string {
	if len(code) == 6 {
		return fmt.Sprintf("%s %s", code[:3], code[3:])
	} else if len(code) == 8 {
		return fmt.Sprintf("%s %s", code[:4], code[4:])
	}
	return code
}

// IsValidSecret checks if a secret is a valid base32 string
func IsValidSecret(secret string) bool {
	// Clean the secret
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	
	// Empty secrets are not valid
	if secret == "" {
		return false
	}
	
	// Check if it's valid base32
	_, err := base32.StdEncoding.DecodeString(secret)
	return err == nil
}
