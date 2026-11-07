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
	Restored int // entries created, also when others failed
	Message  string
	Error    error
}

// processFormSubmission processes form submissions and returns appropriate actions
func processFormSubmission(formMsg FormSubmittedMsg) tea.Cmd {
	switch formMsg.Type {
	case AddPasswordForm:
		return addPasswordCmd(formMsg.Fields)
	case AddTOTPForm:
		return handleAddTOTP(formMsg.Fields)
	case AddTOTPToEntryForm:
		return handleAddTOTPToEntry(formMsg.Entry, formMsg.Fields)
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
		
		// An empty field goes through the generate confirm step first
		// (Model.startGenConfirm), so a password always arrives here.
		if password == "" {
			return ActionResultMsg{
				Success: false,
				Message: "Password is required",
				Action:  "add_password",
			}
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

// deleteEntryCmd moves the selected entry to the trash. It goes by ID: names are
// not unique, and the wrong entry must never be the one deleted.
func (m Model) deleteEntryCmd(entry *Entry) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if entry == nil {
			return ActionResultMsg{
				Success: false,
				Message: "No entry to delete",
				Action:  "delete_entry",
			}
		}
		if v == nil {
			return ActionResultMsg{
				Success: false,
				Message: "Failed to delete entry: not connected",
				Action:  "delete_entry",
			}
		}
		
		if err := v.DeleteEntry(entry.ID); err != nil {
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
