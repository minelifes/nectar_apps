package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPlaceholderWithoutBuild(t *testing.T) {
	h := newHandler(fstest.MapFS{".keep": {}})
	for _, p := range []string{"/", "/projects/3", "/.keep"} {
		rec := get(h, p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "without the web interface") {
			t.Fatalf("%s: %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestFlutterBuild(t *testing.T) {
	h := newHandler(fstest.MapFS{
		".keep":        {},
		"index.html":   {Data: []byte("<html>app</html>")},
		"main.dart.js": {Data: []byte("js")},
	})
	if rec := get(h, "/projects/3"); rec.Code != 200 || rec.Body.String() != "<html>app</html>" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("SPA route: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec := get(h, "/main.dart.js"); rec.Body.String() != "js" || !strings.Contains(rec.Header().Get("Cache-Control"), "max-age=86400") {
		t.Fatalf("asset: %q %v", rec.Body.String(), rec.Header())
	}
	if rec := get(h, "/.keep"); rec.Body.String() != "<html>app</html>" {
		t.Fatalf(".keep must not be served: %q", rec.Body.String())
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
}
