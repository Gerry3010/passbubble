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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
	bcryptCost      = 12
)

var errInvalidToken = errors.New("invalid invitation token")

// Register handles POST /api/v1/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.RegisterRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		respondErr(w, http.StatusBadRequest, "email, name, and password are required")
		return
	}
	if req.PubX25519 == "" || req.PubMLKEM768 == "" || req.EncPrivX25519 == "" || req.EncPrivMLKEM768 == "" {
		respondErr(w, http.StatusBadRequest, "public and encrypted private keys are required")
		return
	}

	inv, err := h.getValidInvitation(r.Context(), req.InvitationToken, req.Email)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, "invalid or expired invitation token")
		return
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	role := "user"
	if inv.InvitedByID == "" {
		role = "admin" // First user bootstrap
	}

	userID := uuid.New().String()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO users (id, email, name, role, status, password_hash, invited_by,
			pub_x25519, pub_mlkem768, enc_priv_x25519, enc_priv_mlkem768,
			kdf_salt, kdf_time, kdf_memory)
		VALUES ($1,$2,$3,$4,'active',$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		userID, req.Email, req.Name, role, string(pwHash), nullableStr(inv.InvitedByID),
		req.PubX25519, req.PubMLKEM768, req.EncPrivX25519, req.EncPrivMLKEM768,
		req.KDFSalt, 3, 65536,
	)
	if err != nil {
		respondErr(w, http.StatusConflict, "email already registered")
		return
	}

	if inv.ID != "" {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE invitations SET accepted_at = NOW() WHERE id = $1`, inv.ID)
	}

	tokens, err := h.issueTokens(r.Context(), userID, role)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}
	// Clients need user_id to address entry_keys to themselves when creating
	// entries — without it, every entry they create silently ends up with no
	// entry_keys row for the owner (the server-side insert fails on an empty
	// UUID, and that error path discards the failure).
	respond(w, http.StatusCreated, map[string]any{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
		"token_type":    "Bearer",
		"user_id":       userID,
		"email":         req.Email,
		"name":          req.Name,
		"role":          role,
	})
}

// Login handles POST /api/v1/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.LoginRequest](r)
	if err != nil || req.Email == "" || req.Password == "" {
		respondErr(w, http.StatusBadRequest, "email and password required")
		return
	}

	var (
		userID          string
		name            string
		role            string
		passwordHash    string
		pubX25519       string
		pubMLKEM768     string
		encPrivX25519   string
		encPrivMLKEM768 string
		kdfSalt         string
		kdfTime         uint32
		kdfMemory       uint32
	)
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, name, role, password_hash, pub_x25519, pub_mlkem768,
			enc_priv_x25519, enc_priv_mlkem768, kdf_salt, kdf_time, kdf_memory
		FROM users WHERE email = $1 AND status = 'active'`, req.Email,
	).Scan(&userID, &name, &role, &passwordHash,
		&pubX25519, &pubMLKEM768,
		&encPrivX25519, &encPrivMLKEM768,
		&kdfSalt, &kdfTime, &kdfMemory)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		respondErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	tokens, err := h.issueTokens(r.Context(), userID, role)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}

	// Return tokens + encrypted private keys so client can decrypt them locally.
	// user_id/email/name/role are required so the client can address
	// entry_keys to itself when creating entries.
	respond(w, http.StatusOK, map[string]any{
		"access_token":      tokens.AccessToken,
		"refresh_token":     tokens.RefreshToken,
		"expires_in":        tokens.ExpiresIn,
		"token_type":        "Bearer",
		"user_id":           userID,
		"email":             req.Email,
		"name":              name,
		"role":              role,
		"enc_priv_x25519":   encPrivX25519,
		"enc_priv_mlkem768": encPrivMLKEM768,
		"pub_x25519":        pubX25519,
		"pub_mlkem768":      pubMLKEM768,
		"kdf_salt":          kdfSalt,
		"kdf_time":          kdfTime,
		"kdf_memory":        kdfMemory,
	})
}

// Refresh handles POST /api/v1/auth/refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.RefreshRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims := &mw.Claims{}
	_, err = jwt.ParseWithClaims(req.RefreshToken, claims, func(t *jwt.Token) (any, error) {
		return h.jwtSecret, nil
	})
	if err != nil || claims.TokenType != "refresh" {
		respondErr(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	tokenHash := tokenSHA256(req.RefreshToken)
	var exists bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>NOW())`,
		tokenHash,
	).Scan(&exists)
	if !exists {
		respondErr(w, http.StatusUnauthorized, "session expired or revoked")
		return
	}

	tokens, err := h.issueTokens(r.Context(), claims.UserID, claims.Role)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	respond(w, http.StatusOK, tokens)
}

// Logout handles POST /api/v1/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	req, _ := decode[models.RefreshRequest](r)
	if req.RefreshToken != "" {
		_, _ = h.pool.Exec(r.Context(),
			`DELETE FROM sessions WHERE token_hash=$1`, tokenSHA256(req.RefreshToken))
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/v1/auth/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	var user models.UserResponse
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, email, name, role, status, created_at::text
		FROM users WHERE id = $1`, claims.UserID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.Status, &user.CreatedAt)
	if err != nil {
		respondErr(w, http.StatusNotFound, "user not found")
		return
	}
	respond(w, http.StatusOK, user)
}

// ─── helpers ──────────────────────────────────────────────────────────────

func (h *Handler) issueTokens(ctx context.Context, userID, role string) (*models.LoginResponse, error) {
	now := time.Now()
	sign := func(ttype string, ttl time.Duration) (string, error) {
		return jwt.NewWithClaims(jwt.SigningMethodHS256, &mw.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			},
			UserID:    userID,
			Role:      role,
			TokenType: ttype,
		}).SignedString(h.jwtSecret)
	}

	access, err := sign("access", accessTokenTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := sign("refresh", refreshTokenTTL)
	if err != nil {
		return nil, err
	}

	_, err = h.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), userID, tokenSHA256(refresh), now.Add(refreshTokenTTL),
	)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(accessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

type invitationRow struct {
	ID          string
	InvitedByID string
}

func (h *Handler) getValidInvitation(ctx context.Context, token, email string) (*invitationRow, error) {
	if token == "" {
		// First-ever user: allow bootstrapping without invitation
		var count int
		_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
		if count == 0 {
			return &invitationRow{}, nil
		}
		return nil, errInvalidToken
	}

	var inv invitationRow
	err := h.pool.QueryRow(ctx, `
		SELECT id, COALESCE(invited_by::text, '') FROM invitations
		WHERE token=$1 AND email=$2 AND accepted_at IS NULL AND expires_at>NOW()`,
		token, email,
	).Scan(&inv.ID, &inv.InvitedByID)
	if err != nil {
		return nil, errInvalidToken
	}
	return &inv, nil
}

func tokenSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(h[:])
}

func randURLSafeToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
