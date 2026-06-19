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

func postJSON(t *testing.T, path string, body any) *http.Request {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestVerifyTOTPValidation(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		name     string
		body     models.VerifyTOTPRequest
		wantCode int
	}{
		{"missing fields", models.VerifyTOTPRequest{}, http.StatusBadRequest},
		{"missing code", models.VerifyTOTPRequest{PendingToken: "x"}, http.StatusBadRequest},
		{"invalid pending token", models.VerifyTOTPRequest{PendingToken: "not-a-jwt", Code: "123456"}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.VerifyTOTP(w, postJSON(t, "/api/v1/auth/verify-totp", tc.body))
			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestConfirmTOTPValidation(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ConfirmTOTP(w, postJSON(t, "/api/v1/auth/totp/confirm", models.ConfirmTOTPRequest{Secret: "", Code: ""}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRequestTOTPRecoveryValidation(t *testing.T) {
	h := newTestHandler(t)

	t.Run("missing token", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.RequestTOTPRecovery(w, postJSON(t, "/api/v1/auth/totp/recover", models.RequestTOTPRecoveryRequest{}))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.RequestTOTPRecovery(w, postJSON(t, "/api/v1/auth/totp/recover", models.RequestTOTPRecoveryRequest{PendingToken: "garbage"}))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}

func TestResetTOTPMissingToken(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/reset-totp", nil)
	w := httptest.NewRecorder()
	h.ResetTOTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
