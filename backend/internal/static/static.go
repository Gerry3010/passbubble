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

//go:embed web
var webFiles embed.FS

// WebFS returns an HTTP filesystem serving the embedded Flutter web build.
func WebFS() http.FileSystem {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic("static: sub fs failed: " + err.Error())
	}
	return http.FS(sub)
}
