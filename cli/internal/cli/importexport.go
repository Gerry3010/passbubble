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
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	localtotp "github.com/Gerry3010/passbubble/cli/pkg/totp"
)

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(exportCmd)

	importCmd.Flags().StringP("format", "f", "csv-generic",
		"Import format: csv-generic, csv-chrome, csv-lastpass, csv-1password, bitwarden, keepass, psono")
	importCmd.Flags().String("keepass-password", "", "KeePass database password")
	importCmd.Flags().String("on-duplicate", "skip", "Duplicate strategy: skip, overwrite (non-empty values from the file win, everything else stays)")
	importCmd.Flags().Bool("replace-all", false, "With --on-duplicate overwrite: the entry becomes exactly what the file holds, everything else in it is deleted")

	exportCmd.Flags().StringP("format", "f", "csv", "Export format: csv, bitwarden")
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import passwords from an external file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkReplaceAll(cmd); err != nil {
			return err
		}
		if err := ensureUnlocked(); err != nil {
			return err
		}

		filePath := args[0]
		format, _ := cmd.Flags().GetString("format")
		onDuplicate, _ := cmd.Flags().GetString("on-duplicate")
		keepassPass, _ := cmd.Flags().GetString("keepass-password")
		replaceAll, _ := cmd.Flags().GetBool("replace-all")

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		// Parse file
		fmt.Printf("Parsing %s (format: %s)...\n", filePath, format)
		result, err := parseImportFile(data, format, keepassPass)
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		result.Warnings = append(result.Warnings, normalizeImportedTags(result.Records)...)
		dropStrayTOTPSecrets(result.Records)
		if len(result.Warnings) > 0 {
			for _, w := range result.Warnings {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
			}
		}
		fmt.Printf("Found %d records (%d skipped during parse)\n", len(result.Records), result.Skipped)

		if len(result.Records) == 0 {
			fmt.Println("Nothing to import.")
			return nil
		}

		// Create job ledger entry
		job, err := v.Client().CreateJob(apiclient.CreateJobRequest{
			Type:        "import",
			Format:      format,
			DupStrategy: onDuplicate,
			TotalItems:  len(result.Records),
			ClientName:  "cli",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create job ledger entry: %v\n", err)
		}

		// Fetch existing entries for duplicate detection
		fmt.Println("Loading existing vault for duplicate detection...")
		existing, err := v.ListAllDecrypted()
		if err != nil {
			_ = failJob(v.Client(), job, err.Error())
			return fmt.Errorf("load vault: %w", err)
		}

		// Seed the folder cache from existing folders so imports reuse them.
		folders, err := v.ListFolders()
		if err != nil {
			_ = failJob(v.Client(), job, err.Error())
			return fmt.Errorf("load folders: %w", err)
		}
		folderCache := buildFolderCache(folders)

		// Process records
		created, updated, skipped, failed := 0, 0, 0, 0
		for i, rec := range result.Records {
			duplicates := vault.FindDuplicates(rec.Name, rec.Username, rec.Type, existing)

			if len(duplicates) > 0 {
				switch {
				case onDuplicate == "overwrite" && len(duplicates) > 1:
					fmt.Fprintf(os.Stderr, "  [%d/%d] Not overwriting %q: %d entries match, IDs %s\n",
						i+1, len(result.Records), rec.Name, len(duplicates), strings.Join(duplicates, ", "))
					failed++
				case onDuplicate == "overwrite":
					if err := overwriteEntry(v, duplicates[0], rec, replaceAll); err != nil {
						fmt.Fprintf(os.Stderr, "  [%d/%d] Failed to update %q: %v\n", i+1, len(result.Records), rec.Name, err)
						failed++
					} else {
						fmt.Printf("  [%d/%d] Updated %q\n", i+1, len(result.Records), rec.Name)
						updated++
					}
				default: // skip
					fmt.Printf("  [%d/%d] Skipped duplicate %q\n", i+1, len(result.Records), rec.Name)
					skipped++
				}
			} else {
				data := importerRecordToEntryData(rec)
				entryType := rec.Type
				if entryType == "" {
					entryType = "password"
				}
				folderID, ferr := resolveFolderID(v, folderCache, rec.FolderPath)
				if ferr != nil {
					fmt.Fprintf(os.Stderr, "  [%d/%d] Folder resolution failed for %q: %v\n", i+1, len(result.Records), rec.Name, ferr)
				}
				if _, err := v.CreateEntry(rec.Name, entryType, rec.URL, &data, folderID, rec.CreatedAt, rec.UpdatedAt, rec.MatchPatterns); err != nil {
					fmt.Fprintf(os.Stderr, "  [%d/%d] Failed to create %q: %v\n", i+1, len(result.Records), rec.Name, err)
					failed++
				} else {
					fmt.Printf("  [%d/%d] Created %q\n", i+1, len(result.Records), rec.Name)
					created++
				}
			}

			// Report progress every 10 records
			if job != nil && (i+1)%10 == 0 {
				proc := i + 1
				cr, up, sk, fa := created, updated, skipped, failed
				_, _ = v.Client().UpdateJob(job.ID, apiclient.UpdateJobRequest{
					ProcessedItems: &proc,
					CreatedItems:   &cr,
					UpdatedItems:   &up,
					SkippedItems:   &sk,
					FailedItems:    &fa,
				})
			}
		}

		// Final job update
		if job != nil {
			status := "completed"
			if failed > 0 && created+updated == 0 {
				status = "failed"
			}
			total := len(result.Records)
			_, _ = v.Client().UpdateJob(job.ID, apiclient.UpdateJobRequest{
				Status:         status,
				ProcessedItems: &total,
				CreatedItems:   &created,
				UpdatedItems:   &updated,
				SkippedItems:   &skipped,
				FailedItems:    &failed,
			})
		}

		fmt.Printf("\nImport complete: %d created, %d updated, %d skipped, %d failed\n",
			created, updated, skipped, failed)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export passwords to a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		filePath := args[0]
		format, _ := cmd.Flags().GetString("format")

		fmt.Println("Loading vault...")
		vaultEntries, err := v.ListAllDecrypted()
		if err != nil {
			return fmt.Errorf("load vault: %w", err)
		}

		// Create job for visibility
		job, err := v.Client().CreateJob(apiclient.CreateJobRequest{
			Type:       "export",
			Format:     format,
			TotalItems: len(vaultEntries),
			ClientName: "cli",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create job ledger entry: %v\n", err)
		}

		records := vaultEntriesToExporterRecords(vaultEntries)

		var outData []byte
		switch format {
		case "bitwarden":
			// File fields travel as hidden data: URIs, and the importer knows them.
			outData, err = exporters.ExportBitwarden(records, exporters.BitwardenExportOptions{IncludeFiles: true, FilesAsBase64: true})
		default: // csv
			outData, err = exporters.ExportCSV(records)
			if err == nil {
				warnAboutCSVLeftOut(records)
			}
		}
		if err != nil {
			_ = failJob(v.Client(), job, err.Error())
			return fmt.Errorf("export: %w", err)
		}

		if err := os.WriteFile(filePath, outData, 0600); err != nil {
			_ = failJob(v.Client(), job, err.Error())
			return fmt.Errorf("write file: %w", err)
		}

		// Mark job complete
		if job != nil {
			total := len(records)
			_, _ = v.Client().UpdateJob(job.ID, apiclient.UpdateJobRequest{
				Status:         "completed",
				ProcessedItems: &total,
				CreatedItems:   &total,
			})
		}

		fmt.Printf("Exported %d entries to %s\n", len(records), filePath)
		return nil
	},
}

