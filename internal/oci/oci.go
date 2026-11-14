// Package oci reads from container registries (OCI distribution API): manifest digests and blobs, with
// the registries' token flow — anonymous, or with a saved login. Only the standard library is used.
package oci

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Ref is a parsed image reference.
type Ref struct {
	Host   string // registry host[:port]; Docker Hub is registry-1.docker.io
	Repo   string // e.g. library/postgres
	Tag    string // "" when only a digest is given
	Digest string // sha256:… when pinned
}

// String is the reference as a pod would pull it.
func (r Ref) String() string {
	host := r.Host
	if host == DockerHubHost {
		host = "docker.io"
	}
	s := host + "/" + r.Repo
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// DockerHubHost is where Docker Hub images are read from.
const DockerHubHost = "registry-1.docker.io"

var (
	repoRe   = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*$`)
	tagRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	digestRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// ParseRef reads an image reference as Docker does: "postgres" → registry-1.docker.io/library/postgres:latest,
// "ghcr.io/acme/web:1.2", "registry.local:5000/x/y@sha256:…".
func ParseRef(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	var r Ref
	if i := strings.Index(s, "@"); i >= 0 {
		r.Digest = s[i+1:]
		s = s[:i]
		if !digestRe.MatchString(r.Digest) {
			return Ref{}, fmt.Errorf("%q: bad digest", r.Digest)
		}
	}
	// a tag is the part after the last ":" that comes after the last "/"
	if i := strings.LastIndex(s, ":"); i > strings.LastIndex(s, "/") {
		r.Tag = s[i+1:]
		s = s[:i]
		if !tagRe.MatchString(r.Tag) {
			return Ref{}, fmt.Errorf("%q: bad tag", r.Tag)
		}
	}
	first, rest, ok := strings.Cut(s, "/")
	if ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		r.Host, r.Repo = first, rest
	} else {
		r.Host, r.Repo = DockerHubHost, s
	}
	if r.Host == "docker.io" || r.Host == "index.docker.io" {
		r.Host = DockerHubHost
	}
	if r.Host == DockerHubHost && !strings.Contains(r.Repo, "/") {
		r.Repo = "library/" + r.Repo
	}
	if r.Repo == "" || !repoRe.MatchString(r.Repo) {
		return Ref{}, fmt.Errorf("%q is not an image reference", s)
	}
	if r.Tag == "" && r.Digest == "" {
		r.Tag = "latest"
	}
	return r, nil
}

// Login is a registry login.
type Login struct{ Username, Password string }

// Client reads from registries. The zero value works (https, anonymous, 30 s timeout).
type Client struct {
	HTTP   *http.Client
	Scheme string                          // default https (tests: http)
	Logins func(host string) (Login, bool) // nil: anonymous only

	mu     sync.Mutex
	tokens map[string]string // host|repo → bearer token
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) scheme() string {
	if c.Scheme != "" {
		return c.Scheme
	}
	return "https"
}

// ManifestAccept lists the manifest types a digest is asked for (an index first: that is the digest a
// node records when it pulls by tag).
const ManifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

// Digest returns the digest the registry serves for the reference's tag now.
func (c *Client) Digest(ctx context.Context, r Ref) (string, error) {
	if r.Tag == "" {
		return r.Digest, nil
	}
	path := "/v2/" + r.Repo + "/manifests/" + r.Tag
	resp, err := c.do(ctx, http.MethodHead, r.Host, r.Repo, path, ManifestAccept)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if d := resp.Header.Get("Docker-Content-Digest"); resp.StatusCode == http.StatusOK && digestRe.MatchString(d) {
		return d, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%s: no such tag", r)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMethodNotAllowed {
		return "", statusErr(r.String(), resp.StatusCode)
	}
	// no digest header (or no HEAD): hash the manifest itself
	body, code, err := c.Get(ctx, r.Host, r.Repo, path, 4<<20)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", statusErr(r.String(), code)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ErrAuth: the registry refused the login (or wants one that is not saved).
var ErrAuth = errors.New("the registry wants a login")

func statusErr(what string, code int) error {
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return fmt.Errorf("%s: %w (save one under Settings → Registries)", what, ErrAuth)
	}
	if code == http.StatusTooManyRequests {
		return fmt.Errorf("%s: the registry's rate limit was reached, trying again later", what)
	}
	return fmt.Errorf("%s: the registry answered HTTP %d", what, code)
}

// Get reads path of a repository (a manifest or a blob), at most limit bytes.
func (c *Client) Get(ctx context.Context, host, repo, path string, limit int64) ([]byte, int, error) {
	resp, err := c.do(ctx, http.MethodGet, host, repo, path, ManifestAccept+", */*")
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(body)) > limit {
		return nil, 0, errors.New("the registry's answer is too large")
	}
	return body, resp.StatusCode, nil
}

func (c *Client) do(ctx context.Context, method, host, repo, path, accept string) (*http.Response, error) {
	key := host + "|" + repo
	basic := false
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.scheme()+"://"+host+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", "nectar-apps")
		c.mu.Lock()
		tok := c.tokens[key]
		c.mu.Unlock()
		if basic {
			if l, ok := c.login(host); ok {
				req.SetBasicAuth(l.Username, l.Password)
			}
		} else if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.http().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized || attempt > 0 {
			return resp, nil
		}
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		switch {
		case strings.HasPrefix(strings.ToLower(challenge), "bearer "):
			t, err := c.token(ctx, host, repo, challenge)
			if err != nil {
				return nil, err
			}
			c.mu.Lock()
			if c.tokens == nil {
				c.tokens = map[string]string{}
			}
			c.tokens[key] = t
			c.mu.Unlock()
		case strings.HasPrefix(strings.ToLower(challenge), "basic"):
			if _, ok := c.login(host); !ok {
				return nil, fmt.Errorf("%s: %w (save one under Settings → Registries)", host, ErrAuth)
			}
			basic = true
		default:
			return nil, fmt.Errorf("%s: %w", host, ErrAuth)
		}
	}
}

func (c *Client) login(host string) (Login, bool) {
	if c.Logins == nil {
		return Login{}, false
	}
	return c.Logins(host)
}

var challengeRe = regexp.MustCompile(`(realm|service|scope)="([^"]*)"`)

// token gets a pull token as the registry's challenge describes (with the saved login when there is one).
func (c *Client) token(ctx context.Context, host, repo, challenge string) (string, error) {
	p := map[string]string{}
	for _, m := range challengeRe.FindAllStringSubmatch(challenge, -1) {
		p[m[1]] = m[2]
	}
	realm, err := url.Parse(p["realm"])
	if err != nil || realm.Host == "" || (realm.Scheme != "https" && c.scheme() != "http") {
		return "", errors.New("the registry's token service is not usable")
	}
	q := realm.Query()
	if p["service"] != "" {
		q.Set("service", p["service"])
	}
	q.Set("scope", "repository:"+repo+":pull")
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	if l, ok := c.login(host); ok {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(l.Username+":"+l.Password)))
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%s: %w", host, ErrAuth)
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return "", fmt.Errorf("the registry's token service answered HTTP %d", resp.StatusCode)
	}
	if out.Token == "" {
		out.Token = out.AccessToken
	}
	if out.Token == "" {
		return "", errors.New("the registry gave no token")
	}
	return out.Token, nil
}

// HostAliases are the names a saved login may use for a registry host (Docker Hub has several).
func HostAliases(host string) []string {
	if host == DockerHubHost {
		return []string{"docker.io", "index.docker.io", "registry-1.docker.io", "https://index.docker.io/v1/"}
	}
	return []string{host}
}
