package keyring

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// KeyringInterface defines the interface for keyring operations
type KeyringInterface interface {
	Store(service, username, password string) error
	Get(service, username string) (string, error)
	Delete(service, username string) error
	List() ([]Entry, error)
	Search(pattern string) ([]Entry, error)
	IsAvailable() bool
}

// Entry represents a password entry
type Entry struct {
	Service  string `json:"service"`
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
	Label    string `json:"label,omitempty"`
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

// Store stores a password in the keyring
func (k *Keyring) Store(service, username, password string) error {
	label := fmt.Sprintf("Password for %s", service)
	if username != "" {
		label = fmt.Sprintf("Password for %s (%s)", service, username)
	}

	args := []string{"store", "--label=" + label}
	
	// Add attributes
	args = append(args, "service", service)
	if username != "" {
		args = append(args, "username", username)
	}

	cmd := exec.Command("secret-tool", args...)
	cmd.Stdin = strings.NewReader(password)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to store password: %w", err)
	}
	
	return nil
}

// Get retrieves a password from the keyring
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
		
		// Look for username attribute
		usernameRegex := regexp.MustCompile(`"username" "([^"]+)"`)
		usernameMatch := usernameRegex.FindStringSubmatch(attrs)
		
		var username string
		if usernameMatch != nil {
			username = usernameMatch[1]
		}

		// Create unique key for deduplication
		key := service
		if username != "" {
			key = fmt.Sprintf("%s:%s", service, username)
		}

		if !seen[key] {
			entries = append(entries, Entry{
				Service:  service,
				Username: username,
			})
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
	cmd := exec.Command("secret-tool", "--version")
	return cmd.Run() == nil
}