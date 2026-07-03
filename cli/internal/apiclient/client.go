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

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is an HTTP client for the Passbubble REST API.
type Client struct {
	BaseURL     string
	AccessToken string
	httpClient  *http.Client
}

// New creates a Client targeting the given server URL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetToken sets the Bearer access token for subsequent requests.
func (c *Client) SetToken(token string) {
	c.AccessToken = token
}

// --- Health ---

// Health checks that the server is reachable and responding.
func (c *Client) Health() error {
	resp, err := c.httpClient.Get(c.BaseURL + "/health")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// --- Auth ---

func (c *Client) Register(req RegisterRequest) (*LoginResponse, error) {
	var resp LoginResponse
	return &resp, c.post("/api/v1/auth/register", req, &resp)
}

func (c *Client) Login(req LoginRequest) (*LoginResponse, error) {
	var resp LoginResponse
	return &resp, c.post("/api/v1/auth/login", req, &resp)
}

func (c *Client) Refresh(refreshToken string) (*RefreshResponse, error) {
	var resp RefreshResponse
	return &resp, c.post("/api/v1/auth/refresh", RefreshRequest{RefreshToken: refreshToken}, &resp)
}

func (c *Client) Logout(refreshToken string) error {
	return c.post("/api/v1/auth/logout", RefreshRequest{RefreshToken: refreshToken}, nil)
}

func (c *Client) Me() (*UserResponse, error) {
	var resp UserResponse
	return &resp, c.get("/api/v1/auth/me", &resp)
}

// --- Entries ---

func (c *Client) ListEntries() ([]EntryResponse, error) {
	var resp []EntryResponse
	return resp, c.get("/api/v1/entries", &resp)
}

func (c *Client) GetEntry(id string) (*EntryResponse, error) {
	var resp EntryResponse
	return &resp, c.get("/api/v1/entries/"+id, &resp)
}

func (c *Client) CreateEntry(req CreateEntryRequest) (*EntryResponse, error) {
	var resp EntryResponse
	return &resp, c.post("/api/v1/entries", req, &resp)
}

func (c *Client) UpdateEntry(id string, req UpdateEntryRequest) (*EntryResponse, error) {
	var resp EntryResponse
	return &resp, c.put("/api/v1/entries/"+id, req, &resp)
}

// DeleteEntry soft-deletes: the entry moves to the trash (restorable ~30 days).
func (c *Client) DeleteEntry(id string) error {
	return c.delete("/api/v1/entries/" + id)
}

func (c *Client) ListTrash() ([]EntryResponse, error) {
	var resp []EntryResponse
	return resp, c.get("/api/v1/entries/trash", &resp)
}

func (c *Client) RestoreEntry(id string) error {
	return c.post("/api/v1/entries/"+id+"/restore", map[string]string{}, nil)
}

// PurgeEntry is irreversible — removes the entry (and its history) for good.
func (c *Client) PurgeEntry(id string) error {
	return c.delete("/api/v1/entries/" + id + "/permanent")
}

func (c *Client) SetFavorite(id string, favorite bool) error {
	return c.put("/api/v1/entries/"+id+"/favorite", map[string]bool{"favorite": favorite}, nil)
}

func (c *Client) ListVersions(id string) ([]EntryVersionResponse, error) {
	var resp []EntryVersionResponse
	return resp, c.get("/api/v1/entries/"+id+"/versions", &resp)
}

func (c *Client) GetVersion(id, versionID string) (*EntryVersionResponse, error) {
	var resp EntryVersionResponse
	return &resp, c.get("/api/v1/entries/"+id+"/versions/"+versionID, &resp)
}

// RestoreVersion is server-side: the current state is snapshotted first, then
// the version's blob and its contemporaneous wrapped keys are copied back.
func (c *Client) RestoreVersion(id, versionID string) error {
	return c.post("/api/v1/entries/"+id+"/versions/"+versionID+"/restore", map[string]string{}, nil)
}

func (c *Client) SearchEntries(query string) ([]EntryResponse, error) {
	var resp []EntryResponse
	return resp, c.get("/api/v1/entries/search?q="+url.QueryEscape(query), &resp)
}

func (c *Client) ShareEntry(id string, req ShareEntryRequest) error {
	return c.post("/api/v1/entries/"+id+"/share", req, nil)
}

// --- Users ---

func (c *Client) GetUserKeys(userID string) (*UserPublicKeys, error) {
	var resp UserPublicKeys
	return &resp, c.get("/api/v1/users/"+userID+"/keys", &resp)
}

func (c *Client) SearchUsers(query string) ([]UserResponse, error) {
	var resp []UserResponse
	return resp, c.get("/api/v1/users/search?q="+url.QueryEscape(query), &resp)
}

// UpdateUserKeys rotates the caller's own key material (post-quantum upgrade).
func (c *Client) UpdateUserKeys(req UpdateKeysRequest) error {
	return c.patch("/api/v1/auth/me/keys", req, nil)
}

// --- Generate ---

func (c *Client) Generate(req GenerateRequest) (*GenerateResponse, error) {
	var resp GenerateResponse
	return &resp, c.post("/api/v1/generate", req, &resp)
}

// --- Folders ---

func (c *Client) ListFolders() ([]FolderResponse, error) {
	var resp []FolderResponse
	return resp, c.get("/api/v1/folders", &resp)
}

// CreateFolder creates a folder and returns its new ID.
func (c *Client) CreateFolder(req CreateFolderRequest) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	return resp.ID, c.post("/api/v1/folders", req, &resp)
}

// UpdateFolder renames or re-parents a folder.
func (c *Client) UpdateFolder(id string, req CreateFolderRequest) error {
	return c.put("/api/v1/folders/"+id, req, nil)
}

// DeleteFolder removes a folder.
func (c *Client) DeleteFolder(id string) error {
	return c.delete("/api/v1/folders/" + id)
}

// --- HTTP helpers ---

func (c *Client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, out)
}

func (c *Client) post(path string, body any, out any) error {
	return c.do(http.MethodPost, path, body, out)
}

func (c *Client) put(path string, body any, out any) error {
	return c.do(http.MethodPut, path, body, out)
}

func (c *Client) patch(path string, body any, out any) error {
	return c.do(http.MethodPatch, path, body, out)
}

func (c *Client) delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil)
}

func (c *Client) do(method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		var apiErr ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error != "" {
			return fmt.Errorf("api error %d: %s", resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("api error %d", resp.StatusCode)
	}

	if out != nil && resp.ContentLength != 0 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
