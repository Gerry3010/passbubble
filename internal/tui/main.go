package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gerry/password-manager/pkg/backup"
	"github.com/gerry/password-manager/pkg/keyring"
)

// Screen represents the current screen
type Screen int

const (
	MainScreen Screen = iota
	DetailScreen
	BackupScreen
	AddScreen
	EditScreen
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
	totpCode    string
	totpRemaining int
	
	// Backup screen state
	backups     []backup.BackupInfo
	backupCursor int
	
	// Input state
	inputMode   bool
	inputValue  string
	inputPrompt string
	
	// Status
	status string
	
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
		screen:   MainScreen,
		selected: make(map[int]struct{}),
		
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
			Padding(1, 2).
			Width(50),
			
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
		
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
		
	case LoadEntriesMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.entries = msg.Entries
		return m, nil
		
	case LoadBackupsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.backups = msg.Backups
		return m, nil
		
	case TickMsg:
		if m.screen == DetailScreen && m.detailEntry.Type == "totp" {
			return m, m.updateTOTP()
		}
		return m, nil
	}
	
	return m, nil
}

// handleKeyPress handles keyboard input based on current screen
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputMode {
		return m.handleInputMode(msg)
	}
	
	switch m.screen {
	case MainScreen:
		return m.handleMainScreen(msg)
	case DetailScreen:
		return m.handleDetailScreen(msg)
	case BackupScreen:
		return m.handleBackupScreen(msg)
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
		}
		
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
		
	case "enter":
		if len(m.entries) > 0 {
			m.detailEntry = m.entries[m.cursor]
			m.screen = DetailScreen
			m.showSecrets = false
			return m, m.loadSecretDetails()
		}
		
	case "a":
		// Add new entry
		m.status = "Adding new entry... (not implemented in TUI yet)"
		return m, nil
		
	case "e":
		// Edit entry
		if len(m.entries) > 0 {
			m.status = "Editing entry... (not implemented in TUI yet)"
		}
		return m, nil
		
	case "d":
		// Delete entry
		if len(m.entries) > 0 {
			m.status = "Deleting entry... (not implemented in TUI yet)"
		}
		return m, nil
		
	case "c":
		// Create backup
		m.status = "Creating backup... (not implemented in TUI yet)"
		return m, nil
		
	case "b":
		// Show backups
		m.screen = BackupScreen
		m.backupCursor = 0
		return m, m.loadBackups()
		
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
		// Toggle show/hide secrets
		m.showSecrets = !m.showSecrets
		if m.showSecrets && m.detailEntry.Type == "totp" {
			return m, m.updateTOTP()
		}
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
			m.status = "Deleting backup... (not implemented in TUI yet)"
		}
		return m, nil
		
	case "r":
		// Restore backup
		if len(m.backups) > 0 {
			m.status = "Restoring backup... (not implemented in TUI yet)"
		}
		return m, nil
		
	case "f":
		// Refresh backup list
		m.status = "Refreshing backups..."
		return m, m.loadBackups()
	}
	
	return m, nil
}

// handleInputMode handles input mode key presses
func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.inputMode = false
		m.inputValue = ""
		return m, nil
		
	case tea.KeyEnter:
		// Process input
		m.inputMode = false
		result := m.inputValue
		m.inputValue = ""
		m.status = fmt.Sprintf("Processed: %s", result)
		return m, nil
		
	case tea.KeyBackspace:
		if len(m.inputValue) > 0 {
			m.inputValue = m.inputValue[:len(m.inputValue)-1]
		}
		
	default:
		m.inputValue += msg.String()
	}
	
	return m, nil
}

// View renders the current view
func (m Model) View() string {
	switch m.screen {
	case MainScreen:
		return m.renderMainScreen()
	case DetailScreen:
		return m.renderDetailScreen()
	case BackupScreen:
		return m.renderBackupScreen()
	default:
		return "Unknown screen"
	}
}

// renderMainScreen renders the main entries list
func (m Model) renderMainScreen() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	
	title := m.titleStyle.Render("🔐 Password Manager")
	
	// Entry list
	var entries []string
	for i, entry := range m.entries {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		
		typeIcon := m.getTypeIcon(entry.Type)
		line := fmt.Sprintf("%s %s %s", cursor, typeIcon, entry.Service)
		if entry.Username != "" {
			line += fmt.Sprintf(" (%s)", entry.Username)
		}
		
		if i == m.cursor {
			line = m.selectedStyle.Render(line)
		}
		
		entries = append(entries, line)
	}
	
	if len(entries) == 0 {
		entries = append(entries, m.helpStyle.Render("No entries found. Press 'a' to add one."))
	}
	
	list := m.listStyle.Render(strings.Join(entries, "\n"))
	
	// Help text
	help := m.helpStyle.Render(
		"Navigation: ↑/↓ or j/k • Enter: view details • a: add • e: edit • d: delete • c: create backup • b: backups • r: refresh • q: quit",
	)
	
	// Status
	status := ""
	if m.status != "" {
		status = m.statusStyle.Render(fmt.Sprintf("Status: %s", m.status))
	}
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		list,
		"",
		help,
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
	
	// Show secrets if toggled
	if m.showSecrets {
		switch m.detailEntry.Type {
		case "password":
			details = append(details, "")
			details = append(details, fmt.Sprintf("Password: %s", m.secretStyle.Render("••••••••••••"))) // Placeholder
			
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
			}
		}
	} else {
		details = append(details, "")
		details = append(details, m.hiddenStyle.Render("Press 's' to show secrets"))
	}
	
	content := m.detailStyle.Render(strings.Join(details, "\n"))
	
	help := m.helpStyle.Render("s: toggle secrets • esc/q: back to list")
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		content,
		"",
		help,
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
	
	help := m.helpStyle.Render("Navigation: ↑/↓ or j/k • d: delete backup • r: restore backup • f: refresh • esc/q: back")
	
	status := ""
	if m.status != "" {
		status = m.statusStyle.Render(fmt.Sprintf("Status: %s", m.status))
	}
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		list,
		"",
		help,
		status,
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

// updateTOTP updates the TOTP code and remaining time
func (m Model) updateTOTP() tea.Cmd {
	return func() tea.Msg {
		// This would generate the actual TOTP code
		// For now, we'll simulate it
		m.totpCode = "123 456" // Placeholder
		m.totpRemaining = 25   // Placeholder
		return nil
	}
}

// StartTUI starts the TUI application
func StartTUI() error {
	model := NewModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}