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
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
	pbcrypto "github.com/Gerry3010/passbubble/backend/pkg/crypto"
)

// shareLinkTokenBytes is the random token length (before base64url encoding).
const shareLinkTokenBytes = 32

func generateShareLinkToken() (string, error) {
	b := make([]byte, shareLinkTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateEntryShareLink handles POST /api/v1/entries/{id}/share-link
func (h *Handler) CreateEntryShareLink(w http.ResponseWriter, r *http.Request) {
	h.createShareLink(w, r, "entries", "entry_id")
}

// CreateFolderShareLink handles POST /api/v1/folders/{id}/share-link
func (h *Handler) CreateFolderShareLink(w http.ResponseWriter, r *http.Request) {
	h.createShareLink(w, r, "folders", "folder_id")
}

func (h *Handler) createShareLink(w http.ResponseWriter, r *http.Request, table, column string) {
	claims := mw.ClaimsFromCtx(r.Context())
	resourceID := chi.URLParam(r, "id")

	req, err := decode[models.CreateShareLinkRequest](r)
	if err != nil || req.EncryptedPayload == "" || req.PayloadNonce == "" || req.ExpiresAt == "" {
		respondErr(w, http.StatusBadRequest, "encrypted_payload, payload_nonce, expires_at required")
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid expires_at")
		return
	}

	payload, err := base64.StdEncoding.DecodeString(req.EncryptedPayload)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid encrypted_payload")
		return
	}
	nonce, err := base64.StdEncoding.DecodeString(req.PayloadNonce)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid payload_nonce")
		return
	}

	// Only owner can create a share link.
	var isOwner bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1 AND owner_id=$2)`,
		resourceID, claims.UserID).Scan(&isOwner)
	if !isOwner {
		respondErr(w, http.StatusForbidden, "only owner can create share link")
		return
	}

	var passwordSalt, passwordHash []byte
	if req.Password != "" {
		params, err := pbcrypto.NewKDFParams()
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "failed to create share link")
			return
		}
		passwordSalt = params.Salt
		passwordHash = pbcrypto.DeriveKey(req.Password, params)
	}

	token, err := generateShareLinkToken()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create share link")
		return
	}

	id := uuid.New().String()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO share_links (id, token, owner_id, `+column+`, encrypted_payload, payload_nonce,
			password_salt, password_hash, max_views, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		id, token, claims.UserID, resourceID, payload, nonce,
		nullableBytes(passwordSalt), nullableBytes(passwordHash), req.MaxViews, expiresAt)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create share link")
		return
	}

	respond(w, http.StatusCreated, models.ShareLinkResponse{
		ID:          id,
		Token:       token,
		HasPassword: req.Password != "",
		MaxViews:    req.MaxViews,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	})
}

func nullableBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

// GetShareLink handles GET /api/v1/share/{token} — unauthenticated.
func (h *Handler) GetShareLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	var (
		id                         string
		encryptedPayload, nonce    []byte
		passwordSalt, passwordHash []byte
		maxViews                   *int
		viewCount                  int
		expiresAt                  time.Time
		revokedAt                  *time.Time
	)
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, encrypted_payload, payload_nonce, password_salt, password_hash,
			max_views, view_count, expires_at, revoked_at
		FROM share_links WHERE token=$1`, token,
	).Scan(&id, &encryptedPayload, &nonce, &passwordSalt, &passwordHash,
		&maxViews, &viewCount, &expiresAt, &revokedAt)
	if err != nil {
		respondErr(w, http.StatusNotFound, "share link not found")
		return
	}

	if revokedAt != nil || time.Now().After(expiresAt) || (maxViews != nil && viewCount >= *maxViews) {
		respondErr(w, http.StatusGone, "share link expired or revoked")
		return
	}

	requiresPassword := len(passwordHash) > 0
	if requiresPassword {
		password := r.URL.Query().Get("password")
		if password == "" {
			respond(w, http.StatusOK, models.PublicShareLinkResponse{RequiresPassword: true})
			return
		}
		hash := pbcrypto.DeriveKey(password, &pbcrypto.KDFParams{
			Salt: passwordSalt, Time: pbcrypto.Argon2DefaultTime, Memory: pbcrypto.Argon2DefaultMemory,
		})
		if subtle.ConstantTimeCompare(hash, passwordHash) != 1 {
			respondErr(w, http.StatusForbidden, "invalid password")
			return
		}
	}

	_, _ = h.pool.Exec(r.Context(), `UPDATE share_links SET view_count = view_count + 1 WHERE id=$1`, id)

	respond(w, http.StatusOK, models.PublicShareLinkResponse{
		RequiresPassword: requiresPassword,
		EncryptedPayload: base64.StdEncoding.EncodeToString(encryptedPayload),
		PayloadNonce:     base64.StdEncoding.EncodeToString(nonce),
	})
}

// RevokeShareLink handles DELETE /api/v1/shares/links/{id}
func (h *Handler) RevokeShareLink(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	linkID := chi.URLParam(r, "id")
	_, err := h.pool.Exec(r.Context(), `
		UPDATE share_links SET revoked_at=NOW() WHERE id=$1 AND owner_id=$2`, linkID, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to revoke share link")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
