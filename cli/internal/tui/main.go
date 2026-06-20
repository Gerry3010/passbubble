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
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	LoginScreen
	RegisterScreen
	UnlockScreen
	SettingsScreen
	PINSetupScreen
)

// isAuthScreen reports whether a screen is part of the login/unlock gate.
func isAuthScreen(s Screen) bool {
	return s == LoginScreen || s == RegisterScreen || s == UnlockScreen
}

// Entry represents a stored secret entry
type Entry struct {
	ID        string
	Service   string
	Username  string
	URL       string
	Type      string // password, totp, api-key, note
	FolderID  *string
	CreatedAt string // RFC3339 metadata timestamp
	UpdatedAt string // RFC3339 metadata timestamp
}

// itemKind distinguishes folders from entries in the navigable list.
type itemKind int

const (
	folderKind itemKind = iota
	entryKind
)

// listItem is one row in the current folder level: either a folder or an entry.
type listItem struct {
	kind   itemKind
	folder *vaultpkg.Folder
	entry  *Entry
}

// sortField selects which attribute the list is ordered by.
type sortField int

const (
	sortByName sortField = iota
	sortByCreated
	sortByUpdated
	sortByURL
)

func (s sortField) String() string {
	switch s {
	case sortByCreated:
		return "created"
	case sortByUpdated:
		return "updated"
	case sortByURL:
		return "url"
	default:
		return "name"
	}
}

func parseSortField(s string) sortField {
	switch s {
	case "created":
		return sortByCreated
	case "updated":
		return sortByUpdated
	case "url":
		return sortByURL
	default:
		return sortByName
	}
}

// Model represents the main TUI model
type Model struct {
	screen   Screen
	cursor   int
	selected map[int]struct{}
	width    int
	height   int
	err      error

	// Vault session (wired in by StartTUI)
	vault   *vaultpkg.Vault
	cfg     *config.Config
	cfgPath string

	// Folder/entry data
	allEntries  []Entry            // every entry (metadata only)
	folders     []*vaultpkg.Folder // folder tree roots
	folderStack []*vaultpkg.Folder // drill-down path; empty = root level

	// Sorting
	sortField    sortField
	sortAsc      bool
	folderFirst  bool
	showSortMenu bool

	// Move-entry overlay
	showMoveMenu bool
	moveEntryID  string
	moveCursor   int
	moveTargets  []moveTarget

	// Share-link overlays: expiry picker, then the QR/URL result.
	showShareMenu  bool
	showShareQR    bool
	shareIsFolder  bool
	shareEntryID   string
	shareEntryName string
	shareURL       string
	shareQR        string

	// Auth screens (login / register / unlock / PIN setup)
	authFields []authField
	authCursor int
	authBusy   bool
	authErr    string
	pinMode    bool // unlock screen: true = PIN entry, false = master password

	// Keybindings + help/settings UI
	keymap      map[string]string // action -> key
	showHelp    bool
	kbCursor    int  // selected action in the keybinding editor
	kbCapturing bool // capturing the next key for a rebind

	// Search / filter
	showSearch bool   // search input is active
	filter     string // active filter query ("" = no filter)

	// Auto-lock
	lastActivity time.Time
	idleTimeout  time.Duration

	// Detail screen state
	detailEntry   Entry
	showSecrets   bool
	maskPasswords bool
	totpCode      string
	totpRemaining int
	password      string

	// List scroll state
	listOffset int

	// Backup screen state
	backups      []backup.BackupInfo
	backupCursor int

	// Generate screen state
	generatedPassword string
	generateType      string // "random", "memorable", or "passphrase"

	// Form state
	showingForm bool
	form        FormModel

	// Status
	status     string
	statusType string // success, error, info

	// Styles
	titleStyle    lipgloss.Style
	listStyle     lipgloss.Style
	selectedStyle lipgloss.Style
	helpStyle     lipgloss.Style
	statusStyle   lipgloss.Style
	progressStyle lipgloss.Style
	detailStyle   lipgloss.Style
	secretStyle   lipgloss.Style
	hiddenStyle   lipgloss.Style
}

