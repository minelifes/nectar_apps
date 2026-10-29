// Package build builds container images from Git repositories with Kaniko Jobs, pushes them to the built-in
// registry and deploys them (PLAN.md M7 "Git builds").
package build

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitUser is the user name sent with access tokens (GitHub, GitLab and Gitea accept any name with a token).
const GitUser = "nectar"

var branchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

// NormalizeRepo checks a repository URL and returns it in https://host/path(.git) form.
func NormalizeRepo(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "git@") { // git@github.com:acme/api.git → https
		host, path, ok := strings.Cut(strings.TrimPrefix(raw, "git@"), ":")
		if !ok {
			return "", errors.New("repository: use an https URL, e.g. https://github.com/acme/api.git")
		}
		raw = "https://" + host + "/" + path
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.Trim(u.Path, "/") == "" {
		return "", errors.New("repository: use an https URL, e.g. https://github.com/acme/api.git")
	}
	if u.User != nil {
		return "", errors.New("repository: put the access token in the token field, not in the URL")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = "/" + strings.Trim(u.Path, "/")
	return u.String(), nil
}

// ValidBranch reports whether a branch name is usable.
func ValidBranch(b string) bool {
	return branchRe.MatchString(b) && !strings.Contains(b, "..") && !strings.HasPrefix(b, "-")
}

// ValidPath reports whether a path inside the repository is safe (no absolute paths or ..).
func ValidPath(p string) bool {
	if p == "" {
		return true
	}
	if strings.HasPrefix(p, "/") || len(p) > 300 {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	return regexp.MustCompile(`^[A-Za-z0-9._/@ -]+$`).MatchString(p)
}

// Remote reads refs of a repository over Git's smart HTTP protocol (like git ls-remote).
type Remote struct {
	HTTP *http.Client
}

// ErrNoBranch means the branch does not exist in the repository.
var ErrNoBranch = errors.New("branch not found")

// Head returns the commit a branch points to.
func (rm Remote) Head(ctx context.Context, repo, branch, token string) (string, error) {
	c := rm.HTTP
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	base := strings.TrimSuffix(repo, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/info/refs?service=git-upload-pack", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "git/2.45.0 (nectar-apps)")
	req.Header.Set("Git-Protocol", "version=1")
	if token != "" {
		req.SetBasicAuth(GitUser, token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("repository: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		if token == "" {
			return "", fmt.Errorf("repository not found or private (HTTP %d): add an access token", resp.StatusCode)
		}
		return "", fmt.Errorf("repository not found or the token cannot read it (HTTP %d)", resp.StatusCode)
	default:
		return "", fmt.Errorf("repository: HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "git-upload-pack-advertisement") {
		return "", errors.New("repository: this URL does not answer like a Git server (check the address)")
	}
	refs, err := parseAdvertisement(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if sha, ok := refs["refs/heads/"+branch]; ok {
		return sha, nil
	}
	return "", fmt.Errorf("%w: %s", ErrNoBranch, branch)
}

// parseAdvertisement reads pkt-lines "<sha> <ref>[\x00caps]".
func parseAdvertisement(r io.Reader) (map[string]string, error) {
	br := bufio.NewReader(r)
	refs := map[string]string{}
	for {
		var lenBuf [4]byte
		if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return refs, nil
			}
			return nil, fmt.Errorf("repository: %w", err)
		}
		n, err := strconv.ParseUint(string(lenBuf[:]), 16, 16)
		if err != nil {
			return nil, errors.New("repository: unexpected answer")
		}
		if n == 0 { // flush
			continue
		}
		if n < 4 {
			return nil, errors.New("repository: unexpected answer")
		}
		line := make([]byte, n-4)
		if _, err := io.ReadFull(br, line); err != nil {
			return nil, fmt.Errorf("repository: %w", err)
		}
		s := strings.TrimRight(string(line), "\n")
		if strings.HasPrefix(s, "#") {
			continue
		}
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		sha, ref, ok := strings.Cut(s, " ")
		if ok && len(sha) == 40 {
			refs[ref] = sha
		}
	}
}

// ------------------------------------------------------------- webhooks --

// Push is a push event from GitHub, GitLab or Gitea/Forgejo.
type Push struct {
	Provider string
	Branch   string // "" for tags
	Commit   string
	Message  string
	Deleted  bool
	Ping     bool
}

// ErrSignature means the webhook signature or token did not match.
var ErrSignature = errors.New("webhook signature does not match")

// ParsePush verifies a webhook request (body already read) and decodes the push.
func ParsePush(h http.Header, body []byte, secret string) (Push, error) {
	var p Push
	switch {
	case h.Get("X-Gitlab-Event") != "":
		p.Provider = "gitlab"
		if !hmac.Equal([]byte(h.Get("X-Gitlab-Token")), []byte(secret)) {
			return p, ErrSignature
		}
		if ev := h.Get("X-Gitlab-Event"); ev != "Push Hook" {
			p.Ping = true
			return p, nil
		}
	case h.Get("X-Gitea-Event") != "" || h.Get("X-Forgejo-Event") != "" || h.Get("X-Gogs-Event") != "":
		p.Provider = "gitea"
		sig := h.Get("X-Gitea-Signature")
		if sig == "" {
			sig = h.Get("X-Forgejo-Signature")
		}
		if !checkHMAC(body, secret, sig) {
			return p, ErrSignature
		}
		ev := h.Get("X-Gitea-Event") + h.Get("X-Forgejo-Event") + h.Get("X-Gogs-Event")
		if !strings.HasPrefix(ev, "push") {
			p.Ping = true
			return p, nil
		}
	case h.Get("X-GitHub-Event") != "":
		p.Provider = "github"
		if !checkHMAC(body, secret, strings.TrimPrefix(h.Get("X-Hub-Signature-256"), "sha256=")) {
			return p, ErrSignature
		}
		if h.Get("X-GitHub-Event") != "push" {
			p.Ping = true
			return p, nil
		}
	default:
		// generic: ?token= is checked by the caller; the body is GitHub-shaped JSON
		p.Provider = "generic"
	}
	var ev struct {
		Ref         string `json:"ref"`
		After       string `json:"after"`
		CheckoutSHA string `json:"checkout_sha"`
		Deleted     bool   `json:"deleted"`
		HeadCommit  *struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"head_commit"`
		Commits []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return p, fmt.Errorf("webhook: %w", err)
	}
	if b, ok := strings.CutPrefix(ev.Ref, "refs/heads/"); ok {
		p.Branch = b
	}
	p.Commit = ev.After
	if ev.CheckoutSHA != "" {
		p.Commit = ev.CheckoutSHA
	}
	p.Deleted = ev.Deleted || strings.Trim(p.Commit, "0") == ""
	switch {
	case ev.HeadCommit != nil:
		p.Message = ev.HeadCommit.Message
	case len(ev.Commits) > 0:
		for _, c := range ev.Commits {
			if c.ID == p.Commit {
				p.Message = c.Message
			}
		}
		if p.Message == "" {
			p.Message = ev.Commits[len(ev.Commits)-1].Message
		}
	}
	p.Message, _, _ = strings.Cut(strings.TrimSpace(p.Message), "\n")
	if len(p.Commit) != 40 {
		if !p.Deleted {
			return p, errors.New("webhook: no commit in the push event")
		}
	}
	return p, nil
}

func checkHMAC(body []byte, secret, sigHex string) bool {
	if sigHex == "" || secret == "" {
		return false
	}
	want, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hmac.Equal(m.Sum(nil), want)
}