// warnAboutCSVLeftOut says what the export leaves behind. The columns of a CSV are
// fixed, and other programs must be able to read it, so custom fields are no way out.
func warnAboutCSVLeftOut(records []exporters.EntryRecord) {
	fields, entries := exporters.CSVLeftOut(records)
	if fields == 0 {
		return
	}
	fieldWord, entryWord := "fields", "entries"
	if fields == 1 {
		fieldWord = "field"
	}
	if entries == 1 {
		entryWord = "entry"
	}
	fmt.Fprintf(os.Stderr, "Warning: CSV has no column for %d %s of %d %s (TOTP secrets, card and bank data, custom fields and the like), so they are NOT in the file. --format bitwarden keeps them.\n",
		fields, fieldWord, entries, entryWord)
}

func parseImportFile(data []byte, format, keepassPass string) (*importers.ImportResult, error) {
	switch format {
	case "csv-generic":
		return importers.ParseCSV(data, importers.CSVFormatGeneric)
	case "csv-chrome":
		return importers.ParseCSV(data, importers.CSVFormatChrome)
	case "csv-lastpass":
		return importers.ParseCSV(data, importers.CSVFormatLastPass)
	case "csv-1password":
		return importers.ParseCSV(data, importers.CSVFormat1Password)
	case "bitwarden":
		return importers.ParseBitwarden(data)
	case "psono":
		return importers.ParsePsono(data)
	case "keepass":
		if keepassPass == "" {
			var err error
			keepassPass, err = promptPassword("KeePass master password: ")
			if err != nil {
				return nil, err
			}
		}
		return importers.ParseKeePass(bytes.NewReader(data), keepassPass)
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// normalizeImportedTags applies the tag rules to what the parsers read and
// returns a warning per tag it had to drop. A tag that is too long costs the
// tag, not the entry: an import must not fail over it.
func normalizeImportedTags(records []importers.EntryRecord) []string {
	var warnings []string
	for i := range records {
		rec := &records[i]
		var keep []string
		for _, tag := range rec.Tags {
			if utf8.RuneCountInString(vault.NormalizeTag(tag)) > vault.MaxTagLength {
				warnings = append(warnings, fmt.Sprintf("%s: dropped tag %q, longer than %d characters",
					rec.Name, tag, vault.MaxTagLength))
				continue
			}
			keep = append(keep, tag)
		}
		tags, err := vault.NormalizeTags(keep)
		if err != nil { // cannot happen: the long ones are out
			warnings = append(warnings, fmt.Sprintf("%s: tags dropped: %v", rec.Name, err))
			tags = nil
		}
		rec.Tags = tags
	}
	return warnings
}

// checkReplaceAll rejects --replace-all without --on-duplicate overwrite. That is
// a wrong invocation, so the usage comes back on.
func checkReplaceAll(cmd *cobra.Command) error {
	replaceAll, _ := cmd.Flags().GetBool("replace-all")
	onDuplicate, _ := cmd.Flags().GetString("on-duplicate")
	if replaceAll && onDuplicate != "overwrite" {
		cmd.SilenceUsage = false
		return fmt.Errorf("--replace-all needs --on-duplicate overwrite")
	}
	return nil
}

// overwriteEntry saves an imported record over an existing entry, merged or, with
// replaceAll, as the whole new state. Both keep the data key, so everyone the
// entry is shared with can still read it, and the folder, match patterns and
// favourite, which the update leaves alone.
func overwriteEntry(v *vault.Vault, id string, rec importers.EntryRecord, replaceAll bool) error {
	imported := importerRecordToEntryData(rec)
	if replaceAll {
		return v.ReplaceEntry(id, rec.Name, rec.URL, &imported)
	}
	current, err := v.GetEntry(id)
	if err != nil {
		return err
	}
	merged := mergeImportedData(*current.Data, imported)
	return v.UpdateEntry(id, rec.Name, rec.URL, &merged, nil)
}

// mergeImportedData is what a merging overwrite saves over an existing entry.
// Non-empty known values from the file win, empty ones leave the existing value
// alone. Everything the file cannot speak to stays, unknown keys included. Tags
// are replaced as a whole, only when the record has any. A new TOTP secret takes
// its code options from the file or goes without: the stored ones belong to the
// old secret and would produce wrong codes. Keep in step with mergeImportedData
// in the app (flutter_app/lib/core/jobs/job_runner.dart).
func mergeImportedData(existing, imported vault.EntryData) vault.EntryData {
	merged := existing
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&merged.Username, imported.Username},
		{&merged.Password, imported.Password},
		{&merged.TOTPSecret, imported.TOTPSecret},
		{&merged.Notes, imported.Notes},
		{&merged.CardNumber, imported.CardNumber},
		{&merged.HolderName, imported.HolderName},
		{&merged.ExpiryMonth, imported.ExpiryMonth},
		{&merged.ExpiryYear, imported.ExpiryYear},
		{&merged.CVV, imported.CVV},
		{&merged.Title, imported.Title},
		{&merged.FirstName, imported.FirstName},
		{&merged.LastName, imported.LastName},
		{&merged.Company, imported.Company},
		{&merged.Email, imported.Email},
		{&merged.Phone, imported.Phone},
		{&merged.Street, imported.Street},
		{&merged.City, imported.City},
		{&merged.State, imported.State},
		{&merged.PostalCode, imported.PostalCode},
		{&merged.Country, imported.Country},
		{&merged.LicenseKey, imported.LicenseKey},
		{&merged.ProductName, imported.ProductName},
	} {
		if f.src != "" {
			*f.dst = f.src
		}
	}
	if imported.TOTPSecret != "" && !localtotp.SameSecret(imported.TOTPSecret, existing.TOTPSecret) {
		merged.Period, merged.Digits, merged.Algorithm = imported.Period, imported.Digits, imported.Algorithm
	}
	merged.CustomFields = mergeCustomFields(existing.CustomFields, imported.CustomFields)
	if len(imported.Tags) > 0 {
		merged.Tags = imported.Tags
	}
	return merged
}

