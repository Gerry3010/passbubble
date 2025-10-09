package keyring

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// KeyringInterface defines the interface for keyring operations
type KeyringInterface interface {
	Store(service, username, password string) error
	StoreEntry(entry Entry) error
	Get(service, username string) (string, error)
	GetEntry(service, username string) (*Entry, error)
	Delete(service, username string) error
	List() ([]Entry, error)
	Search(pattern string) ([]Entry, error)
	IsAvailable() bool
}

// SecretType represents the type of secret stored
type SecretType string

const (
	SecretTypePassword SecretType = "password"
	SecretTypeTOTP     SecretType = "totp"
	SecretTypeNote     SecretType = "note"
	SecretTypeAPIKey   SecretType = "api-key"
	SecretTypeSSHKey   SecretType = "ssh-key"
	SecretTypeCert     SecretType = "certificate"
)

// Entry represents a secret entry
type Entry struct {
	Service    string     `json:"service"`
	Username   string     `json:"username,omitempty"`
	Password   string     `json:"password"` // For password type or TOTP secret
	Label      string     `json:"label,omitempty"`
	SecretType SecretType `json:"secret_type,omitempty"` // Type of secret
	Issuer     string     `json:"issuer,omitempty"`      // For TOTP issuer
	Period     int        `json:"period,omitempty"`      // For TOTP period (default 30)
	Digits     int        `json:"digits,omitempty"`      // For TOTP digits (default 6)
	Algorithm  string     `json:"algorithm,omitempty"`   // For TOTP algorithm (default SHA1)
	CreatedAt  string     `json:"created_at,omitempty"`
	UpdatedAt  string     `json:"updated_at,omitempty"`
	Notes      string     `json:"notes,omitempty"` // Additional notes
}

// Keyring provides interface to GNOME Keyring
type Keyring struct {
	schema string
}

// New creates a new Keyring instance
func New() *Keyring {
	return &Keyring{
		schema: "org.freedesktop.Secret.Generic",
	}
}

// Store stores a password in the keyring (legacy method)
func (k *Keyring) Store(service, username, password string) error {
	entry := Entry{
		Service:    service,
		Username:   username,
		Password:   password,
		SecretType: SecretTypePassword,
	}
	return k.StoreEntry(entry)
}

// StoreEntry stores a complete entry with all metadata
func (k *Keyring) StoreEntry(entry Entry) error {
	label := fmt.Sprintf("%s for %s", k.getTypeLabel(entry.SecretType), entry.Service)
	if entry.Username != "" {
		label = fmt.Sprintf("%s for %s (%s)", k.getTypeLabel(entry.SecretType), entry.Service, entry.Username)
	}
	if entry.Label != "" {
		label = entry.Label
	}

	args := []string{"store", "--label=" + label}

	// Add basic attributes
	args = append(args, "service", entry.Service)
	if entry.Username != "" {
		args = append(args, "username", entry.Username)
	}

	// Add secret type and metadata
	if entry.SecretType != "" {
		args = append(args, "secret_type", string(entry.SecretType))
	}
	if entry.Issuer != "" {
		args = append(args, "issuer", entry.Issuer)
	}
	if entry.Period > 0 {
		args = append(args, "period", fmt.Sprintf("%d", entry.Period))
	}
	if entry.Digits > 0 {
		args = append(args, "digits", fmt.Sprintf("%d", entry.Digits))
	}
	if entry.Algorithm != "" {
		args = append(args, "algorithm", entry.Algorithm)
	}
	if entry.Notes != "" {
		args = append(args, "notes", entry.Notes)
	}

	cmd := exec.Command("secret-tool", args...)
	cmd.Stdin = strings.NewReader(entry.Password)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to store entry: %w", err)
	}

	return nil
}

func (k *Keyring) getTypeLabel(secretType SecretType) string {
	switch secretType {
	case SecretTypeTOTP:
		return "TOTP Secret"
	case SecretTypeNote:
		return "Secure Note"
	case SecretTypeAPIKey:
		return "API Key"
	case SecretTypeSSHKey:
		return "SSH Key"
	case SecretTypeCert:
		return "Certificate"
	default:
		return "Password"
	}
}

