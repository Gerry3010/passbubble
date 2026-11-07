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
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// Restoring a version gives its wrapped keys back to the people who still have
// access, without locking out later recipients or letting removed ones back in.
func TestRestoreVersionKeepsCurrentRecipients(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	newUser := func(label string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (email, name, status, password_hash, pub_x25519, pub_mlkem768,
			                   enc_priv_x25519, enc_priv_mlkem768, kdf_salt)
			VALUES ($1, $2, 'active', 'x', 'x', 'x', 'x', 'x', 'x')
			RETURNING id`, label+"-"+uuid.NewString()+"@example.test", label).Scan(&id); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id) })
		return id
	}
	owner, stayed, joinedLater, removed := newUser("owner"), newUser("stayed"), newUser("joined"), newUser("removed")

	var entryID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO entries (owner_id, type, name, encrypted_data, data_nonce)
		VALUES ($1, 'password', 'v1', 'blob-1', 'nonce-1') RETURNING id`, owner).Scan(&entryID); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM entries WHERE id=$1`, entryID) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO entry_permissions (entry_id, user_id, permission) VALUES ($1, $2, 'owner')`, entryID, owner); err != nil {
		t.Fatalf("insert permission: %v", err)
	}
	setKey := func(userID, key string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO entry_keys (entry_id, user_id, encrypted_key) VALUES ($1, $2, $3)
			ON CONFLICT (entry_id, user_id) DO UPDATE SET encrypted_key = EXCLUDED.encrypted_key`,
			entryID, userID, []byte(key)); err != nil {
			t.Fatalf("set key: %v", err)
		}
	}
	setKey(owner, "owner-v1")
	setKey(stayed, "stayed-v1")
	setKey(removed, "removed-v1")

	h := New(pool, nil, testSecret, "", nil)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := h.snapshotVersionTx(ctx, tx, entryID, owner, ""); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var versionID string
	if err := pool.QueryRow(ctx, `SELECT id FROM entry_versions WHERE entry_id=$1`, entryID).Scan(&versionID); err != nil {
		t.Fatalf("version id: %v", err)
	}

	// After the snapshot: the entry is re-keyed, one person joins, one is removed.
	setKey(owner, "owner-v2")
	setKey(stayed, "stayed-v2")
	setKey(joinedLater, "joined-v2")
	if _, err := pool.Exec(ctx, `DELETE FROM entry_keys WHERE entry_id=$1 AND user_id=$2`, entryID, removed); err != nil {
		t.Fatalf("remove key: %v", err)
	}

	login, err := h.issueTokens(ctx, owner, "user")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	router := chi.NewRouter()
	router.Use(mw.JWTAuth(testSecret))
	router.Post("/entries/{id}/versions/{vid}/restore", h.RestoreVersion)
	req := httptest.NewRequest(http.MethodPost, "/entries/"+entryID+"/versions/"+versionID+"/restore", nil)
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("restore: got %d, want 204: %s", rec.Code, rec.Body.String())
	}

	keys := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT user_id::text, encrypted_key FROM entry_keys WHERE entry_id=$1`, entryID)
	if err != nil {
		t.Fatalf("read keys: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var key []byte
		if err := rows.Scan(&userID, &key); err != nil {
			t.Fatalf("scan key: %v", err)
		}
		keys[userID] = string(key)
	}
	want := map[string]string{owner: "owner-v1", stayed: "stayed-v1", joinedLater: "joined-v2"}
	if len(keys) != len(want) {
		t.Fatalf("got %d keys, want %d: %v", len(keys), len(want), keys)
	}
	for userID, key := range want {
		if keys[userID] != key {
			t.Errorf("key of %s: got %q, want %q", userID, keys[userID], key)
		}
	}
}

