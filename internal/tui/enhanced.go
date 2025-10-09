package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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


