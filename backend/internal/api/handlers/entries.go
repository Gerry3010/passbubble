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
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// ListEntries handles GET /api/v1/entries — returns metadata only (no encrypted_data).
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT e.id, e.folder_id, e.owner_id, e.type, e.name, e.url,
			e.created_at::text, e.updated_at::text
		FROM entries e
		LEFT JOIN entry_permissions ep ON ep.entry_id=e.id AND ep.user_id=$1
		WHERE e.owner_id=$1 OR ep.user_id=$1
		ORDER BY e.name`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list entries")
		return
	}
	defer rows.Close()

	entries := []models.EntryResponse{}
	for rows.Next() {
		var e models.EntryResponse
		var folderID *string
		if err := rows.Scan(&e.ID, &folderID, &e.OwnerID, &e.Type, &e.Name, &e.URL,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			continue
		}
		e.FolderID = folderID
		entries = append(entries, e)
	}
	respond(w, http.StatusOK, entries)
}

// GetEntry handles GET /api/v1/entries/{id} — returns full entry with encrypted_data and caller's key.
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")

	var e models.EntryResponse
	var folderID *string
	var encryptedData, dataNonce []byte
	err := h.pool.QueryRow(r.Context(), `
		SELECT e.id, e.folder_id, e.owner_id, e.type, e.name, e.url,
			e.encrypted_data, e.data_nonce,
			e.created_at::text, e.updated_at::text
		FROM entries e
		LEFT JOIN entry_permissions ep ON ep.entry_id=e.id AND ep.user_id=$2
		WHERE e.id=$1 AND (e.owner_id=$2 OR ep.user_id=$2)`,
		entryID, claims.UserID,
	).Scan(&e.ID, &folderID, &e.OwnerID, &e.Type, &e.Name, &e.URL,
		&encryptedData, &dataNonce, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		respondErr(w, http.StatusNotFound, "entry not found")
		return
	}
	e.FolderID = folderID
	// Encode in Go, not via Postgres's encode(...,'base64') — that wraps
	// output every 76 chars with a newline (RFC 2045 style), which strict
	// base64 decoders (e.g. Dart's base64.decode) reject outright.
	e.EncryptedData = base64.StdEncoding.EncodeToString(encryptedData)
	e.DataNonce = base64.StdEncoding.EncodeToString(dataNonce)

	var rawEncKey []byte
	_ = h.pool.QueryRow(r.Context(), `
		SELECT encrypted_key FROM entry_keys
		WHERE entry_id=$1 AND user_id=$2`, entryID, claims.UserID,
	).Scan(&rawEncKey)
	if len(rawEncKey) > 0 {
		e.EntryKey = &models.EntryKey{
			UserID:       claims.UserID,
			EncryptedKey: base64.StdEncoding.EncodeToString(rawEncKey),
		}
	}
	respond(w, http.StatusOK, e)
}

// CreateEntry handles POST /api/v1/entries
func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	req, err := decode[models.CreateEntryRequest](r)
	if err != nil || req.Name == "" || req.Type == "" || req.EncryptedData == "" || req.DataNonce == "" {
		respondErr(w, http.StatusBadRequest, "name, type, encrypted_data, data_nonce required")
		return
	}
	if !validEntryType(req.Type) {
		respondErr(w, http.StatusBadRequest, "invalid entry type")
		return
	}
	if len(req.EntryKeys) == 0 {
		respondErr(w, http.StatusBadRequest, "at least one entry_key required")
		return
	}

	entryID := uuid.New().String()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO entries (id, folder_id, owner_id, type, name, url, encrypted_data, data_nonce)
		VALUES ($1,$2,$3,$4,$5,$6,decode($7,'base64'),decode($8,'base64'))`,
		entryID, req.FolderID, claims.UserID, req.Type, req.Name, req.URL,
		req.EncryptedData, req.DataNonce,
	)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create entry")
		return
	}

	for _, ek := range req.EntryKeys {
		_, _ = h.pool.Exec(r.Context(), `
			INSERT INTO entry_keys (id, entry_id, user_id, encrypted_key)
			VALUES ($1,$2,$3,decode($4,'base64'))
			ON CONFLICT (entry_id, user_id) DO UPDATE SET encrypted_key=EXCLUDED.encrypted_key`,
			uuid.New().String(), entryID, ek.UserID, ek.EncryptedKey)
	}
	_, _ = h.pool.Exec(r.Context(), `
		INSERT INTO entry_permissions (entry_id, user_id, permission)
		VALUES ($1,$2,'owner') ON CONFLICT DO NOTHING`, entryID, claims.UserID)

	respond(w, http.StatusCreated, map[string]string{"id": entryID})
}

