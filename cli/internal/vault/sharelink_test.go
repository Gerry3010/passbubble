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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

func TestBuildShareLink_RoundTrip(t *testing.T) {
	var captured apiclient.CreateShareLinkRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/entries/e1/share-link" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(apiclient.ShareLinkResponse{Token: "TESTTOKEN"})
	}))
	defer srv.Close()

	cfg := &config.Config{ServerURL: srv.URL}
	v := New(cfg, "")

	payload := map[string]any{
		"name": "GitHub",
		"type": "password",
		"url":  "https://github.com",
		"data": map[string]any{"username": "octocat", "password": "s3cr3t!"},
	}
	link, err := v.buildShareLink(7*24*time.Hour, payload, func(req apiclient.CreateShareLinkRequest) (*apiclient.ShareLinkResponse, error) {
		return v.client.CreateEntryShareLink("e1", req)
	})
	if err != nil {
		t.Fatalf("buildShareLink: %v", err)
	}

	// URL shape: <server>/web/#/share/TESTTOKEN?k=<urlescaped base64url key>
	wantPrefix := srv.URL + "/web/#/share/TESTTOKEN?k="
	if !strings.HasPrefix(link, wantPrefix) {
		t.Fatalf("unexpected link: %s (want prefix %s)", link, wantPrefix)
	}

	// Recover the fragment key from the URL and decrypt the captured payload —
	// proving the public viewer (which does exactly this) can read it. The key
	// lives in the hash-route query (`#/share/...?k=`), so it's parsed out of the
	// fragment, not the URL query.
	_, after, ok := strings.Cut(link, "?k=")
	if !ok {
		t.Fatalf("no ?k= in link: %s", link)
	}
	k, err := url.QueryUnescape(after)
	if err != nil {
		t.Fatalf("unescape k: %v", err)
	}
	linkKey, err := base64.URLEncoding.DecodeString(k)
	if err != nil {
		t.Fatalf("decode k: %v", err)
	}
	ct, err := crypto.B64Dec(captured.EncryptedPayload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	plain, err := crypto.Decrypt(linkKey, ct)
	if err != nil {
		t.Fatalf("decrypt payload: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["name"] != "GitHub" {
		t.Errorf("name = %v, want GitHub", got["name"])
	}
	data, _ := got["data"].(map[string]any)
	if data["password"] != "s3cr3t!" {
		t.Errorf("password = %v, want s3cr3t!", data["password"])
	}
}

func TestBuildShareLink_NeverExpiry(t *testing.T) {
	var captured apiclient.CreateShareLinkRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(apiclient.ShareLinkResponse{Token: "T"})
	}))
	defer srv.Close()

	v := New(&config.Config{ServerURL: srv.URL}, "")
	_, err := v.buildShareLink(0, map[string]any{"name": "x"}, func(req apiclient.CreateShareLinkRequest) (*apiclient.ShareLinkResponse, error) {
		return v.client.CreateEntryShareLink("e1", req)
	})
	if err != nil {
		t.Fatalf("buildShareLink: %v", err)
	}
	exp, err := time.Parse(time.RFC3339, captured.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at %q: %v", captured.ExpiresAt, err)
	}
	if exp.Year() < 2100 {
		t.Errorf("never-expiry should be far future, got %s", captured.ExpiresAt)
	}
}
