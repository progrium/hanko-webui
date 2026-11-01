package frontend

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rt struct{ h http.Handler }

func (r rt) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// Fake Hanko: cookie "hanko=admin" -> verified admin@x.io, "hanko=bob" -> bob, "hanko=unverified" -> unverified admin email.
func fake(w http.ResponseWriter, r *http.Request) {
	if r.Host == "public.test" && r.URL.Path == "/sessions/validate" {
		c, _ := r.Cookie("hanko")
		switch {
		case c != nil && c.Value == "admin":
			io.WriteString(w, `{"is_valid":true,"claims":{"subject":"u1","email":{"address":"Admin@X.io","is_verified":true}}}`)
		case c != nil && c.Value == "unverified":
			io.WriteString(w, `{"is_valid":true,"claims":{"subject":"u3","email":{"address":"admin@x.io","is_verified":false}}}`)
		case c != nil && c.Value == "bob":
			io.WriteString(w, `{"is_valid":true,"claims":{"subject":"u2","username":"bob"}}`)
		case c != nil && c.Value == "orgadmin":
			io.WriteString(w, `{"is_valid":true,"claims":{"subject":"u4","username":"carol"},"organizations":[{"id":"o1","name":"Staff","roles":["admin"]}]}`)
		case c != nil && c.Value == "orgmember":
			io.WriteString(w, `{"is_valid":true,"claims":{"subject":"u5","username":"dave"},"organizations":[{"id":"o1","name":"Staff","roles":["viewer"]}]}`)
		default:
			io.WriteString(w, `{"is_valid":false}`)
		}
		return
	}
	io.WriteString(w, r.Host+r.URL.Path+"|auth="+r.Header.Get("Authorization")+"|cookie="+r.Header.Get("Cookie"))
}

func TestFrontend(t *testing.T) {
	http.DefaultTransport = rt{http.HandlerFunc(fake)}
	admin, _ := ParseUpstream("a", "http://admin.test")
	pub, _ := ParseUpstream("p", "http://public.test")
	h := Handler(Options{Admin: admin, Public: pub, Token: "tok", AdminAccess: AccessRule{Users: []string{"admin@x.io"}, Organization: "Staff", Roles: []string{"admin"}}})
	do := func(method, path, cookie string, hdr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if cookie != "" {
			req.Header.Set("Cookie", "hanko="+cookie)
		}
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	check := func(name string, rec *httptest.ResponseRecorder, code int, contains string) {
		t.Helper()
		body := rec.Body.String()
		loc := rec.Header().Get("Location")
		if rec.Code != code || !strings.Contains(body+"|loc="+loc, contains) {
			t.Errorf("%s: got %d loc=%q body=%.120q; want %d containing %q", name, rec.Code, loc, body, code, contains)
		} else {
			t.Logf("ok %s: %d %s", name, rec.Code, loc)
		}
	}
	check("anon admin -> signin", do("GET", "/admin/?x=1", ""), 302, "loc=/signin?next=%2Fadmin%2F%3Fx%3D1")
	check("/admin -> /admin/", do("GET", "/admin", ""), 301, "loc=/admin/")
	check("admin page", do("GET", "/admin/", "admin"), 200, "<title>Hanko Admin")
	check("non-admin page", do("GET", "/admin/", "bob"), 403, "bob")
	check("unverified email denied", do("GET", "/admin/", "unverified"), 403, "Access denied")
	check("anon api", do("GET", "/admin/api/users", ""), 401, "Sign in required")
	check("non-admin api", do("GET", "/admin/api/users", "bob"), 403, "Not an admin")
	check("admin api proxied", do("GET", "/admin/api/users", "admin"), 200, "admin.test/users|auth=Bearer tok|cookie=")
	check("admin api mutation needs header", do("POST", "/admin/api/users", "admin"), 403, "Missing UI header")
	check("admin api mutation ok", do("POST", "/admin/api/users", "admin", "X-Admin-UI", "1"), 200, "admin.test/users")
	check("session json", do("GET", "/admin/session", "admin"), 200, `"email":"Admin@X.io"`)
	check("signin page", do("GET", "/signin?next=/admin/", ""), 200, "hanko-auth")
	check("signin when signed in", do("GET", "/signin?next=/admin/", "bob"), 302, "loc=/admin/")
	check("signin open redirect", do("GET", "/signin?next=//evil.com", "bob"), 302, "loc=/")
	for _, next := range []string{"//evil.com", "/%09/evil.com", "/%0d/evil.com", "/%5C/evil.com", "/%5C%5Cevil.com", "https://evil.com"} {
		if l := do("GET", "/signin?next="+next, "bob").Header().Get("Location"); l != "/" {
			t.Errorf("open redirect via %s: %q", next, l)
		}
	}
	check("public passthrough keeps cookie", do("POST", "/login", "admin"), 200, "public.test/login|auth=|cookie=hanko=admin")
	check("old /api not exposed", do("GET", "/api/users", "admin"), 200, "public.test/api/users")
	check("unknown admin path", do("GET", "/admin/nope", "admin"), 404, "")
	check("org admin role allowed", do("GET", "/admin/", "orgadmin"), 200, "<title>Hanko Admin")
	check("org member without role denied", do("GET", "/admin/", "orgmember"), 403, "dave")
	check("session json has organizations", do("GET", "/admin/session", "orgadmin"), 200, `"roles":["admin"]`)
}
