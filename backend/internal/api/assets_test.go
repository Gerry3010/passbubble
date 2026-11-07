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
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func assetHandler() http.Handler {
	files := http.FS(fstest.MapFS{
		"index.html":   {Data: []byte("<html>shell</html>")},
		"main.dart.js": {Data: []byte("console.log('app')")},
		"assets/a.txt": {Data: []byte("a")},
	})
	return spaHandler(revalidatedAssets(files, http.FileServer(files)))
}

func getAsset(h http.Handler, path, ifNoneMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAssetsMustRevalidate(t *testing.T) {
	h := assetHandler()
	// "/" and the client-side routes serve the shell, files serve themselves.
	for _, path := range []string{"/", "/entries", "/main.dart.js", "/assets/a.txt"} {
		rec := getAsset(h, path, "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control %q, want no-cache", path, got)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("%s: no ETag", path)
		}
	}
}

func TestAssetsAnswerNotModified(t *testing.T) {
	h := assetHandler()
	etag := getAsset(h, "/main.dart.js", "").Header().Get("ETag")

	rec := getAsset(h, "/main.dart.js", etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("unchanged file: got %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatal("304 must not carry a body")
	}

	// Cloudflare weakens ETags when it compresses a response.
	if rec := getAsset(h, "/main.dart.js", "W/"+etag); rec.Code != http.StatusNotModified {
		t.Fatalf("weak match: got %d, want 304", rec.Code)
	}
	if rec := getAsset(h, "/main.dart.js", `"something-else"`); rec.Code != http.StatusOK {
		t.Fatalf("changed file: got %d, want 200", rec.Code)
	}
	if getAsset(h, "/", "").Header().Get("ETag") == etag {
		t.Fatal("different files must have different ETags")
	}
}

func TestMissingAssetGetsNoCacheHeaders(t *testing.T) {
	rec := getAsset(assetHandler(), "/missing.js", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
	if rec.Header().Get("ETag") != "" {
		t.Fatal("a 404 must not carry an ETag")
	}
}
