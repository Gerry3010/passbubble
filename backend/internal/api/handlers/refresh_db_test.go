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

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gerry3010/passbubble/backend/internal/api/models"
	"github.com/Gerry3010/passbubble/backend/internal/db"
)

// testDB connects to DATABASE_URL (CI provides a Postgres service) and applies
// the migrations; the test is skipped without one.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := db.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestRefreshRotationGraceWindow(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, status, password_hash, pub_x25519, pub_mlkem768,
		                   enc_priv_x25519, enc_priv_mlkem768, kdf_salt)
		VALUES ($1, 'refresh test', 'active', 'x', 'x', 'x', 'x', 'x', 'x')
		RETURNING id`, "refresh-"+uuid.NewString()+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID) })

	h := New(pool, nil, testSecret, "", nil)
	refresh := func(token string) (int, models.LoginResponse) {
		t.Helper()
		body, _ := json.Marshal(models.RefreshRequest{RefreshToken: token})
		rec := httptest.NewRecorder()
		h.Refresh(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(string(body))))
		var resp models.LoginResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return rec.Code, resp
	}

	login, err := h.issueTokens(ctx, userID, "user")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}

	// Two clients racing on the same token both end up with a working session.
	code1, first := refresh(login.RefreshToken)
	code2, second := refresh(login.RefreshToken)
	if code1 != http.StatusOK || code2 != http.StatusOK {
		t.Fatalf("racing refreshes: got %d and %d, want 200 twice", code1, code2)
	}
	if first.RefreshToken == second.RefreshToken {
		t.Fatal("racing refreshes must get distinct sessions")
	}
	if code, _ := refresh(second.RefreshToken); code != http.StatusOK {
		t.Fatalf("token from the losing client: got %d, want 200", code)
	}

	// Once the grace period is over, a rotated token is dead.
	saved := refreshGracePeriod
	refreshGracePeriod = 0
	t.Cleanup(func() { refreshGracePeriod = saved })
	if code, _ := refresh(first.RefreshToken); code != http.StatusOK {
		t.Fatalf("rotating with no grace: got %d, want 200", code)
	}
	if code, _ := refresh(first.RefreshToken); code != http.StatusUnauthorized {
		t.Fatalf("reusing a retired token: got %d, want 401", code)
	}
}
