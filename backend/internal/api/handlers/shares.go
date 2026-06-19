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
	"net/http"

	"github.com/go-chi/chi/v5"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// ListMyShares handles GET /api/v1/shares
// Returns all share links, entry shares, and folder shares owned/granted by the caller.
func (h *Handler) ListMyShares(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())

	// Share links (with the entry/folder name for display)
	linkRows, err := h.pool.Query(r.Context(), `
		SELECT sl.id, sl.token, sl.entry_id, sl.folder_id,
			COALESCE(e.name, f.name, '') AS resource_name,
			sl.password_hash, sl.max_views, sl.view_count,
			sl.expires_at::text, sl.created_at::text, sl.revoked_at::text
		FROM share_links sl
		LEFT JOIN entries e ON e.id = sl.entry_id
		LEFT JOIN folders f ON f.id = sl.folder_id
		WHERE sl.owner_id=$1 ORDER BY sl.created_at DESC`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list share links")
		return
	}
	defer linkRows.Close()

	shareLinks := []models.ShareLinkResponse{}
	for linkRows.Next() {
		var l models.ShareLinkResponse
		var passwordHash []byte
		var revokedAt *string
		if err := linkRows.Scan(&l.ID, &l.Token, &l.EntryID, &l.FolderID, &l.ResourceName,
			&passwordHash, &l.MaxViews, &l.ViewCount, &l.ExpiresAt, &l.CreatedAt, &revokedAt); err != nil {
			continue
		}
		l.HasPassword = len(passwordHash) > 0
		l.RevokedAt = revokedAt
		shareLinks = append(shareLinks, l)
	}

	// Direct entry shares
	entryRows, err := h.pool.Query(r.Context(), `
		SELECT ep.entry_id, e.name, ep.user_id, u.email, ep.permission, ep.created_at::text
		FROM entry_permissions ep
		JOIN entries e ON e.id = ep.entry_id
		JOIN users u ON u.id = ep.user_id
		WHERE ep.granted_by=$1
		ORDER BY ep.created_at DESC`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list entry shares")
		return
	}
	defer entryRows.Close()

	entryShares := []models.DirectShareResponse{}
	for entryRows.Next() {
		var s models.DirectShareResponse
		if err := entryRows.Scan(&s.ResourceID, &s.ResourceName, &s.UserID, &s.UserEmail,
			&s.Permission, &s.CreatedAt); err != nil {
			continue
		}
		entryShares = append(entryShares, s)
	}

	// Direct folder shares
	folderRows, err := h.pool.Query(r.Context(), `
		SELECT fp.folder_id, f.name, fp.user_id, u.email, fp.permission, fp.created_at::text
		FROM folder_permissions fp
		JOIN folders f ON f.id = fp.folder_id
		JOIN users u ON u.id = fp.user_id
		WHERE fp.granted_by=$1
		ORDER BY fp.created_at DESC`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list folder shares")
		return
	}
	defer folderRows.Close()

	folderShares := []models.DirectShareResponse{}
	for folderRows.Next() {
		var s models.DirectShareResponse
		if err := folderRows.Scan(&s.ResourceID, &s.ResourceName, &s.UserID, &s.UserEmail,
			&s.Permission, &s.CreatedAt); err != nil {
			continue
		}
		folderShares = append(folderShares, s)
	}

	respond(w, http.StatusOK, models.MySharesResponse{
		ShareLinks:   shareLinks,
		EntryShares:  entryShares,
		FolderShares: folderShares,
	})
}

// RevokeEntryShare handles DELETE /api/v1/entries/{id}/share/{userId}
func (h *Handler) RevokeEntryShare(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	entryID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	// Allow if caller is the entry owner or the one who granted the share.
	var ownerID string
	_ = h.pool.QueryRow(r.Context(), `SELECT owner_id FROM entries WHERE id=$1`, entryID).Scan(&ownerID)

	tag, err := h.pool.Exec(r.Context(), `
		DELETE FROM entry_permissions
		WHERE entry_id=$1 AND user_id=$2 AND (granted_by=$3 OR $3=$4)`,
		entryID, targetUserID, claims.UserID, ownerID)
	if err != nil || tag.RowsAffected() == 0 {
		respondErr(w, http.StatusNotFound, "share not found or not authorized")
		return
	}
	// Drop the recipient's wrapped key as well so access is fully revoked.
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM entry_keys WHERE entry_id=$1 AND user_id=$2`, entryID, targetUserID)
	w.WriteHeader(http.StatusNoContent)
}

// RevokeFolderShare handles DELETE /api/v1/folders/{id}/share/{userId}
func (h *Handler) RevokeFolderShare(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	var ownerID string
	_ = h.pool.QueryRow(r.Context(), `SELECT owner_id FROM folders WHERE id=$1`, folderID).Scan(&ownerID)

	tag, err := h.pool.Exec(r.Context(), `
		DELETE FROM folder_permissions
		WHERE folder_id=$1 AND user_id=$2 AND (granted_by=$3 OR $3=$4)`,
		folderID, targetUserID, claims.UserID, ownerID)
	if err != nil || tag.RowsAffected() == 0 {
		respondErr(w, http.StatusNotFound, "share not found or not authorized")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
