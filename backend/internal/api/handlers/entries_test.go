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

package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// ── CreateEntry validation ────────────────────────────────────────────────────

func TestCreateEntryValidation(t *testing.T) {
	h := newTestHandler(t)

	validKey := models.EntryKey{UserID: "user-1", EncryptedKey: "base64key=="}

	cases := []struct {
		name     string
		body     any
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing name",
			body:     models.CreateEntryRequest{Type: "password", EncryptedData: "abc", DataNonce: "def", EntryKeys: []models.EntryKey{validKey}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing type",
			body:     models.CreateEntryRequest{Name: "gmail", EncryptedData: "abc", DataNonce: "def", EntryKeys: []models.EntryKey{validKey}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing encrypted_data",
			body:     models.CreateEntryRequest{Name: "gmail", Type: "password", DataNonce: "def", EntryKeys: []models.EntryKey{validKey}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing data_nonce",
			body:     models.CreateEntryRequest{Name: "gmail", Type: "password", EncryptedData: "abc", EntryKeys: []models.EntryKey{validKey}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid type",
			body:     models.CreateEntryRequest{Name: "x", Type: "evil-type", EncryptedData: "abc", DataNonce: "def", EntryKeys: []models.EntryKey{validKey}},
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid entry type",
		},
		{
			name:     "missing entry_keys",
			body:     models.CreateEntryRequest{Name: "x", Type: "password", EncryptedData: "abc", DataNonce: "def"},
			wantCode: http.StatusBadRequest,
			wantErr:  "at least one entry_key required",
		},
		{
			name:     "invalid json",
			body:     "not json at all",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.CreateEntry(w, req)

			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
			if tc.wantErr != "" {
				var resp map[string]string
				_ = json.NewDecoder(w.Body).Decode(&resp)
				if resp["error"] != tc.wantErr {
					t.Fatalf("expected error %q, got %q", tc.wantErr, resp["error"])
				}
			}
		})
	}
}

// ── Entry type whitelist ──────────────────────────────────────────────────────

func TestEntryTypeWhitelist(t *testing.T) {
	h := newTestHandler(t)
	validKey := models.EntryKey{UserID: "u1", EncryptedKey: "k1"}

	validTypes := []string{
		"password", "totp", "note", "api-key", "ssh-key",
		"certificate", "credit-card", "bank-account", "identity", "license",
	}

	for _, typ := range validTypes {
		t.Run("valid/"+typ, func(t *testing.T) {
			// Valid type but no entry_keys → 400 for "entry_key required" not "invalid type"
			body, _ := json.Marshal(models.CreateEntryRequest{
				Name: "test", Type: typ, EncryptedData: "abc", DataNonce: "def",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.CreateEntry(w, req)

			var resp map[string]string
			_ = json.NewDecoder(w.Body).Decode(&resp)
			if resp["error"] == "invalid entry type" {
				t.Fatalf("type %q was rejected as invalid", typ)
			}
		})
	}

	invalidTypes := []string{"exe", "script", "admin", "", "SQL", "password'--"}
	for _, typ := range invalidTypes {
		if typ == "" {
			continue // empty type caught by earlier validation
		}
		t.Run("invalid/"+typ, func(t *testing.T) {
			body, _ := json.Marshal(models.CreateEntryRequest{
				Name: "test", Type: typ, EncryptedData: "abc", DataNonce: "def",
				EntryKeys: []models.EntryKey{validKey},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.CreateEntry(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("type %q should be rejected, got %d", typ, w.Code)
			}
			var resp map[string]string
			_ = json.NewDecoder(w.Body).Decode(&resp)
			if resp["error"] != "invalid entry type" {
				t.Fatalf("expected 'invalid entry type', got %q", resp["error"])
			}
		})
	}
}

// ── UpdateEntry validation ────────────────────────────────────────────────────

func TestUpdateEntryInvalidJSON(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/entries/some-id", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateEntry(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// ── ShareEntry validation ─────────────────────────────────────────────────────

func TestShareEntryValidation(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		name     string
		body     models.ShareEntryRequest
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing user_id",
			body:     models.ShareEntryRequest{EncryptedKey: "k1", Permission: "read"},
			wantCode: http.StatusBadRequest,
			wantErr:  "user_id and encrypted_key required",
		},
		{
			name:     "missing encrypted_key",
			body:     models.ShareEntryRequest{UserID: "user-2", Permission: "read"},
			wantCode: http.StatusBadRequest,
			wantErr:  "user_id and encrypted_key required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/some-id/share", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.ShareEntry(w, req)

			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
			var resp map[string]string
			_ = json.NewDecoder(w.Body).Decode(&resp)
			if resp["error"] != tc.wantErr {
				t.Fatalf("expected error %q, got %q", tc.wantErr, resp["error"])
			}
		})
	}
}

// ── SearchEntries — empty query ───────────────────────────────────────────────

func TestSearchEntriesEmptyQuery(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/search?q=", nil)
	w := httptest.NewRecorder()

	h.SearchEntries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp []any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty array, got %d entries", len(resp))
	}
}
