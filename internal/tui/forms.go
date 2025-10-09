package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gerry/password-manager/pkg/totp"
)

// FormType represents different form types
type FormType int

const (
	AddPasswordForm FormType = iota
	AddTOTPForm
	AddTOTPToEntryForm
	EditEntryForm
	ConfirmDeleteForm
	CreateBackupForm
	SavePasswordForm
)

// FormField represents a single input field
type FormField struct {
	Label       string
	Value       string
	Placeholder string
	IsPassword  bool
	IsRequired  bool
	Validator   func(string) error
}

// FormModel represents the form state
type FormModel struct {
	Type        FormType
	Title       string
	Fields      []FormField
	CurrentField int
	Submitted   bool
	Confirmed   bool
	Error       string
	Entry       *Entry // For edit forms
}

// FormSubmittedMsg indicates form submission
type FormSubmittedMsg struct {
	Type   FormType
	Fields map[string]string
	Entry  *Entry
}

// FormCancelledMsg indicates form cancellation
type FormCancelledMsg struct{}

// ConfirmationMsg indicates confirmation dialog result
type ConfirmationMsg struct {
	Confirmed bool
	Action    string
	Data      interface{}
}

// CreateAddPasswordForm creates a form for adding passwords
func CreateAddPasswordForm() FormModel {
	return FormModel{
		Type:  AddPasswordForm,
		Title: "Add New Password",
		Fields: []FormField{
			{
				Label:       "Service",
				Placeholder: "e.g., gmail, github, bank",
				IsRequired:  true,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("service name is required")
					}
					return nil
				},
			},
			{
				Label:       "Username/Email",
				Placeholder: "user@example.com",
				IsRequired:  false,
			},
			{
				Label:       "Password (leave empty to generate)",
				Placeholder: "Enter password or leave blank",
				IsPassword:  true,
				IsRequired:  false,
			},
		},
	}
}

// CreateAddTOTPForm creates a form for adding TOTP secrets
func CreateAddTOTPForm() FormModel {
	return FormModel{
		Type:  AddTOTPForm,
		Title: "Add TOTP Secret",
		Fields: []FormField{
			{
				Label:       "Service",
				Placeholder: "e.g., google, github, aws",
				IsRequired:  true,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("service name is required")
					}
					return nil
				},
			},
			{
				Label:       "Username/Email",
				Placeholder: "user@example.com",
				IsRequired:  false,
			},
			{
				Label:       "Issuer",
				Placeholder: "e.g., Google, GitHub",
				IsRequired:  false,
			},
			{
				Label:       "Secret",
				Placeholder: "Base32 TOTP secret (required)",
				IsRequired:  true,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("TOTP secret is required")
					}
					if !totp.IsValidSecret(s) {
						return fmt.Errorf("invalid base32 secret")
					}
					return nil
				},
			},
			{
				Label:       "Algorithm",
				Value:       "SHA1",
				Placeholder: "SHA1, SHA256, SHA512 (default: SHA1)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to SHA1
					}
					s = strings.ToUpper(strings.TrimSpace(s))
					if s != "SHA1" && s != "SHA256" && s != "SHA512" {
						return fmt.Errorf("algorithm must be SHA1, SHA256, or SHA512")
					}
					return nil
				},
			},
			{
				Label:       "Length",
				Value:       "6",
				Placeholder: "Code length (6 or 8, default: 6)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to 6
					}
					s = strings.TrimSpace(s)
					if s != "6" && s != "8" {
						return fmt.Errorf("length must be 6 or 8")
					}
					return nil
				},
			},
			{
				Label:       "Period",
				Value:       "30",
				Placeholder: "Time period in seconds (default: 30)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to 30
					}
					s = strings.TrimSpace(s)
					if period, err := strconv.Atoi(s); err != nil || period < 10 || period > 300 {
						return fmt.Errorf("period must be between 10 and 300 seconds")
					}
					return nil
				},
			},
		},
	}
}

