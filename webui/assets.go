//go:build !dev

package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed admin.html signin.html
var pages embed.FS

//go:embed vendor/*.js
var vendor embed.FS

// Page returns the named UI page, embedded at build time.
func Page(name string) []byte {
	b, err := pages.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return b
}

// Assets serves the vendored scripts (webui/vendor) embedded at build time.
func Assets() http.Handler {
	sub, _ := fs.Sub(vendor, "vendor")
	return assetHandler(http.FS(sub))
}

// Reload is a no-op outside dev builds.
func Reload(next http.Handler) http.Handler { return next }
