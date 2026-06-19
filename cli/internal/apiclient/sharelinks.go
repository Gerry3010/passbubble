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

package apiclient

// CreateShareLinkRequest creates a zero-knowledge share link. The payload is
// encrypted client-side under a key that only ever lives in the URL fragment;
// the server stores the ciphertext and never sees the key.
type CreateShareLinkRequest struct {
	EncryptedPayload string `json:"encrypted_payload"` // base64(nonce||ciphertext)
	PayloadNonce     string `json:"payload_nonce"`     // base64; placeholder (nonce is in the ciphertext)
	ExpiresAt        string `json:"expires_at"`        // RFC3339
	Password         string `json:"password,omitempty"`
	MaxViews         *int   `json:"max_views,omitempty"`
}

// ShareLinkResponse is returned on share-link creation. Token is the public
// path segment of the share URL.
type ShareLinkResponse struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// CreateEntryShareLink registers a share link for an entry.
func (c *Client) CreateEntryShareLink(entryID string, req CreateShareLinkRequest) (*ShareLinkResponse, error) {
	var resp ShareLinkResponse
	return &resp, c.post("/api/v1/entries/"+entryID+"/share-link", req, &resp)
}

// CreateFolderShareLink registers a share link for an entire folder.
func (c *Client) CreateFolderShareLink(folderID string, req CreateShareLinkRequest) (*ShareLinkResponse, error) {
	var resp ShareLinkResponse
	return &resp, c.post("/api/v1/folders/"+folderID+"/share-link", req, &resp)
}
