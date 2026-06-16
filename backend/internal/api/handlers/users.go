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
	"strings"

	"github.com/go-chi/chi/v5"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// GetUserKeys handles GET /api/v1/users/{id}/keys
// Returns a user's public keys for client-side key encapsulation (sharing).
func (h *Handler) GetUserKeys(w http.ResponseWriter, r *http.Request) {
	mw.ClaimsFromCtx(r.Context()) // auth check already done by middleware
	targetID := chi.URLParam(r, "id")

	var keys models.UserPublicKeys
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, pub_x25519, pub_mlkem768 FROM users WHERE id=$1 AND status='active'`, targetID,
	).Scan(&keys.UserID, &keys.PubX25519, &keys.PubMLKEM768)
	if err != nil {
		respondErr(w, http.StatusNotFound, "user not found")
		return
	}
	respond(w, http.StatusOK, keys)
}

// SearchUsers handles GET /api/v1/users/search?q=email
func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 3 {
		respondErr(w, http.StatusBadRequest, "query must be at least 3 characters")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, email, name, role, status, created_at::text
		FROM users WHERE (email ILIKE $1 OR name ILIKE $1) AND status='active'
		ORDER BY name LIMIT 20`, "%"+q+"%")
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	users := []models.UserResponse{}
	for rows.Next() {
		var u models.UserResponse
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	respond(w, http.StatusOK, users)
}