// mergeCustomFields merges by exact label: a field with a known label replaces
// value and kind in place and keeps any keys the CLI does not model; filename and
// MIME type belong to the old value, so a file field in the file brings its own
// and anything else drops them. A new label is appended, and fields the file does
// not mention stay. As for the known fields, an empty value leaves the existing
// one alone. Each existing field pairs with one field in the file, so a label
// that repeats on either side loses nothing.
func mergeCustomFields(existing, imported []vault.CustomField) []vault.CustomField {
	merged := slices.Clone(existing)
	paired := make(map[int]bool, len(imported))
	for _, in := range imported {
		slot := -1
		for i := range merged {
			if !paired[i] && merged[i].Label == in.Label {
				slot = i
				break
			}
		}
		if slot < 0 {
			merged = append(merged, in)
			paired[len(merged)-1] = true
			continue
		}
		paired[slot] = true
		if in.Value == "" {
			continue
		}
		merged[slot].Value = in.Value
		merged[slot].Type = in.Type
		merged[slot].Filename = in.Filename
		merged[slot].MimeType = in.MimeType
	}
	return merged
}

// dropStrayTOTPSecrets treats a TOTP secret that is just the password as absent:
// an export from an entry the old TUI added can carry one. A record typed totp
// because of it goes back to password.
func dropStrayTOTPSecrets(records []importers.EntryRecord) {
	for i := range records {
		rec := &records[i]
		if vault.IsStrayTOTPSecret(rec.TOTPSecret, rec.Password) {
			rec.TOTPSecret = ""
			if rec.Type == "totp" {
				rec.Type = "password"
			}
		}
	}
}