// NewModel creates a new TUI model bound to an authenticated vault session.
func NewModel(v *vaultpkg.Vault, cfg *config.Config, cfgPath string) Model {
	m := Model{
		screen:        MainScreen,
		selected:      make(map[int]struct{}),
		vault:         v,
		cfg:           cfg,
		cfgPath:       cfgPath,
		sortAsc:       true,
		folderFirst:   true,
		lastActivity:  time.Now(),
		idleTimeout:   10 * time.Minute,
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

	// Keybindings + sort preferences from config (with sensible defaults).
	if cfg != nil {
		m.keymap = buildKeymap(cfg.Keybindings)
		if cfg.SortField != "" {
			m.sortField = parseSortField(cfg.SortField)
		}
		if cfg.SortAsc != nil {
			m.sortAsc = *cfg.SortAsc
		}
		if cfg.FolderFirst != nil {
			m.folderFirst = *cfg.FolderFirst
		}
		if cfg.LogoutInterval != nil {
			// 0 disables auto-lock; any positive value is in minutes.
			m.idleTimeout = time.Duration(*cfg.LogoutInterval) * time.Minute
		}
	} else {
		m.keymap = defaultKeymap()
	}

	// Gate: pick the initial screen based on session state.
	switch {
	case v == nil || cfg == nil || !cfg.IsLoggedIn():
		m.screen = LoginScreen
		m.authFields = newLoginFields(cfg)
	case !v.IsUnlocked():
		m.screen = UnlockScreen
		// Default to PIN entry when a PIN is configured and still in its window.
		if v.PINEnabled() && !v.PINPwExpired() {
			m.pinMode = true
			m.authFields = newPINUnlockFields()
		} else {
			m.authFields = newUnlockFields()
		}
	default:
		m.screen = MainScreen
	}
	return m
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	tick := tea.Every(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
	// Only load entries once the vault is unlocked; otherwise the auth gate runs first.
	if m.screen == MainScreen {
		return tea.Batch(m.loadEntries(), tick)
	}
	return tick
}

// TickMsg represents a time tick for TOTP updates
type TickMsg struct {
	Time time.Time
}

// LoadEntriesMsg represents loaded entries + folder tree
type LoadEntriesMsg struct {
	Entries []Entry
	Folders []*vaultpkg.Folder
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
			// Folder forms need vault access, so they are handled here rather
			// than in the vault-less processFormSubmission helper.
			switch msg.Type {
			case NewFolderForm:
				return m, m.createFolderCmd(msg.Fields["folder_name"], msg.ParentID)
			case RenameFolderForm:
				return m, m.renameFolderCmd(msg.FolderID, msg.Fields["folder_name"], msg.ParentID)
			default:
				return m, processFormSubmission(msg)
			}

		case FormCancelledMsg:
			m.showingForm = false
			m.status = "Action cancelled"
			m.statusType = "info"
			return m, nil

		case ConfirmationMsg:
			m.showingForm = false
			if msg.Confirmed {
				switch msg.Action {
				case "delete":
					if entry, ok := msg.Data.(*Entry); ok {
						return m, handleDeleteEntry(entry)
					}
				case "delete_folder":
					if id, ok := msg.Data.(string); ok {
						return m, m.deleteFolderCmd(id)
					}
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
		m.lastActivity = time.Now()
		return m.handleMouse(msg)

	case tea.KeyMsg:
		m.lastActivity = time.Now()
		return m.handleKeyPress(msg)

	case AuthResultMsg:
		m.authBusy = false
		if msg.err != nil {
			m.authErr = msg.err.Error()
			// PIN expired or wiped after too many attempts: fall back to the
			// master-password field so the user can still get in.
			if m.screen == UnlockScreen && m.pinMode &&
				(errors.Is(msg.err, vaultpkg.ErrPINExpired) || errors.Is(msg.err, vaultpkg.ErrPINLockedOut)) {
				m.pinMode = false
				m.authFields = newUnlockFields()
				m.authCursor = 0
			}
			return m, nil
		}
		if msg.vault != nil {
			m.vault = msg.vault
		}
		if msg.cfg != nil {
			m.cfg = msg.cfg
		}
		// Point the keyring shim at the freshly authenticated vault so the
		// detail/TOTP/store paths use the current session.
		keyring.SetGlobal(vaultpkg.NewKeyringAdapter(m.vault))
		m.screen = MainScreen
		m.authErr = ""
		m.authFields = nil
		m.pinMode = false
		m.lastActivity = time.Now()
		return m, m.loadEntries()

	case PINConfigMsg:
		m.authBusy = false
		if msg.err != nil {
			m.authErr = msg.err.Error()
			return m, nil
		}
		// PIN enable/disable succeeded: return to settings with a status line.
		m.screen = SettingsScreen
		m.authErr = ""
		m.authFields = nil
		m.status = msg.status
		m.statusType = "success"
		return m, nil

	case LoadEntriesMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.allEntries = msg.Entries
		m.folders = msg.Folders
		m.clampCursor()
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
			// Refresh entries after successful add/edit/delete/folder ops
			if msg.Action == "add_password" || msg.Action == "add_totp" ||
				msg.Action == "edit_entry" || msg.Action == "delete_entry" ||
				msg.Action == "folder" {
				return m, m.loadEntries()
			}
		} else {
			m.statusType = "error"
		}
		return m, nil

	case ClipboardClearMsg:
		return m, clearClipboardIfMatches(msg.value)

	case CopiedSecretMsg:
		if msg.err != nil {
			m.status = "Copy failed: " + msg.err.Error()
			m.statusType = "error"
			return m, nil
		}
		m.status = fmt.Sprintf("%s copied — clears in %ds", msg.label, int(clipboardClearDelay.Seconds()))
		m.statusType = "success"
		return m, tea.Tick(clipboardClearDelay, func(time.Time) tea.Msg {
			return ClipboardClearMsg{value: msg.value}
		})

	case ShareLinkCreatedMsg:
		if msg.err != nil {
			m.status = "Share link failed: " + msg.err.Error()
			m.statusType = "error"
			return m, nil
		}
		m.shareURL = msg.url
		m.shareQR = renderQR(msg.url)
		m.showShareQR = true
		m.status = "Share link created"
		m.statusType = "success"
		// Best-effort copy to clipboard so the long URL is easy to paste, too.
		writeClipboard(msg.url)
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

		// Auto-lock: if the vault is unlocked and idle past the timeout, lock it.
		if m.idleTimeout > 0 && m.vault != nil && m.vault.IsUnlocked() &&
			!isAuthScreen(m.screen) && time.Since(m.lastActivity) >= m.idleTimeout {
			locked := m.lockVault()
			locked.status = "Vault locked (idle timeout)"
			locked.statusType = "info"
			return locked, nextTickCmd
		}

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
	case LoginScreen, RegisterScreen, UnlockScreen, PINSetupScreen:
		return m.handleAuthScreen(msg)
	case SettingsScreen:
		return m.handleSettingsScreen(msg)
	default:
		return m, nil
	}
}

// handleMainScreen handles main screen key presses. Navigation keys are fixed;
// action keys are resolved through the (rebindable) keymap.
func (m Model) handleMainScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showSortMenu {
		return m.handleSortMenu(msg)
	}
	if m.showMoveMenu {
		return m.handleMoveMenu(msg)
	}
	if m.showShareQR {
		m.showShareQR = false // any key closes the share-link result
		m.status = ""
		return m, nil
	}
	if m.showShareMenu {
		return m.handleShareMenu(msg)
	}
	if m.showHelp {
		m.showHelp = false // any key closes the help overlay
		return m, nil
	}
	if m.showSearch {
		return m.handleSearchInput(msg)
	}

	key := msg.String()

	// Fixed navigation keys (not rebindable).
	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.listOffset {
				m.listOffset = m.cursor
			}
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.currentItems())-1 {
			m.cursor++
			listHeight := m.listVisibleHeight()
			if m.cursor >= m.listOffset+listHeight {
				m.listOffset = m.cursor - listHeight + 1
			}
		}
		return m, nil

	case "esc", "left", "h":
		// Clear an active filter first, then drill out of the current folder.
		if m.filter != "" {
			m.filter = ""
			m.clampCursor()
			return m, nil
		}
		if len(m.folderStack) > 0 {
			m.folderStack = m.folderStack[:len(m.folderStack)-1]
			m.cursor = 0
			m.listOffset = 0
		}
		return m, nil

	case "enter", "right", "l":
		item, ok := m.selectedItem()
		if !ok {
			return m, nil
		}
		if item.kind == folderKind {
			m.folderStack = append(m.folderStack, item.folder)
			m.cursor = 0
			m.listOffset = 0
			return m, nil
		}
		m.detailEntry = *item.entry
		m.screen = DetailScreen
		m.showSecrets = false
		if m.detailEntry.Type == "password" {
			return m, m.loadPasswordReal()
		}
		return m, m.loadSecretDetails()
	}

	// Rebindable actions.
	action, ok := m.actionForKey(key)
	if !ok {
		return m, nil
	}
	return m.runAction(action)
}

