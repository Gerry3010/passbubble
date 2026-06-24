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

// UpdateFolder handles PUT /api/v1/folders/{id} — used for both renaming and
// moving (re-parenting) a folder.
func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")
	req, err := decode[models.CreateFolderRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Cycle guard: a folder may not become its own descendant's child (or its
	// own parent), which would corrupt the tree built by ListFolders.
	if req.ParentID != nil {
		if *req.ParentID == folderID {
			respondErr(w, http.StatusBadRequest, "cannot move folder into itself")
			return
		}
		var isDescendant bool
		err = h.pool.QueryRow(r.Context(), `
			WITH RECURSIVE descendants AS (
				SELECT id FROM folders WHERE parent_id=$1
				UNION ALL
				SELECT f.id FROM folders f JOIN descendants d ON f.parent_id=d.id
			)
			SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$2)`,
			folderID, *req.ParentID).Scan(&isDescendant)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "failed to update folder")
			return
		}
		if isDescendant {
			respondErr(w, http.StatusBadRequest, "cannot move folder into its own subfolder")
			return
		}
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

// DeleteFolder handles DELETE /api/v1/folders/{id} — recursively deletes the
// folder, all of its subfolders, and every entry contained in any of them.
// Dependent rows (entry_keys, entry_permissions, folder_permissions, share_links)
// are removed automatically via ON DELETE CASCADE on those tables.
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	folderID := chi.URLParam(r, "id")

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	// Collect the folder and all of its descendants, scoped to the owner so a
	// non-owner cannot delete anything.
	rows, err := tx.Query(r.Context(), `
		WITH RECURSIVE descendants AS (
			SELECT id FROM folders WHERE id=$1 AND owner_id=$2
			UNION ALL
			SELECT f.id FROM folders f JOIN descendants d ON f.parent_id=d.id
		)
		SELECT id FROM descendants`, folderID, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			respondErr(w, http.StatusInternalServerError, "failed to delete folder")
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		// Folder doesn't exist or isn't owned by the caller — nothing to do.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Entries first (cascades entry_keys / entry_permissions / share_links),
	// then the folders themselves (cascades folder_permissions / share_links).
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM entries WHERE folder_id = ANY($1)`, ids); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM folders WHERE id = ANY($1)`, ids); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
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
