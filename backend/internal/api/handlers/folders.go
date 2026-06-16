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
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// ListFolders handles GET /api/v1/folders — returns tree structure.
func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT f.id, f.name, f.parent_id, f.created_at::text
		FROM folders f
		LEFT JOIN folder_permissions fp ON fp.folder_id=f.id AND fp.user_id=$1
		WHERE f.owner_id=$1 OR fp.user_id=$1
		ORDER BY f.name`, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list folders")
		return
	}
	defer rows.Close()

	all := []*models.FolderResponse{}
	byID := map[string]*models.FolderResponse{}
	for rows.Next() {
		f := &models.FolderResponse{}
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt); err != nil {
			continue
		}
		all = append(all, f)
		byID[f.ID] = f
	}

	// Build tree
	roots := []*models.FolderResponse{}
	for _, f := range all {
		if f.ParentID == nil {
			roots = append(roots, f)
		} else if parent, ok := byID[*f.ParentID]; ok {
			parent.Children = append(parent.Children, f)
		}
	}
	respond(w, http.StatusOK, roots)
}

// CreateFolder handles POST /api/v1/folders
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	req, err := decode[models.CreateFolderRequest](r)
	if err != nil || req.Name == "" {
		respondErr(w, http.StatusBadRequest, "name required")
		return
	}

	id := uuid.New().String()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO folders (id, name, parent_id, owner_id) VALUES ($1,$2,$3,$4)`,
		id, req.Name, req.ParentID, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create folder")
		return
	}
	respond(w, http.StatusCreated, map[string]string{"id": id})
}

// UpdateFolder handles PUT /api/v1/folders/{id}
func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")
	req, err := decode[models.CreateFolderRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	_, err = h.pool.Exec(r.Context(), `
		UPDATE folders SET name=$2, parent_id=$3, updated_at=NOW()
		WHERE id=$1 AND owner_id=$4`,
		folderID, req.Name, req.ParentID, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to update folder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteFolder handles DELETE /api/v1/folders/{id}
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")
	_, err := h.pool.Exec(r.Context(), `
		DELETE FROM folders WHERE id=$1 AND owner_id=$2`, folderID, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ShareFolder handles POST /api/v1/folders/{id}/share
func (h *Handler) ShareFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")
	req, err := decode[models.ShareFolderRequest](r)
	if err != nil || req.UserID == "" {
		respondErr(w, http.StatusBadRequest, "user_id required")
		return
	}

	// Only owner can share
	var isOwner bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND owner_id=$2)`,
		folderID, claims.UserID).Scan(&isOwner)
	if !isOwner {
		respondErr(w, http.StatusForbidden, "only owner can share folder")
		return
	}

	perm := req.Permission
	if perm == "" {
		perm = "read"
	}
	_, _ = h.pool.Exec(r.Context(), `
		INSERT INTO folder_permissions (folder_id, user_id, permission, granted_by)
		VALUES ($1,$2,$3,$4) ON CONFLICT (folder_id, user_id) DO UPDATE SET permission=EXCLUDED.permission`,
		folderID, req.UserID, perm, claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}
