package frontend

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	p, err := ParseProxies([]string{"10.0.0.0/8", "192.0.2.7"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ remote, xff, want string }{
		{"198.51.100.1:5555", "", "198.51.100.1"},
		{"198.51.100.1:5555", "6.6.6.6", "198.51.100.1"}, // untrusted peer: XFF ignored
		{"10.1.2.3:5555", "6.6.6.6, 203.0.113.9, 10.9.9.9", "203.0.113.9"},
		{"192.0.2.7:1", "203.0.113.9", "203.0.113.9"}, // bare IP entry
		{"10.1.2.3:5555", "", "10.1.2.3"},
		{"10.1.2.3:5555", "<script>", ""}, // never echo junk
		{"[::1]:80", "", "::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := p.ClientIP(r); got != c.want {
			t.Errorf("%s / %q: got %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
	if _, err := ParseProxies([]string{"nope"}); err == nil {
		t.Error("invalid proxy entry accepted")
	}
}

func TestSigninShowsClientIP(t *testing.T) {
	h := Handler(Options{})
	r := httptest.NewRequest("GET", "/signin", nil)
	r.RemoteAddr = "203.0.113.5:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `>203.0.113.5</p>`) {
		t.Errorf("client IP not on the sign-in page: %d", w.Code)
	}
}
