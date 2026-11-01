package oidc

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
)

// tokenError writes an RFC 6749 §5.2 error response.
func tokenError(w http.ResponseWriter, status int, code, desc string) {
	noStore(w)
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="oidc"`)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// redirectError sends an authorization error back to a validated redirect URI (RFC 6749 §4.1.2.1).
func redirectError(w http.ResponseWriter, r *http.Request, redirectURI, state, code, desc string) {
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("error", code)
	if desc != "" {
		q.Set("error_description", desc)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// errorPage is shown when the client or redirect URI can't be trusted, so we must not redirect.
func errorPage(w http.ResponseWriter, status int, title, msg string) {
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	fmt.Fprintf(w, page, html.EscapeString(title), html.EscapeString(title), html.EscapeString(msg))
}

const page = `<!doctype html><meta charset="utf-8"><title>%s</title>
<body style="font:14px system-ui;background:#f4f6f9;color:#263248;display:grid;place-items:center;min-height:90vh">
<div style="background:white;border:1px solid #dbe1ea;border-radius:9px;padding:28px;max-width:420px;text-align:center">
<h1 style="font-size:20px;margin:0 0 8px">%s</h1><p>%s</p></div>`

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
