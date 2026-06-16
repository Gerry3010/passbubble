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
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
	"github.com/Gerry3010/passbubble/cli/pkg/totp"
)

// ActionResultMsg represents the result of an action
type ActionResultMsg struct {
	Success bool
	Message string
	Action  string
	Error   error
}

// BackupCreatedMsg indicates a backup was created
type BackupCreatedMsg struct {
	Filename string
	Error    error
}

// BackupRestoredMsg indicates a backup was restored
type BackupRestoredMsg struct {
	Success bool
	Message string
	Error   error
}

// processFormSubmission processes form submissions and returns appropriate actions
func processFormSubmission(formMsg FormSubmittedMsg) tea.Cmd {
	switch formMsg.Type {
	case AddPasswordForm:
		return handleAddPassword(formMsg.Fields)
	case AddTOTPForm:
		return handleAddTOTP(formMsg.Fields)
	case AddTOTPToEntryForm:
		return handleAddTOTPToEntry(formMsg.Entry, formMsg.Fields)
	case EditEntryForm:
		return handleEditEntry(formMsg.Entry, formMsg.Fields)
	case CreateBackupForm:
		return handleCreateBackup(formMsg.Fields)
	case SavePasswordForm:
		return handleSavePassword(formMsg.Fields)
	default:
		return func() tea.Msg {
			return ActionResultMsg{
				Success: false,
				Message: "Unknown form type",
				Action:  "unknown",
			}
		}
	}
}

// handleAddPassword processes adding a new password entry
func handleAddPassword(fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		service := strings.TrimSpace(fields["service"])
		username := strings.TrimSpace(fields["username"])
		password := strings.TrimSpace(fields["password"])
		
		if service == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Service name is required",
				Action:  "add_password",
			}
		}
		
		// Generate password if not provided
		if password == "" {
			gen := generator.New(nil)
			passwords, err := gen.Generate()
			if err != nil {
				return ActionResultMsg{
					Success: false,
					Message: "Failed to generate password",
					Action:  "add_password",
					Error:   err,
				}
			}
			password = passwords[0]
		}
		
		// Create entry
		entry := keyring.Entry{
			Service:    service,
			Username:   username,
			Password:   password,
			SecretType: keyring.SecretTypePassword,
		}
		
		// Store in keyring
		kr := keyring.New()
		if err := kr.StoreEntry(entry); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to store password: %v", err),
				Action:  "add_password",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("Password for %s", service)
		if username != "" {
			message += fmt.Sprintf(" (%s)", username)
		}
		message += " added successfully"
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "add_password",
		}
	}
}

// handleAddTOTP processes adding a new TOTP entry
func handleAddTOTP(fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		service := strings.TrimSpace(fields["service"])
		username := strings.TrimSpace(fields["username"])
		issuer := strings.TrimSpace(fields["issuer"])
		secret := strings.TrimSpace(fields["secret"])
		algorithm := strings.TrimSpace(fields["algorithm"])
		lengthStr := strings.TrimSpace(fields["length"])
		periodStr := strings.TrimSpace(fields["period"])
		
		if service == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Service name is required",
				Action:  "add_totp",
			}
		}
		
		// Validate secret (now required)
		if secret == "" {
			return ActionResultMsg{
				Success: false,
				Message: "TOTP secret is required",
				Action:  "add_totp",
			}
		}
		
		if !totp.IsValidSecret(secret) {
			return ActionResultMsg{
				Success: false,
				Message: "Invalid TOTP secret format",
				Action:  "add_totp",
			}
		}
		
		// Parse and validate algorithm
		if algorithm == "" {
			algorithm = "SHA1" // Default
		} else {
			algorithm = strings.ToUpper(algorithm)
			if algorithm != "SHA1" && algorithm != "SHA256" && algorithm != "SHA512" {
				return ActionResultMsg{
					Success: false,
					Message: "Algorithm must be SHA1, SHA256, or SHA512",
					Action:  "add_totp",
				}
			}
		}
		
		// Parse and validate length
		digits := 6 // Default
		if lengthStr != "" {
			parsedDigits, err := strconv.Atoi(lengthStr)
			if err != nil || (parsedDigits != 6 && parsedDigits != 8) {
				return ActionResultMsg{
					Success: false,
					Message: "Length must be 6 or 8",
					Action:  "add_totp",
				}
			}
			digits = parsedDigits
		}
		
		// Parse and validate period
		period := 30 // Default
		if periodStr != "" {
			parsedPeriod, err := strconv.Atoi(periodStr)
			if err != nil || parsedPeriod < 10 || parsedPeriod > 300 {
				return ActionResultMsg{
					Success: false,
					Message: "Period must be between 10 and 300 seconds",
					Action:  "add_totp",
				}
			}
			period = parsedPeriod
		}
		
		// Create TOTP entry
		entry := keyring.Entry{
			Service:    service,
			Username:   username,
			Password:   secret,
			SecretType: keyring.SecretTypeTOTP,
			Issuer:     issuer,
			Period:     period,
			Digits:     digits,
			Algorithm:  algorithm,
		}
		
		// Store in keyring
		kr := keyring.New()
		if err := kr.StoreEntry(entry); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to store TOTP secret: %v", err),
				Action:  "add_totp",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("TOTP secret for %s", service)
		if username != "" {
			message += fmt.Sprintf(" (%s)", username)
		}
		message += " added successfully"
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "add_totp",
		}
	}
}

