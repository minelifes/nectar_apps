package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	for in, want := range map[string]string{
		"postgres":                           "docker.io/library/postgres:latest",
		"postgres:17":                        "docker.io/library/postgres:17",
		"docker.io/library/redis:7-alpine":   "docker.io/library/redis:7-alpine",
		"index.docker.io/grafana/grafana:11": "docker.io/grafana/grafana:11",
		"ghcr.io/acme/web:1.2.3":             "ghcr.io/acme/web:1.2.3",
		"registry.local:5000/team/app":       "registry.local:5000/team/app:latest",
		"localhost/x":                        "localhost/x:latest",
		"traefik/whoami:v1.10@sha256:" + strings.Repeat("a", 64): "docker.io/traefik/whoami:v1.10@sha256:" + strings.Repeat("a", 64),
	} {
		r, err := ParseRef(in)
		if err != nil || r.String() != want {
			t.Errorf("%s → %s %v, want %s", in, r, err, want)
		}
	}
	if r, _ := ParseRef("postgres:17"); r.Host != DockerHubHost || r.Repo != "library/postgres" || r.Tag != "17" {
		t.Fatalf("%+v", r)
	}
	for _, bad := range []string{"", "Upper/Case", "x:", "x@sha256:12", "a//b"} {
		if _, err := ParseRef(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// Digests over the token flow, anonymous and with a saved login; a registry without the digest header.
func TestDigest(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2}`)
	sum := sha256.Sum256(manifest)
	want := "sha256:" + hex.EncodeToString(sum[:])
	var srv *httptest.Server
	tokens := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokens++
			if r.URL.Query().Get("scope") == "repository:acme/private:pull" {
				if u, p, ok := r.BasicAuth(); !ok || u != "bob" || p != "pw" {
					w.WriteHeader(401)
					return
				}
			}
			fmt.Fprint(w, `{"token":"t-`+r.URL.Query().Get("scope")+`"}`)
		case !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer t-repository:"):
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="reg"`)
			w.WriteHeader(401)
		case strings.HasSuffix(r.URL.Path, "/manifests/missing"):
			w.WriteHeader(404)
		case strings.HasPrefix(r.URL.Path, "/v2/acme/nohead/"):
			if r.Method == http.MethodHead {
				w.WriteHeader(405)
				return
			}
			w.Write(manifest)
		default:
			if !strings.Contains(r.Header.Get("Accept"), "image.index") {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Docker-Content-Digest", "sha256:"+strings.Repeat("b", 64))
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := func(s string) Ref { r, _ := ParseRef(host + "/" + s); return r }
	c := &Client{Scheme: "http"}
	ctx := context.Background()
	if d, err := c.Digest(ctx, ref("acme/web:1")); err != nil || d != "sha256:"+strings.Repeat("b", 64) {
		t.Fatalf("digest: %s %v", d, err)
	}
	if _, err := c.Digest(ctx, ref("acme/web:2")); err != nil || tokens != 1 {
		t.Fatalf("token reused: %d %v", tokens, err)
	}
	if d, err := c.Digest(ctx, ref("acme/nohead:1")); err != nil || d != want {
		t.Fatalf("hashed manifest: %s %v", d, err)
	}
	if _, err := c.Digest(ctx, ref("acme/web:missing")); err == nil || !strings.Contains(err.Error(), "no such tag") {
		t.Fatalf("missing tag: %v", err)
	}
	if _, err := c.Digest(ctx, ref("acme/private:1")); !errors.Is(err, ErrAuth) {
		t.Fatalf("private without a login: %v", err)
	}
	c.Logins = func(h string) (Login, bool) { return Login{"bob", "pw"}, h == host }
	if _, err := c.Digest(ctx, ref("acme/private:1")); err != nil {
		t.Fatalf("private with a login: %v", err)
	}
	if d, _ := c.Digest(ctx, Ref{Host: host, Repo: "x", Digest: want}); d != want {
		t.Fatal("pinned reference")
	}
}
