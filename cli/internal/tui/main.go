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

package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
)

// Screen represents the current screen
type Screen int

const (
	MainScreen Screen = iota
	DetailScreen
	BackupScreen
	AddScreen
	EditScreen
	GenerateScreen
)

// Entry represents a stored secret entry
type Entry struct {
	Service  string
	Username string
	Type     string // password, totp, api-key, note
}

// Model represents the main TUI model
type Model struct {
	screen      Screen
	entries     []Entry
	cursor      int
	selected    map[int]struct{}
	width       int
	height      int
	err         error
	
	// Detail screen state
	detailEntry Entry
	showSecrets bool
	maskPasswords bool
	totpCode    string
	totpRemaining int
	password    string
	
	// List scroll state
	listOffset int

	// Backup screen state
	backups     []backup.BackupInfo
	backupCursor int
	
	// Generate screen state
	generatedPassword string
	generateType      string // "random", "memorable", or "passphrase"
	
	// Form state
	showingForm bool
	form        FormModel
	
	// Status
	status string
	statusType string // success, error, info
	
	// Styles
	titleStyle       lipgloss.Style
	listStyle        lipgloss.Style
	selectedStyle    lipgloss.Style
	helpStyle        lipgloss.Style
	statusStyle      lipgloss.Style
	progressStyle    lipgloss.Style
	detailStyle      lipgloss.Style
	secretStyle      lipgloss.Style
	hiddenStyle      lipgloss.Style
}

// NewModel creates a new TUI model
func NewModel() Model {
	return Model{
		screen:        MainScreen,
		selected:      make(map[int]struct{}),
			maskPasswords: true, // Mask passwords by default
		
		// Styles
		titleStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Padding(0, 1),
			
		listStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2),
			
		selectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")),
			
		helpStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true),
			
		statusStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Bold(true),
			
		progressStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")),
			
		detailStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2),
			
		secretStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),
			
		hiddenStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Italic(true),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadEntries(),
		tea.Every(time.Second, func(t time.Time) tea.Msg {
			return TickMsg{Time: t}
		}),
	)
}

// TickMsg represents a time tick for TOTP updates
type TickMsg struct {
	Time time.Time
}

// LoadEntriesMsg represents loaded entries
type LoadEntriesMsg struct {
	Entries []Entry
	Err     error
}

