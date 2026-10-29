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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterIdleTTL is how long an idle client's limiter is kept before the map
// forgets it; a fresh limiter starts with a full burst, so this only needs to
// outlast the refill time of the strictest limit.
const limiterIdleTTL = 10 * time.Minute

// PerIPRateLimiter returns a middleware that limits requests per client IP
// (see ClientIP). r is requests per second, b is burst size.
func PerIPRateLimiter(r rate.Limit, b int) func(http.Handler) http.Handler {
	type entry struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu        sync.Mutex
		clients   = map[string]*entry{}
		lastSweep = time.Now()
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := ClientIP(req)
			now := time.Now()
			mu.Lock()
			if now.Sub(lastSweep) > limiterIdleTTL {
				for k, e := range clients {
					if now.Sub(e.lastSeen) > limiterIdleTTL {
						delete(clients, k)
					}
				}
				lastSweep = now
			}
			e, ok := clients[ip]
			if !ok {
				e = &entry{limiter: rate.NewLimiter(r, b)}
				clients[ip] = e
			}
			e.lastSeen = now
			mu.Unlock()

			if !e.limiter.Allow() {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// ClientIP returns the IP address of the client that sent the request.
//
// A request whose peer is a loopback or private address came through the
// reverse proxy in front of the container (nginx on the host, or the Caddy
// service), so the peer is the proxy and the client is the last
// X-Forwarded-For hop: the proxy appends it (nginx) or replaces the header
// (Caddy), and anything before it was supplied by the client. X-Real-IP is
// only a fallback because Caddy passes a client-sent one through unchanged.
// Any other peer is a direct connection, and its address is used as is.
func ClientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || (!peer.IsLoopback() && !peer.IsPrivate()) {
		return host
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(hops[len(hops)-1])); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(req.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	return host
}
