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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

// roundTrip exports vault records in the format and imports the result the way
// `pwmgr import` does, down to the entry data that would be encrypted.
func roundTrip(t *testing.T, records []vault.EntryRecord, exportFormat, importFormat string) []vault.EntryData {
	t.Helper()
	exported := vaultEntriesToExporterRecords(records)
	var (
		data []byte
		err  error
	)
	if exportFormat == "bitwarden" {
		data, err = exporters.ExportBitwarden(exported, exporters.BitwardenExportOptions{})
	} else {
		data, err = exporters.ExportCSV(exported)
	}
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseImportFile(data, importFormat, "")
	if err != nil {
		t.Fatal(err)
	}
	if warnings := normalizeImportedTags(result.Records); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %q", warnings)
	}
	out := make([]vault.EntryData, len(result.Records))
	for i, rec := range result.Records {
		out[i] = importerRecordToEntryData(rec)
	}
	return out
}

func TestImportExportKeepsTags(t *testing.T) {
	records := []vault.EntryRecord{
		{Name: "Tagged", Type: "password", Data: vault.EntryData{Username: "octo", Password: "pw", Tags: []string{"work", "two words"}}},
		{Name: "Untagged", Type: "password", Data: vault.EntryData{Username: "octo", Password: "pw"}},
	}
	for name, formats := range map[string][2]string{
		"csv":       {"csv", "csv-generic"},
		"bitwarden": {"bitwarden", "bitwarden"},
	} {
		t.Run(name, func(t *testing.T) {
			got := roundTrip(t, records, formats[0], formats[1])
			if len(got) != 2 {
				t.Fatalf("want 2 entries, got %d", len(got))
			}
			if !slices.Equal(got[0].Tags, []string{"work", "two words"}) {
				t.Errorf("tags = %q", got[0].Tags)
			}
			for _, cf := range got[0].CustomFields {
				if cf.Label == "passbubble_tags" {
					t.Errorf("the tags field stayed a custom field: %+v", got[0].CustomFields)
				}
			}
			if got[1].Tags != nil {
				t.Errorf("tags = %q, want none", got[1].Tags)
			}
			// "omitted when empty": the entry JSON of an untagged entry has no key.
			blob, err := json.Marshal(got[1])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(blob), `"tags"`) {
				t.Errorf("an untagged entry should have no tags key: %s", blob)
			}
		})
	}
}

func TestImportNormalizesTags(t *testing.T) {
	data := []byte("name,url,username,password,notes,tags\n" +
		`GitHub,,,,,"#Work,  two   words , work,, #"` + "\n")
	result, err := parseImportFile(data, "csv-generic", "")
	if err != nil {
		t.Fatal(err)
	}
	if warnings := normalizeImportedTags(result.Records); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %q", warnings)
	}
	got := importerRecordToEntryData(result.Records[0]).Tags
	if want := []string{"Work", "two words"}; !slices.Equal(got, want) {
		t.Fatalf("tags = %q, want %q", got, want)
	}
}

func TestImportDropsOverlongTagWithWarning(t *testing.T) {
	long := strings.Repeat("x", vault.MaxTagLength+1)
	fits := strings.Repeat("ü", vault.MaxTagLength)
	records := []importers.EntryRecord{{Name: "GitHub", Password: "pw", Tags: []string{"work", long, fits}}}

	warnings := normalizeImportedTags(records)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "GitHub") || !strings.Contains(warnings[0], long) {
		t.Fatalf("warnings = %q", warnings)
	}
	if want := []string{"work", fits}; !slices.Equal(records[0].Tags, want) {
		t.Fatalf("tags = %q, want %q", records[0].Tags, want)
	}
	if records[0].Password != "pw" {
		t.Error("the entry itself must survive")
	}
}

func TestImportWithoutTagsStaysNil(t *testing.T) {
	records := []importers.EntryRecord{{Name: "GitHub"}}
	if warnings := normalizeImportedTags(records); len(warnings) != 0 {
		t.Fatalf("warnings = %q", warnings)
	}
	if records[0].Tags != nil {
		t.Fatalf("tags = %q", records[0].Tags)
	}
}

// commandVault serves stored entries to the real commands and records every change.
type commandVault struct {
	mu        sync.Mutex
	dataKey   []byte // of the entry commandEntryID
	entries   []apiclient.EntryResponse
	trash     []apiclient.EntryResponse
	puts      []apiclient.UpdateEntryRequest
	putIDs    []string
	posts     []apiclient.CreateEntryRequest
	deleted   []string
	purged    []string
	restored  []string
	favorites []string
	versions  []string
}

const (
	commandEntryID  = "e1"
	commandFolderID = "f1"
)

