// Package frontend is the "frontend" server component. It serves a Hanko sign-in page at
// /signin, the admin UI at /admin/ (restricted to an allowlist of Hanko users) with the admin
// API proxied under /admin/api/, and forwards everything else to the Hanko public API.
// Add new frontend endpoints in Handler.
package frontend

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/progrium/hankoid/webui"
)

// Options configures the frontend handler.
type Options struct {
	// Admin is the Hanko admin API root URL; it may include a path prefix.
	Admin *url.URL
	// Public is the Hanko public API URL that receives all other requests.
	Public *url.URL
	// Token, if set, is sent upstream as the admin API bearer token.
	Token string
	// Admins lists the users allowed into /admin, by user ID, username, or verified email.
	Admins []string
}

// ParseUpstream validates an upstream http(s) URL without query, fragment, or credentials.
func ParseUpstream(name, raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.RawQuery != "" || target.Fragment != "" || target.User != nil {
		return nil, fmt.Errorf("invalid %s URL", name)
	}
	return target, nil
}

const (
	adminCSP  = "default-src 'none'; script-src 'self' 'unsafe-inline' https://unpkg.com; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	signinCSP = "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'"
)

func securityHeaders(w http.ResponseWriter, csp string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", csp)
}

func servePage(name, csp string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w, csp)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			_, _ = w.Write(webui.Page(name))
		}
	})
}

// Handler routes the frontend:
//
//	/signin        Hanko sign-in page (hanko-elements); ?next= is where to go afterwards
//	/admin/        admin UI, for signed-in allowlisted admins only
//	/admin/api/*   admin API proxy, same protection
//	/admin/session the signed-in admin as JSON
//	everything else is forwarded to the Hanko public API (which hanko-elements calls same-origin)
func Handler(opts Options) http.Handler {
	target, publicTarget, token := opts.Admin, opts.Public, opts.Token
	auth := NewAuth(publicTarget, opts.Admins)
	adminAPI := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.Out.URL.Path = strings.TrimPrefix(p.In.URL.Path, "/admin/api")
			p.Out.URL.RawPath = strings.TrimPrefix(p.In.URL.EscapedPath(), "/admin/api")
			p.SetURL(target)
			p.Out.Host = target.Host
			p.Out.Header.Del("Authorization")
			p.Out.Header.Del("Cookie")
			p.Out.Header.Del("Origin")
			p.Out.Header.Del("Referer")
			if token != "" {
				p.Out.Header.Set("Authorization", "Bearer "+token)
			}
		},
		ModifyResponse: func(r *http.Response) error {
			r.Header.Del("Set-Cookie")
			if location := r.Header.Get("Location"); location != "" {
				// Do not follow upstream redirects outside the same-origin proxy.
				r.Header.Del("Location")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("upstream request failed: %v", err)
			http.Error(w, `{"message":"Upstream unavailable"}`, http.StatusBadGateway)
		},
	}
	publicProxy := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(publicTarget)
			p.SetXForwarded()
			p.Out.Host = publicTarget.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("public request failed: %v", err)
			http.Error(w, "Upstream unavailable", http.StatusBadGateway)
		},
	}
	apiGuard := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		// JSON mutations require a same-origin custom header, preventing form CSRF.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Admin-UI") != "1" {
			jsonError(w, http.StatusForbidden, "Missing UI header")
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			jsonError(w, http.StatusForbidden, "Cross-site request denied")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		adminAPI.ServeHTTP(w, r)
	})
	signinPage := servePage("signin.html", signinCSP)

	mux := http.NewServeMux()
	mux.Handle("GET /signin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Already signed in: skip straight to the destination.
		if next := r.URL.Query().Get("next"); next != "" {
			if s, err := auth.Session(r); err == nil && s != nil {
				http.Redirect(w, r, safeNext(next), http.StatusFound)
				return
			}
		}
		signinPage.ServeHTTP(w, r)
	}))
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("GET /admin/{$}", auth.RequireAdmin(servePage("admin.html", adminCSP), false))
	mux.Handle("GET /admin/session", auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(SessionFrom(r.Context()))
	}), true))
	mux.Handle("/admin/api/", auth.RequireAdmin(apiGuard, true))
	mux.Handle("/admin/", http.NotFoundHandler())
	mux.Handle("/", publicProxy)
	return webui.Reload(mux)
}

// NewServer returns an HTTP server for the frontend with conservative timeouts.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
}

// Addrs overrides addresses read from the config. Admin and Public are full URLs and may include a path prefix.
type Addrs struct {
	Listen, Admin, Public string
}

// Serve loads server addresses from the Hanko config file, applies overrides, and serves the frontend.
// HANKO_ADMIN_TOKEN sets the upstream admin API bearer token.
func Serve(configFile string, override Addrs) error {
	sc, err := LoadServerConfig(configFile)
	if err != nil {
		return err
	}
	listen, adminRaw, publicRaw := override.Listen, override.Admin, override.Public
	if listen == "" {
		listen = sc.Frontend
	}
	if adminRaw == "" {
		adminRaw = AddrURL(sc.Admin)
	}
	if publicRaw == "" {
		publicRaw = AddrURL(sc.Public)
	}
	adminURL, err := ParseUpstream("admin", adminRaw)
	if err != nil {
		return err
	}
	publicURL, err := ParseUpstream("public", publicRaw)
	if err != nil {
		return err
	}
	h := Handler(Options{Admin: adminURL, Public: publicURL, Token: os.Getenv("HANKO_ADMIN_TOKEN"), Admins: sc.Admins})
	if len(sc.Admins) == 0 {
		log.Printf("frontend: no admins configured (server.frontend.admins); /admin will deny everyone")
	}
	log.Printf("frontend: http://%s (admin %s, public %s)", listen, adminURL, publicURL)
	return NewServer(listen, h).ListenAndServe()
}
