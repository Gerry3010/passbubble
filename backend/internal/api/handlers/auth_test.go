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

	"github.com/Gerry3010/passbubble/backend/internal/api/handlers"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// newTestHandler creates a Handler with nil pool/rdb for unit tests.
// Only usable for tests that mock DB behaviour.
func newTestHandler(t *testing.T) *handlers.Handler {
	t.Helper()
	return handlers.New(nil, nil, []byte("test-secret-minimum-32-bytes-long!!"))
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.Health(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", resp["status"])
	}
}

func TestLoginMissingFields(t *testing.T) {
	h := newTestHandler(t)
	body, _ := json.Marshal(models.LoginRequest{Email: "", Password: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

func TestRegisterMissingKeys(t *testing.T) {
	h := newTestHandler(t)
	body, _ := json.Marshal(models.RegisterRequest{
		Email:    "test@example.com",
		Name:     "Test",
		Password: "s3cur3!",
		// No crypto keys
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing keys, got %d", w.Code)
	}
}

func TestGeneratePasswords(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name    string
		req     models.GenerateRequest
		wantMin int
		wantMax int
	}{
		{"default", models.GenerateRequest{}, 20, 20},
		{"custom length", models.GenerateRequest{Length: 32}, 32, 32},
		{"multiple", models.GenerateRequest{Length: 16, Count: 3}, 16, 16},
		{"clamp too short", models.GenerateRequest{Length: 2}, 8, 8},
		{"clamp too long", models.GenerateRequest{Length: 999}, 128, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/generate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.Generate(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var resp models.GenerateResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatal(err)
			}
			for _, p := range resp.Passwords {
				if len(p.Password) < tt.wantMin || len(p.Password) > tt.wantMax {
					t.Fatalf("password length %d not in [%d,%d]", len(p.Password), tt.wantMin, tt.wantMax)
				}
				if p.Strength < 0 || p.Strength > 100 {
					t.Fatalf("strength %d out of range", p.Strength)
				}
			}
		})
	}
}