// LoadBackupsMsg represents loaded backups
type LoadBackupsMsg struct {
	Backups []backup.BackupInfo
	Err     error
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle form updates when showing form
	if m.showingForm {
		switch msg := msg.(type) {
		case FormSubmittedMsg:
			m.showingForm = false
			return m, processFormSubmission(msg)
			
		case FormCancelledMsg:
			m.showingForm = false
			m.status = "Action cancelled"
			m.statusType = "info"
			return m, nil
			
		case ConfirmationMsg:
			m.showingForm = false
			if msg.Confirmed && msg.Action == "delete" {
				if entry, ok := msg.Data.(*Entry); ok {
					return m, handleDeleteEntry(entry)
				}
			} else {
				m.status = "Action cancelled"
				m.statusType = "info"
			}
			return m, nil
			
		default:
			updatedForm, cmd := m.form.Update(msg)
			m.form = updatedForm
			return m, cmd
		}
	}
	
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.form.width = msg.Width
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
		
	case LoadEntriesMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.entries = msg.Entries
		m.status = "Entries refreshed"
		m.statusType = "success"
		return m, nil
		
	case LoadBackupsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.backups = msg.Backups
		m.status = "Backups refreshed"
		m.statusType = "success"
		return m, nil
		
	case TOTPUpdateMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("TOTP Error: %v", msg.Error)
			m.statusType = "error"
			return m, nil
		}
		m.totpCode = msg.Code
		m.totpRemaining = msg.Remaining
		return m, nil
		
	case SecretLoadMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Error loading secret: %v", msg.Error)
			m.statusType = "error"
			return m, nil
		}
		m.password = msg.Password
		return m, nil
		
	case ActionResultMsg:
		m.status = msg.Message
		if msg.Success {
			m.statusType = "success"
			// Refresh entries after successful add/edit/delete
			if msg.Action == "add_password" || msg.Action == "add_totp" || 
			   msg.Action == "edit_entry" || msg.Action == "delete_entry" {
				return m, m.loadEntries()
			}
		} else {
			m.statusType = "error"
		}
		return m, nil
		
	case BackupCreatedMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Backup failed: %v", msg.Error)
			m.statusType = "error"
		} else {
			m.status = fmt.Sprintf("Backup created: %s", msg.Filename)
			m.statusType = "success"
			// Refresh backup list
			if m.screen == BackupScreen {
				return m, m.loadBackups()
			}
		}
		return m, nil
		
	case BackupRestoredMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Restore failed: %v", msg.Error)
			m.statusType = "error"
		} else {
			m.status = msg.Message
			m.statusType = "success"
			// Refresh entries after restore
			return m, m.loadEntries()
		}
		return m, nil
		
	case TickMsg:
		// Schedule next tick to keep timer running
		nextTickCmd := tea.Every(time.Second, func(t time.Time) tea.Msg {
			return TickMsg{Time: t}
		})
		
		if m.screen == DetailScreen && m.detailEntry.Type == "totp" && m.showSecrets {
			// Decrement remaining time if we have an active TOTP code
			if m.totpRemaining > 0 {
				m.totpRemaining--
				// Only regenerate TOTP code when time expires
				if m.totpRemaining <= 0 {
					return m, tea.Batch(m.updateTOTPReal(), nextTickCmd)
				}
				// Return model with updated countdown and continue timer
				return m, nextTickCmd
			} else {
				// If we don't have a code or remaining time, generate one
				return m, tea.Batch(m.updateTOTPReal(), nextTickCmd)
			}
		}
		return m, nextTickCmd
		
	case PasswordGeneratedMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Password generation failed: %v", msg.Error)
			m.statusType = "error"
			return m, nil
		}
		m.generatedPassword = msg.Password
		m.generateType = msg.Type
		m.status = fmt.Sprintf("%s password generated successfully", msg.Type)
		m.statusType = "success"
		return m, nil
	}
	
	return m, nil
}

// handleKeyPress handles keyboard input based on current screen
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case MainScreen:
		return m.handleMainScreen(msg)
	case DetailScreen:
		return m.handleDetailScreen(msg)
	case BackupScreen:
		return m.handleBackupScreen(msg)
	case GenerateScreen:
		return m.handleGenerateScreen(msg)
	default:
		return m, nil
	}
}

// handleMainScreen handles main screen key presses
func (m Model) handleMainScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
		
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.listOffset {
				m.listOffset = m.cursor
			}
		}

	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			listHeight := m.listVisibleHeight()
			if m.cursor >= m.listOffset+listHeight {
				m.listOffset = m.cursor - listHeight + 1
			}
		}
		
	case "enter":
		if len(m.entries) > 0 {
			m.detailEntry = m.entries[m.cursor]
			m.screen = DetailScreen
			m.showSecrets = false
			// Auto-load passwords for password entries
			if m.detailEntry.Type == "password" {
				return m, m.loadPasswordReal()
			}
			return m, m.loadSecretDetails()
		}
		
	case "p":
		m.showingForm = true
		m.form = CreateAddPasswordForm()
		m.form.width = m.width
		m.status = "Adding new password"
		m.statusType = "info"
		return m, nil

	case "t":
		m.showingForm = true
		m.form = CreateAddTOTPForm()
		m.form.width = m.width
		m.status = "Adding new TOTP secret"
		m.statusType = "info"
		return m, nil

	case "e":
		if len(m.entries) > 0 {
			m.showingForm = true
			m.form = CreateEditEntryForm(m.entries[m.cursor])
			m.form.width = m.width
			m.status = "Editing entry"
			m.statusType = "info"
		}
		return m, nil

	case "d":
		if len(m.entries) > 0 {
			m.showingForm = true
			m.form = CreateConfirmDeleteForm(m.entries[m.cursor])
			m.form.width = m.width
			m.status = "Confirm deletion"
			m.statusType = "info"
		}
		return m, nil

	case "c":
		m.showingForm = true
		m.form = CreateCreateBackupForm()
		m.form.width = m.width
		m.status = "Creating backup"
		m.statusType = "info"
		return m, nil
		
	case "b":
		// Show backups
		m.screen = BackupScreen
		m.backupCursor = 0
		return m, m.loadBackups()
		
	case "g":
		// Generate password
		m.screen = GenerateScreen
		m.generatedPassword = ""
		m.generateType = ""
		m.status = "Password Generator"
		m.statusType = "info"
		return m, nil
		
	case "r":
		// Refresh entries
		m.status = "Refreshing entries..."
		return m, m.loadEntries()
	}
	
	return m, nil
}

