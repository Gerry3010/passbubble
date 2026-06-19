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

package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequiresTOTP(t *testing.T) {
	if (&LoginResponse{}).RequiresTOTP() {
		t.Fatal("empty response should not require TOTP")
	}
	if !(&LoginResponse{Status: "2fa_required"}).RequiresTOTP() {
		t.Fatal("status 2fa_required should require TOTP")
	}
}

func TestLoginThenVerifyTOTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "2fa_required", "pending_token": "pending-xyz", "expires_in": 300,
			})
		case "/api/v1/auth/verify-totp":
			var body VerifyTOTPRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.PendingToken != "pending-xyz" || body.Code != "123456" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid code"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "acc", "refresh_token": "ref", "user_id": "u1", "name": "Alice",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL)

	resp, err := c.Login(LoginRequest{Email: "a@b.c", Password: "pw"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !resp.RequiresTOTP() {
		t.Fatalf("expected 2fa_required, got %+v", resp)
	}

	// Wrong code is rejected.
	if _, err := c.VerifyTOTP(resp.PendingToken, "000000"); err == nil {
		t.Fatal("expected error for wrong code")
	}

	// Correct code yields a full session.
	final, err := c.VerifyTOTP(resp.PendingToken, "123456")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if final.RefreshToken != "ref" || final.UserID != "u1" {
		t.Fatalf("unexpected session: %+v", final)
	}
}
