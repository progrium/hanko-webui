// Package frontend is the "frontend" server component. It serves a Hanko sign-in page at
// /signin, the admin UI at /admin/ (restricted to an allowlist of Hanko users) with the admin
// API proxied under /admin/api/, and forwards everything else to the Hanko public API.
// Add new frontend endpoints in Handler.
package frontend

import (
	"bytes"
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
	// AdminAccess decides who may use /admin. An empty rule denies everyone.
	AdminAccess AccessRule
	// Extra mounts additional handlers on the frontend mux, keyed by ServeMux pattern
	// (e.g. "/oidc/"). They get the same live-reload wrapping as built-in routes.
	Extra map[string]http.Handler
	// Labelers name sign-in destinations for the /signin page ("Sign in to continue to …").
	Labelers []NextLabeler
	// Proxies are trusted reverse proxies, for finding the client IP shown on /signin.
	Proxies Proxies
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
	// Scripts come only from this origin (/assets/ is vendored, not a CDN). Inline scripts remain
	// for the pages' own code. img-src is limited to self/data so injected markup can't exfiltrate.
	adminCSP  = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	signinCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
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
	auth := NewAuth(publicTarget, opts.AdminAccess)
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
	labelNext := func(next string) string {
		for _, l := range opts.Labelers {
			if label := l(next); label != "" {
				return label
			}
		}
		return ""
	}

	mux := http.NewServeMux()
	mux.Handle("GET /signin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		next := q.Get("next")
		// Already signed in: skip straight to the destination, unless re-authentication was asked
		// for (fresh=1), in which case the form is shown and a new sign-in replaces the session.
		if next != "" && q.Get("fresh") != "1" {
			if s, err := auth.Session(r); err == nil && s != nil {
				http.Redirect(w, r, safeNext(next), http.StatusFound)
				return
			}
		}
		// The page shows ?to= as the destination; only the server may set it, from a known label.
		label := ""
		if next != "" {
			label = labelNext(safeNext(next))
		}
		if q.Get("to") != label {
			if label == "" {
				q.Del("to")
			} else {
				q.Set("to", label)
			}
			http.Redirect(w, r, "/signin?"+q.Encode(), http.StatusFound)
			return
		}
		// The client's IP, as the server sees it, is shown faintly on the page (useful when
		// setting up IP allowlists). It's a parsed IP, so it needs no further escaping.
		securityHeaders(w, signinCSP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(bytes.Replace(webui.Page("signin.html"), []byte("<!--client-ip-->"), []byte(opts.Proxies.ClientIP(r)), 1))
	}))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", webui.Assets()))
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("GET /admin/{$}", auth.RequireAdmin(servePage("admin.html", adminCSP), false))
	mux.Handle("GET /admin/session", auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(SessionFrom(r.Context()))
	}), true))
	mux.Handle("/admin/api/", auth.RequireAdmin(apiGuard, true))
	mux.Handle("/admin/", http.NotFoundHandler())
	for pattern, h := range opts.Extra {
		mux.Handle(pattern, h)
	}
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

// Upstreams are the resolved frontend listen address and Hanko API URLs.
type Upstreams struct {
	Listen string
	Admin  *url.URL
	Public *url.URL
}

// Resolve applies overrides to the config's addresses and validates the upstream URLs.
func (c Config) Resolve(override Addrs) (Upstreams, error) {
	listen, adminRaw, publicRaw := override.Listen, override.Admin, override.Public
	if listen == "" {
		listen = c.Frontend
	}
	if adminRaw == "" {
		adminRaw = AddrURL(c.Admin)
	}
	if publicRaw == "" {
		publicRaw = AddrURL(c.Public)
	}
	adminURL, err := ParseUpstream("admin", adminRaw)
	if err != nil {
		return Upstreams{}, err
	}
	publicURL, err := ParseUpstream("public", publicRaw)
	if err != nil {
		return Upstreams{}, err
	}
	return Upstreams{Listen: listen, Admin: adminURL, Public: publicURL}, nil
}

// Serve serves the frontend with the given config and upstreams. extra mounts additional
// handlers (see Options.Extra). HANKO_ADMIN_TOKEN sets the upstream admin API bearer token.
func Serve(cfg Config, up Upstreams, extra map[string]http.Handler, labelers ...NextLabeler) error {
	proxies, err := ParseProxies(cfg.TrustedProxies)
	if err != nil {
		return err
	}
	h := Handler(Options{Admin: up.Admin, Public: up.Public, Token: AdminToken(), AdminAccess: cfg.AdminAccess, Extra: extra, Labelers: labelers, Proxies: proxies})
	if cfg.AdminAccess.Empty() {
		log.Printf("frontend: no admin access configured (server.frontend.admin_access); /admin will deny everyone")
	}
	for _, w := range cfg.Warnings() {
		log.Printf("frontend: WARNING: %s", w)
	}
	log.Printf("frontend: %s, listening on %s (admin %s, public %s)", cfg.PublicURL, up.Listen, up.Admin, up.Public)
	return NewServer(up.Listen, h).ListenAndServe()
}

// AdminToken is the Hanko admin API bearer token from HANKO_ADMIN_TOKEN.
func AdminToken() string {
	return os.Getenv("HANKO_ADMIN_TOKEN")
}
