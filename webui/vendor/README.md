# Vendored scripts

Served from `/assets/` by the frontend so the pages load no third-party code at runtime.

| File | Source | SHA-256 (base64) | License |
|---|---|---|---|
| `hanko-elements-3.1.0.js` | `@teamhanko/hanko-elements@3.1.0/dist/elements.js` | `Tcwr/qFds9wA0+sv75VnhjwFXjrg3TSOwy6gAQBZzRs=` | MIT (bundled notices in `hanko-elements-3.1.0.js.LICENSE.txt`) |
| `mithril-2.3.8.min.js` | `mithril@2.3.8/mithril.min.js` | `SX5qU0FEGASsLEPKvOOzFTvCk1uAV2eJmHRX6b5R7HU=` | MIT |

Hashes match jsdelivr's published file hashes. To update: download the new version, verify its hash against `https://data.jsdelivr.com/v1/packages/npm/<pkg>@<ver>?structure=flat`, replace the file, and update the paths in `webui/*.html` and the test in `webui/vendor_test.go`.
