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

package middleware

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// PerIPRateLimiter returns a middleware that limits requests per IP.
// r is requests per second, b is burst size.
func PerIPRateLimiter(r rate.Limit, b int) func(http.Handler) http.Handler {
	type entry struct{ limiter *rate.Limiter }
	var (
		mu      sync.Mutex
		clients = map[string]*entry{}
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := req.RemoteAddr
			mu.Lock()
			e, ok := clients[ip]
			if !ok {
				e = &entry{rate.NewLimiter(r, b)}
				clients[ip] = e
			}
			mu.Unlock()

			if !e.limiter.Allow() {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
