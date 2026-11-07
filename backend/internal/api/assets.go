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
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
)

// revalidatedAssets serves the embedded web app with "check before use" caching.
// Flutter's build output has no hashed file names, so any lasting cache (a CDN's
// default, say) keeps serving the old main.dart.js after a deploy. The ETag keeps
// repeat visits cheap: an unchanged file answers 304 without a body.
func revalidatedAssets(files http.FileSystem, next http.Handler) http.Handler {
	var etags sync.Map // path → ETag; the files are embedded, so they never change
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag, ok := etags.Load(r.URL.Path)
		if !ok {
			if computed := assetETag(files, r.URL.Path); computed != "" {
				etag, _ = etags.LoadOrStore(r.URL.Path, computed)
			}
		}
		if etag != nil {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("ETag", etag.(string))
		}
		next.ServeHTTP(w, r)
	})
}

// assetETag returns a strong ETag for the file at path, or "" if there is none.
func assetETag(files http.FileSystem, path string) string {
	if strings.HasSuffix(path, "/") {
		path += "index.html"
	}
	f, err := files.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || info.IsDir() {
		return ""
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return ""
	}
	return `"` + hex.EncodeToString(sum.Sum(nil)[:16]) + `"`
}