// CreateAddTOTPToEntryForm creates a form for adding TOTP to an existing entry
func CreateAddTOTPToEntryForm(entry Entry) FormModel {
	return FormModel{
		Type:  AddTOTPToEntryForm,
		Title: fmt.Sprintf("Add TOTP to %s", entry.Service),
		Entry: &entry,
		Fields: []FormField{
			{
				Label:       "Service (read-only)",
				Value:       entry.Service,
				IsRequired:  false,
			},
			{
				Label:       "Username (read-only)",
				Value:       entry.Username,
				IsRequired:  false,
			},
			{
				Label:       "Issuer",
				Placeholder: "e.g., Google, GitHub",
				IsRequired:  false,
			},
			{
				Label:       "TOTP Secret",
				Placeholder: "Base32 TOTP secret (required)",
				IsRequired:  true,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("TOTP secret is required")
					}
					if !totp.IsValidSecret(s) {
						return fmt.Errorf("invalid base32 secret")
					}
					return nil
				},
			},
			{
				Label:       "Algorithm",
				Value:       "SHA1",
				Placeholder: "SHA1, SHA256, SHA512 (default: SHA1)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to SHA1
					}
					s = strings.ToUpper(strings.TrimSpace(s))
					if s != "SHA1" && s != "SHA256" && s != "SHA512" {
						return fmt.Errorf("algorithm must be SHA1, SHA256, or SHA512")
					}
					return nil
				},
			},
			{
				Label:       "Length",
				Value:       "6",
				Placeholder: "Code length (6 or 8, default: 6)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to 6
					}
					s = strings.TrimSpace(s)
					if s != "6" && s != "8" {
						return fmt.Errorf("length must be 6 or 8")
					}
					return nil
				},
			},
			{
				Label:       "Period",
				Value:       "30",
				Placeholder: "Time period in seconds (default: 30)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty defaults to 30
					}
					s = strings.TrimSpace(s)
					if period, err := strconv.Atoi(s); err != nil || period < 10 || period > 300 {
						return fmt.Errorf("period must be between 10 and 300 seconds")
					}
					return nil
				},
			},
		},
	}
}

// CreateEditEntryForm creates a form for editing entries
func CreateEditEntryForm(entry Entry) FormModel {
	var fields []FormField
	
	if entry.Type == "totp" {
		fields = []FormField{
			{
				Label:       "Service",
				Value:       entry.Service,
				IsRequired:  true,
			},
			{
				Label:       "Username/Email",
				Value:       entry.Username,
				IsRequired:  false,
			},
			{
				Label:       "Issuer",
				Value:       "", // Would need to load from keyring metadata
				IsRequired:  false,
			},
			{
				Label:       "TOTP Secret",
				Placeholder: "Base32 TOTP secret (leave empty to keep current)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s != "" && !totp.IsValidSecret(s) {
						return fmt.Errorf("invalid base32 secret")
					}
					return nil
				},
			},
			{
				Label:       "Algorithm",
				Value:       "SHA1", // Default for edit
				Placeholder: "SHA1, SHA256, SHA512 (leave empty to keep current)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty keeps current
					}
					s = strings.ToUpper(strings.TrimSpace(s))
					if s != "SHA1" && s != "SHA256" && s != "SHA512" {
						return fmt.Errorf("algorithm must be SHA1, SHA256, or SHA512")
					}
					return nil
				},
			},
			{
				Label:       "Length",
				Value:       "6", // Default for edit
				Placeholder: "Code length (6 or 8, leave empty to keep current)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty keeps current
					}
					s = strings.TrimSpace(s)
					if s != "6" && s != "8" {
						return fmt.Errorf("length must be 6 or 8")
					}
					return nil
				},
			},
			{
				Label:       "Period",
				Value:       "30", // Default for edit
				Placeholder: "Time period in seconds (leave empty to keep current)",
				IsRequired:  false,
				Validator: func(s string) error {
					if s == "" {
						return nil // Empty keeps current
					}
					s = strings.TrimSpace(s)
					if period, err := strconv.Atoi(s); err != nil || period < 10 || period > 300 {
						return fmt.Errorf("period must be between 10 and 300 seconds")
					}
					return nil
				},
			},
		}
	} else {
		fields = []FormField{
			{
				Label:       "Service",
				Value:       entry.Service,
				IsRequired:  true,
			},
			{
				Label:       "Username/Email",
				Value:       entry.Username,
				IsRequired:  false,
			},
			{
				Label:       "New Password (leave empty to keep current)",
				Placeholder: "Enter new password or leave blank",
				IsPassword:  true,
				IsRequired:  false,
			},
		}
	}

	return FormModel{
		Type:   EditEntryForm,
		Title:  fmt.Sprintf("Edit %s", entry.Service),
		Fields: fields,
		Entry:  &entry,
	}
}

