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

func TestUpdateKeysInvalidBody(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me/keys", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateKeys(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestUpdateKeysMissingFields(t *testing.T) {
	cases := map[string]models.UpdateKeysRequest{
		"all empty":         {},
		"missing mlkem pub": {PubX25519: "a", EncPrivX25519: "b", EncPrivMLKEM768: "c"},
		"missing enc priv":  {PubX25519: "a", PubMLKEM768: "b"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h := newTestHandler(t)
			b, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me/keys", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.UpdateKeys(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d", name, w.Code)
			}
		})
	}
}
