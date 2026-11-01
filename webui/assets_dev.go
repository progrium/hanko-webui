//go:build dev

package webui

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var watchGlobs = []string{"*.html", "*.css", "*.js"}

// srcDir is this package's source directory, so pages load from disk regardless of the working directory.
var srcDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}()

const reloadScript = `<script>new EventSource("/__reload").onmessage=()=>location.reload()</script>`

// Page reads the named UI page from disk and injects the live-reload script.
func Page(name string) []byte {
	b, err := os.ReadFile(filepath.Join(srcDir, name))
	if err != nil {
		return []byte(fmt.Sprintf("<pre>dev: %v</pre>%s", err, reloadScript))
	}
	if i := bytes.LastIndex(b, []byte("</body>")); i >= 0 {
		return append(b[:i:i], append([]byte(reloadScript), b[i:]...)...)
	}
	return append(b, reloadScript...)
}

func lastModified() time.Time {
	var latest time.Time
	for _, glob := range watchGlobs {
		matches, _ := filepath.Glob(filepath.Join(srcDir, glob))
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.ModTime().After(latest) {
				latest = fi.ModTime()
			}
		}
	}
	return latest
}

// Reload serves /__reload, an event stream that fires when UI files change, and passes other requests to next.
func Reload(next http.Handler) http.Handler {
	log.Printf("dev mode: serving pages from disk with live reload")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__reload" {
			next.ServeHTTP(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		// Server WriteTimeout would otherwise cut the stream.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		seen := lastModified()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if mod := lastModified(); mod.After(seen) {
					seen = mod
					fmt.Fprint(w, "data: reload\n\n")
					flusher.Flush()
				}
			}
		}
	})
}