// runAction executes a resolved main-screen action.
func (m Model) runAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case actQuit:
		return m, tea.Quit

	case actSortCycle:
		m.sortField = (m.sortField + 1) % 4
		m.persistSortPrefs()
		m.status = "Sort: " + m.sortLabel()
		m.statusType = "info"
		return m, nil

	case actSortDir:
		m.sortAsc = !m.sortAsc
		m.persistSortPrefs()
		m.status = "Sort: " + m.sortLabel()
		m.statusType = "info"
		return m, nil

	case actFolderFirst:
		m.folderFirst = !m.folderFirst
		m.persistSortPrefs()
		m.status = "Sort: " + m.sortLabel()
		m.statusType = "info"
		return m, nil

	case actSortMenu:
		m.showSortMenu = true
		return m, nil

	case actSearch:
		m.showSearch = true
		m.status = ""
		return m, nil

	case actHelp:
		m.showHelp = true
		return m, nil

	case actSettings:
		m.screen = SettingsScreen
		m.kbCursor = 0
		m.kbCapturing = false
		m.status = ""
		return m, nil

	case actAddPassword:
		m.showingForm = true
		m.form = CreateAddPasswordForm()
		m.form.width = m.width
		m.status = "Adding new password"
		m.statusType = "info"
		return m, nil

	case actAddTOTP:
		m.showingForm = true
		m.form = CreateAddTOTPForm()
		m.form.width = m.width
		m.status = "Adding new TOTP secret"
		m.statusType = "info"
		return m, nil

	case actNewFolder:
		m.showingForm = true
		m.form = CreateNewFolderForm(m.currentFolderID())
		m.form.width = m.width
		m.status = "New folder"
		m.statusType = "info"
		return m, nil

	case actEdit:
		if item, ok := m.selectedItem(); ok {
			m.showingForm = true
			if item.kind == folderKind {
				m.form = CreateRenameFolderForm(item.folder.ID, item.folder.Name, item.folder.ParentID)
				m.status = "Renaming folder"
			} else {
				m.form = CreateEditEntryForm(*item.entry)
				m.status = "Editing entry"
			}
			m.form.width = m.width
			m.statusType = "info"
		}
		return m, nil

	case actMove:
		if item, ok := m.selectedItem(); ok && item.kind == entryKind {
			m.openMoveMenu(item.entry)
		}
		return m, nil

	case actCopyPass:
		if item, ok := m.selectedItem(); ok && item.kind == entryKind {
			m.status = "Copying password…"
			m.statusType = "info"
			return m, m.copyEntryFieldCmd(item.entry.ID, "password")
		}
		return m, nil

	case actCopyUser:
		if item, ok := m.selectedItem(); ok && item.kind == entryKind {
			m.status = "Copying username…"
			m.statusType = "info"
			return m, m.copyEntryFieldCmd(item.entry.ID, "username")
		}
		return m, nil

	case actShareLink:
		if item, ok := m.selectedItem(); ok {
			switch item.kind {
			case entryKind:
				m.shareIsFolder = false
				m.shareEntryID = item.entry.ID
				m.shareEntryName = item.entry.Service
				m.showShareMenu = true
				m.status = "Share link — choose how long it stays valid"
				m.statusType = "info"
			case folderKind:
				m.shareIsFolder = true
				m.shareEntryID = item.folder.ID
				m.shareEntryName = item.folder.Name
				m.showShareMenu = true
				m.status = "Share folder — choose how long the link stays valid"
				m.statusType = "info"
			}
		}
		return m, nil

	case actDelete:
		if item, ok := m.selectedItem(); ok {
			if item.kind == folderKind {
				if n := m.folderContentCount(item.folder); n > 0 {
					m.status = fmt.Sprintf("Folder '%s' is not empty (%d items) — move or delete its contents first", item.folder.Name, n)
					m.statusType = "error"
					return m, nil
				}
				m.showingForm = true
				m.form = CreateConfirmDeleteFolderForm(item.folder.ID, item.folder.Name)
				m.form.width = m.width
				m.status = "Confirm folder deletion"
			} else {
				m.showingForm = true
				m.form = CreateConfirmDeleteForm(*item.entry)
				m.form.width = m.width
				m.status = "Confirm deletion"
			}
			m.statusType = "info"
		}
		return m, nil

	case actBackup:
		m.showingForm = true
		m.form = CreateCreateBackupForm()
		m.form.width = m.width
		m.status = "Creating backup"
		m.statusType = "info"
		return m, nil

	case actBackups:
		m.screen = BackupScreen
		m.backupCursor = 0
		return m, m.loadBackups()

	case actGenerate:
		m.screen = GenerateScreen
		m.generatedPassword = ""
		m.generateType = ""
		m.status = "Password Generator"
		m.statusType = "info"
		return m, nil

	case actRefresh:
		m.status = "Refreshing entries..."
		return m, m.loadEntries()
	}
	return m, nil
}

