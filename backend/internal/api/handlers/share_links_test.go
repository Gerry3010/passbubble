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

func TestCreateEntryShareLinkValidation(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		name     string
		body     models.CreateShareLinkRequest
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing encrypted_payload",
			body:     models.CreateShareLinkRequest{PayloadNonce: "bm9uY2U=", ExpiresAt: "2026-07-01T00:00:00Z"},
			wantCode: http.StatusBadRequest,
			wantErr:  "encrypted_payload, payload_nonce, expires_at required",
		},
		{
			name:     "missing payload_nonce",
			body:     models.CreateShareLinkRequest{EncryptedPayload: "Y2lwaGVy", ExpiresAt: "2026-07-01T00:00:00Z"},
			wantCode: http.StatusBadRequest,
			wantErr:  "encrypted_payload, payload_nonce, expires_at required",
		},
		{
			name:     "missing expires_at",
			body:     models.CreateShareLinkRequest{EncryptedPayload: "Y2lwaGVy", PayloadNonce: "bm9uY2U="},
			wantCode: http.StatusBadRequest,
			wantErr:  "encrypted_payload, payload_nonce, expires_at required",
		},
		{
			name: "invalid expires_at format",
			body: models.CreateShareLinkRequest{
				EncryptedPayload: "Y2lwaGVy", PayloadNonce: "bm9uY2U=", ExpiresAt: "not-a-date",
			},
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid expires_at",
		},
		{
			name: "invalid base64 payload",
			body: models.CreateShareLinkRequest{
				EncryptedPayload: "not-base64!!", PayloadNonce: "bm9uY2U=", ExpiresAt: "2026-07-01T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid encrypted_payload",
		},
		{
			name: "invalid base64 nonce",
			body: models.CreateShareLinkRequest{
				EncryptedPayload: "Y2lwaGVy", PayloadNonce: "not-base64!!", ExpiresAt: "2026-07-01T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid payload_nonce",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/some-id/share-link", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.CreateEntryShareLink(w, req)

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

func TestCreateFolderShareLinkValidation(t *testing.T) {
	h := newTestHandler(t)

	body, _ := json.Marshal(models.CreateShareLinkRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/folders/some-id/share-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateFolderShareLink(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}