func importerRecordToEntryData(rec importers.EntryRecord) vault.EntryData {
	cfs := make([]vault.CustomField, len(rec.CustomFields))
	for i, cf := range rec.CustomFields {
		cfs[i] = vault.CustomField{Label: cf.Label, Value: cf.Value, Type: cf.Type, Filename: cf.Filename, MimeType: cf.MimeType}
	}
	return vault.EntryData{
		Username:     rec.Username,
		Password:     rec.Password,
		TOTPSecret:   rec.TOTPSecret,
		Notes:        rec.Notes,
		CardNumber:   rec.CardNumber,
		HolderName:   rec.HolderName,
		ExpiryMonth:  rec.ExpiryMonth,
		ExpiryYear:   rec.ExpiryYear,
		CVV:          rec.CVV,
		Title:        rec.Title,
		FirstName:    rec.FirstName,
		LastName:     rec.LastName,
		Company:      rec.Company,
		Email:        rec.Email,
		Phone:        rec.Phone,
		Street:       rec.Street,
		City:         rec.City,
		State:        rec.State,
		PostalCode:   rec.PostalCode,
		Country:      rec.Country,
		LicenseKey:   rec.LicenseKey,
		ProductName:  rec.ProductName,
		CustomFields: cfs,
		Tags:         rec.Tags,
	}
}

