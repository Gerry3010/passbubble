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

	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// ListBackups handles GET /api/v1/backup
func (h *Handler) ListBackups(w http.ResponseWriter, r *http.Request) {
	// Backups are server-side metadata only — actual encrypted data is managed by admin
	rows, err := h.pool.Query(r.Context(), `
		SELECT filename, size, created_at::text FROM backups ORDER BY created_at DESC`)
	if err != nil {
		// Table may not exist yet — return empty
		respond(w, http.StatusOK, []models.BackupInfo{})
		return
	}
	defer rows.Close()

	backups := []models.BackupInfo{}
	for rows.Next() {
		var b models.BackupInfo
		if err := rows.Scan(&b.Filename, &b.Size, &b.CreatedAt); err != nil {
			continue
		}
		backups = append(backups, b)
	}
	respond(w, http.StatusOK, backups)
}

// CreateBackup handles POST /api/v1/backup
// The actual backup (encrypted export) is created client-side; this records metadata.
func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	respondErr(w, http.StatusNotImplemented, "backup creation via API not yet implemented — use CLI")
}

// RestoreBackup handles POST /api/v1/backup/restore
func (h *Handler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	respondErr(w, http.StatusNotImplemented, "restore via API not yet implemented — use CLI")
}

// VerifyBackup handles GET /api/v1/backup/{name}/verify
func (h *Handler) VerifyBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	_ = name
	respondErr(w, http.StatusNotImplemented, "backup verify not yet implemented")
}