// persistSortPrefs writes the current sort settings back to the config file.
func (m Model) persistSortPrefs() {
	if m.cfg == nil {
		return
	}
	asc, ff := m.sortAsc, m.folderFirst
	m.cfg.SortField = m.sortField.String()
	m.cfg.SortAsc = &asc
	m.cfg.FolderFirst = &ff
	_ = m.cfg.Save(m.cfgPath)
}

// handleSortMenu handles key input while the sort overlay is open.
func (m Model) handleSortMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "o", "enter", "q":
		m.showSortMenu = false
	case "1":
		m.sortField = sortByName
	case "2":
		m.sortField = sortByCreated
	case "3":
		m.sortField = sortByUpdated
	case "4":
		m.sortField = sortByURL
	case "d":
		m.sortAsc = !m.sortAsc
	case "f":
		m.folderFirst = !m.folderFirst
	}
	return m, nil
}

// sortLabel returns a human-readable description of the active sort settings.
func (m Model) sortLabel() string {
	dir := "↑ asc"
	if !m.sortAsc {
		dir = "↓ desc"
	}
	s := fmt.Sprintf("%s, %s", m.sortField, dir)
	if m.folderFirst {
		s += ", folders first"
	}
	return s
}

// renderSortMenu renders the sort overlay.
func (m Model) renderSortMenu() string {
	check := func(active bool) string {
		if active {
			return "●"
		}
		return "○"
	}
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}

	var b strings.Builder
	b.WriteString(m.titleStyle.Render("⇅ Sort"))
	b.WriteString("\n\n")
	b.WriteString("Field:\n")
	fmt.Fprintf(&b, "  [1] %s Name\n", check(m.sortField == sortByName))
	fmt.Fprintf(&b, "  [2] %s Created\n", check(m.sortField == sortByCreated))
	fmt.Fprintf(&b, "  [3] %s Updated\n", check(m.sortField == sortByUpdated))
	fmt.Fprintf(&b, "  [4] %s URL\n", check(m.sortField == sortByURL))
	b.WriteString("\n")
	dir := "ascending"
	if !m.sortAsc {
		dir = "descending"
	}
	fmt.Fprintf(&b, "  [d] Direction: %s\n", dir)
	fmt.Fprintf(&b, "  [f] Folders first: %s\n", onOff(m.folderFirst))
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("1-4: field  d: direction  f: folders  Esc/Enter: close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())

	return lipgloss.JoinVertical(lipgloss.Left, box)
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
				return m, copyWithClear(m.password, "Password")
			}
			m.status = "Password not loaded yet"
			m.statusType = "info"
		case "totp":
			if m.totpCode != "" {
				return m, copyWithClear(m.totpCode, "TOTP code")
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
	// Show sort overlay if active
	if m.showSortMenu {
		return m.renderSortMenu()
	}
	// Show move overlay if active
	if m.showMoveMenu {
		return m.renderMoveMenu()
	}
	// Show share-link QR result / expiry picker if active
	if m.showShareQR {
		return m.renderShareQR()
	}
	if m.showShareMenu {
		return m.renderShareMenu()
	}
	// Show help overlay if active
	if m.showHelp {
		return m.renderHelp()
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
	case LoginScreen, RegisterScreen, UnlockScreen, PINSetupScreen:
		return m.renderAuthScreen()
	case SettingsScreen:
		return m.renderSettingsScreen()
	default:
		return "Unknown screen"
	}
}