// Get retrieves a password from the keyring (legacy method)
func (k *Keyring) Get(service, username string) (string, error) {
	args := []string{"lookup", "service", service}
	if username != "" {
		args = append(args, "username", username)
	}

	cmd := exec.Command("secret-tool", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("password not found for %s", service)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetEntry retrieves a complete entry with metadata
func (k *Keyring) GetEntry(service, username string) (*Entry, error) {
	// First get the password/secret
	password, err := k.Get(service, username)
	if err != nil {
		return nil, err
	}

	// Get all entries and find the matching one with metadata
	entries, err := k.List()
	if err != nil {
		return nil, fmt.Errorf("failed to get entry metadata: %w", err)
	}

	for _, entry := range entries {
		if entry.Service == service && entry.Username == username {
			entry.Password = password // Set the actual password
			return &entry, nil
		}
	}

	// If not found in metadata, return basic entry
	return &Entry{
		Service:    service,
		Username:   username,
		Password:   password,
		SecretType: SecretTypePassword,
	}, nil
}

// Delete removes a password from the keyring
func (k *Keyring) Delete(service, username string) error {
	args := []string{"clear", "service", service}
	if username != "" {
		args = append(args, "username", username)
	}

	cmd := exec.Command("secret-tool", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete password: %w", err)
	}

	return nil
}

// List retrieves all password entries from the keyring
func (k *Keyring) List() ([]Entry, error) {
	// Use busctl to enumerate keyring entries via D-Bus (same approach as bash version)
	cmd := exec.Command("busctl", "--user", "call",
		"org.freedesktop.secrets",
		"/org/freedesktop/secrets/collection/login",
		"org.freedesktop.Secret.Collection",
		"SearchItems", "a{ss}", "0")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate keyring entries: %w", err)
	}

	// Extract item paths
	pathRegex := regexp.MustCompile(`/org/freedesktop/secrets/collection/login/[0-9]+`)
	paths := pathRegex.FindAllString(string(output), -1)

	var entries []Entry
	seen := make(map[string]bool)

	for _, path := range paths {
		// Get attributes for each item
		attrCmd := exec.Command("busctl", "--user", "call",
			"org.freedesktop.secrets", path,
			"org.freedesktop.DBus.Properties", "Get",
			"ss", "org.freedesktop.Secret.Item", "Attributes")

		attrOutput, err := attrCmd.Output()
		if err != nil {
			continue
		}

		attrs := string(attrOutput)

		// Look for service attribute
		serviceRegex := regexp.MustCompile(`"service" "([^"]+)"`)
		serviceMatch := serviceRegex.FindStringSubmatch(attrs)
		if serviceMatch == nil {
			continue
		}

		service := serviceMatch[1]

		// Parse all attributes
		entry := Entry{
			Service: service,
		}

		// Look for username
		if usernameMatch := regexp.MustCompile(`"username" "([^"]+)"`).FindStringSubmatch(attrs); usernameMatch != nil {
			entry.Username = usernameMatch[1]
		}

		// Look for secret_type
		if typeMatch := regexp.MustCompile(`"secret_type" "([^"]+)"`).FindStringSubmatch(attrs); typeMatch != nil {
			entry.SecretType = SecretType(typeMatch[1])
		} else {
			entry.SecretType = SecretTypePassword // Default type
		}

		// Look for issuer (TOTP)
		if issuerMatch := regexp.MustCompile(`"issuer" "([^"]+)"`).FindStringSubmatch(attrs); issuerMatch != nil {
			entry.Issuer = issuerMatch[1]
		}

		// Look for period (TOTP)
		if periodMatch := regexp.MustCompile(`"period" "([^"]+)"`).FindStringSubmatch(attrs); periodMatch != nil {
			if period := parseInt(periodMatch[1]); period > 0 {
				entry.Period = period
			}
		}

		// Look for digits (TOTP)
		if digitsMatch := regexp.MustCompile(`"digits" "([^"]+)"`).FindStringSubmatch(attrs); digitsMatch != nil {
			if digits := parseInt(digitsMatch[1]); digits > 0 {
				entry.Digits = digits
			}
		}

		// Look for algorithm (TOTP)
		if algoMatch := regexp.MustCompile(`"algorithm" "([^"]+)"`).FindStringSubmatch(attrs); algoMatch != nil {
			entry.Algorithm = algoMatch[1]
		}

		// Look for notes
		if notesMatch := regexp.MustCompile(`"notes" "([^"]+)"`).FindStringSubmatch(attrs); notesMatch != nil {
			entry.Notes = notesMatch[1]
		}

		// Create unique key for deduplication
		key := entry.Service
		if entry.Username != "" {
			key = fmt.Sprintf("%s:%s", entry.Service, entry.Username)
		}

		if !seen[key] {
			entries = append(entries, entry)
			seen[key] = true
		}
	}

	// Sort entries by service name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Service == entries[j].Service {
			return entries[i].Username < entries[j].Username
		}
		return entries[i].Service < entries[j].Service
	})

	return entries, nil
}

// Search finds entries matching a pattern
func (k *Keyring) Search(pattern string) ([]Entry, error) {
	entries, err := k.List()
	if err != nil {
		return nil, err
	}

	var matches []Entry
	pattern = strings.ToLower(pattern)

	for _, entry := range entries {
		service := strings.ToLower(entry.Service)
		username := strings.ToLower(entry.Username)

		if strings.Contains(service, pattern) || strings.Contains(username, pattern) {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

// IsAvailable checks if secret-tool is available
func (k *Keyring) IsAvailable() bool {
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

// Helper function to parse integers
func parseInt(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 0
}
