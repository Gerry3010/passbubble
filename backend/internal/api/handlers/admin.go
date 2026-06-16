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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

// InviteUser handles POST /api/v1/admin/invite
func (h *Handler) InviteUser(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	req, err := decode[models.InviteRequest](r)
	if err != nil || req.Email == "" {
		respondErr(w, http.StatusBadRequest, "email required")
		return
	}

	token, err := randURLSafeToken()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	invID := uuid.New().String()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO invitations (id, email, token, invited_by, expires_at)
		VALUES ($1,$2,$3,$4,$5)`,
		invID, req.Email, token, claims.UserID, expiresAt)
	if err != nil {
		respondErr(w, http.StatusConflict, "invitation already exists for this email")
		return
	}

	respond(w, http.StatusCreated, models.InvitationResponse{
		ID:        invID,
		Email:     req.Email,
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}

// ListUsers handles GET /api/v1/admin/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, email, name, role, status, created_at::text
		FROM users ORDER BY created_at`)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list users")
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

// UpdateUser handles PUT /api/v1/admin/users/{id}
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	targetID := chi.URLParam(r, "id")
	if targetID == claims.UserID {
		respondErr(w, http.StatusBadRequest, "cannot modify own account via admin endpoint")
		return
	}
	req, err := decode[models.UpdateUserRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	_, err = h.pool.Exec(r.Context(), `
		UPDATE users SET
			status = CASE WHEN $2!='' THEN $2 ELSE status END,
			role   = CASE WHEN $3!='' THEN $3 ELSE role END
		WHERE id=$1`,
		targetID, req.Status, req.Role)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListInvitations handles GET /api/v1/admin/invitations
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, email, token, expires_at::text, accepted_at IS NOT NULL
		FROM invitations ORDER BY created_at DESC`)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list invitations")
		return
	}
	defer rows.Close()

	invitations := []models.InvitationResponse{}
	for rows.Next() {
		var inv models.InvitationResponse
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Token, &inv.ExpiresAt, &inv.Used); err != nil {
			continue
		}
		invitations = append(invitations, inv)
	}
	respond(w, http.StatusOK, invitations)
}
