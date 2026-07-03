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

// Trash (soft delete), favorites, and entry version history. All history
// operations only ever copy ciphertext the server already stores — E2E is
// preserved; version restore in particular needs no client-side crypto.

package handlers

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// Versions kept per entry; older ones are pruned inside the update tx.
const maxVersionsPerEntry = 20

// snapshotVersionTx copies an entry's current blob into entry_versions and its
// entry_keys into entry_version_keys, then prunes to maxVersionsPerEntry.
// Must run inside the tx that overwrites the entry, so snapshot and overwrite
// are atomic.
func (h *Handler) snapshotVersionTx(ctx context.Context, tx pgx.Tx, entryID, editorID string) error {
	var versionID string
	err := tx.QueryRow(ctx, `
		INSERT INTO entry_versions (entry_id, name, url, encrypted_data, data_nonce, edited_by)
		SELECT id, name, url, encrypted_data, data_nonce, $2 FROM entries WHERE id=$1
		RETURNING id`, entryID, editorID).Scan(&versionID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entry_version_keys (version_id, user_id, encrypted_key)
		SELECT $1, user_id, encrypted_key FROM entry_keys WHERE entry_id=$2`,
		versionID, entryID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM entry_versions WHERE id IN (
			SELECT id FROM entry_versions WHERE entry_id=$1
			ORDER BY created_at DESC OFFSET $2)`, entryID, maxVersionsPerEntry)
	return err
}

// ListTrash handles GET /api/v1/entries/trash — soft-deleted entries (metadata).
func (h *Handler) ListTrash(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT e.id, e.folder_id, e.owner_id, e.type, e.name, e.url, e.favorite,
			e.created_at::text, e.updated_at::text, e.deleted_at::text
		FROM entries e
		WHERE e.owner_id=$1 AND e.deleted_at IS NOT NULL
		ORDER BY e.deleted_at DESC`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list trash")
		return
	}
	defer rows.Close()

	entries := []models.EntryResponse{}
	for rows.Next() {
		var e models.EntryResponse
		var folderID, deletedAt *string
		if err := rows.Scan(&e.ID, &folderID, &e.OwnerID, &e.Type, &e.Name, &e.URL,
			&e.Favorite, &e.CreatedAt, &e.UpdatedAt, &deletedAt); err != nil {
			continue
		}
		e.FolderID = folderID
		e.DeletedAt = deletedAt
		entries = append(entries, e)
	}
	respond(w, http.StatusOK, entries)
}

// RestoreEntry handles POST /api/v1/entries/{id}/restore.
func (h *Handler) RestoreEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "owner") {
		respondErr(w, http.StatusForbidden, "only owner can restore")
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE entries SET deleted_at=NULL WHERE id=$1 AND deleted_at IS NOT NULL`, entryID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore entry")
		return
	}
	if tag.RowsAffected() == 0 {
		respondErr(w, http.StatusNotFound, "entry not in trash")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PurgeEntry handles DELETE /api/v1/entries/{id}/permanent — irreversible.
func (h *Handler) PurgeEntry(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "owner") {
		respondErr(w, http.StatusForbidden, "only owner can delete permanently")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM entries WHERE id=$1`, entryID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetFavorite handles PUT /api/v1/entries/{id}/favorite.
func (h *Handler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	req, err := decode[models.SetFavoriteRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "write", "owner") {
		respondErr(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE entries SET favorite=$2 WHERE id=$1 AND deleted_at IS NULL`,
		entryID, req.Favorite); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to set favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireEntryAccess reports whether the user may read the entry (owner or any
// permission row) — used by the version endpoints, which must also work for
// entries currently in the trash.
func (h *Handler) requireEntryAccess(ctx context.Context, entryID, userID string) bool {
	var ok bool
	_ = h.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM entries e
			LEFT JOIN entry_permissions ep ON ep.entry_id=e.id AND ep.user_id=$2
			WHERE e.id=$1 AND (e.owner_id=$2 OR ep.user_id=$2))`,
		entryID, userID).Scan(&ok)
	return ok
}

// ListVersions handles GET /api/v1/entries/{id}/versions — metadata only.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	if !h.requireEntryAccess(r.Context(), entryID, claims.UserID) {
		respondErr(w, http.StatusNotFound, "entry not found")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT v.id, v.entry_id, v.name, v.url, v.edited_by, v.created_at::text
		FROM entry_versions v
		WHERE v.entry_id=$1
		ORDER BY v.created_at DESC`, entryID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	defer rows.Close()

	versions := []models.EntryVersionResponse{}
	for rows.Next() {
		var v models.EntryVersionResponse
		var url, editedBy *string
		if err := rows.Scan(&v.ID, &v.EntryID, &v.Name, &url, &editedBy, &v.CreatedAt); err != nil {
			continue
		}
		if url != nil {
			v.URL = *url
		}
		v.EditedBy = editedBy
		versions = append(versions, v)
	}
	respond(w, http.StatusOK, versions)
}

// GetVersion handles GET /api/v1/entries/{id}/versions/{vid} — blob + the
// caller's contemporaneous wrapped key (EntryResponse-shaped for reuse of the
// client decrypt path).
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	versionID := chi.URLParam(r, "vid")
	if !h.requireEntryAccess(r.Context(), entryID, claims.UserID) {
		respondErr(w, http.StatusNotFound, "entry not found")
		return
	}

	var v models.EntryVersionResponse
	var url *string
	var encryptedData, dataNonce []byte
	err := h.pool.QueryRow(r.Context(), `
		SELECT v.id, v.entry_id, v.name, v.url, v.edited_by, v.encrypted_data, v.data_nonce, v.created_at::text
		FROM entry_versions v
		WHERE v.id=$1 AND v.entry_id=$2`, versionID, entryID,
	).Scan(&v.ID, &v.EntryID, &v.Name, &url, &v.EditedBy, &encryptedData, &dataNonce, &v.CreatedAt)
	if err != nil {
		respondErr(w, http.StatusNotFound, "version not found")
		return
	}
	if url != nil {
		v.URL = *url
	}
	v.EncryptedData = base64.StdEncoding.EncodeToString(encryptedData)
	v.DataNonce = base64.StdEncoding.EncodeToString(dataNonce)

	var rawEncKey []byte
	_ = h.pool.QueryRow(r.Context(), `
		SELECT encrypted_key FROM entry_version_keys
		WHERE version_id=$1 AND user_id=$2`, versionID, claims.UserID,
	).Scan(&rawEncKey)
	if len(rawEncKey) > 0 {
		v.EntryKey = &models.EntryKey{
			UserID:       claims.UserID,
			EncryptedKey: base64.StdEncoding.EncodeToString(rawEncKey),
		}
	}
	respond(w, http.StatusOK, v)
}