// renderMainScreen renders the main entries list
func (m Model) renderMainScreen() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	title := m.titleStyle.Render(m.breadcrumb())

	items := m.sortedItems()

	// Visible slice of items
	listHeight := m.listVisibleHeight()
	end := m.listOffset + listHeight
	if end > len(items) {
		end = len(items)
	}
	visible := items
	if len(items) > 0 {
		visible = items[m.listOffset:end]
	}

	var rows []string
	for i, item := range visible {
		absIdx := m.listOffset + i
		cursor := " "
		if absIdx == m.cursor {
			cursor = ">"
		}

		var line string
		if item.kind == folderKind {
			label := fmt.Sprintf("%s 📁 %s", cursor, item.folder.Name)
			if n := len(item.folder.Children); n > 0 {
				label += fmt.Sprintf("  (%d)", n)
			}
			line = label
		} else {
			line = fmt.Sprintf("%s %s %s", cursor, m.getTypeIcon(item.entry.Type), item.entry.Service)
			if item.entry.Username != "" {
				line += fmt.Sprintf(" (%s)", item.entry.Username)
			}
		}

		if absIdx == m.cursor {
			line = m.selectedStyle.Render(line)
		}
		rows = append(rows, line)
	}

	if len(items) == 0 {
		rows = append(rows, m.helpStyle.Render("Empty. Press 'p' to add a password, 't' for TOTP, 'n' for a folder."))
	}

	// Scroll indicator
	if len(items) > listHeight {
		scrollInfo := fmt.Sprintf(" %d–%d / %d", m.listOffset+1, end, len(items))
		rows = append(rows, m.helpStyle.Render(scrollInfo))
	}

	listWidth := m.width - 4
	if listWidth < 20 {
		listWidth = 20
	}
	list := m.listStyle.Width(listWidth).Render(strings.Join(rows, "\n"))

	searchLine := ""
	switch {
	case m.showSearch:
		searchLine = m.titleStyle.Render("🔎 " + m.filter + "█")
	case m.filter != "":
		searchLine = m.helpStyle.Render(fmt.Sprintf("🔎 filter: %q  (Esc to clear)", m.filter))
	}

	help1 := m.helpStyle.Render(fmt.Sprintf("Enter: open  Esc: up/clear  %s: search  %s: keys/help  %s: settings  %s: quit",
		m.keyFor(actSearch), m.keyFor(actHelp), m.keyFor(actSettings), m.keyFor(actQuit)))
	help2 := m.helpStyle.Render("sort: " + m.sortLabel())

	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		searchLine,
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

