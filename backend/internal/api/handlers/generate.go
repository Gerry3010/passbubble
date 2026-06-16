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
	"math/big"
	"net/http"
	"unicode"

	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

const (
	charsetLower  = "abcdefghijklmnopqrstuvwxyz"
	charsetUpper  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	charsetDigits = "0123456789"
	charsetSymbol = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	charsetAmb    = "0Ol1I"
)

// Generate handles POST /api/v1/generate
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	req, err := decode[models.GenerateRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	length := req.Length
	if length <= 0 {
		length = 20 // default
	} else if length < 8 {
		length = 8 // minimum
	}
	if length > 128 {
		length = 128
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	passwords := make([]models.GeneratedPassword, 0, count)
	for i := 0; i < count; i++ {
		pw, err := generatePassword(length, req.Type, req.NoAmbiguous, req.ExcludeChars)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "generation failed")
			return
		}
		passwords = append(passwords, models.GeneratedPassword{
			Password: pw,
			Strength: passwordStrength(pw),
		})
	}
	respond(w, http.StatusOK, models.GenerateResponse{Passwords: passwords})
}

func generatePassword(length int, genType string, noAmbiguous bool, exclude string) (string, error) {
	var charset string
	switch genType {
	case "alphanum":
		charset = charsetLower + charsetUpper + charsetDigits
	case "numbers":
		charset = charsetDigits
	case "lower":
		charset = charsetLower
	default: // "strong" or empty
		charset = charsetLower + charsetUpper + charsetDigits + charsetSymbol
	}

	// Remove excluded chars
	for _, c := range exclude {
		charset = removeChar(charset, string(c))
	}
	if noAmbiguous {
		for _, c := range charsetAmb {
			charset = removeChar(charset, string(c))
		}
	}
	if len(charset) == 0 {
		charset = charsetLower
	}

	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

func passwordStrength(pw string) int {
	var (
		hasLower, hasUpper, hasDigit, hasSymbol bool
		length = len(pw)
	)
	for _, c := range pw {
		switch {
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsDigit(c):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	score := 0
	if length >= 8 {
		score += 20
	}
	if length >= 16 {
		score += 20
	}
	if length >= 24 {
		score += 10
	}
	if hasLower {
		score += 10
	}
	if hasUpper {
		score += 10
	}
	if hasDigit {
		score += 10
	}
	if hasSymbol {
		score += 20
	}
	if score > 100 {
		score = 100
	}
	return score
}

func removeChar(s, c string) string {
	result := []byte{}
	for i := 0; i < len(s); i++ {
		found := false
		for j := 0; j < len(c); j++ {
			if s[i] == c[j] {
				found = true
				break
			}
		}
		if !found {
			result = append(result, s[i])
		}
	}
	return string(result)
}
