package frontend

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Session is the signed-in Hanko user, as reported by the public API's /sessions/validate.
type Session struct {
	UserID        string `json:"user_id"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified"`
	Username      string `json:"username,omitempty"`
}

// Name is the best human-readable identifier for the user.
func (s *Session) Name() string {
	if s.Email != "" {
		return s.Email
	}
	if s.Username != "" {
		return s.Username
	}
	return s.UserID
}

// Auth checks Hanko sessions against an allowlist of admin users.
type Auth struct {
	public *url.URL
	admins []string
	client *http.Client
}

// NewAuth validates sessions with the Hanko public API at public. Each admin entry matches a
// user ID, a username, or a verified email address (case-insensitive).
func NewAuth(public *url.URL, admins []string) *Auth {
	var clean []string
	for _, a := range admins {
		if a = strings.TrimSpace(a); a != "" {
			clean = append(clean, a)
		}
	}
	return &Auth{public: public, admins: clean, client: &http.Client{Timeout: 10 * time.Second}}
}

// Session returns the request's Hanko session, or nil if there is no valid session.
// The session cookie (or bearer token) is forwarded to Hanko, which checks signature,
// expiry, revocation, and idle timeout.
func (a *Auth) Session(r *http.Request) (*Session, error) {
	cookie, authz := r.Header.Get("Cookie"), r.Header.Get("Authorization")
	if cookie == "" && authz == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.public.JoinPath("sessions", "validate").String(), nil)
	if err != nil {
		return nil, err
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("session validation returned %s", res.Status)
	}
	var body struct {
		IsValid bool `json:"is_valid"`
		Claims  *struct {
			Subject string `json:"subject"`
			Email   *struct {
				Address    string `json:"address"`
				IsVerified bool   `json:"is_verified"`
			} `json:"email"`
			Username *string `json:"username"`
		} `json:"claims"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode session validation: %w", err)
	}
	if !body.IsValid || body.Claims == nil || body.Claims.Subject == "" {
		return nil, nil
	}
	s := &Session{UserID: body.Claims.Subject}
	if e := body.Claims.Email; e != nil {
		s.Email, s.EmailVerified = e.Address, e.IsVerified
	}
	if body.Claims.Username != nil {
		s.Username = *body.Claims.Username
	}
	return s, nil
}

// IsAdmin reports whether the session's user is on the allowlist. Emails only match when verified.
func (a *Auth) IsAdmin(s *Session) bool {
	if s == nil {
		return false
	}
	for _, entry := range a.admins {
		switch {
		case strings.EqualFold(entry, s.UserID):
			return true
		case s.Username != "" && strings.EqualFold(entry, s.Username):
			return true
		case s.Email != "" && s.EmailVerified && strings.EqualFold(entry, s.Email):
			return true
		}
	}
	return false
}

// SignInURL is the sign-in page path that returns to next afterwards.
func SignInURL(next string) string {
	return "/signin?next=" + url.QueryEscape(next)
}

// safeNext only allows same-origin absolute paths, preventing open redirects.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// RequireAdmin only passes requests from signed-in allowlisted admins to next. When api is true,
// failures are JSON errors; otherwise anonymous users are redirected to the sign-in page.
func (a *Auth) RequireAdmin(next http.Handler, api bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := a.Session(r)
		if err != nil {
			log.Printf("frontend: session validation failed: %v", err)
			if api {
				jsonError(w, http.StatusBadGateway, "Session validation unavailable")
			} else {
				http.Error(w, "Session validation unavailable", http.StatusBadGateway)
			}
			return
		}
		switch {
		case s == nil && api:
			jsonError(w, http.StatusUnauthorized, "Sign in required")
		case s == nil:
			http.Redirect(w, r, SignInURL(r.URL.RequestURI()), http.StatusFound)
		case !a.IsAdmin(s):
			log.Printf("frontend: denied admin access to %s (%s)", s.Name(), s.UserID)
			if api {
				jsonError(w, http.StatusForbidden, "Not an admin")
				return
			}
			securityHeaders(w, "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, deniedPage, html.EscapeString(s.Name()))
		default:
			next.ServeHTTP(w, r.WithContext(withSession(r.Context(), s)))
		}
	})
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

const deniedPage = `<!doctype html><meta charset="utf-8"><title>Access denied</title>
<body style="font:14px system-ui;background:#f4f6f9;color:#263248;display:grid;place-items:center;min-height:90vh">
<div style="background:white;border:1px solid #dbe1ea;border-radius:9px;padding:28px;max-width:380px;text-align:center">
<h1 style="font-size:20px;margin:0 0 8px">Access denied</h1>
<p>You're signed in as <b>%s</b>, which isn't an admin account.</p>
<button style="cursor:pointer;border:1px solid #cbd3df;border-radius:6px;background:white;padding:9px 13px" onclick="fetch('/logout',{method:'POST'}).finally(()=>location='/signin?next=/admin/')">Sign in as someone else</button>
</div>`

type sessionKey struct{}

func withSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFrom returns the session attached by RequireAdmin, or nil.
func SessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}
