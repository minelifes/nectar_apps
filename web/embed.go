// Package web embeds the Flutter web build (web/dist, produced by `make ui`) into the binary and serves it
// as a SPA. A binary built without it serves a small placeholder page instead.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

//go:embed placeholder.html
var placeholder []byte

// Built reports whether the Flutter web build is embedded.
func Built() bool {
	_, err := fs.Stat(dist, "dist/index.html")
	return err == nil
}

// Handler serves static files from dist/ and falls back to index.html for client-side routes.
func Handler() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return newHandler(root)
}

func newHandler(root fs.FS) http.Handler {
	_, err := fs.Stat(root, "index.html")
	built := err == nil
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if !built {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(placeholder)
			return
		}
		if _, err := fs.Stat(root, p); err != nil || p == ".keep" {
			// unknown path → SPA route (e.g. /projects/42): serve index.html
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			p = "index.html"
		}
		switch {
		case p == "index.html", p == "flutter_service_worker.js", p == "flutter_bootstrap.js", p == "version.json", p == "manifest.json":
			// entry points must be revalidated so a panel update shows the new UI
			w.Header().Set("Cache-Control", "no-cache")
		default:
			// Flutter does not fingerprint its asset names: cache, but revalidate after a day
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		files.ServeHTTP(w, r)
	})
}
