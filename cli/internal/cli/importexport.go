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
	"strings"

	"github.com/spf13/cobra"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(exportCmd)

	importCmd.Flags().StringP("format", "f", "csv-generic",
		"Import format: csv-generic, csv-chrome, csv-lastpass, csv-1password, bitwarden, keepass, psono")
	importCmd.Flags().String("keepass-password", "", "KeePass database password")
	importCmd.Flags().String("on-duplicate", "skip", "Duplicate strategy: skip, overwrite")

	exportCmd.Flags().StringP("format", "f", "csv", "Export format: csv, bitwarden")
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import passwords from an external file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureUnlocked(); err != nil {
			return err
		}

		filePath := args[0]
		format, _ := cmd.Flags().GetString("format")
		onDuplicate, _ := cmd.Flags().GetString("on-duplicate")
		keepassPass, _ := cmd.Flags().GetString("keepass-password")

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
			existingID, isDup := vault.IsDuplicate(rec.Name, rec.Username, existing)

			if isDup {
				switch onDuplicate {
				case "overwrite":
					data := importerRecordToEntryData(rec)
					if err := v.UpdateEntry(existingID, rec.Name, rec.URL, &data, rec.MatchPatterns); err != nil {
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
			outData, err = exporters.ExportBitwarden(records, exporters.BitwardenExportOptions{})
		default: // csv
			outData, err = exporters.ExportCSV(records)
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

func importerRecordToEntryData(rec importers.EntryRecord) vault.EntryData {
	cfs := make([]vault.CustomField, len(rec.CustomFields))
	for i, cf := range rec.CustomFields {
		cfs[i] = vault.CustomField{Label: cf.Label, Value: cf.Value}
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
	}
}

func vaultEntriesToExporterRecords(entries []vault.EntryRecord) []exporters.EntryRecord {
	out := make([]exporters.EntryRecord, len(entries))
	for i, e := range entries {
		cfs := make([]exporters.CustomField, len(e.Data.CustomFields))
		for j, cf := range e.Data.CustomFields {
			cfs[j] = exporters.CustomField{Label: cf.Label, Value: cf.Value}
		}
		out[i] = exporters.EntryRecord{
			Name:         e.Name,
			URL:          e.URL,
			Type:         e.Type,
			Username:     e.Data.Username,
			Password:     e.Data.Password,
			TOTPSecret:   e.Data.TOTPSecret,
			Notes:        e.Data.Notes,
			CardNumber:   e.Data.CardNumber,
			HolderName:   e.Data.HolderName,
			ExpiryMonth:  e.Data.ExpiryMonth,
			ExpiryYear:   e.Data.ExpiryYear,
			CVV:          e.Data.CVV,
			FirstName:    e.Data.FirstName,
			LastName:     e.Data.LastName,
			Company:      e.Data.Company,
			Email:        e.Data.Email,
			Phone:        e.Data.Phone,
			Street:       e.Data.Street,
			City:         e.Data.City,
			State:        e.Data.State,
			PostalCode:   e.Data.PostalCode,
			Country:      e.Data.Country,
			LicenseKey:   e.Data.LicenseKey,
			ProductName:  e.Data.ProductName,
			CustomFields: cfs,
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
