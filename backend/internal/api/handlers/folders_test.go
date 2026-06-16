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

// ── CreateFolder validation ───────────────────────────────────────────────────

func TestCreateFolderValidation(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		name     string
		body     any
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing name",
			body:     models.CreateFolderRequest{},
			wantCode: http.StatusBadRequest,
			wantErr:  "name required",
		},
		{
			name:     "empty name string",
			body:     models.CreateFolderRequest{Name: ""},
			wantCode: http.StatusBadRequest,
			wantErr:  "name required",
		},
		{
			name:     "invalid json",
			body:     "not-json",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/folders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.CreateFolder(w, req)

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

// ── ShareFolder validation ────────────────────────────────────────────────────

func TestShareFolderValidation(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		name     string
		body     models.ShareFolderRequest
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing user_id",
			body:     models.ShareFolderRequest{Permission: "read"},
			wantCode: http.StatusBadRequest,
			wantErr:  "user_id required",
		},
		{
			name:     "invalid json",
			body:     models.ShareFolderRequest{},
			wantCode: http.StatusBadRequest,
			wantErr:  "user_id required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/folders/some-id/share", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.ShareFolder(w, req)

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
