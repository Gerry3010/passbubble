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

package vault

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

// shareNeverExpires is the far-future expiry used for "never expires" links,
// matching the Flutter client so the shares list shows them as unlimited.
var shareNeverExpires = time.Date(2125, 1, 1, 0, 0, 0, 0, time.UTC)

// CreateEntryShareLink builds a zero-knowledge share link for an entry and
// returns the full shareable URL.
//
// The entry is decrypted locally, re-encrypted under a fresh random link key,
// and only the ciphertext is sent to the server. The link key is placed in the
// URL fragment-style query (`?k=`) and never reaches the server. A validity of
// <= 0 means the link never expires.
func (v *Vault) CreateEntryShareLink(entryID string, validity time.Duration) (string, error) {
	entry, err := v.GetEntry(entryID) // also enforces IsUnlocked + decrypts
	if err != nil {
		return "", err
	}
	return v.buildShareLink(validity, map[string]any{
		"name": entry.Name,
		"type": entry.Type,
		"url":  entry.URL,
		"data": entry.Data,
	}, func(req apiclient.CreateShareLinkRequest) (*apiclient.ShareLinkResponse, error) {
		return v.client.CreateEntryShareLink(entryID, req)
	})
}

// buildShareLink encrypts payload under a random key, registers the link via
// create, and assembles the public URL (`<server>/web/#/share/<token>?k=<key>`).
func (v *Vault) buildShareLink(
	validity time.Duration,
	payload map[string]any,
	create func(apiclient.CreateShareLinkRequest) (*apiclient.ShareLinkResponse, error),
) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	linkKey, err := crypto.RandKey()
	if err != nil {
		return "", err
	}
	ciphertext, err := crypto.Encrypt(linkKey, plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt payload: %w", err)
	}

	expiresAt := shareNeverExpires
	if validity > 0 {
		expiresAt = time.Now().UTC().Add(validity)
	}

	resp, err := create(apiclient.CreateShareLinkRequest{
		EncryptedPayload: crypto.B64Enc(ciphertext),
		PayloadNonce:     crypto.B64Enc(make([]byte, 12)), // nonce is embedded in the ciphertext
		ExpiresAt:        expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}

	// base64url(key) with padding + query-escaping matches the Flutter generator
	// and what the public viewer's base64Url.decode expects.
	k := base64.URLEncoding.EncodeToString(linkKey)
	base := strings.TrimRight(v.cfg.ServerURL, "/")
	return fmt.Sprintf("%s/web/#/share/%s?k=%s", base, resp.Token, url.QueryEscape(k)), nil
}
