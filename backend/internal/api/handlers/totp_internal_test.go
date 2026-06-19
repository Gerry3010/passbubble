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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
)

func newTOTPTestHandler() *Handler {
	return New(nil, nil, []byte("test-secret-minimum-32-bytes-long!!"), "", nil)
}

func signTokenForTest(secret []byte, userID, tokenType string) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &mw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
		UserID:    userID,
		TokenType: tokenType,
	}).SignedString(secret)
}

func TestPendingTokenRoundTrip(t *testing.T) {
	h := newTOTPTestHandler()

	tok, err := h.issuePendingToken("user-123", "user")
	if err != nil {
		t.Fatalf("issuePendingToken: %v", err)
	}

	claims, err := h.parsePendingToken(tok)
	if err != nil {
		t.Fatalf("parsePendingToken: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Fatalf("expected user-123, got %q", claims.UserID)
	}
	if claims.TokenType != pendingTokenType {
		t.Fatalf("expected token type %q, got %q", pendingTokenType, claims.TokenType)
	}
}

func TestParsePendingTokenRejectsWrongType(t *testing.T) {
	h := newTOTPTestHandler()

	// A token signed with the same secret but a non-pending type must be rejected.
	wrong, err := signTokenForTest(h.jwtSecret, "user-123", "access")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := h.parsePendingToken(wrong); err == nil {
		t.Fatal("expected access-typed token to be rejected as pending token")
	}
}

func TestParsePendingTokenRejectsGarbage(t *testing.T) {
	h := newTOTPTestHandler()
	if _, err := h.parsePendingToken("not.a.jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestTOTPSecretEncryptRoundTrip(t *testing.T) {
	h := newTOTPTestHandler()

	const secret = "JBSWY3DPEHPK3PXP"
	enc, err := h.encryptTOTPSecret(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == secret {
		t.Fatal("secret stored in plaintext")
	}
	dec, err := h.decryptTOTPSecret(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != secret {
		t.Fatalf("roundtrip mismatch: got %q", dec)
	}
}
