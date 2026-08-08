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

var testSecret = []byte("test-secret-minimum-32-bytes-long!!")

// Two tokens minted for the same user in the same second must differ. They used to be
// byte-identical (iat/exp are second-resolution), which made the refresh token collide
// with the UNIQUE sessions.token_hash and turned a renewal into a 500 — a client
// refreshing right after login could never get a new session.
func TestSignTokenIsUniquePerCall(t *testing.T) {
	now := time.Now()
	seen := make(map[string]bool)
	for i := range 10 {
		tok, err := signToken(testSecret, "user-1", "user", "refresh", now, refreshTokenTTL)
		if err != nil {
			t.Fatalf("signToken: %v", err)
		}
		if seen[tok] {
			t.Fatalf("token %d is a byte-identical repeat — session hashes would collide", i)
		}
		seen[tok] = true
	}
}

// The unique jti must not disturb what the rest of the stack reads off a token.
func TestSignTokenClaims(t *testing.T) {
	now := time.Now()
	tok, err := signToken(testSecret, "user-1", "admin", "access", now, accessTokenTTL)
	if err != nil {
		t.Fatalf("signToken: %v", err)
	}

	claims := &mw.Claims{}
	if _, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) { return testSecret, nil }); err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if claims.UserID != "user-1" || claims.Subject != "user-1" {
		t.Errorf("user = %q/%q, want user-1", claims.UserID, claims.Subject)
	}
	if claims.Role != "admin" {
		t.Errorf("role = %q, want admin", claims.Role)
	}
	if claims.TokenType != "access" {
		t.Errorf("token type = %q, want access", claims.TokenType)
	}
	if claims.ID == "" {
		t.Error("jti is empty — tokens would repeat within a second")
	}
	if got := claims.ExpiresAt.Sub(claims.IssuedAt.Time); got != accessTokenTTL {
		t.Errorf("lifetime = %v, want %v", got, accessTokenTTL)
	}
}
