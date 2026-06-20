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

package cli

import (
	"fmt"
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(searchCmd)

	addCmd.Flags().StringP("type", "t", "password", "Entry type (password, totp, note, api-key, ssh-key, credit-card, bank-account, identity, license)")
	addCmd.Flags().StringP("url", "u", "", "URL / website")
	addCmd.Flags().StringP("notes", "n", "", "Notes")
	updateCmd.Flags().StringP("url", "u", "", "URL / website")
	updateCmd.Flags().StringP("notes", "n", "", "Notes")
}

var addCmd = &cobra.Command{
	Use:   "add <name> [username]",
	Short: "Add a new entry",
	Long: `Add a new password entry to your vault.

Examples:
  pwmgr add gmail john@gmail.com
  pwmgr add github -t password
  pwmgr add my-note -t note`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		name := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}
		entryType, _ := cmd.Flags().GetString("type")
		url, _ := cmd.Flags().GetString("url")
		notes, _ := cmd.Flags().GetString("notes")

		data, err := collectEntryData(entryType, username, notes, name, v)
		if err != nil {
			return err
		}

		entry, err := v.CreateEntry(name, entryType, url, data, nil, "", "", nil)
		if err != nil {
			return fmt.Errorf("create entry: %w", err)
		}

		fmt.Printf("✓ Entry '%s' saved (id: %s)\n", entry.Name, entry.ID)
		return nil
	},
}

var getCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Retrieve a password entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		matches, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no entry found for '%s'", args[0])
		}

		// Pick first exact match or first result
		id := matches[0].ID
		for _, m := range matches {
			if m.Name == args[0] {
				id = m.ID
				break
			}
		}

		entry, err := v.GetEntry(id)
		if err != nil {
			return err
		}

		printEntry(entry)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		entries, err := v.ListEntries()
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No entries found. Use 'pwmgr add' to store your first entry.")
			return nil
		}

		fmt.Printf("%-40s  %-12s  %s\n", "NAME", "TYPE", "URL")
		fmt.Printf("%-40s  %-12s  %s\n", "----", "----", "---")
		for _, e := range entries {
			url := e.URL
			if len(url) > 40 {
				url = url[:37] + "..."
			}
			fmt.Printf("%-40s  %-12s  %s\n", e.Name, e.Type, url)
		}
		fmt.Printf("\n%d entries total\n", len(entries))
		return nil
	},
}

var updateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an existing entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		matches, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no entry found for '%s'", args[0])
		}

		id := matches[0].ID
		for _, m := range matches {
			if m.Name == args[0] {
				id = m.ID
				break
			}
		}

		// Fetch and decrypt current entry
		entry, err := v.GetEntry(id)
		if err != nil {
			return err
		}

		fmt.Printf("Updating '%s' — leave fields empty to keep current values\n", entry.Name)

		newName, _ := promptInput(fmt.Sprintf("Name [%s]: ", entry.Name))
		if newName == "" {
			newName = entry.Name
		}
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			url = entry.URL
		}

		data := entry.Data
		if data == nil {
			data = &vault.EntryData{}
		}

		newPass, err := promptPassword("New password (leave empty to keep current): ")
		if err != nil {
			return err
		}
		if newPass != "" {
			data.Password = newPass
		}

		notes, _ := cmd.Flags().GetString("notes")
		if notes != "" {
			data.Notes = notes
		}

		if err := v.UpdateEntry(id, newName, url, data, nil); err != nil {
			return fmt.Errorf("update entry: %w", err)
		}

		fmt.Printf("✓ Entry '%s' updated\n", newName)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		matches, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no entry found for '%s'", args[0])
		}

		entry := matches[0]
		for _, m := range matches {
			if m.Name == args[0] {
				entry = m
				break
			}
		}

		if !confirm(fmt.Sprintf("Delete entry '%s'?", entry.Name)) {
			fmt.Println("Cancelled.")
			return nil
		}

		if err := v.DeleteEntry(entry.ID); err != nil {
			return fmt.Errorf("delete entry: %w", err)
		}

		fmt.Printf("✓ Entry '%s' deleted\n", entry.Name)
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Search entries by name or URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}

		entries, err := v.SearchEntries(args[0])
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Printf("No entries found matching '%s'\n", args[0])
			return nil
		}

		fmt.Printf("Found %d entries:\n\n", len(entries))
		for _, e := range entries {
			if e.URL != "" {
				fmt.Printf("  %s (%s)\n", e.Name, e.URL)
			} else {
				fmt.Printf("  %s\n", e.Name)
			}
		}
		return nil
	},
}

