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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
	pbcrypto "github.com/Gerry3010/passbubble/backend/pkg/crypto"
)

const (
	// pendingTokenTTL is how long the short-lived "2fa_pending" token issued
	// between password verification and TOTP entry stays valid.
	pendingTokenTTL = 5 * time.Minute
	// totpRecoveryTTL is the lifetime of an email recovery link.
	totpRecoveryTTL = 30 * time.Minute
	// pendingTokenType marks the JWT token type for the intermediate 2FA step.
	pendingTokenType = "2fa_pending"
	totpIssuer       = "Passbubble"
)

const totpResetHTML = `<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:480px;margin:80px auto;text-align:center;color:#222">
  <h2 style="color:#16a34a">&#10003; Two-factor authentication disabled</h2>
  <p>You can now log in with just your password, then set up 2FA again from the app.</p>
</body>
</html>`

const totpResetErrHTML = `<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:480px;margin:80px auto;text-align:center;color:#222">
  <h2 style="color:#dc2626">&#10007; Reset link invalid</h2>
  <p>This link is invalid or has expired. Request a new recovery email and try again.</p>
</body>
</html>`

// issuePendingToken signs a short-lived JWT used to complete the 2FA login step.
func (h *Handler) issuePendingToken(userID, role string) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &mw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(pendingTokenTTL)),
		},
		UserID:    userID,
		Role:      role,
		TokenType: pendingTokenType,
	}).SignedString(h.jwtSecret)
}

// parsePendingToken validates a pending token and returns its claims.
func (h *Handler) parsePendingToken(tokenStr string) (*mw.Claims, error) {
	claims := &mw.Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return h.jwtSecret, nil
	})
	if err != nil || claims.TokenType != pendingTokenType {
		return nil, fmt.Errorf("invalid pending token")
	}
	return claims, nil
}

// totpSecretKey derives the symmetric key used to encrypt TOTP secrets at rest.
// Defense in depth: a database-only leak does not expose usable TOTP seeds.
func (h *Handler) totpSecretKey() []byte {
	sum := sha256.Sum256(append([]byte("passbubble-totp-secret-v1:"), h.jwtSecret...))
	return sum[:]
}

func (h *Handler) encryptTOTPSecret(secret string) (string, error) {
	ct, err := pbcrypto.Encrypt(h.totpSecretKey(), []byte(secret))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (h *Handler) decryptTOTPSecret(stored string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return "", err
	}
	pt, err := pbcrypto.Decrypt(h.totpSecretKey(), ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// SetupTOTP handles POST /api/v1/auth/totp/setup — returns a new (unconfirmed)
// secret + otpauth URL. Nothing is persisted until ConfirmTOTP succeeds.
func (h *Handler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())

	var email string
	if err := h.pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id=$1`, claims.UserID).Scan(&email); err != nil {
		respondErr(w, http.StatusNotFound, "user not found")
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: email})
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}

	respond(w, http.StatusOK, models.SetupTOTPResponse{
		Secret:     key.Secret(),
		OTPAuthURL: key.URL(),
	})
}

// ConfirmTOTP handles POST /api/v1/auth/totp/confirm — verifies a code against
// the provided secret and, on success, enables 2FA for the account.
func (h *Handler) ConfirmTOTP(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	req, err := decode[models.ConfirmTOTPRequest](r)
	if err != nil || req.Secret == "" || req.Code == "" {
		respondErr(w, http.StatusBadRequest, "secret and code required")
		return
	}

	if !totp.Validate(req.Code, req.Secret) {
		respondErr(w, http.StatusBadRequest, "invalid code")
		return
	}

	enc, err := h.encryptTOTPSecret(req.Secret)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to store secret")
		return
	}

	_, err = h.pool.Exec(r.Context(), `
		UPDATE users SET totp_secret=$1, totp_enabled=true, updated_at=NOW() WHERE id=$2`,
		enc, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to enable 2fa")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DisableTOTP handles POST /api/v1/auth/totp/disable — requires a valid current
// code or the account password.
func (h *Handler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	req, err := decode[models.DisableTOTPRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var encSecret, passwordHash string
	var enabled bool
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(totp_secret,''), totp_enabled, password_hash FROM users WHERE id=$1`,
		claims.UserID).Scan(&encSecret, &enabled, &passwordHash); err != nil {
		respondErr(w, http.StatusNotFound, "user not found")
		return
	}
	if !enabled {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	authorized := false
	if req.Code != "" && encSecret != "" {
		if secret, err := h.decryptTOTPSecret(encSecret); err == nil && totp.Validate(req.Code, secret) {
			authorized = true
		}
	}
	if !authorized && req.Password != "" {
		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) == nil {
			authorized = true
		}
	}
	if !authorized {
		respondErr(w, http.StatusForbidden, "valid code or password required")
		return
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE users SET totp_secret=NULL, totp_enabled=false, updated_at=NOW() WHERE id=$1`,
		claims.UserID); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to disable 2fa")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VerifyTOTP handles POST /api/v1/auth/verify-totp — the second login step.
func (h *Handler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.VerifyTOTPRequest](r)
	if err != nil || req.PendingToken == "" || req.Code == "" {
		respondErr(w, http.StatusBadRequest, "pending_token and code required")
		return
	}

	claims, err := h.parsePendingToken(req.PendingToken)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, "invalid or expired pending token")
		return
	}

	var encSecret string
	var enabled bool
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(totp_secret,''), totp_enabled FROM users WHERE id=$1`,
		claims.UserID).Scan(&encSecret, &enabled); err != nil || !enabled || encSecret == "" {
		respondErr(w, http.StatusUnauthorized, "2fa not configured")
		return
	}

	secret, err := h.decryptTOTPSecret(encSecret)
	if err != nil || !totp.Validate(req.Code, secret) {
		respondErr(w, http.StatusUnauthorized, "invalid code")
		return
	}

	h.respondWithSession(w, r.Context(), claims.UserID)
}