// An update keeps the URL unless it says otherwise; an empty one clears it.
func TestUpdateEntryURL(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	var owner string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, status, password_hash, pub_x25519, pub_mlkem768,
		                   enc_priv_x25519, enc_priv_mlkem768, kdf_salt)
		VALUES ($1, 'url test', 'active', 'x', 'x', 'x', 'x', 'x', 'x')
		RETURNING id`, "url-"+uuid.NewString()+"@example.test").Scan(&owner); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner) })
	var entryID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO entries (owner_id, type, name, url, encrypted_data, data_nonce)
		VALUES ($1, 'password', 'n', 'https://old.example', 'b', 'n') RETURNING id`, owner).Scan(&entryID); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM entries WHERE id=$1`, entryID) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO entry_permissions (entry_id, user_id, permission) VALUES ($1, $2, 'owner')`, entryID, owner); err != nil {
		t.Fatalf("insert permission: %v", err)
	}

	h := New(pool, nil, testSecret, "", nil)
	login, err := h.issueTokens(ctx, owner, "user")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	router := chi.NewRouter()
	router.Use(mw.JWTAuth(testSecret))
	router.Put("/entries/{id}", h.UpdateEntry)
	router.Get("/entries/{id}", h.GetEntry)
	router.Get("/entries", h.ListEntries)

	for _, tc := range []struct {
		name, body, want string
	}{
		{"missing keeps it", `{"name":"n2"}`, "https://old.example"},
		{"a value changes it", `{"url":"https://new.example"}`, "https://new.example"},
		{"empty clears it", `{"url":""}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/entries/"+entryID, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+login.AccessToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("update: got %d: %s", rec.Code, rec.Body.String())
			}

			// Read it back the way clients do: the entry must stay readable.
			req = httptest.NewRequest(http.MethodGet, "/entries/"+entryID, nil)
			req.Header.Set("Authorization", "Bearer "+login.AccessToken)
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("get: got %d: %s", rec.Code, rec.Body.String())
			}
			var got models.EntryResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.URL != tc.want {
				t.Fatalf("url: got %q, want %q", got.URL, tc.want)
			}

			req = httptest.NewRequest(http.MethodGet, "/entries", nil)
			req.Header.Set("Authorization", "Bearer "+login.AccessToken)
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), entryID) {
				t.Fatalf("list: got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// The oldest version stays restorable even though the snapshot taken first would
// push it over the limit.
func TestRestoreOldestVersionAtTheLimit(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	var owner string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, status, password_hash, pub_x25519, pub_mlkem768,
		                   enc_priv_x25519, enc_priv_mlkem768, kdf_salt)
		VALUES ($1, 'limit test', 'active', 'x', 'x', 'x', 'x', 'x', 'x')
		RETURNING id`, "limit-"+uuid.NewString()+"@example.test").Scan(&owner); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner) })
	var entryID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO entries (owner_id, type, name, encrypted_data, data_nonce)
		VALUES ($1, 'password', 'current', 'b', 'n') RETURNING id`, owner).Scan(&entryID); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM entries WHERE id=$1`, entryID) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO entry_permissions (entry_id, user_id, permission) VALUES ($1, $2, 'owner')`, entryID, owner); err != nil {
		t.Fatalf("insert permission: %v", err)
	}
	var oldest string
	for i := range maxVersionsPerEntry {
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO entry_versions (entry_id, name, encrypted_data, data_nonce, created_at)
			VALUES ($1, 'old', 'b', 'n', NOW() - make_interval(hours => $2)) RETURNING id`,
			entryID, maxVersionsPerEntry-i).Scan(&id); err != nil {
			t.Fatalf("insert version: %v", err)
		}
		if i == 0 {
			oldest = id
		}
	}

	h := New(pool, nil, testSecret, "", nil)
	login, err := h.issueTokens(ctx, owner, "user")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	router := chi.NewRouter()
	router.Use(mw.JWTAuth(testSecret))
	router.Post("/entries/{id}/versions/{vid}/restore", h.RestoreVersion)
	req := httptest.NewRequest(http.MethodPost, "/entries/"+entryID+"/versions/"+oldest+"/restore", nil)
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("restore: got %d, want 204: %s", rec.Code, rec.Body.String())
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM entries WHERE id=$1`, entryID).Scan(&name); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if name != "old" {
		t.Fatalf("entry name: got %q, want the restored one", name)
	}
}
