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

package static

import (
	"embed"
	"io/fs"
	"net/http"
)

// The same Flutter app is built twice with different --base-href values
// (one assumes it's mounted at /web/, the other at /admin/) so that
// asset URLs (JS, CSS, icons) resolve correctly under either mount path.

//go:embed web
var webFiles embed.FS

//go:embed admin
var adminFiles embed.FS

// WebFS returns an HTTP filesystem serving the embedded Flutter web build
// built for the /web/ mount path.
func WebFS() http.FileSystem {
	return subFS(webFiles, "web")
}

// AdminFS returns an HTTP filesystem serving the embedded Flutter web build
// built for the /admin/ mount path.
func AdminFS() http.FileSystem {
	return subFS(adminFiles, "admin")
}

func subFS(f embed.FS, dir string) http.FileSystem {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic("static: sub fs failed: " + err.Error())
	}
	return http.FS(sub)
}
