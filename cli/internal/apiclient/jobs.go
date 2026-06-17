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

import "net/http"

type CreateJobRequest struct {
	Type        string `json:"type"`
	Format      string `json:"format"`
	DupStrategy string `json:"dup_strategy,omitempty"`
	TotalItems  int    `json:"total_items"`
	ClientName  string `json:"client_name"`
}

type UpdateJobRequest struct {
	Status         string `json:"status,omitempty"`
	ProcessedItems *int   `json:"processed_items,omitempty"`
	CreatedItems   *int   `json:"created_items,omitempty"`
	UpdatedItems   *int   `json:"updated_items,omitempty"`
	SkippedItems   *int   `json:"skipped_items,omitempty"`
	FailedItems    *int   `json:"failed_items,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type JobResponse struct {
	ID string `json:"id"`
}

func (c *Client) CreateJob(req CreateJobRequest) (*JobResponse, error) {
	var out JobResponse
	if err := c.post("/jobs", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateJob(id string, req UpdateJobRequest) (*JobResponse, error) {
	var out JobResponse
	if err := c.do(http.MethodPatch, "/jobs/"+id, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