// handleDetailScreen handles detail screen key presses
func (m Model) handleDetailScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.screen = MainScreen
		return m, nil
		
	case "s":
		// Toggle password masking for password entries, or TOTP secrets for TOTP entries
		switch m.detailEntry.Type {
		case "password":
			m.maskPasswords = !m.maskPasswords
		case "totp":
			m.showSecrets = !m.showSecrets
			if m.showSecrets {
				return m, m.updateTOTPReal()
			} else {
				// Clear TOTP data when hiding
				m.totpCode = ""
				m.totpRemaining = 0
			}
		}
		return m, nil
		
	case "c":
		switch m.detailEntry.Type {
		case "password":
			if m.password != "" {
				return m, copyToClipboard(m.password)
			}
			m.status = "Password not loaded yet"
			m.statusType = "info"
		case "totp":
			if m.totpCode != "" {
				return m, copyToClipboard(m.totpCode)
			}
			m.status = "No TOTP code available"
			m.statusType = "info"
		}
		return m, nil

	case "t":
		if m.detailEntry.Type == "password" {
			m.showingForm = true
			m.form = CreateAddTOTPToEntryForm(m.detailEntry)
			m.form.width = m.width
			m.status = "Adding TOTP to entry"
			m.statusType = "info"
		}
		return m, nil

	case "e":
		m.showingForm = true
		m.form = CreateEditEntryForm(m.detailEntry)
		m.form.width = m.width
		m.status = "Editing entry"
		m.statusType = "info"
		return m, nil
	}
	
	return m, nil
}

// handleBackupScreen handles backup screen key presses
func (m Model) handleBackupScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.screen = MainScreen
		return m, nil
		
	case "up", "k":
		if m.backupCursor > 0 {
			m.backupCursor--
		}
		
	case "down", "j":
		if m.backupCursor < len(m.backups)-1 {
			m.backupCursor++
		}
		
	case "d":
		// Delete backup
		if len(m.backups) > 0 {
			return m, handleDeleteBackup(m.backups[m.backupCursor])
		}
		return m, nil
		
	case "r":
		// Restore backup
		if len(m.backups) > 0 {
			return m, handleRestoreBackup(m.backups[m.backupCursor])
		}
		return m, nil
		
	case "f":
		// Refresh backup list
		m.status = "Refreshing backups..."
		return m, m.loadBackups()
	}
	
	return m, nil
}

// handleGenerateScreen handles generate screen key presses
func (m Model) handleGenerateScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.screen = MainScreen
		return m, nil
		
	case "r":
		// Generate random password
		m.generateType = "random"
		return m, m.generatePassword("strong")
		
	case "m":
		// Generate memorable password
		m.generateType = "memorable"
		return m, m.generatePassword("memorable")
		
	case "p":
		// Generate passphrase
		m.generateType = "passphrase"
		return m, m.generatePassword("passphrase")
		
	case "s":
		if m.generatedPassword != "" {
			m.showingForm = true
			m.form = CreateSavePasswordForm(m.generatedPassword)
			m.form.width = m.width
			m.status = "Save generated password"
			m.statusType = "info"
		}
		return m, nil
	}
	
	return m, nil
}


// View renders the current view
func (m Model) View() string {
	// Show form overlay if active
	if m.showingForm {
		return m.form.View()
	}
	
	switch m.screen {
	case MainScreen:
		return m.renderMainScreen()
	case DetailScreen:
		return m.renderDetailScreen()
	case BackupScreen:
		return m.renderBackupScreen()
	case GenerateScreen:
		return m.renderGenerateScreen()
	default:
		return "Unknown screen"
	}
}