// handleAddTOTPToEntry processes adding TOTP to an existing entry
func handleAddTOTPToEntry(entry *Entry, fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		if entry == nil {
			return ActionResultMsg{
				Success: false,
				Message: "No entry provided",
				Action:  "add_totp_to_entry",
			}
		}
		
		issuer := strings.TrimSpace(fields["issuer"])
		secret := strings.TrimSpace(fields["secret"])
		algorithm := strings.TrimSpace(fields["algorithm"])
		lengthStr := strings.TrimSpace(fields["length"])
		periodStr := strings.TrimSpace(fields["period"])
		
		// Validate secret (now required)
		if secret == "" {
			return ActionResultMsg{
				Success: false,
				Message: "TOTP secret is required",
				Action:  "add_totp_to_entry",
			}
		}
		
		if !totp.IsValidSecret(secret) {
			return ActionResultMsg{
				Success: false,
				Message: "Invalid TOTP secret format",
				Action:  "add_totp_to_entry",
			}
		}
		
		// Parse and validate algorithm
		if algorithm == "" {
			algorithm = "SHA1" // Default
		} else {
			algorithm = strings.ToUpper(algorithm)
			if algorithm != "SHA1" && algorithm != "SHA256" && algorithm != "SHA512" {
				return ActionResultMsg{
					Success: false,
					Message: "Algorithm must be SHA1, SHA256, or SHA512",
					Action:  "add_totp_to_entry",
				}
			}
		}
		
		// Parse and validate length
		digits := 6 // Default
		if lengthStr != "" {
			parsedDigits, err := strconv.Atoi(lengthStr)
			if err != nil || (parsedDigits != 6 && parsedDigits != 8) {
				return ActionResultMsg{
					Success: false,
					Message: "Length must be 6 or 8",
					Action:  "add_totp_to_entry",
				}
			}
			digits = parsedDigits
		}
		
		// Parse and validate period
		period := 30 // Default
		if periodStr != "" {
			parsedPeriod, err := strconv.Atoi(periodStr)
			if err != nil || parsedPeriod < 10 || parsedPeriod > 300 {
				return ActionResultMsg{
					Success: false,
					Message: "Period must be between 10 and 300 seconds",
					Action:  "add_totp_to_entry",
				}
			}
			period = parsedPeriod
		}
		
		// Create TOTP service name to avoid conflicts with existing password
		totpService := entry.Service + "-totp"
		
		// Create TOTP entry
		totpEntry := keyring.Entry{
			Service:    totpService,
			Username:   entry.Username,
			Password:   secret,
			SecretType: keyring.SecretTypeTOTP,
			Issuer:     issuer,
			Period:     period,
			Digits:     digits,
			Algorithm:  algorithm,
		}
		
		// Store in keyring
		kr := keyring.New()
		if err := kr.StoreEntry(totpEntry); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to store TOTP secret: %v", err),
				Action:  "add_totp_to_entry",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("TOTP added to %s", entry.Service)
		if entry.Username != "" {
			message += fmt.Sprintf(" (%s)", entry.Username)
		}
		message += " as " + totpService
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "add_totp_to_entry",
		}
	}
}