// loadEntries loads all entry metadata and the folder tree from the vault.
func (m Model) loadEntries() tea.Cmd {
	return func() tea.Msg {
		if m.vault == nil {
			return LoadEntriesMsg{Err: fmt.Errorf("not connected — run 'pwmgr login' first")}
		}
		vaultEntries, err := m.vault.ListEntries()
		if err != nil {
			return LoadEntriesMsg{Err: err}
		}
		folders, err := m.vault.ListFolders()
		if err != nil {
			return LoadEntriesMsg{Err: err}
		}

		tuiEntries := make([]Entry, len(vaultEntries))
		for i, e := range vaultEntries {
			tuiEntries[i] = Entry{
				ID:        e.ID,
				Service:   e.Name,
				URL:       e.URL,
				Type:      e.Type,
				FolderID:  e.FolderID,
				CreatedAt: e.CreatedAt,
				UpdatedAt: e.UpdatedAt,
			}
		}

		return LoadEntriesMsg{Entries: tuiEntries, Folders: folders}
	}
}

// --- Folder navigation helpers ---

// currentFolderID returns the ID of the folder the user is currently inside,
// or nil when at the root level.
func (m Model) currentFolderID() *string {
	if len(m.folderStack) == 0 {
		return nil
	}
	return &m.folderStack[len(m.folderStack)-1].ID
}