// RestoreVersion handles POST /api/v1/entries/{id}/versions/{vid}/restore.
// Entirely server-side: the current state is snapshotted as a new version,
// then the chosen version's blob and its wrapped keys are copied back.
func (h *Handler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	versionID := chi.URLParam(r, "vid")
	if !h.entryPerm(r.Context(), entryID, claims.UserID, "write", "owner") {
		respondErr(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore version")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := h.snapshotVersionTx(r.Context(), tx, entryID, claims.UserID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore version")
		return
	}

	tag, err := tx.Exec(r.Context(), `
		UPDATE entries e SET
			name = v.name,
			url = COALESCE(v.url, e.url),
			encrypted_data = v.encrypted_data,
			data_nonce = v.data_nonce,
			updated_at = NOW()
		FROM entry_versions v
		WHERE e.id=$1 AND v.id=$2 AND v.entry_id=e.id`, entryID, versionID)
	if err != nil || tag.RowsAffected() == 0 {
		respondErr(w, http.StatusNotFound, "version not found")
		return
	}

	// The restored blob is only readable with its contemporaneous keys.
	if _, err := tx.Exec(r.Context(), `DELETE FROM entry_keys WHERE entry_id=$1`, entryID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore version")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO entry_keys (id, entry_id, user_id, encrypted_key)
		SELECT gen_random_uuid(), $1, user_id, encrypted_key
		FROM entry_version_keys WHERE version_id=$2`, entryID, versionID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore version")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to restore version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
