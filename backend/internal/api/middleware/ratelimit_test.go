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
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		realIP     string
		want       string
	}{
		{"direct client, headers ignored", "203.0.113.7:51234", "198.51.100.1", "198.51.100.2", "203.0.113.7"},
		{"nginx on loopback appends the client", "127.0.0.1:40000", "10.9.9.9, 198.51.100.1", "", "198.51.100.1"},
		{"docker bridge peer counts as proxy", "172.28.0.1:40000", "198.51.100.1", "", "198.51.100.1"},
		{"X-Real-IP only as fallback", "127.0.0.1:40000", "", "198.51.100.2", "198.51.100.2"},
		{"last hop wins over X-Real-IP", "127.0.0.1:40000", "198.51.100.1", "198.51.100.2", "198.51.100.1"},
		{"garbage header falls back to peer", "127.0.0.1:40000", "not-an-ip", "", "127.0.0.1"},
		{"IPv6 direct client", "[2001:db8::1]:443", "198.51.100.1", "", "2001:db8::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := ClientIP(req); got != tc.want {
				t.Fatalf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Behind nginx every request arrives on a new upstream connection, so keying
// on RemoteAddr (ip:port) gave each request a fresh bucket.
func TestPerIPRateLimiterKeysOnClientNotConnection(t *testing.T) {
	h := PerIPRateLimiter(rate.Limit(0.001), 2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	do := func(port, client string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:" + port
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i, port := range []string{"40001", "40002"} {
		if code := do(port, "198.51.100.1"); code != http.StatusNoContent {
			t.Fatalf("request %d within burst: got %d", i+1, code)
		}
	}
	if code := do("40003", "198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("third request from the same client: got %d, want 429", code)
	}
	if code := do("40004", "198.51.100.9"); code != http.StatusNoContent {
		t.Fatalf("another client must have its own bucket: got %d", code)
	}
}