// currentItems returns the rows shown at the current folder level: subfolders
// first, then the entries that belong to this folder. When a filter is active,
// it instead returns matching entries from ALL folders (flat), no subfolders.
func (m Model) currentItems() []listItem {
	if m.filter != "" {
		q := strings.ToLower(m.filter)
		var items []listItem
		for i := range m.allEntries {
			e := &m.allEntries[i]
			if strings.Contains(strings.ToLower(e.Service), q) || strings.Contains(strings.ToLower(e.URL), q) {
				items = append(items, listItem{kind: entryKind, entry: e})
			}
		}
		return items
	}

	var subfolders []*vaultpkg.Folder
	if len(m.folderStack) == 0 {
		subfolders = m.folders
	} else {
		subfolders = m.folderStack[len(m.folderStack)-1].Children
	}

	curID := m.currentFolderID()
	items := make([]listItem, 0, len(subfolders)+len(m.allEntries))
	for _, f := range subfolders {
		items = append(items, listItem{kind: folderKind, folder: f})
	}
	for i := range m.allEntries {
		e := &m.allEntries[i]
		if sameFolder(e.FolderID, curID) {
			items = append(items, listItem{kind: entryKind, entry: e})
		}
	}
	return items
}

// sameFolder reports whether an entry's folder pointer matches the current level.
func sameFolder(entryFolderID, levelFolderID *string) bool {
	if entryFolderID == nil || *entryFolderID == "" {
		return levelFolderID == nil
	}
	return levelFolderID != nil && *entryFolderID == *levelFolderID
}

// selectedItem returns the item under the cursor in the displayed (sorted) order.
func (m Model) selectedItem() (listItem, bool) {
	items := m.sortedItems()
	if m.cursor < 0 || m.cursor >= len(items) {
		return listItem{}, false
	}
	return items[m.cursor], true
}

// sortedItems returns currentItems ordered by the active sort settings.
func (m Model) sortedItems() []listItem {
	items := m.currentItems()
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if m.folderFirst && a.kind != b.kind {
			return a.kind == folderKind // folders before entries
		}
		return m.itemLess(a, b)
	})
	return items
}

// itemLess reports whether a sorts before b under the active field + direction.
func (m Model) itemLess(a, b listItem) bool {
	ka, kb := m.sortKey(a), m.sortKey(b)
	if ka == kb {
		// Stable tiebreak on name so ordering is deterministic.
		na, nb := strings.ToLower(itemName(a)), strings.ToLower(itemName(b))
		if m.sortAsc {
			return na < nb
		}
		return na > nb
	}
	if m.sortAsc {
		return ka < kb
	}
	return ka > kb
}

// sortKey returns the comparable key for an item under the active sort field.
func (m Model) sortKey(it listItem) string {
	switch m.sortField {
	case sortByCreated:
		if it.kind == folderKind {
			return it.folder.CreatedAt
		}
		return it.entry.CreatedAt
	case sortByUpdated:
		if it.kind == folderKind {
			return it.folder.CreatedAt // folders have no updated_at; fall back to created
		}
		return it.entry.UpdatedAt
	case sortByURL:
		if it.kind == folderKind {
			return ""
		}
		return strings.ToLower(it.entry.URL)
	default: // sortByName
		return strings.ToLower(itemName(it))
	}
}

