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

package api

import (
	"net/http"
	"strings"

	chi "github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"github.com/Gerry3010/passbubble/backend/internal/api/handlers"
	mw "github.com/Gerry3010/passbubble/backend/internal/api/middleware"
	"github.com/Gerry3010/passbubble/backend/internal/static"
)

func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(mw.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(mw.SecurityHeaders)
	r.Use(mw.CORS(corsOrigins(s.cfg)))

	// Health check (no auth)
	r.Get("/health", handlers.Health)

	// Flutter web app at /web/* and /admin/*. The SPA shell itself is public
	// (it contains no secrets) — it shows its own login screen when the user
	// isn't authenticated. The actual authorization boundary is the
	// /api/v1/admin/* routes below, gated by JWTAuth + AdminOnly.
	webHandler := http.FileServer(static.WebFS())
	adminHandler := http.FileServer(static.AdminFS())
	// Bare host → the Flutter web app.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusFound)
	})
	r.Handle("/web", http.RedirectHandler("/web/", http.StatusMovedPermanently))
	r.Handle("/web/*", http.StripPrefix("/web", spaHandler(webHandler)))
	r.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	r.Handle("/admin/*", http.StripPrefix("/admin", spaHandler(adminHandler)))

	// API v1
	h := handlers.New(s.pool, s.rdb, s.cfg.JWTSecret, s.cfg.AdminEmail, s.mailer)
	r.Route("/api/v1", func(r chi.Router) {
		// Auth (rate-limited, no JWT required)
		r.Group(func(r chi.Router) {
			r.Use(mw.PerIPRateLimiter(rate.Limit(5.0/60.0), 5)) // 5 req/min
			r.Post("/auth/register", h.Register)
			r.Post("/auth/login", h.Login)
			r.Post("/auth/refresh", h.Refresh)
			r.Get("/auth/verify-email", h.VerifyEmail)
		})

		// Public, unauthenticated endpoints (zero-knowledge share-link retrieval
		// and the second step of 2FA login), rate-limited per IP.
		r.Group(func(r chi.Router) {
			r.Use(mw.PerIPRateLimiter(rate.Limit(10.0/60.0), 10)) // 10 req/min
			r.Get("/share/{token}", h.GetShareLink)
			r.Post("/auth/verify-totp", h.VerifyTOTP)
			r.Post("/auth/totp/recover", h.RequestTOTPRecovery)
			r.Get("/auth/reset-totp", h.ResetTOTP)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(mw.JWTAuth(s.cfg.JWTSecret))

			r.Post("/auth/logout", h.Logout)
			r.Get("/auth/me", h.Me)

			// Account 2FA management (TOTP)
			r.Post("/auth/totp/setup", h.SetupTOTP)
			r.Post("/auth/totp/confirm", h.ConfirmTOTP)
			r.Post("/auth/totp/disable", h.DisableTOTP)

			// Entries
			r.Get("/entries", h.ListEntries)
			r.Post("/entries", h.CreateEntry)
			r.Get("/entries/search", h.SearchEntries)
			r.Get("/entries/{id}", h.GetEntry)
			r.Put("/entries/{id}", h.UpdateEntry)
			r.Delete("/entries/{id}", h.DeleteEntry)
			r.Post("/entries/{id}/share", h.ShareEntry)
			r.Post("/entries/{id}/share-link", h.CreateEntryShareLink)
			r.Delete("/entries/{id}/share/{userId}", h.RevokeEntryShare)

			// Folders
			r.Get("/folders", h.ListFolders)
			r.Post("/folders", h.CreateFolder)
			r.Put("/folders/{id}", h.UpdateFolder)
			r.Delete("/folders/{id}", h.DeleteFolder)
			r.Post("/folders/{id}/share", h.ShareFolder)
			r.Post("/folders/{id}/share-link", h.CreateFolderShareLink)
			r.Delete("/folders/{id}/share/{userId}", h.RevokeFolderShare)

			// Shares aggregation + share-link management
			r.Get("/shares", h.ListMyShares)
			r.Delete("/shares/links/{id}", h.RevokeShareLink)
			r.Delete("/shares/links/{id}/permanent", h.DeleteShareLink)

			// Import/export job ledger
			r.Post("/jobs", h.CreateJob)
			r.Get("/jobs", h.ListJobs)
			r.Get("/jobs/{id}", h.GetJob)
			r.Patch("/jobs/{id}", h.UpdateJob)

			// User public keys (for sharing)
			r.Get("/users/{id}/keys", h.GetUserKeys)
			r.Get("/users/search", h.SearchUsers)

			// Password generator
			r.Post("/generate", h.Generate)

			// Backups
			r.Get("/backup", h.ListBackups)
			r.Post("/backup", h.CreateBackup)
			r.Post("/backup/restore", h.RestoreBackup)
			r.Get("/backup/{name}/verify", h.VerifyBackup)

			// Admin (additional admin-only check inside handlers)
			r.Group(func(r chi.Router) {
				r.Use(mw.AdminOnly)
				r.Post("/admin/invite", h.InviteUser)
				r.Get("/admin/users", h.ListUsers)
				r.Put("/admin/users/{id}", h.UpdateUser)
				r.Get("/admin/invitations", h.ListInvitations)
				r.Delete("/admin/invitations/{id}", h.DeleteInvitation)
			})
		})
	})

	return r
}

// spaHandler returns a handler that serves index.html for unknown paths
// (Single Page Application routing).
func spaHandler(fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the path has a file extension (JS, CSS, etc.), serve directly
		if strings.Contains(r.URL.Path, ".") {
			fs.ServeHTTP(w, r)
			return
		}
		// Otherwise serve index.html (let the Flutter router handle it)
		r.URL.Path = "/"
		fs.ServeHTTP(w, r)
	})
}

func corsOrigins(cfg *Config) []string {
	if cfg.IsDevelopment() {
		return []string{
			"http://localhost:*",
			"http://127.0.0.1:*",
			"chrome-extension://*",
			"moz-extension://*",
		}
	}
	return []string{"https://*", "chrome-extension://*", "moz-extension://*"}
}