// UpdateEntry handles PUT /api/v1/entries/{id}
func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	req, err := decode[models.UpdateEntryRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !h.entryPerm(r.Context(), entryID, claims.UserID, "write", "owner") {
		respondErr(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	_, err = h.pool.Exec(r.Context(), `
		UPDATE entries SET
			name = COALESCE(NULLIF($2,''), name),
			url  = COALESCE(NULLIF($3,''), url),
			encrypted_data = CASE WHEN $4!='' THEN decode($4,'base64') ELSE encrypted_data END,
			data_nonce     = CASE WHEN $5!='' THEN decode($5,'base64') ELSE data_nonce END,
			folder_id = $6, updated_at = NOW()
		WHERE id=$1`,
		entryID, req.Name, req.URL, req.EncryptedData, req.DataNonce, req.FolderID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to update entry")
		return
	}
	for _, ek := range req.EntryKeys {
		_, _ = h.pool.Exec(r.Context(), `
			INSERT INTO entry_keys (id, entry_id, user_id, encrypted_key)
			VALUES ($1,$2,$3,decode($4,'base64'))
			ON CONFLICT (entry_id, user_id) DO UPDATE SET encrypted_key=EXCLUDED.encrypted_key`,
			uuid.New().String(), entryID, ek.UserID, ek.EncryptedKey)
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteEntry handles DELETE /api/v1/entries/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "owner") {
		respondErr(w, http.StatusForbidden, "only owner can delete")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM entries WHERE id=$1`, entryID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ShareEntry handles POST /api/v1/entries/{id}/share
func (h *Handler) ShareEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	req, err := decode[models.ShareEntryRequest](r)
	if err != nil || req.UserID == "" || req.EncryptedKey == "" {
		respondErr(w, http.StatusBadRequest, "user_id and encrypted_key required")
		return
	}
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "owner") {
		respondErr(w, http.StatusForbidden, "only owner can share")
		return
	}

	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO entry_keys (id, entry_id, user_id, encrypted_key)
		VALUES ($1,$2,$3,decode($4,'base64'))
		ON CONFLICT (entry_id, user_id) DO UPDATE SET encrypted_key=EXCLUDED.encrypted_key`,
		uuid.New().String(), entryID, req.UserID, req.EncryptedKey)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to share entry")
		return
	}

	perm := req.Permission
	if perm == "" {
		perm = "read"
	}
	_, _ = h.pool.Exec(r.Context(), `
		INSERT INTO entry_permissions (entry_id, user_id, permission, granted_by)
		VALUES ($1,$2,$3,$4) ON CONFLICT (entry_id, user_id) DO UPDATE SET permission=EXCLUDED.permission`,
		entryID, req.UserID, perm, claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

// SearchEntries handles GET /api/v1/entries/search?q=
func (h *Handler) SearchEntries(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		respond(w, http.StatusOK, []models.EntryResponse{})
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT e.id, e.folder_id, e.owner_id, e.type, e.name, e.url,
			e.created_at::text, e.updated_at::text
		FROM entries e
		LEFT JOIN entry_permissions ep ON ep.entry_id=e.id AND ep.user_id=$1
		WHERE (e.owner_id=$1 OR ep.user_id=$1) AND (e.name ILIKE $2 OR e.url ILIKE $2)
		ORDER BY e.name LIMIT 50`,
		claims.UserID, "%"+q+"%")
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	entries := []models.EntryResponse{}
	for rows.Next() {
		var e models.EntryResponse
		var folderID *string
		if err := rows.Scan(&e.ID, &folderID, &e.OwnerID, &e.Type, &e.Name, &e.URL,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			continue
		}
		e.FolderID = folderID
		entries = append(entries, e)
	}
	respond(w, http.StatusOK, entries)
}

// ─── permission helper ────────────────────────────────────────────────────

func (h *Handler) entryPerm(ctx context.Context, entryID, userID string, perms ...string) bool {
	placeholders := make([]string, len(perms))
	args := make([]any, 2+len(perms))
	args[0] = entryID
	args[1] = userID
	for i, p := range perms {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args[i+2] = p
	}
	var exists bool
	_ = h.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM entry_permissions
			WHERE entry_id=$1 AND user_id=$2 AND permission IN (`+
		strings.Join(placeholders, ",")+`))`, args...,
	).Scan(&exists)
	return exists
}


var allowedEntryTypes = map[string]bool{
	"password":     true,
	"totp":         true,
	"note":         true,
	"api-key":      true,
	"ssh-key":      true,
	"certificate":  true,
	"credit-card":  true,
	"bank-account": true,
	"identity":     true,
	"license":      true,
}

func validEntryType(t string) bool {
	return allowedEntryTypes[t]
}
