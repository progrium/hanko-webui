//go:build !dev

package webui

import (
	"embed"
	"net/http"
)

//go:embed admin.html signin.html
var pages embed.FS

// Page returns the named UI page, embedded at build time.
func Page(name string) []byte {
	b, err := pages.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return b
}

// Reload is a no-op outside dev builds.
func Reload(next http.Handler) http.Handler { return next }