// RequestTOTPRecovery handles POST /api/v1/auth/totp/recover — emails a link
// that disables 2FA. Only callable with a valid pending token (i.e. after the
// password has already been verified).
func (h *Handler) RequestTOTPRecovery(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.RequestTOTPRecoveryRequest](r)
	if err != nil || req.PendingToken == "" {
		respondErr(w, http.StatusBadRequest, "pending_token required")
		return
	}

	claims, err := h.parsePendingToken(req.PendingToken)
	if err != nil {
		respondErr(w, http.StatusUnauthorized, "invalid or expired pending token")
		return
	}

	var email string
	if err := h.pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id=$1`, claims.UserID).Scan(&email); err != nil {
		respondErr(w, http.StatusNotFound, "user not found")
		return
	}

	token, err := randURLSafeToken()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create recovery token")
		return
	}
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO totp_recovery_tokens (id, user_id, token, expires_at)
		VALUES ($1,$2,$3,$4)`,
		uuid.New().String(), claims.UserID, token, time.Now().Add(totpRecoveryTTL))
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create recovery token")
		return
	}

	if h.mailer != nil {
		if err := h.mailer.SendTOTPRecoveryEmail(email, token); err != nil {
			// Don't reveal mail failures to the caller; log server-side.
			slog.Error("failed to send TOTP recovery email", "error", err)
		}
	}

	// Always 204 — do not leak whether the address exists/was mailed.
	w.WriteHeader(http.StatusNoContent)
}

// ResetTOTP handles GET /api/v1/auth/reset-totp?token=... — the link target
// from the recovery email. Disables 2FA and returns an HTML confirmation page.
func (h *Handler) ResetTOTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	writeHTML := func(code int, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(code)
		_, _ = fmt.Fprint(w, body)
	}

	if token == "" {
		writeHTML(http.StatusBadRequest, totpResetErrHTML)
		return
	}

	var userID string
	if err := h.pool.QueryRow(r.Context(), `
		SELECT user_id FROM totp_recovery_tokens WHERE token=$1 AND expires_at > NOW()`,
		token).Scan(&userID); err != nil {
		writeHTML(http.StatusBadRequest, totpResetErrHTML)
		return
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE users SET totp_secret=NULL, totp_enabled=false, updated_at=NOW() WHERE id=$1`,
		userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM totp_recovery_tokens WHERE user_id=$1`, userID)

	writeHTML(http.StatusOK, totpResetHTML)
}