// handleEditEntry processes editing an existing entry
func handleEditEntry(entry *Entry, fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		if entry == nil {
			return ActionResultMsg{
				Success: false,
				Message: "No entry to edit",
				Action:  "edit_entry",
			}
		}
		
		newService := strings.TrimSpace(fields["service"])
		newUsername := strings.TrimSpace(fields["username"])
		newPassword := strings.TrimSpace(fields["password"])
		newIssuer := strings.TrimSpace(fields["issuer"])
		newSecret := strings.TrimSpace(fields["secret"])
		newAlgorithm := strings.TrimSpace(fields["algorithm"])
		newLengthStr := strings.TrimSpace(fields["length"])
		newPeriodStr := strings.TrimSpace(fields["period"])
		
		if newService == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Service name is required",
				Action:  "edit_entry",
			}
		}
		
		kr := keyring.New()
		
		// If service or username changed, we need to delete old and create new
		if newService != entry.Service || newUsername != entry.Username {
			// Delete old entry
			if err := kr.Delete(entry.Service, entry.Username); err != nil {
				return ActionResultMsg{
					Success: false,
					Message: fmt.Sprintf("Failed to delete old entry: %v", err),
					Action:  "edit_entry",
					Error:   err,
				}
			}
		}
		
		// Create updated entry
		var updatedEntry keyring.Entry
		if entry.Type == "totp" {
			// Load existing entry for TOTP updates
			existingEntry, err := kr.GetEntry(entry.Service, entry.Username)
			if err != nil {
				return ActionResultMsg{
					Success: false,
					Message: fmt.Sprintf("Failed to load existing TOTP entry: %v", err),
					Action:  "edit_entry",
					Error:   err,
				}
			}
			
			// Use new secret if provided, otherwise keep existing
			secret := existingEntry.Password
			if newSecret != "" {
				// Validate new secret
				if !totp.IsValidSecret(newSecret) {
					return ActionResultMsg{
						Success: false,
						Message: "Invalid TOTP secret format",
						Action:  "edit_entry",
					}
				}
				secret = newSecret
			}
			
			// Parse and validate algorithm
			algorithm := existingEntry.Algorithm
			if newAlgorithm != "" {
				algorithm = strings.ToUpper(newAlgorithm)
				if algorithm != "SHA1" && algorithm != "SHA256" && algorithm != "SHA512" {
					return ActionResultMsg{
						Success: false,
						Message: "Algorithm must be SHA1, SHA256, or SHA512",
						Action:  "edit_entry",
					}
				}
			}
			
			// Parse and validate length
			digits := existingEntry.Digits
			if newLengthStr != "" {
				parsedDigits, err := strconv.Atoi(newLengthStr)
				if err != nil || (parsedDigits != 6 && parsedDigits != 8) {
					return ActionResultMsg{
						Success: false,
						Message: "Length must be 6 or 8",
						Action:  "edit_entry",
					}
				}
				digits = parsedDigits
			}
			
			// Parse and validate period
			period := existingEntry.Period
			if newPeriodStr != "" {
				parsedPeriod, err := strconv.Atoi(newPeriodStr)
				if err != nil || parsedPeriod < 10 || parsedPeriod > 300 {
					return ActionResultMsg{
						Success: false,
						Message: "Period must be between 10 and 300 seconds",
						Action:  "edit_entry",
					}
				}
				period = parsedPeriod
			}
			
		updatedEntry = keyring.Entry{
			Service:    newService,
			Username:   newUsername,
			Password:   secret,
			SecretType: keyring.SecretTypeTOTP,
			Issuer:     newIssuer,
			Period:     period,
			Digits:     digits,
			Algorithm:  algorithm,
		}
	} else {
		// For passwords, use new password if provided, otherwise keep existing
			var password string
			if newPassword != "" {
				password = newPassword
			} else {
				// Load existing password
				existingPassword, err := kr.Get(entry.Service, entry.Username)
				if err != nil {
					return ActionResultMsg{
						Success: false,
						Message: fmt.Sprintf("Failed to load existing password: %v", err),
						Action:  "edit_entry",
						Error:   err,
					}
				}
				password = existingPassword
			}
			
			updatedEntry = keyring.Entry{
				Service:    newService,
				Username:   newUsername,
				Password:   password,
				SecretType: keyring.SecretTypePassword,
			}
		}
		
		// Store updated entry
		if err := kr.StoreEntry(updatedEntry); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to update entry: %v", err),
				Action:  "edit_entry",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("Entry %s", newService)
		if newUsername != "" {
			message += fmt.Sprintf(" (%s)", newUsername)
		}
		message += " updated successfully"
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "edit_entry",
		}
	}
}