// renderMainScreen renders the main entries list
func (m Model) renderMainScreen() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	title := m.titleStyle.Render("🔐 Passbubble")

	// Visible slice of entries
	listHeight := m.listVisibleHeight()
	end := m.listOffset + listHeight
	if end > len(m.entries) {
		end = len(m.entries)
	}
	visible := m.entries
	if len(m.entries) > 0 {
		visible = m.entries[m.listOffset:end]
	}

	var entries []string
	for i, entry := range visible {
		absIdx := m.listOffset + i
		cursor := " "
		if absIdx == m.cursor {
			cursor = ">"
		}

		typeIcon := m.getTypeIcon(entry.Type)
		line := fmt.Sprintf("%s %s %s", cursor, typeIcon, entry.Service)
		if entry.Username != "" {
			line += fmt.Sprintf(" (%s)", entry.Username)
		}

		if absIdx == m.cursor {
			line = m.selectedStyle.Render(line)
		}
		entries = append(entries, line)
	}

	if len(m.entries) == 0 {
		entries = append(entries, m.helpStyle.Render("No entries found. Press 'p' to add a password or 't' to add a TOTP."))
	}

	// Scroll indicator
	if len(m.entries) > listHeight {
		scrollInfo := fmt.Sprintf(" %d–%d / %d", m.listOffset+1, end, len(m.entries))
		entries = append(entries, m.helpStyle.Render(scrollInfo))
	}

	listWidth := m.width - 4
	if listWidth < 20 {
		listWidth = 20
	}
	list := m.listStyle.Width(listWidth).Render(strings.Join(entries, "\n"))

	help1 := m.helpStyle.Render("↑/↓ j/k: navigate  Enter/click: open  p: add password  t: add TOTP  e: edit  d: delete")
	help2 := m.helpStyle.Render("g: generate  c: create backup  b: backups  r: refresh  q: quit")

	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		list,
		"",
		help1,
		help2,
		status,
	)
}

// renderDetailScreen renders the entry detail view
func (m Model) renderDetailScreen() string {
	title := m.titleStyle.Render(fmt.Sprintf("📋 %s Details", m.detailEntry.Service))
	
	var details []string
	details = append(details, fmt.Sprintf("Service: %s", m.detailEntry.Service))
	
	if m.detailEntry.Username != "" {
		details = append(details, fmt.Sprintf("Username: %s", m.detailEntry.Username))
	}
	
	details = append(details, fmt.Sprintf("Type: %s %s", m.getTypeIcon(m.detailEntry.Type), m.detailEntry.Type))
	
	// Always show password field for password entries
	if m.detailEntry.Type == "password" {
		details = append(details, "")
		if m.password != "" {
			if m.maskPasswords {
				// Show password as dots when masked
				maskedPassword := strings.Repeat("•", len(m.password))
				details = append(details, fmt.Sprintf("Password: %s", m.secretStyle.Render(maskedPassword)))
				details = append(details, m.helpStyle.Render("🔒 Password hidden ("+fmt.Sprintf("%d chars", len(m.password))+")"))
				details = append(details, m.helpStyle.Render("   Press 's' to reveal"))
			} else {
				// Show actual password when unmasked
				details = append(details, fmt.Sprintf("Password: %s", m.secretStyle.Render(m.password)))
				details = append(details, m.helpStyle.Render("🔓 Password visible"))
				details = append(details, m.helpStyle.Render("   Press 's' to hide"))
			}
		} else {
			details = append(details, m.hiddenStyle.Render("Loading password..."))
		}
	}
	
	// Show TOTP codes only if secrets are toggled
	if m.detailEntry.Type == "totp" {
		if m.showSecrets {
			if m.totpCode != "" {
				details = append(details, "")
				codeDisplay := m.secretStyle.Render(m.totpCode)
				details = append(details, fmt.Sprintf("TOTP Code: %s", codeDisplay))
				
				// Progress bar for remaining time
				if m.totpRemaining > 0 {
					progress := m.renderProgressBar(m.totpRemaining, 30)
					details = append(details, fmt.Sprintf("Valid for: %ds %s", m.totpRemaining, progress))
				}
			}
		} else {
			details = append(details, "")
			details = append(details, m.hiddenStyle.Render("🔒 TOTP code hidden - Press 's' to reveal"))
		}
	}
	
	detailWidth := m.width - 8
	if detailWidth < 40 {
		detailWidth = 40
	}
	content := m.detailStyle.Width(detailWidth).Render(strings.Join(details, "\n"))

	var helpText string
	switch m.detailEntry.Type {
	case "password":
		helpText = "s: show/hide  c: copy to clipboard  t: add TOTP  e: edit  esc/q: back"
	case "totp":
		helpText = "s: show/hide TOTP  c: copy code  e: edit  esc/q: back"
	default:
		helpText = "e: edit  esc/q: back"
	}
	help := m.helpStyle.Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		content,
		"",
		help,
		m.renderStatus(),
	)
}