// CreateConfirmDeleteForm creates a confirmation dialog
func CreateConfirmDeleteForm(entry Entry) FormModel {
	return FormModel{
		Type:  ConfirmDeleteForm,
		Title: fmt.Sprintf("Delete %s?", entry.Service),
		Entry: &entry,
	}
}

// CreateBackupForm creates a backup creation form
func CreateCreateBackupForm() FormModel {
	return FormModel{
		Type:  CreateBackupForm,
		Title: "Create Backup",
		Fields: []FormField{
			{
				Label:       "Backup Name (optional)",
				Placeholder: "Leave empty for auto-generated name",
				IsRequired:  false,
			},
		},
	}
}

// CreateSavePasswordForm creates a form for saving a generated password
func CreateSavePasswordForm(generatedPassword string) FormModel {
	return FormModel{
		Type:  SavePasswordForm,
		Title: "Save Generated Password",
		Fields: []FormField{
			{
				Label:       "Service",
				Placeholder: "e.g., gmail, github, bank",
				IsRequired:  true,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("service name is required")
					}
					return nil
				},
			},
			{
				Label:       "Username/Email",
				Placeholder: "user@example.com (optional)",
				IsRequired:  false,
			},
			{
				Label:       "Generated Password (read-only)",
				Value:       generatedPassword,
				IsRequired:  false,
			},
		},
	}
}

// Update handles form updates
func (f FormModel) Update(msg tea.Msg) (FormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return f, func() tea.Msg { return FormCancelledMsg{} }
			
		case "ctrl+c":
			return f, tea.Quit
			
		case "tab", "down":
			if f.Type == ConfirmDeleteForm {
				return f, nil
			}
			if f.CurrentField < len(f.Fields)-1 {
				f.CurrentField++
			}
			
		case "shift+tab", "up":
			if f.Type == ConfirmDeleteForm {
				return f, nil
			}
			if f.CurrentField > 0 {
				f.CurrentField--
			}
			
		case "enter":
			switch f.Type {
			case ConfirmDeleteForm:
				f.Confirmed = true
				return f, func() tea.Msg {
					return ConfirmationMsg{
						Confirmed: true,
						Action:    "delete",
						Data:      f.Entry,
					}
				}
				
			default:
				// Validate current field if required
				if f.CurrentField < len(f.Fields) {
					field := &f.Fields[f.CurrentField]
					if field.Validator != nil {
						if err := field.Validator(field.Value); err != nil {
							f.Error = err.Error()
							return f, nil
						}
					}
				}
				
				// If on last field or all fields valid, submit
				if f.CurrentField >= len(f.Fields)-1 || f.validateAllFields() {
					return f.submitForm()
				} else {
					f.CurrentField++
				}
			}
			
		case "backspace":
			if f.Type == ConfirmDeleteForm {
				return f, nil
			}
			if f.CurrentField < len(f.Fields) {
				field := &f.Fields[f.CurrentField]
				if len(field.Value) > 0 {
					field.Value = field.Value[:len(field.Value)-1]
				}
			}
			
		case "y", "Y":
			if f.Type == ConfirmDeleteForm {
				return f, func() tea.Msg {
					return ConfirmationMsg{
						Confirmed: true,
						Action:    "delete",
						Data:      f.Entry,
					}
				}
			}
			fallthrough
			
		case "n", "N":
			if f.Type == ConfirmDeleteForm {
				return f, func() tea.Msg {
					return ConfirmationMsg{
						Confirmed: false,
						Action:    "delete",
						Data:      f.Entry,
					}
				}
			}
			fallthrough
			
		default:
			// Add character to current field
			if f.Type != ConfirmDeleteForm && f.CurrentField < len(f.Fields) {
				field := &f.Fields[f.CurrentField]
				if len(msg.String()) == 1 {
					field.Value += msg.String()
					f.Error = "" // Clear error on new input
				}
			}
		}
	}
	
	return f, nil
}

