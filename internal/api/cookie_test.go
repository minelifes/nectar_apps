package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The first-run address is plain HTTP (http://<server-ip>:30480): a Secure cookie there is dropped by the
// browser and the owner is "not logged in" right after setup.
func TestSessionCookieSecureOnlyOverHTTPS(t *testing.T) {
	s := &Server{}
	cases := []struct {
		name   string
		remote string
		proto  string
		tls    bool
		secure bool
	}{
		{"plain http by IP", "203.0.113.7:51000", "", false, false},
		{"direct TLS", "203.0.113.7:51000", "", true, true},
		{"gateway https", "10.42.0.12:40000", "https", false, true},
		{"gateway http", "10.42.0.12:40000", "http", false, false},
		{"spoofed proto from the internet", "203.0.113.7:51000", "https", false, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/setup", nil)
		r.RemoteAddr = c.remote
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		if c.tls {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		s.setSessionCookie(w, r, "v", time.Now().Add(time.Hour))
		ck := (&http.Response{Header: w.Header()}).Cookies()
		if len(ck) != 1 {
			t.Fatalf("%s: cookies %v", c.name, ck)
		}
		if ck[0].Secure != c.secure || !ck[0].HttpOnly || ck[0].SameSite != http.SameSiteStrictMode {
			t.Errorf("%s: secure=%v httponly=%v samesite=%v, want secure=%v", c.name, ck[0].Secure, ck[0].HttpOnly, ck[0].SameSite, c.secure)
		}
	}
}
