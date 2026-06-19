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
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/api/models"
)

var validJobTypes = map[string]bool{
	"import": true,
	"export": true,
}

var validJobFormats = map[string]bool{
	"csv-generic":   true,
	"csv-chrome":    true,
	"csv-lastpass":  true,
	"csv-1password": true,
	"bitwarden":     true,
	"keepass":       true,
	"psono":         true,
	"onepassword":   true,
	"csv":           true,
}

var validDupStrategies = map[string]bool{
	"skip":      true,
	"overwrite": true,
}

const jobCols = `SELECT id, type, format, status, dup_strategy, total_items, processed_items,
	created_items, updated_items, skipped_items, failed_items,
	error_message, client_name, created_at::text, updated_at::text, finished_at::text
FROM jobs`

func scanJob(row interface {
	Scan(dest ...any) error
}, job *models.JobResponse) error {
	return row.Scan(&job.ID, &job.Type, &job.Format, &job.Status, &job.DupStrategy,
		&job.TotalItems, &job.ProcessedItems, &job.CreatedItems, &job.UpdatedItems,
		&job.SkippedItems, &job.FailedItems, &job.ErrorMessage, &job.ClientName,
		&job.CreatedAt, &job.UpdatedAt, &job.FinishedAt)
}

// CreateJob handles POST /api/v1/jobs
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())

	req, err := decode[models.CreateJobRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !validJobTypes[req.Type] {
		respondErr(w, http.StatusBadRequest, "invalid job type")
		return
	}
	if !validJobFormats[req.Format] {
		respondErr(w, http.StatusBadRequest, "invalid job format")
		return
	}
	if req.DupStrategy == "" {
		req.DupStrategy = "skip"
	}
	if !validDupStrategies[req.DupStrategy] {
		respondErr(w, http.StatusBadRequest, "invalid dup_strategy")
		return
	}

	id := uuid.New().String()
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO jobs (id, user_id, type, format, status, dup_strategy, total_items, client_name)
		VALUES ($1, $2, $3, $4, 'running', $5, $6, $7)`,
		id, claims.UserID, req.Type, req.Format, req.DupStrategy, req.TotalItems, nullableString(req.ClientName))
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	var job models.JobResponse
	if err := scanJob(h.pool.QueryRow(r.Context(), jobCols+` WHERE id=$1`, id), &job); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to read job")
		return
	}

	respond(w, http.StatusCreated, job)
}

// GetJob handles GET /api/v1/jobs/{id}
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	jobID := chi.URLParam(r, "id")

	var job models.JobResponse
	if err := scanJob(h.pool.QueryRow(r.Context(), jobCols+` WHERE id=$1 AND user_id=$2`, jobID, claims.UserID), &job); err != nil {
		respondErr(w, http.StatusNotFound, "job not found")
		return
	}

	respond(w, http.StatusOK, job)
}

// ListJobs handles GET /api/v1/jobs?status=running
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	statusFilter := r.URL.Query().Get("status")

	jobs := []models.JobResponse{}
	scanRows := func(query string, args ...any) error {
		rows, qErr := h.pool.Query(r.Context(), query, args...)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()
		for rows.Next() {
			var job models.JobResponse
			if sErr := scanJob(rows, &job); sErr != nil {
				continue
			}
			jobs = append(jobs, job)
		}
		return nil
	}

	var err error
	if statusFilter != "" {
		err = scanRows(jobCols+` WHERE user_id=$1 AND status=$2 ORDER BY created_at DESC LIMIT 20`, claims.UserID, statusFilter)
	} else {
		err = scanRows(jobCols+` WHERE user_id=$1 ORDER BY created_at DESC LIMIT 20`, claims.UserID)
	}
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	respond(w, http.StatusOK, jobs)
}

// UpdateJob handles PATCH /api/v1/jobs/{id}
func (h *Handler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	claims := mw.ClaimsFromCtx(r.Context())
	jobID := chi.URLParam(r, "id")

	req, err := decode[models.UpdateJobRequest](r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check ownership first
	var ownerID string
	if err := h.pool.QueryRow(r.Context(), `SELECT user_id FROM jobs WHERE id=$1`, jobID).Scan(&ownerID); err != nil {
		respondErr(w, http.StatusNotFound, "job not found")
		return
	}
	if ownerID != claims.UserID {
		respondErr(w, http.StatusNotFound, "job not found")
		return
	}

	if req.Status != "" && req.Status != "running" && req.Status != "completed" && req.Status != "failed" && req.Status != "cancelled" {
		respondErr(w, http.StatusBadRequest, "invalid status")
		return
	}

	finishedStatuses := map[string]bool{"completed": true, "failed": true, "cancelled": true}
	finishedClause := ""
	if req.Status != "" && finishedStatuses[req.Status] {
		finishedClause = ", finished_at=NOW()"
	}

	_, err = h.pool.Exec(r.Context(), `UPDATE jobs SET
			status=COALESCE(NULLIF($2,''), status),
			processed_items=COALESCE($3, processed_items),
			created_items=COALESCE($4, created_items),
			updated_items=COALESCE($5, updated_items),
			skipped_items=COALESCE($6, skipped_items),
			failed_items=COALESCE($7, failed_items),
			error_message=COALESCE(NULLIF($8,''), error_message),
			updated_at=NOW()`+finishedClause+`
		WHERE id=$1 AND user_id=$9`,
		jobID, req.Status, req.ProcessedItems, req.CreatedItems,
		req.UpdatedItems, req.SkippedItems, req.FailedItems, req.ErrorMessage, claims.UserID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to update job")
		return
	}

	var job models.JobResponse
	if err := scanJob(h.pool.QueryRow(r.Context(), jobCols+` WHERE id=$1`, jobID), &job); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to read job")
		return
	}

	respond(w, http.StatusOK, job)
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