// validateAllFields validates all required fields
func (f FormModel) validateAllFields() bool {
	for _, field := range f.Fields {
		if field.IsRequired && strings.TrimSpace(field.Value) == "" {
			return false
		}
		if field.Validator != nil && field.Validator(field.Value) != nil {
			return false
		}
	}
	return true
}

// submitForm submits the form
func (f FormModel) submitForm() (FormModel, tea.Cmd) {
	// Validate all fields first
	for i, field := range f.Fields {
		if field.IsRequired && strings.TrimSpace(field.Value) == "" {
			f.Error = fmt.Sprintf("%s is required", field.Label)
			f.CurrentField = i
			return f, nil
		}
		if field.Validator != nil {
			if err := field.Validator(field.Value); err != nil {
				f.Error = err.Error()
				f.CurrentField = i
				return f, nil
			}
		}
	}
	
	// Create field map
	fieldMap := make(map[string]string)
	for i, field := range f.Fields {
		fieldMap[fmt.Sprintf("field_%d", i)] = field.Value
		
		// Also add by label for easier access
		switch field.Label {
		case "Service":
			fieldMap["service"] = field.Value
		case "Username/Email":
			fieldMap["username"] = field.Value
		case "Password (leave empty to generate)", "New Password (leave empty to keep current)":
			fieldMap["password"] = field.Value
	case "Secret (leave empty to generate)", "TOTP Secret":
			fieldMap["secret"] = field.Value
		case "Issuer":
			fieldMap["issuer"] = field.Value
		case "Backup Name (optional)":
			fieldMap["backup_name"] = field.Value
		case "Generated Password (read-only)":
			fieldMap["password"] = field.Value
		case "Algorithm":
			fieldMap["algorithm"] = field.Value
		case "Length":
			fieldMap["length"] = field.Value
		case "Period":
			fieldMap["period"] = field.Value
		}
	}
	
	f.Submitted = true
	
	return f, func() tea.Msg {
		return FormSubmittedMsg{
			Type:   f.Type,
			Fields: fieldMap,
			Entry:  f.Entry,
		}
	}
}

// View renders the form
func (f FormModel) View() string {
	var b strings.Builder
	
	// Title
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Padding(0, 1).
		Render(f.Title)
	
	b.WriteString(title)
	b.WriteString("\n\n")
	
	// Handle confirmation dialog differently
	if f.Type == ConfirmDeleteForm {
		message := fmt.Sprintf("Are you sure you want to delete '%s'", f.Entry.Service)
		if f.Entry.Username != "" {
			message += fmt.Sprintf(" (%s)", f.Entry.Username)
		}
		message += "?"
		
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render(message))
		b.WriteString("\n\n")
		
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("This action cannot be undone."))
		b.WriteString("\n\n")
		
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Render("Press 'y' to confirm, 'n' to cancel, or Esc to go back"))
		
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(1, 2).
			Render(b.String())
	}
	
	// Regular form fields
	for i, field := range f.Fields {
		// Field label
		label := field.Label
		if field.IsRequired {
			label += " *"
		}
		
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)
		
		if i == f.CurrentField {
			labelStyle = labelStyle.Background(lipgloss.Color("57"))
		}
		
		b.WriteString(labelStyle.Render(label))
		b.WriteString("\n")
		
		// Field input
		value := field.Value
		if field.IsPassword && value != "" {
			value = strings.Repeat("•", len(value))
		}
		
		if value == "" && field.Placeholder != "" {
			value = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Italic(true).
				Render(field.Placeholder)
		}
		
		inputStyle := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1).
			Width(40)
		
		if i == f.CurrentField {
			inputStyle = inputStyle.BorderForeground(lipgloss.Color("39"))
			value += "█" // Cursor
		}
		
		b.WriteString(inputStyle.Render(value))
		b.WriteString("\n\n")
	}
	
	// Error message
	if f.Error != "" {
		error := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render("Error: " + f.Error)
		b.WriteString(error)
		b.WriteString("\n\n")
	}
	
	// Help text
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true).
		Render("Tab/↑↓: navigate • Enter: submit/next • Esc: cancel")
	
	b.WriteString(help)
	
	// Wrap in border
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())
}