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

// Package services holds background maintenance loops of the server.
package services

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// trashRetention is how long soft-deleted entries stay restorable.
const trashRetention = 30 * 24 * time.Hour

// StartPurgeLoop hard-deletes entries whose trash retention has elapsed, once
// at startup and then every 24h, until ctx is cancelled. Versions and keys go
// with them via ON DELETE CASCADE.
func StartPurgeLoop(ctx context.Context, pool *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			purgeOnce(ctx, pool)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func purgeOnce(ctx context.Context, pool *pgxpool.Pool) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM entries WHERE deleted_at IS NOT NULL AND deleted_at < NOW() - $1::interval`,
		trashRetention.String())
	if err != nil {
		log.Printf("trash purge failed: %v", err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		log.Printf("trash purge: removed %d expired entries", n)
	}
}