// itemName returns the display name of an item (folder name or entry service).
func itemName(it listItem) string {
	if it.kind == folderKind {
		return it.folder.Name
	}
	return it.entry.Service
}

// clampCursor keeps the cursor/offset valid after the item count changes.
func (m *Model) clampCursor() {
	n := len(m.currentItems())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.listOffset > m.cursor {
		m.listOffset = m.cursor
	}
}

// breadcrumb renders the title with the current folder path.
func (m Model) breadcrumb() string {
	s := "🔐 Passbubble"
	for _, f := range m.folderStack {
		s += " › " + f.Name
	}
	return s
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
	// overhead: title(1) + search(1) + list border+pad top(2) + list border+pad bottom(2)
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
	items := m.sortedItems()
	absIdx := m.listOffset + itemRow
	if absIdx < 0 || absIdx >= len(items) {
		return m, nil
	}
	m.cursor = absIdx
	item := items[absIdx]
	if item.kind == folderKind {
		// Click on a folder drills into it.
		m.folderStack = append(m.folderStack, item.folder)
		m.cursor = 0
		m.listOffset = 0
		return m, nil
	}
	m.detailEntry = *item.entry
	m.screen = DetailScreen
	m.showSecrets = false
	m.password = ""
	if m.detailEntry.Type == "password" {
		return m, m.loadPasswordReal()
	}
	return m, m.loadSecretDetails()
}

// clipboardClearDelay is how long a copied secret stays on the clipboard.
const clipboardClearDelay = 20 * time.Second

// ClipboardClearMsg fires after a copy to clear the secret from the clipboard.
type ClipboardClearMsg struct{ value string }

// writeClipboard writes text to the system clipboard. Returns false if no tool found.
func writeClipboard(text string) bool {
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
				return true
			}
		}
	}
	return false
}

// CopiedSecretMsg is produced by quick-copy after fetching + decrypting an entry.
type CopiedSecretMsg struct {
	value string
	label string
	err   error
}

// copyEntryFieldCmd fetches+decrypts an entry and copies one field to the clipboard.
func (m Model) copyEntryFieldCmd(entryID, field string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return CopiedSecretMsg{err: fmt.Errorf("vault is locked")}
		}
		e, err := v.GetEntry(entryID)
		if err != nil {
			return CopiedSecretMsg{err: err}
		}
		if e.Data == nil {
			return CopiedSecretMsg{err: fmt.Errorf("no data")}
		}
		var val, label string
		switch field {
		case "username":
			val, label = e.Data.Username, "Username"
		default:
			val, label = e.Data.Password, "Password"
		}
		if val == "" {
			return CopiedSecretMsg{err: fmt.Errorf("%s is empty", label)}
		}
		if !writeClipboard(val) {
			return CopiedSecretMsg{err: fmt.Errorf("no clipboard tool found")}
		}
		return CopiedSecretMsg{value: val, label: label}
	}
}

// copyWithClear copies a secret and schedules an automatic clipboard wipe.
func copyWithClear(text, label string) tea.Cmd {
	copyCmd := func() tea.Msg {
		if writeClipboard(text) {
			return ActionResultMsg{Success: true, Message: fmt.Sprintf("%s copied — clears in %ds", label, int(clipboardClearDelay.Seconds())), Action: "copy"}
		}
		return ActionResultMsg{Success: false, Message: "No clipboard tool found (install xclip, xsel, or wl-copy)", Action: "copy"}
	}
	clearTick := tea.Tick(clipboardClearDelay, func(time.Time) tea.Msg {
		return ClipboardClearMsg{value: text}
	})
	return tea.Batch(copyCmd, clearTick)
}

// clearClipboardIfMatches wipes the clipboard only if it still holds the secret.
func clearClipboardIfMatches(text string) tea.Cmd {
	return func() tea.Msg {
		if cur, err := readClipboard(); err == nil && strings.TrimRight(cur, "\n\r") == text {
			writeClipboard("")
		}
		return nil
	}
}

// StartTUI starts the TUI application bound to an authenticated vault session.
func StartTUI(v *vaultpkg.Vault, cfg *config.Config, cfgPath string) error {
	model := NewModel(v, cfg, cfgPath)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
