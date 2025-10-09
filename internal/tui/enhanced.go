package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Gerry3010/passbubble/pkg/keyring"
	"github.com/Gerry3010/passbubble/pkg/totp"
)

// TOTPUpdateMsg represents a TOTP code update
type TOTPUpdateMsg struct {
	Code      string
	Remaining int
	Error     error
}

// SecretLoadMsg represents loaded secret data
type SecretLoadMsg struct {
	Password string
	Error    error
}

// Enhanced methods with real integrations

// updateTOTPReal updates the TOTP code with real data
func (m Model) updateTOTPReal() tea.Cmd {
	if m.detailEntry.Type != "totp" {
		return nil
	}
	
	return func() tea.Msg {
		// Load the TOTP secret from keyring
		kr := keyring.New()
		secret, err := kr.Get(m.detailEntry.Service, m.detailEntry.Username)
		if err != nil {
			return TOTPUpdateMsg{Error: fmt.Errorf("failed to load TOTP secret: %w", err)}
		}
		
		// Parse TOTP options from the stored secret metadata
		opts := totp.DefaultOptions()
		
		// Generate current TOTP code
		code, err := totp.GenerateCode(secret, opts)
		if err != nil {
			return TOTPUpdateMsg{Error: fmt.Errorf("failed to generate TOTP code: %w", err)}
		}
		
		// Format code with spaces
		formattedCode := totp.FormatCode(code)
		
		// Calculate remaining time
		remaining := totp.GetTimeRemaining(opts.Period)
		
		return TOTPUpdateMsg{
			Code:      formattedCode,
			Remaining: remaining,
		}
	}
}

// loadPasswordReal loads the actual password from keyring
func (m Model) loadPasswordReal() tea.Cmd {
	if m.detailEntry.Type != "password" {
		return nil
	}
	
	return func() tea.Msg {
		kr := keyring.New()
		password, err := kr.Get(m.detailEntry.Service, m.detailEntry.Username)
		if err != nil {
			return SecretLoadMsg{Error: fmt.Errorf("failed to load password: %w", err)}
		}
		
		return SecretLoadMsg{Password: password}
	}
}

// Enhanced update method that handles real data
func (m Model) UpdateEnhanced(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TOTPUpdateMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("TOTP Error: %v", msg.Error)
			return m, nil
		}
		
		m.totpCode = msg.Code
		m.totpRemaining = msg.Remaining
		return m, nil
		
	case SecretLoadMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Secret Error: %v", msg.Error)
			return m, nil
		}
		
		// Store password securely (in real implementation, this would be handled more carefully)
		m.status = "Password loaded successfully"
		return m, nil
		
	default:
		return m.Update(msg)
	}
}

// Enhanced detail screen handler with real secret loading
func (m Model) handleDetailScreenEnhanced(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.screen = MainScreen
		// Clear sensitive data when leaving detail screen
		m.totpCode = ""
		m.totpRemaining = 0
		return m, nil
		
	case "s":
		// Toggle show/hide secrets
		m.showSecrets = !m.showSecrets
		if m.showSecrets {
			switch m.detailEntry.Type {
			case "totp":
				return m, m.updateTOTPReal()
			case "password":
				return m, m.loadPasswordReal()
			}
		} else {
			// Clear sensitive data when hiding
			m.totpCode = ""
			m.totpRemaining = 0
		}
		return m, nil
	}
	
	return m, nil
}

// Enhanced render detail screen with real secret display
func (m Model) renderDetailScreenEnhanced() string {
	title := m.titleStyle.Render(fmt.Sprintf("📋 %s Details", m.detailEntry.Service))
	
	var details []string
	details = append(details, fmt.Sprintf("Service: %s", m.detailEntry.Service))
	
	if m.detailEntry.Username != "" {
		details = append(details, fmt.Sprintf("Username: %s", m.detailEntry.Username))
	}
	
	details = append(details, fmt.Sprintf("Type: %s %s", m.getTypeIcon(m.detailEntry.Type), m.detailEntry.Type))
	
	// Show secrets if toggled
	if m.showSecrets {
		switch m.detailEntry.Type {
		case "password":
			details = append(details, "")
			// In a real implementation, we'd load the actual password
			details = append(details, fmt.Sprintf("Password: %s", m.secretStyle.Render("[REDACTED - Use CLI to copy]")))
			details = append(details, m.helpStyle.Render("Use CLI commands to safely copy passwords to clipboard"))
			
		case "totp":
			if m.totpCode != "" {
				details = append(details, "")
				codeDisplay := m.secretStyle.Render(m.totpCode)
				details = append(details, fmt.Sprintf("TOTP Code: %s", codeDisplay))
				
				// Progress bar for remaining time
				if m.totpRemaining > 0 {
					progress := m.renderProgressBar(m.totpRemaining, 30)
					details = append(details, fmt.Sprintf("Valid for: %ds %s", m.totpRemaining, progress))
				}
			} else {
				details = append(details, "")
				details = append(details, m.hiddenStyle.Render("Loading TOTP code..."))
			}
		}
	} else {
		details = append(details, "")
		details = append(details, m.hiddenStyle.Render("Press 's' to show secrets"))
	}
	
	content := m.detailStyle.Render(strings.Join(details, "\n"))
	
	help := m.helpStyle.Render("s: toggle secrets • esc/q: back to list")
	
	// Security notice
	securityNotice := m.helpStyle.Render("🔒 Passwords are not displayed in TUI for security. Use CLI commands for secure access.")
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		content,
		"",
		help,
		"",
		securityNotice,
	)
}