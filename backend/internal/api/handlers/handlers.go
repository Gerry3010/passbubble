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
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	pool       *pgxpool.Pool
	rdb        *redis.Client
	jwtSecret  []byte
	adminEmail string // if non-empty, only this email may bootstrap the first admin
}

func New(pool *pgxpool.Pool, rdb *redis.Client, jwtSecret []byte, adminEmail string) *Handler {
	return &Handler{pool: pool, rdb: rdb, jwtSecret: jwtSecret, adminEmail: adminEmail}
}

// Health handles GET /health
func Health(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}

func decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, err
	}
	return v, nil
}