// newCommandVault installs an unlocked vault as the commands' global v. Its
// config lives in a temp dir: the real one is never touched.
func newCommandVault(t *testing.T, stored string) *commandVault {
	t.Helper()
	s := &commandVault{}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)

	const password = "correct horse"
	privX, pubX, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	privM, pubM, err := crypto.GenerateMLKEM768()
	if err != nil {
		t.Fatal(err)
	}
	params := &crypto.KDFParams{Salt: make([]byte, 16), Time: 1, Memory: 8 * 1024}
	masterKey := crypto.DeriveKey(password, params)
	encX, err := crypto.Encrypt(masterKey, privX)
	if err != nil {
		t.Fatal(err)
	}
	encM, err := crypto.Encrypt(masterKey, privM)
	if err != nil {
		t.Fatal(err)
	}
	testCfg := &config.Config{
		ServerURL:       srv.URL,
		UserID:          "u1",
		RefreshToken:    "refresh",
		PubX25519:       crypto.B64Enc(pubX),
		PubMLKEM768:     crypto.B64Enc(pubM),
		EncPrivX25519:   crypto.B64Enc(encX),
		EncPrivMLKEM768: crypto.B64Enc(encM),
		KDFSalt:         crypto.B64Enc(params.Salt),
		KDFTime:         int(params.Time),
		KDFMemory:       int(params.Memory),
	}
	testVault := vault.New(testCfg, filepath.Join(t.TempDir(), "pwmgr.yaml"))
	if err := testVault.Unlock(password); err != nil {
		t.Fatal(err)
	}
	origV, origCfg := v, cfg
	v, cfg = testVault, testCfg
	t.Cleanup(func() { v, cfg = origV, origCfg })

	s.dataKey = s.addEntry(t, apiclient.EntryResponse{
		ID: commandEntryID, Name: "GitHub", URL: "https://github.com", Type: "password",
		FolderID: strPtr(commandFolderID),
	}, stored)
	return s
}

func strPtr(s string) *string { return &s }