// vaultEntriesToExporterRecords hands the exporters everything an entry holds. What
// a format has no place for is for the format to decide, not for this mapping.
func vaultEntriesToExporterRecords(entries []vault.EntryRecord) []exporters.EntryRecord {
	out := make([]exporters.EntryRecord, len(entries))
	for i, e := range entries {
		cfs := make([]exporters.CustomField, len(e.Data.CustomFields))
		for j, cf := range e.Data.CustomFields {
			cfs[j] = exporters.CustomField{Label: cf.Label, Value: cf.Value, Type: cf.Type, Filename: cf.Filename, MimeType: cf.MimeType}
		}
		var unknown []exporters.UnknownField
		for _, f := range e.Data.UnknownFields() {
			unknown = append(unknown, exporters.UnknownField{Key: f.Key, Value: f.Value})
		}
		out[i] = exporters.EntryRecord{
			Name:          e.Name,
			URL:           e.URL,
			Type:          e.Type,
			Username:      e.Data.Username,
			Password:      e.Data.Password,
			TOTPSecret:    e.Data.TOTPSecret,
			Notes:         e.Data.Notes,
			SignInWith:    e.Data.SignInWith,
			Issuer:        e.Data.Issuer,
			Period:        e.Data.Period,
			Digits:        e.Data.Digits,
			Algorithm:     e.Data.Algorithm,
			CardNumber:    e.Data.CardNumber,
			HolderName:    e.Data.HolderName,
			ExpiryMonth:   e.Data.ExpiryMonth,
			ExpiryYear:    e.Data.ExpiryYear,
			CVV:           e.Data.CVV,
			BankName:      e.Data.BankName,
			IBAN:          e.Data.IBAN,
			BIC:           e.Data.BIC,
			AccountNumber: e.Data.AccountNumber,
			AccountType:   e.Data.AccountType,
			Title:         e.Data.Title,
			FirstName:     e.Data.FirstName,
			LastName:      e.Data.LastName,
			Company:       e.Data.Company,
			Email:         e.Data.Email,
			Phone:         e.Data.Phone,
			Street:        e.Data.Street,
			City:          e.Data.City,
			State:         e.Data.State,
			PostalCode:    e.Data.PostalCode,
			Country:       e.Data.Country,
			LicenseKey:    e.Data.LicenseKey,
			ProductName:   e.Data.ProductName,
			PurchaseEmail: e.Data.PurchaseEmail,
			PurchaseDate:  e.Data.PurchaseDate,
			ExpiresAt:     e.Data.ExpiresAt,
			CustomFields:  cfs,
			Tags:          e.Data.Tags,
			UnknownFields: unknown,
		}
	}
	return out
}

// folderKey joins a folder path into a cache key. Uses NUL as separator because
// folder names may themselves contain "/".
func folderKey(path []string) string {
	return strings.Join(path, "\x00")
}

// buildFolderCache flattens the existing folder tree into a path→ID lookup.
func buildFolderCache(folders []*vault.Folder) map[string]string {
	cache := map[string]string{}
	var walk func(prefix []string, fs []*vault.Folder)
	walk = func(prefix []string, fs []*vault.Folder) {
		for _, f := range fs {
			p := append(append([]string{}, prefix...), f.Name)
			cache[folderKey(p)] = f.ID
			walk(p, f.Children)
		}
	}
	walk(nil, folders)
	return cache
}

// resolveFolderID maps a folder path to a folder ID, creating any missing
// folders along the way (and caching them). Returns nil for the root level.
func resolveFolderID(v *vault.Vault, cache map[string]string, path []string) (*string, error) {
	if len(path) == 0 {
		return nil, nil
	}
	var parentID *string
	for i := 1; i <= len(path); i++ {
		key := folderKey(path[:i])
		if id, ok := cache[key]; ok {
			idCopy := id
			parentID = &idCopy
			continue
		}
		newID, err := v.CreateFolder(path[i-1], parentID)
		if err != nil {
			return nil, err
		}
		cache[key] = newID
		idCopy := newID
		parentID = &idCopy
	}
	return parentID, nil
}

func failJob(c *apiclient.Client, job *apiclient.JobResponse, msg string) error {
	if job == nil {
		return nil
	}
	_, err := c.UpdateJob(job.ID, apiclient.UpdateJobRequest{
		Status:       "failed",
		ErrorMessage: msg,
	})
	return err
}