// renderBackupScreen renders the backup management screen
func (m Model) renderBackupScreen() string {
	title := m.titleStyle.Render("💾 Backup Management")
	
	var backupList []string
	for i, backup := range m.backups {
		cursor := " "
		if i == m.backupCursor {
			cursor = ">"
		}
		
		line := fmt.Sprintf("%s 📦 %s (%s)", cursor, backup.Name, backup.ModTime.Format("2006-01-02 15:04"))
		
		if i == m.backupCursor {
			line = m.selectedStyle.Render(line)
		}
		
		backupList = append(backupList, line)
	}
	
	if len(backupList) == 0 {
		backupList = append(backupList, m.helpStyle.Render("No backups found. Press 'c' in main screen to create one."))
	}
	
	list := m.listStyle.Render(strings.Join(backupList, "\n"))
	
	help := m.helpStyle.Render("↑/↓ j/k: navigate  d: delete  r: restore  f: refresh  esc/q: back")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		list,
		"",
		help,
		m.renderStatus(),
	)
}

// renderGenerateScreen renders the password generation screen
func (m Model) renderGenerateScreen() string {
	title := m.titleStyle.Render("🎲 Password Generator")
	
	var content []string
	content = append(content, "Generate secure passwords with different styles:")
	content = append(content, "")
	
	// Show generation options
	content = append(content, "Available generation types:")
	content = append(content, "  [r] Random - Strong random password with mixed characters")
	content = append(content, "  [m] Memorable - Easy to type with alternating case")
	content = append(content, "  [p] Passphrase - Word-based password with separators")
	content = append(content, "")
	
	// Show generated password if available
	if m.generatedPassword != "" {
		content = append(content, fmt.Sprintf("Generated %s password:", m.generateType))
		content = append(content, "")
		passwordDisplay := m.secretStyle.Render(m.generatedPassword)
		content = append(content, fmt.Sprintf("Password: %s", passwordDisplay))
		content = append(content, "")
		content = append(content, "Press [s] to save this password to keyring")
	} else {
		content = append(content, "Press [r], [m], or [p] to generate a password")
	}
	
	detailWidth := m.width - 8
	if detailWidth < 40 {
		detailWidth = 40
	}
	contentBox := m.detailStyle.Width(detailWidth).Render(strings.Join(content, "\n"))

	help := m.helpStyle.Render("r: random  m: memorable  p: passphrase  s: save password  esc/q: back")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		contentBox,
		"",
		help,
		m.renderStatus(),
	)
}

// getTypeIcon returns an icon for the entry type
func (m Model) getTypeIcon(entryType string) string {
	switch entryType {
	case "password":
		return "🔑"
	case "totp":
		return "🔐"
	case "api-key":
		return "🗝️"
	case "note":
		return "📝"
	default:
		return "❓"
	}
}

// renderProgressBar renders a simple progress bar
func (m Model) renderProgressBar(current, max int) string {
	width := 20
	filled := int(float64(current) / float64(max) * float64(width))
	
	var bar strings.Builder
	bar.WriteString("[")
	for i := 0; i < width; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}
	bar.WriteString("]")
	
	return m.progressStyle.Render(bar.String())
}

// loadEntries loads all entries from the keyring
func (m Model) loadEntries() tea.Cmd {
	return func() tea.Msg {
		kr := keyring.New()
		entries, err := kr.List()
		if err != nil {
			return LoadEntriesMsg{Err: err}
		}
		
		var tuiEntries []Entry
		for _, entry := range entries {
			tuiEntries = append(tuiEntries, Entry{
				Service:  entry.Service,
				Username: entry.Username,
				Type:     string(entry.SecretType),
			})
		}
		
		return LoadEntriesMsg{Entries: tuiEntries}
	}
}