// handleDeleteEntry processes deleting an entry
func handleDeleteEntry(entry *Entry) tea.Cmd {
	return func() tea.Msg {
		if entry == nil {
			return ActionResultMsg{
				Success: false,
				Message: "No entry to delete",
				Action:  "delete_entry",
			}
		}
		
		kr := keyring.New()
		if err := kr.Delete(entry.Service, entry.Username); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to delete entry: %v", err),
				Action:  "delete_entry",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("Deleted %s", entry.Service)
		if entry.Username != "" {
			message += fmt.Sprintf(" (%s)", entry.Username)
		}
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "delete_entry",
		}
	}
}

// handleCreateBackup processes creating a backup
func handleCreateBackup(fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		backupName := strings.TrimSpace(fields["backup_name"])
		
		kr := keyring.New()
		opts := &backup.BackupOptions{
			BackupDir:  "", // Use default
			MaxBackups: 10,
		}
		
		if backupName != "" {
			opts.OutputPath = backupName
		}
		
		backupMgr := backup.New(kr, opts)
		filename, err := backupMgr.CreateBackup()
		if err != nil {
			return BackupCreatedMsg{
				Error: fmt.Errorf("failed to create backup: %w", err),
			}
		}
		
		return BackupCreatedMsg{
			Filename: filename,
		}
	}
}

// handleRestoreBackup processes restoring a backup
func handleRestoreBackup(backupInfo backup.BackupInfo) tea.Cmd {
	return func() tea.Msg {
		kr := keyring.New()
		backupMgr := backup.New(kr, nil)
		
		if err := backupMgr.RestoreBackup(backupInfo.Path); err != nil {
			return BackupRestoredMsg{
				Success: false,
				Error:   fmt.Errorf("failed to restore backup: %w", err),
			}
		}
		
		return BackupRestoredMsg{
			Success: true,
			Message: fmt.Sprintf("Backup %s restored successfully", backupInfo.Name),
		}
	}
}

// handleDeleteBackup processes deleting a backup file
func handleDeleteBackup(backupInfo backup.BackupInfo) tea.Cmd {
	return func() tea.Msg {
		// This would implement backup file deletion
		// For now, return a placeholder message
		return ActionResultMsg{
			Success: true,
			Message: fmt.Sprintf("Backup %s deleted", backupInfo.Name),
			Action:  "delete_backup",
		}
	}
}

// handleSavePassword processes saving a generated password
func handleSavePassword(fields map[string]string) tea.Cmd {
	return func() tea.Msg {
		service := strings.TrimSpace(fields["service"])
		username := strings.TrimSpace(fields["username"])
		password := strings.TrimSpace(fields["password"])
		
		if service == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Service name is required",
				Action:  "save_password",
			}
		}
		
		if password == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Password is required",
				Action:  "save_password",
			}
		}
		
		// Create password entry
		entry := keyring.Entry{
			Service:    service,
			Username:   username,
			Password:   password,
			SecretType: keyring.SecretTypePassword,
		}
		
		// Store in keyring
		kr := keyring.New()
		if err := kr.StoreEntry(entry); err != nil {
			return ActionResultMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to save password: %v", err),
				Action:  "save_password",
				Error:   err,
			}
		}
		
		message := fmt.Sprintf("Generated password for %s", service)
		if username != "" {
			message += fmt.Sprintf(" (%s)", username)
		}
		message += " saved successfully"
		
		return ActionResultMsg{
			Success: true,
			Message: message,
			Action:  "save_password",
		}
	}
}