func collectEntryData(entryType, username, notes, name string, v *vault.Vault) (*vault.EntryData, error) {
	data := &vault.EntryData{Username: username, Notes: notes}

	switch entryType {
	case "password", "api-key", "":
		pw, err := promptPassword(fmt.Sprintf("Password/Key for %s (leave empty to generate): ", name))
		if err != nil {
			return nil, err
		}
		if pw == "" {
			resp, err := v.Client().Generate(generateDefaults())
			if err != nil {
				return nil, fmt.Errorf("generate password: %w", err)
			}
			if len(resp.Passwords) > 0 {
				pw = resp.Passwords[0].Password
				fmt.Printf("Generated: %s\n", pw)
			}
		}
		data.Password = pw

	case "totp":
		secret, err := promptInput("TOTP Secret (base32): ")
		if err != nil {
			return nil, err
		}
		data.TOTPSecret = secret

	case "ssh-key":
		fmt.Println("Paste private key (end with a line containing only 'END'):")
		var lines []string
		for {
			line, err := promptInput("> ")
			if err != nil {
				return nil, err
			}
			if line == "END" {
				break
			}
			lines = append(lines, line)
		}
		data.Password = strings.Join(lines, "\n")

	case "credit-card":
		cardNum, _ := promptInput("Card Number: ")
		holder, _ := promptInput("Cardholder Name: ")
		expiryM, _ := promptInput("Expiry Month (MM): ")
		expiryY, _ := promptInput("Expiry Year (YYYY): ")
		cvv, err := promptPassword("CVV: ")
		if err != nil {
			return nil, err
		}
		data.CardNumber = cardNum
		data.HolderName = holder
		data.ExpiryMonth = expiryM
		data.ExpiryYear = expiryY
		data.CVV = cvv

	case "bank-account":
		bankName, _ := promptInput("Bank Name: ")
		iban, _ := promptInput("IBAN: ")
		bic, _ := promptInput("BIC / SWIFT: ")
		accNum, _ := promptInput("Account Number: ")
		accType, _ := promptInput("Account Type (checking/savings) [checking]: ")
		if accType == "" {
			accType = "checking"
		}
		data.BankName = bankName
		data.IBAN = iban
		data.BIC = bic
		data.AccountNumber = accNum
		data.AccountType = accType

	case "identity":
		title, _ := promptInput("Title (Mr/Ms/Mrs/Dr): ")
		firstName, _ := promptInput("First Name: ")
		lastName, _ := promptInput("Last Name: ")
		company, _ := promptInput("Company: ")
		email, _ := promptInput("Email: ")
		phone, _ := promptInput("Phone: ")
		street, _ := promptInput("Street: ")
		city, _ := promptInput("City: ")
		state, _ := promptInput("State: ")
		postal, _ := promptInput("Postal Code: ")
		country, _ := promptInput("Country: ")
		data.Title = title
		data.FirstName = firstName
		data.LastName = lastName
		data.Company = company
		data.Email = email
		data.Phone = phone
		data.Street = street
		data.City = city
		data.State = state
		data.PostalCode = postal
		data.Country = country

	case "license":
		product, _ := promptInput("Product Name: ")
		key, _ := promptPassword("License Key: ")
		purchaseEmail, _ := promptInput("Purchase Email: ")
		purchaseDate, _ := promptInput("Purchase Date (YYYY-MM-DD): ")
		expiresAt, _ := promptInput("Expires At (YYYY-MM-DD, leave empty if perpetual): ")
		if err := (error)(nil); err != nil {
			return nil, err
		}
		data.ProductName = product
		data.LicenseKey = key
		data.PurchaseEmail = purchaseEmail
		data.PurchaseDate = purchaseDate
		data.ExpiresAt = expiresAt

	case "note":
		// notes already captured via flag; prompt if empty
		if data.Notes == "" {
			content, err := promptInput("Content: ")
			if err != nil {
				return nil, err
			}
			data.Notes = content
		}
	}

	return data, nil
}

func printEntry(e *vault.Entry) {
	fmt.Printf("Name:   %s\n", e.Name)
	fmt.Printf("Type:   %s\n", e.Type)
	if e.URL != "" {
		fmt.Printf("URL:    %s\n", e.URL)
	}
	if e.Data == nil {
		return
	}
	d := e.Data

	printField := func(label, value string) {
		if value != "" {
			fmt.Printf("%-14s %s\n", label+":", value)
		}
	}

	// Common
	printField("Username", d.Username)
	printField("Password", d.Password)

	// TOTP
	printField("TOTP Secret", d.TOTPSecret)

	// Credit card
	printField("Card Number", d.CardNumber)
	printField("Cardholder", d.HolderName)
	if d.ExpiryMonth != "" || d.ExpiryYear != "" {
		fmt.Printf("%-14s %s/%s\n", "Expires:", d.ExpiryMonth, d.ExpiryYear)
	}
	printField("CVV", d.CVV)

	// Bank
	printField("Bank", d.BankName)
	printField("IBAN", d.IBAN)
	printField("BIC", d.BIC)
	printField("Account No.", d.AccountNumber)
	printField("Account Type", d.AccountType)

	// Identity
	if d.FirstName != "" || d.LastName != "" {
		name := strings.TrimSpace(d.Title + " " + d.FirstName + " " + d.LastName)
		printField("Name", name)
	}
	printField("Company", d.Company)
	printField("Email", d.Email)
	printField("Phone", d.Phone)
	if d.Street != "" {
		addr := strings.Join(filterEmpty(d.Street, d.City, d.State, d.PostalCode, d.Country), ", ")
		printField("Address", addr)
	}

	// License
	printField("Product", d.ProductName)
	printField("License Key", d.LicenseKey)
	printField("Purch. Email", d.PurchaseEmail)
	printField("Purchased", d.PurchaseDate)
	printField("Expires", d.ExpiresAt)

	// Notes (always last)
	if d.Notes != "" {
		fmt.Printf("Notes:\n%s\n", d.Notes)
	}

	// Custom fields
	for _, cf := range d.CustomFields {
		if cf.Label != "" {
			printField(cf.Label, cf.Value)
		}
	}
}

func filterEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