// loadBackups loads backup information
func (m Model) loadBackups() tea.Cmd {
	return func() tea.Msg {
		kr := keyring.New()
		backupMgr := backup.New(kr, nil)
		backups, err := backupMgr.ListBackups()
		if err != nil {
			return LoadBackupsMsg{Err: err}
		}
		
		return LoadBackupsMsg{Backups: backups}
	}
}

// loadSecretDetails loads additional details for the selected entry
func (m Model) loadSecretDetails() tea.Cmd {
	return func() tea.Msg {
		// This would load additional secret details
		// For now, we'll just return nil
		return nil
	}
}


// PasswordGeneratedMsg represents a generated password result
type PasswordGeneratedMsg struct {
	Password string
	Type     string
	Error    error
}

// generatePassword generates a password of the specified type
func (m Model) generatePassword(passwordType string) tea.Cmd {
	return func() tea.Msg {
		// Create options based on password type
		var opts *generator.Options
		
		switch passwordType {
		case "strong":
			// Generate strong random password
			opts = generator.DefaultOptions()
			opts.Type = generator.Strong
		case "memorable":
			// Generate memorable password
			opts = generator.DefaultOptions()
			opts.Type = generator.Memorable
		case "passphrase":
			// Generate passphrase
			opts = generator.DefaultOptions()
			opts.Type = generator.Passphrase
		default:
			return PasswordGeneratedMsg{
				Error: fmt.Errorf("unknown password type: %s", passwordType),
			}
		}
		
		// Create generator and generate password
		gen := generator.New(opts)
		passwords, err := gen.Generate()
		
		if err != nil {
			return PasswordGeneratedMsg{
				Error: fmt.Errorf("failed to generate password: %w", err),
			}
		}
		
		if len(passwords) == 0 {
			return PasswordGeneratedMsg{
				Error: fmt.Errorf("no passwords generated"),
			}
		}
		
		return PasswordGeneratedMsg{
			Password: passwords[0],
			Type:     passwordType,
		}
	}
}

// listVisibleHeight returns the number of list rows that fit in the terminal.
func (m Model) listVisibleHeight() int {
	// overhead: title(1) + blank(1) + list border+pad top(2) + list border+pad bottom(2)
	//           + blank(1) + help(2) + status(1) = 10
	h := m.height - 10
	if h < 3 {
		h = 3
	}
	return h
}

// renderStatus returns a styled status line (empty string when no status).
func (m Model) renderStatus() string {
	if m.status == "" {
		return ""
	}
	var s lipgloss.Style
	switch m.statusType {
	case "success":
		s = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)
	case "error":
		s = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	default:
		s = m.statusStyle
	}
	return s.Render("» " + m.status)
}

// handleMouse handles mouse events.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.screen != MainScreen {
		return m, nil
	}
	// List content starts at row 4 (title=0, blank=1, border=2, pad=3, item0=4).
	itemRow := msg.Y - 4
	if itemRow < 0 {
		return m, nil
	}
	absIdx := m.listOffset + itemRow
	if absIdx < 0 || absIdx >= len(m.entries) {
		return m, nil
	}
	m.cursor = absIdx
	m.detailEntry = m.entries[m.cursor]
	m.screen = DetailScreen
	m.showSecrets = false
	m.password = ""
	if m.detailEntry.Type == "password" {
		return m, m.loadPasswordReal()
	}
	return m, m.loadSecretDetails()
}

// copyToClipboard copies text to the system clipboard asynchronously.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		commands := []struct {
			name string
			args []string
		}{
			{"wl-copy", nil},
			{"xclip", []string{"-selection", "clipboard"}},
			{"xsel", []string{"--clipboard", "--input"}},
		}
		for _, c := range commands {
			if _, err := exec.LookPath(c.name); err == nil {
				cmd := exec.Command(c.name, c.args...)
				cmd.Stdin = strings.NewReader(text)
				if err := cmd.Run(); err == nil {
					return ActionResultMsg{Success: true, Message: "Copied to clipboard", Action: "copy"}
				}
			}
		}
		return ActionResultMsg{Success: false, Message: "No clipboard tool found (install xclip, xsel, or wl-copy)", Action: "copy"}
	}
}

// StartTUI starts the TUI application
func StartTUI() error {
	model := NewModel()
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}