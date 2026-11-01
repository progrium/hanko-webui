package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"
)

// The vendored scripts must match the hashes recorded in vendor/README.md.
func TestVendorHashes(t *testing.T) {
	for file, want := range map[string]string{
		"vendor/hanko-elements-3.1.0.js": "Tcwr/qFds9wA0+sv75VnhjwFXjrg3TSOwy6gAQBZzRs=",
		"vendor/mithril-2.3.8.min.js":    "SX5qU0FEGASsLEPKvOOzFTvCk1uAV2eJmHRX6b5R7HU=",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := base64.StdEncoding.EncodeToString(sum[:]); got != want {
			t.Errorf("%s: sha256 %s, want %s", file, got, want)
		}
	}
}
