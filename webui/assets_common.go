package webui

import (
	"net/http"
	"strings"
)

// assetHandler serves .js files only, with long-lived caching (filenames are versioned).
func assetHandler(fsys http.FileSystem) http.Handler {
	files := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".js") || strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		files.ServeHTTP(w, r)
	})
}