// addEntry stores an entry encrypted for the installed vault and returns its data key.
func (s *commandVault) addEntry(t *testing.T, e apiclient.EntryResponse, plaintext string) []byte {
	t.Helper()
	dataKey, err := crypto.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := crypto.Encrypt(dataKey, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	pubX, _ := crypto.B64Dec(v.Config().PubX25519)
	pubM, _ := crypto.B64Dec(v.Config().PubMLKEM768)
	wrapped, err := crypto.EncryptDataKey(dataKey, pubX, pubM)
	if err != nil {
		t.Fatal(err)
	}
	e.EncryptedData = crypto.B64Enc(ciphertext)
	e.EntryKey = &apiclient.EntryKey{UserID: "u1", EncryptedKey: crypto.B64Enc(wrapped)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return dataKey
}

func (s *commandVault) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata := func(entries []apiclient.EntryResponse) []apiclient.EntryResponse {
		out := make([]apiclient.EntryResponse, len(entries))
		for i, e := range entries {
			e.EncryptedData, e.EntryKey = "", nil
			out[i] = e
		}
		return out
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/entries/")
	route := r.Method + " " + r.URL.Path
	switch {
	case route == "POST /api/v1/auth/refresh":
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "a", "refresh_token": "r"})
	case route == "GET /api/v1/entries":
		_ = json.NewEncoder(w).Encode(metadata(s.entries))
	case route == "GET /api/v1/entries/trash":
		_ = json.NewEncoder(w).Encode(metadata(s.trash))
	case route == "GET /api/v1/folders":
		_ = json.NewEncoder(w).Encode([]apiclient.FolderResponse{})
	case route == "POST /api/v1/entries":
		var req apiclient.CreateEntryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.posts = append(s.posts, req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-id"})
	case !strings.HasPrefix(r.URL.Path, "/api/v1/entries/"):
		w.WriteHeader(http.StatusNotFound) // the job ledger is optional for the commands
	case r.Method == http.MethodPut && strings.HasSuffix(path, "/favorite"):
		s.favorites = append(s.favorites, strings.TrimSuffix(path, "/favorite"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/restore") && strings.Contains(path, "/versions/"):
		s.versions = append(s.versions, path)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/restore"):
		s.restored = append(s.restored, strings.TrimSuffix(path, "/restore"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasSuffix(path, "/permanent"):
		s.purged = append(s.purged, strings.TrimSuffix(path, "/permanent"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete:
		s.deleted = append(s.deleted, path)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		var req apiclient.UpdateEntryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.puts = append(s.puts, req)
		s.putIDs = append(s.putIDs, path)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet:
		for _, e := range s.entries {
			if e.ID == path {
				_ = json.NewEncoder(w).Encode(e)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// runImportFile runs the real import command on a file with the given content.
func runImportFile(t *testing.T, content string, flags map[string]string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "import.dat")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for flag, value := range map[string]string{"format": "csv-generic", "on-duplicate": "skip", "replace-all": "false"} {
			_ = importCmd.Flags().Set(flag, value)
		}
	})
	for flag, value := range flags {
		if err := importCmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}
	return importCmd.RunE(importCmd, []string{path})
}

// runImport overwrites with the merge mode.
func runImport(t *testing.T, format, content string) {
	t.Helper()
	if err := runImportFile(t, content, map[string]string{"format": format, "on-duplicate": "overwrite"}); err != nil {
		t.Fatalf("import: %v", err)
	}
}

// runReplaceAll overwrites with --replace-all.
func runReplaceAll(t *testing.T, format, content string) {
	t.Helper()
	err := runImportFile(t, content, map[string]string{"format": format, "on-duplicate": "overwrite", "replace-all": "true"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
}

// saved returns the entry JSON of the only update, opened with the entry's
// original data key: failing to open it means the key changed.
func (s *commandVault) saved(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.posts) != 0 {
		t.Fatalf("an overwrite must not create entries, got %d", len(s.posts))
	}
	if len(s.puts) != 1 {
		t.Fatalf("want exactly 1 update, got %d", len(s.puts))
	}
	put := s.puts[0]
	if s.putIDs[0] != commandEntryID {
		t.Errorf("updated %q", s.putIDs[0])
	}
	if put.EncryptedKey != "" || len(put.EntryKeys) != 0 {
		t.Errorf("an overwrite must not touch the entry keys: %+v", put)
	}
	if put.MatchPatterns != nil {
		t.Errorf("an overwrite must not touch the match patterns: %v", put.MatchPatterns)
	}
	if put.FolderID == nil || *put.FolderID != commandFolderID {
		t.Errorf("the entry left its folder: %v", put.FolderID)
	}
	ciphertext, err := crypto.B64Dec(put.EncryptedData)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypto.Decrypt(s.dataKey, ciphertext)
	if err != nil {
		t.Fatalf("the update did not keep the data key: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(plaintext, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

const overwriteStored = `{
	"username": "octo",
	"password": "old-password",
	"notes": "old notes",
	"card_number": "4111",
	"sign_in_with": "google",
	"totp_secret": "JBSWY3DPEHPK3PXP",
	"issuer": "AWS",
	"iban": "AT00 1234",
	"future_flag": {"nested": true},
	"tags": ["work", "client"],
	"custom_fields": [
		{"label": "PIN", "value": "1234", "type": "password"},
		{"label": "Scan", "value": "aGVsbG8=", "type": "file", "filename": "scan.png", "mime_type": "image/png"},
		{"label": "Motto", "value": "old motto", "future_cf": 1}
	]
}`

// overwriteWant is overwriteStored with the given keys replaced (nil removes).
func overwriteWant(t *testing.T, changes map[string]any) map[string]any {
	t.Helper()
	var want map[string]any
	if err := json.Unmarshal([]byte(overwriteStored), &want); err != nil {
		t.Fatal(err)
	}
	for k, v := range changes {
		if v == nil {
			delete(want, k)
		} else {
			want[k] = v
		}
	}
	return want
}

func TestMergeOnlyTakesNonEmptyValuesFromTheFile(t *testing.T) {
	s := newCommandVault(t, overwriteStored)
	runImport(t, "csv-generic", "name,url,username,password,notes\nGitHub,,octo,new-password,\n")

	// Only the password changes: the empty notes, the missing card number, TOTP
	// secret, tags and custom fields stay, and so do unknown keys.
	want := overwriteWant(t, map[string]any{"password": "new-password"})
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged entry\n got: %v\nwant: %v", got, want)
	}
	if put := s.puts[0]; put.URL != nil {
		t.Errorf("a file without a URL must keep the stored one, sent %q", *put.URL)
	}
}

func TestMergeTakesWhatTheFileHas(t *testing.T) {
	s := newCommandVault(t, overwriteStored)
	const file = `{"items":[{"type":1,"name":"GitHub","notes":"new notes",
		"login":{"username":"octo","password":"new-password","totp":"GEZDGNBVGY3TQOJQ","uris":[{"uri":"https://new.example"}]},
		"fields":[
			{"name":"Motto","value":"new motto","type":1},
			{"name":"PIN","value":"","type":0},
			{"name":"New","value":"y","type":0},
			{"name":"passbubble_tags","value":"imported, #Second","type":0}
		]}]}`
	runImport(t, "bitwarden", file)

	want := overwriteWant(t, map[string]any{
		"password":    "new-password",
		"notes":       "new notes",
		"totp_secret": "GEZDGNBVGY3TQOJQ",
		"tags":        []any{"imported", "Second"},
		"custom_fields": []any{
			// An empty value leaves the field alone, like every other known value.
			map[string]any{"label": "PIN", "value": "1234", "type": "password"},
			// Not in the file: stays in place, file content and all.
			map[string]any{"label": "Scan", "value": "aGVsbG8=", "type": "file", "filename": "scan.png", "mime_type": "image/png"},
			// Same label: new value and type, in place, with its unknown key.
			map[string]any{"label": "Motto", "value": "new motto", "type": "password", "future_cf": float64(1)},
			// New label: appended.
			map[string]any{"label": "New", "value": "y"},
		},
	})
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged entry\n got: %v\nwant: %v", got, want)
	}
	if put := s.puts[0]; put.URL == nil || *put.URL != "https://new.example" {
		t.Errorf("url = %v", put.URL)
	}
}

func TestMergeDoesNotTurnAStrayTOTPSecretIntoARealOne(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"old-password","totp_secret":"old-password"}`)
	runImport(t, "csv-generic", "name,url,username,password,notes\nGitHub,,octo,new-password,\n")

	want := map[string]any{"username": "octo", "password": "new-password"}
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged entry\n got: %v\nwant: %v", got, want)
	}
}

func TestMergeKeepsMatchPatternsAndFolder(t *testing.T) {
	s := newCommandVault(t, `{"username":"user","password":"old"}`)
	const psono = `{"items":[{"type":"website_password","name":"GitHub",
		"website_password_title":"GitHub","website_password_username":"user",
		"website_password_password":"new","urlfilter":"github.com"}]}`
	runImport(t, "psono", psono)

	// saved() checks the folder, the data key and the patterns.
	if got := s.saved(t); got["password"] != "new" {
		t.Fatalf("saved %v", got)
	}
}

func TestReplaceAllMakesTheEntryExactlyWhatTheFileHolds(t *testing.T) {
	s := newCommandVault(t, overwriteStored)
	runReplaceAll(t, "csv-generic", "name,url,username,password,notes,tags\nGitHub,,octo,new-password,,fresh\n")

	want := map[string]any{"username": "octo", "password": "new-password", "tags": []any{"fresh"}}
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("replaced entry\n got: %v\nwant: %v", got, want)
	}
	if put := s.puts[0]; put.URL == nil || *put.URL != "" {
		t.Errorf(`a file without a URL must clear it: sent %v, want ""`, put.URL)
	}
}

func TestReplaceAllTakesTheFilesURLAndCustomFields(t *testing.T) {
	s := newCommandVault(t, overwriteStored)
	const file = `{"items":[{"type":1,"name":"GitHub",
		"login":{"username":"octo","password":"new-password","uris":[{"uri":"https://new.example"}]},
		"fields":[{"name":"PIN","value":"9","type":1}]}]}`
	runReplaceAll(t, "bitwarden", file)

	want := map[string]any{
		"username":      "octo",
		"password":      "new-password",
		"custom_fields": []any{map[string]any{"label": "PIN", "value": "9", "type": "password"}},
	}
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("replaced entry\n got: %v\nwant: %v", got, want)
	}
	if put := s.puts[0]; put.URL == nil || *put.URL != "https://new.example" {
		t.Errorf("url = %v", put.URL)
	}
}

func TestReplaceAllNeedsOverwrite(t *testing.T) {
	s := newCommandVault(t, overwriteStored)
	for name, flags := range map[string]map[string]string{
		"default strategy": {"replace-all": "true"},
		"skip":             {"replace-all": "true", "on-duplicate": "skip"},
	} {
		t.Run(name, func(t *testing.T) {
			err := runImportFile(t, "name,url,username,password,notes\nGitHub,,octo,pw,\n", flags)
			if err == nil || !strings.Contains(err.Error(), "--on-duplicate overwrite") {
				t.Fatalf("err = %v", err)
			}
			if len(s.puts) != 0 || len(s.posts) != 0 {
				t.Fatal("nothing may be written")
			}
		})
	}
}

func TestReplaceAllWithoutOverwriteShowsUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.csv")
	t.Cleanup(func() { _ = importCmd.Flags().Set("replace-all", "false") })
	out, err := runRoot(t, "import", path, "--replace-all")
	if err == nil || !strings.Contains(out, "Usage:") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

// Old TUI edits left duplicates. Overwriting the first one would be a guess.
func TestOverwriteLeavesSeveralMatchesAlone(t *testing.T) {
	for name, flags := range map[string]map[string]string{
		"merge":       {"on-duplicate": "overwrite"},
		"replace all": {"on-duplicate": "overwrite", "replace-all": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newCommandVault(t, `{"username":"octo","password":"one"}`)
			s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "GitHub", Type: "password"}, `{"username":"octo","password":"two"}`)

			if err := runImportFile(t, "name,url,username,password,notes\nGitHub,,octo,new,\n", flags); err != nil {
				t.Fatal(err)
			}
			if len(s.puts) != 0 || len(s.posts) != 0 {
				t.Fatalf("puts %v, posts %d", s.putIDs, len(s.posts))
			}
		})
	}
}

const cardFile = `{"items":[{"type":3,"name":"GitHub","card":{"cardholderName":"Max","number":"4111","code":"123"}}]}`

// A card must not land on a login of the same name: the update cannot change the
// type, so the entry would stay a password entry holding card data.
func TestImportOfAnotherTypeIsNoDuplicate(t *testing.T) {
	for name, flags := range map[string]map[string]string{
		"skip":        {"format": "bitwarden", "on-duplicate": "skip"},
		"merge":       {"format": "bitwarden", "on-duplicate": "overwrite"},
		"replace all": {"format": "bitwarden", "on-duplicate": "overwrite", "replace-all": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newCommandVault(t, `{"password":"pw"}`)
			if err := runImportFile(t, cardFile, flags); err != nil {
				t.Fatal(err)
			}
			if len(s.puts) != 0 {
				t.Fatalf("the login was overwritten: %v", s.putIDs)
			}
			if len(s.posts) != 1 || s.posts[0].Type != "credit-card" || s.posts[0].Name != "GitHub" {
				t.Fatalf("the card should be created as its own entry: %+v", s.posts)
			}
		})
	}
}

func TestImportOverwritesWithinTheTypeFamily(t *testing.T) {
	const secretOnly = `{"items":[{"type":1,"name":"GitHub","login":{"totp":"JBSWY3DPEHPK3PXP"}}]}`
	const withPassword = `{"items":[{"type":1,"name":"GitHub","login":{"password":"new"}}]}`
	for _, tc := range []struct {
		name, stored, file string
		wantKey, wantValue string
	}{
		{"totp record over a login", "password", secretOnly, "totp_secret", "JBSWY3DPEHPK3PXP"},
		{"login record over a totp entry", "totp", withPassword, "password", "new"},
		{"login over an entry without a type", "", withPassword, "password", "new"},
		{"card over a card", "credit-card", cardFile, "card_number", "4111"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newCommandVault(t, `{"password":"pw"}`)
			s.entries[0].Type = tc.stored
			runImport(t, "bitwarden", tc.file)
			if got := s.saved(t); got[tc.wantKey] != tc.wantValue {
				t.Fatalf("saved %v", got)
			}
		})
	}
}

// Another entry of the same name but another type must not make the match
// ambiguous, and a skip must not hide the record behind it either.
func TestImportFindsItsMatchAmongOtherTypes(t *testing.T) {
	s := newCommandVault(t, `{"password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "GitHub", Type: "note"}, `{"notes":"n"}`)

	runImport(t, "csv-generic", "name,url,username,password,notes\nGitHub,,,new,\n")
	if got := s.saved(t); got["password"] != "new" {
		t.Fatalf("saved %v", got)
	}
}

func TestImportDropsAStrayTOTPSecretFromTheFile(t *testing.T) {
	records := []importers.EntryRecord{
		{Name: "Stray", Type: "totp", Password: "pw", TOTPSecret: "pw"},
		{Name: "Real", Type: "totp", Password: "pw", TOTPSecret: "JBSWY3DPEHPK3PXP"},
		{Name: "Only secret", Type: "totp", TOTPSecret: "JBSWY3DPEHPK3PXP"},
	}
	dropStrayTOTPSecrets(records)
	if r := records[0]; r.TOTPSecret != "" || r.Type != "password" || r.Password != "pw" {
		t.Errorf("stray = %+v", r)
	}
	if r := records[1]; r.TOTPSecret != "JBSWY3DPEHPK3PXP" || r.Type != "totp" {
		t.Errorf("real = %+v", r)
	}
	if r := records[2]; r.TOTPSecret != "JBSWY3DPEHPK3PXP" || r.Type != "totp" {
		t.Errorf("only secret = %+v", r)
	}
}

// runExport runs the real export command in Bitwarden format and returns the items.
func runBitwardenExport(t *testing.T) []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.json")
	if err := exportCmd.Flags().Set("format", "bitwarden"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exportCmd.Flags().Set("format", "csv") })
	if err := exportCmd.RunE(exportCmd, []string{path}); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return file.Items
}

func TestBitwardenExportCarriesARealTOTPSecretOnly(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Stray", Type: "password"}, `{"username":"octo","password":"pw","totp_secret":"pw"}`)

	items := runBitwardenExport(t)
	if got := items[0]["login"].(map[string]any)["totp"]; got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("real secret = %v", got)
	}
	login := items[1]["login"].(map[string]any)
	if _, has := login["totp"]; has {
		t.Errorf("a stray secret must not be exported: %v", login)
	}
	if login["password"] != "pw" {
		t.Errorf("login = %v", login)
	}
}

// fieldsByName indexes the custom fields of an exported item.
func fieldsByName(t *testing.T, item map[string]any) map[string]map[string]any {
	t.Helper()
	raw, _ := item["fields"].([]any)
	out := make(map[string]map[string]any, len(raw))
	for _, f := range raw {
		field := f.(map[string]any)
		out[field["name"].(string)] = field
	}
	return out
}

// Whatever the entry holds has to reach the file, whichever type it has.
func TestBitwardenExportKeepsEverythingInTheEntry(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw","totp_secret":"JBSWY3DPEHPK3PXP",
		"issuer":"AWS","period":60,"digits":8,"algorithm":"SHA256","sign_in_with":"google",
		"future_flag":{"nested":true},"future_text":"hello",
		"custom_fields":[{"label":"PIN","value":"1234","type":"password","future_cf":1}]}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Giro", Type: "bank-account"},
		`{"bank_name":"Erste","iban":"AT00 1234","bic":"GIBAATWW","account_number":"1234","account_type":"checking"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e3", Name: "IDE", Type: "license"},
		`{"product_name":"GoLand","license_key":"KEY-1","purchase_email":"max@example.com","purchase_date":"2026-01-01","expires_at":"2027-01-01"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e4", Name: "Visa", Type: "credit-card", URL: "https://bank.example"},
		`{"card_number":"4111","cvv":"123","iban":"AT99"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e5", Name: "Me", Type: "identity"},
		`{"title":"Dr","first_name":"Max","license_key":"K"}`)

	items := runBitwardenExport(t)
	want := []map[string]struct {
		value     string
		fieldType float64
	}{
		{ // GitHub, a login
			"issuer":        {"AWS", 0},
			"period":        {"60", 0},
			"digits":        {"8", 0},
			"algorithm":     {"SHA256", 0},
			"sign_in_with":  {"google", 0},
			"PIN":           {"1234", 1},
			"PIN.future_cf": {"1", 1},
			"future_flag":   {`{"nested":true}`, 1},
			"future_text":   {"hello", 1},
		},
		{ // Giro
			"bank_name":      {"Erste", 0},
			"iban":           {"AT00 1234", 1},
			"bic":            {"GIBAATWW", 0},
			"account_number": {"1234", 1},
			"account_type":   {"checking", 0},
			"type":           {"bank-account", 0},
		},
		{ // IDE
			"product_name":   {"GoLand", 0},
			"license_key":    {"KEY-1", 1},
			"purchase_email": {"max@example.com", 0},
			"purchase_date":  {"2026-01-01", 0},
			"expires_at":     {"2027-01-01", 0},
			"type":           {"license", 0},
		},
		{ // Visa
			"url":  {"https://bank.example", 0},
			"iban": {"AT99", 1},
		},
		{ // Me
			"license_key": {"K", 1},
		},
	}
	for i, fields := range want {
		got := fieldsByName(t, items[i])
		if len(got) != len(fields) {
			t.Errorf("%s: fields %v", items[i]["name"], got)
		}
		for name, f := range fields {
			field, ok := got[name]
			if !ok {
				t.Errorf("%s: field %q is missing, have %v", items[i]["name"], name, got)
				continue
			}
			if field["value"] != f.value || field["type"] != f.fieldType {
				t.Errorf("%s: field %q = %v, want %+v", items[i]["name"], name, field, f)
			}
		}
	}
	if login := items[0]["login"].(map[string]any); login["totp"] != "JBSWY3DPEHPK3PXP" || login["password"] != "pw" {
		t.Errorf("login = %v", login)
	}
	if identity := items[4]["identity"].(map[string]any); identity["title"] != "Dr" {
		t.Errorf("identity = %v", identity)
	}
}

// And back: a title the export now writes must survive the import.
func TestIdentityTitleRoundTrip(t *testing.T) {
	got := roundTrip(t, []vault.EntryRecord{
		{Name: "Me", Type: "identity", Data: vault.EntryData{Title: "Dr", FirstName: "Max"}},
	}, "bitwarden", "bitwarden")
	if got[0].Title != "Dr" || got[0].FirstName != "Max" {
		t.Fatalf("got %+v", got[0])
	}
}

// captureStderr returns what fn prints to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	return <-done
}

func runCSVExport(t *testing.T) (file, stderr string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.csv")
	stderr = captureStderr(t, func() {
		if err := exportCmd.RunE(exportCmd, []string{path}); err != nil {
			t.Errorf("export: %v", err)
		}
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), stderr
}

func TestCSVExportWarnsAboutWhatItLeavesOut(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw","totp_secret":"JBSWY3DPEHPK3PXP",
		"future_flag":true,"custom_fields":[{"label":"PIN","value":"1234","type":"password"}]}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Plain", Type: "password"}, `{"username":"octo","password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e3", Name: "Giro", Type: "bank-account"}, `{"iban":"AT00 1234"}`)

	file, stderr := runCSVExport(t)
	if !strings.Contains(stderr, "Warning") || !strings.Contains(stderr, "4 fields of 2 entries") {
		t.Errorf("stderr = %q", stderr)
	}
	for _, leftOut := range []string{"PIN", "1234", "AT00", "JBSWY3DPEHPK3PXP", "future_flag"} {
		if strings.Contains(file, leftOut) {
			t.Errorf("%q should not be in the file:\n%s", leftOut, file)
		}
	}
	if !strings.HasPrefix(file, "name,url,username,password,notes,tags\n") {
		t.Errorf("the columns changed:\n%s", file)
	}
}

func TestCSVExportSaysNothingWhenNothingIsLeftOut(t *testing.T) {
	newCommandVault(t, `{"username":"octo","password":"pw","notes":"n","tags":["work"]}`)
	if _, stderr := runCSVExport(t); strings.Contains(stderr, "Warning: CSV") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCSVExportCountsInTheSingular(t *testing.T) {
	newCommandVault(t, `{"username":"octo","password":"pw","iban":"AT00"}`)
	if _, stderr := runCSVExport(t); !strings.Contains(stderr, "1 field of 1 entry") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestBitwardenExportKeepsFieldKinds(t *testing.T) {
	newCommandVault(t, `{"username":"octo","password":"pw","custom_fields":[
		{"label":"PIN","value":"1234","type":"password"},
		{"label":"Key","value":"-----BEGIN","type":"ssh"},
		{"label":"Note","value":"plain"},
		{"label":"Scan","value":"aGVsbG8=","type":"file","filename":"scan.png","mime_type":"image/png"}
	]}`)
	path := filepath.Join(t.TempDir(), "export.json")
	if err := exportCmd.Flags().Set("format", "bitwarden"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exportCmd.Flags().Set("format", "csv") })
	if err := exportCmd.RunE(exportCmd, []string{path}); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var file struct {
		Items []struct {
			Fields []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
				Type  int    `json:"type"`
			} `json:"fields"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	type field = struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Type  int    `json:"type"`
	}
	wantFile := []field{
		{"PIN", "1234", 1},
		{"Key", "-----BEGIN", 1},
		{"Note", "plain", 0},
		{"scan.png", "data:image/png;base64,aGVsbG8=", 1},
	}
	if got := file.Items[0].Fields; !slices.Equal(got, wantFile) {
		t.Fatalf("exported fields\n got: %+v\nwant: %+v", got, wantFile)
	}

	// And back: password and file fields come home as what they were.
	result, err := parseImportFile(data, "bitwarden", "")
	if err != nil {
		t.Fatal(err)
	}
	cfs := importerRecordToEntryData(result.Records[0]).CustomFields
	if len(cfs) != 4 {
		t.Fatalf("custom fields = %+v", cfs)
	}
	if cfs[0].Label != "PIN" || cfs[0].Value != "1234" || cfs[0].Type != "password" {
		t.Errorf("password field = %+v", cfs[0])
	}
	if cfs[1].Value != "-----BEGIN" || cfs[1].Type != "password" {
		t.Errorf("ssh field (hidden in the format, so a password here) = %+v", cfs[1])
	}
	if cfs[2].Value != "plain" || (cfs[2].Type != "" && cfs[2].Type != "text") {
		t.Errorf("text field = %+v", cfs[2])
	}
	if f := cfs[3]; f.Type != "file" || f.Value != "aGVsbG8=" || f.Filename != "scan.png" || f.MimeType != "image/png" {
		t.Errorf("file field = %+v", f)
	}
}

func TestCustomFieldMappingKeepsTheKind(t *testing.T) {
	fileField := vault.CustomField{Label: "Scan", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"}
	exported := vaultEntriesToExporterRecords([]vault.EntryRecord{
		{Name: "GitHub", Data: vault.EntryData{CustomFields: []vault.CustomField{fileField}}},
	})
	got := exported[0].CustomFields[0]
	if got.Type != "file" || got.Filename != "scan.png" || got.MimeType != "image/png" || got.Value != "aGVsbG8=" {
		t.Errorf("export mapping = %+v", got)
	}

	data := importerRecordToEntryData(importers.EntryRecord{CustomFields: []importers.CustomField{
		{Label: "Scan", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"},
	}})
	back := data.CustomFields[0]
	if back.Type != "file" || back.Filename != "scan.png" || back.MimeType != "image/png" || back.Value != "aGVsbG8=" {
		t.Errorf("import mapping = %+v", back)
	}
}

func TestMergeCustomFieldEdgeCases(t *testing.T) {
	text := func(label, value string) vault.CustomField { return vault.CustomField{Label: label, Value: value} }
	secret := func(label, value string) vault.CustomField {
		return vault.CustomField{Label: label, Value: value, Type: "password"}
	}
	file := func(label, value, name, mime string) vault.CustomField {
		return vault.CustomField{Label: label, Value: value, Type: "file", Filename: name, MimeType: mime}
	}
	for _, tc := range []struct {
		name                     string
		existing, imported, want []vault.CustomField
	}{
		{
			name:     "nothing in the file leaves all fields alone",
			existing: []vault.CustomField{secret("PIN", "1"), file("Scan", "aGVsbG8=", "s.png", "image/png")},
			want:     []vault.CustomField{secret("PIN", "1"), file("Scan", "aGVsbG8=", "s.png", "image/png")},
		},
		{
			name:     "labels match exactly",
			existing: []vault.CustomField{text("PIN", "1")},
			imported: []vault.CustomField{text("pin", "2")},
			want:     []vault.CustomField{text("PIN", "1"), text("pin", "2")},
		},
		{
			name:     "an empty value leaves the field alone",
			existing: []vault.CustomField{secret("PIN", "1")},
			imported: []vault.CustomField{text("PIN", "")},
			want:     []vault.CustomField{secret("PIN", "1")},
		},
		{
			name:     "a label twice on both sides pairs one to one",
			existing: []vault.CustomField{text("A", "1"), text("B", "b"), text("A", "2")},
			imported: []vault.CustomField{text("A", "x"), text("A", "y")},
			want:     []vault.CustomField{text("A", "x"), text("B", "b"), text("A", "y")},
		},
		{
			name:     "a label twice in the file loses neither",
			existing: []vault.CustomField{text("A", "1")},
			imported: []vault.CustomField{text("A", "x"), text("A", "y")},
			want:     []vault.CustomField{text("A", "x"), text("A", "y")},
		},
		{
			name:     "a label new to the entry, twice in the file, loses neither",
			imported: []vault.CustomField{text("A", "x"), text("A", "y")},
			want:     []vault.CustomField{text("A", "x"), text("A", "y")},
		},
		{
			name:     "a label twice in the entry keeps the unpaired one",
			existing: []vault.CustomField{text("A", "1"), text("A", "2")},
			imported: []vault.CustomField{text("A", "x")},
			want:     []vault.CustomField{text("A", "x"), text("A", "2")},
		},
		{
			name:     "an empty value still takes its partner",
			existing: []vault.CustomField{text("A", "1"), text("A", "2")},
			imported: []vault.CustomField{text("A", ""), text("A", "y")},
			want:     []vault.CustomField{text("A", "1"), text("A", "y")},
		},
		{
			name:     "a text field over a file field drops the old file details",
			existing: []vault.CustomField{file("Scan", "aGVsbG8=", "s.png", "image/png")},
			imported: []vault.CustomField{text("Scan", "now text")},
			want:     []vault.CustomField{text("Scan", "now text")},
		},
		{
			name:     "a file field brings its own details",
			existing: []vault.CustomField{file("Scan", "aGVsbG8=", "s.png", "image/png")},
			imported: []vault.CustomField{file("Scan", "bmV3", "n.pdf", "application/pdf")},
			want:     []vault.CustomField{file("Scan", "bmV3", "n.pdf", "application/pdf")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := slices.Clone(tc.existing)
			got := mergeCustomFields(tc.existing, tc.imported)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
			if !reflect.DeepEqual(tc.existing, before) {
				t.Errorf("the input changed: %+v", tc.existing)
			}
		})
	}
}

func TestReplacedCustomFieldKeepsUnknownKeys(t *testing.T) {
	var existing []vault.CustomField
	const stored = `[{"label":"Scan","value":"aGVsbG8=","type":"file","filename":"s.png","mime_type":"image/png","future_cf":1}]`
	if err := json.Unmarshal([]byte(stored), &existing); err != nil {
		t.Fatal(err)
	}
	merged := mergeCustomFields(existing, []vault.CustomField{{Label: "Scan", Value: "secret", Type: "password"}})
	got, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	var gotAny, wantAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`[{"label":"Scan","value":"secret","type":"password","future_cf":1}]`), &wantAny); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("got %s", got)
	}
}

// The password changes on these paths, so a stray totp_secret would no longer
// equal it and pass for a real secret. A real one and its options must not move.
func TestPasswordChangeNeverPromotesAStrayTOTPSecret(t *testing.T) {
	const newPassword = "new-password"
	csv := "name,url,username,password,notes\nGitHub,,octo," + newPassword + ",\n"
	stray := `{"username":"octo","password":"old","totp_secret":"old"}`
	realSecret := `{"username":"octo","password":"old","totp_secret":"JBSWY3DPEHPK3PXP","issuer":"AWS","period":60,"digits":8,"algorithm":"SHA256"}`

	for _, path := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{"merge import", func(t *testing.T) { runImport(t, "csv-generic", csv) }},
		{"pwmgr update", func(t *testing.T) {
			orig := promptNewPassword
			promptNewPassword = func(string) (string, error) { return newPassword, nil }
			t.Cleanup(func() { promptNewPassword = orig })
			withStdin(t, "\n") // keep the name
			if err := updateCmd.RunE(updateCmd, []string{commandEntryID}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(path.name+", stray", func(t *testing.T) {
			s := newCommandVault(t, stray)
			path.run(t)
			want := map[string]any{"username": "octo", "password": newPassword}
			if got := s.saved(t); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
		t.Run(path.name+", real", func(t *testing.T) {
			s := newCommandVault(t, realSecret)
			path.run(t)
			want := map[string]any{
				"username": "octo", "password": newPassword,
				"totp_secret": "JBSWY3DPEHPK3PXP", "issuer": "AWS",
				"period": float64(60), "digits": float64(8), "algorithm": "SHA256",
			}
			if got := s.saved(t); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

const storedWithTOTPOptions = `{"username":"octo","password":"pw","totp_secret":"JBSWY3DPEHPK3PXP",
	"issuer":"AWS","period":60,"digits":8,"algorithm":"SHA256"}`

func bitwardenLoginWithSecret(secret string) string {
	return `{"items":[{"type":1,"name":"GitHub","login":{"username":"octo","password":"pw","totp":"` + secret + `"}}]}`
}

// The options belong to the old secret: with the new one they make wrong codes.
func TestMergeDropsTheOptionsOfAReplacedSecret(t *testing.T) {
	s := newCommandVault(t, storedWithTOTPOptions)
	runImport(t, "bitwarden", bitwardenLoginWithSecret("GEZDGNBVGY3TQOJQ"))

	want := map[string]any{
		"username": "octo", "password": "pw", "totp_secret": "GEZDGNBVGY3TQOJQ", "issuer": "AWS",
	}
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergeKeepsTheOptionsOfTheSameSecret(t *testing.T) {
	for name, secret := range map[string]string{
		"the same":            "JBSWY3DPEHPK3PXP",
		"written differently": "jbsw y3dp-ehpk 3pxp",
	} {
		t.Run(name, func(t *testing.T) {
			s := newCommandVault(t, storedWithTOTPOptions)
			runImport(t, "bitwarden", bitwardenLoginWithSecret(secret))

			want := map[string]any{
				"username": "octo", "password": "pw", "totp_secret": secret, "issuer": "AWS",
				"period": float64(60), "digits": float64(8), "algorithm": "SHA256",
			}
			if got := s.saved(t); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func TestMergeTakesTheOptionsThatComeWithANewSecret(t *testing.T) {
	existing := vault.EntryData{TOTPSecret: "JBSWY3DPEHPK3PXP", Period: 60, Digits: 8, Algorithm: "SHA256"}
	got := mergeImportedData(existing, vault.EntryData{TOTPSecret: "GEZDGNBVGY3TQOJQ", Digits: 6})
	if got.TOTPSecret != "GEZDGNBVGY3TQOJQ" || got.Digits != 6 || got.Period != 0 || got.Algorithm != "" {
		t.Fatalf("merged %+v", got)
	}
}

func TestMergeTakesARealSecretFromTheFileOverAStrayOne(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"old","totp_secret":"old"}`)
	runImport(t, "bitwarden", `{"items":[{"type":1,"name":"GitHub",
		"login":{"username":"octo","password":"new-password","totp":"GEZDGNBVGY3TQOJQ"}}]}`)

	want := map[string]any{"username": "octo", "password": "new-password", "totp_secret": "GEZDGNBVGY3TQOJQ"}
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
