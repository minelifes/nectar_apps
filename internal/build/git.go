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
	if sha, ok := refs[fullRef(branch)]; ok {
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

// fullRef turns a branch name into its ref; refs/… (e.g. refs/pull/12/head of a preview) stay as they are.
func fullRef(branch string) string {
	if strings.HasPrefix(branch, "refs/") {
		return branch
	}
	return "refs/heads/" + branch
}

// Push is a push or pull request event from GitHub, GitLab or Gitea/Forgejo.
type Push struct {
	Provider string
	Branch   string // "" for tags
	Commit   string
	Message  string
	Deleted  bool
	Ping     bool         // an event that starts nothing (ping, issues, …)
	PR       *PullRequest // a pull / merge request event
}

// Pull request actions.
const (
	PROpen   = "open"   // opened or reopened
	PRUpdate = "update" // new commits
	PRClose  = "close"  // closed or merged
)

// PullRequest is a pull (GitHub, Gitea) or merge (GitLab) request event.
type PullRequest struct {
	Number     int
	Action     string // open | update | close ("" = an event that changes nothing, e.g. a new label)
	BaseBranch string // the branch it merges into
	HeadRef    string // the ref the provider keeps for it: refs/pull/12/head, refs/merge-requests/12/head
	HeadSHA    string
	Title      string
	URL        string
	Author     string
	Fork       bool // from another repository
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
		switch h.Get("X-Gitlab-Event") {
		case "Push Hook":
		case "Merge Request Hook":
			return parseGitLabMR(p, body)
		default:
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
		if strings.HasPrefix(ev, "pull_request") {
			return parsePR(p, body)
		}
		if !strings.HasPrefix(ev, "push") {
			p.Ping = true
			return p, nil
		}
	case h.Get("X-GitHub-Event") != "":
		p.Provider = "github"
		if !checkHMAC(body, secret, strings.TrimPrefix(h.Get("X-Hub-Signature-256"), "sha256=")) {
			return p, ErrSignature
		}
		switch h.Get("X-GitHub-Event") {
		case "push":
		case "pull_request":
			return parsePR(p, body)
		default:
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

// parsePR decodes a GitHub or Gitea/Forgejo pull_request event.
func parsePR(p Push, body []byte) (Push, error) {
	type repo struct {
		FullName string `json:"full_name"`
	}
	var ev struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
			User    struct {
				Login string `json:"login"`
			} `json:"user"`
			Head struct {
				SHA  string `json:"sha"`
				Ref  string `json:"ref"`
				Repo *repo  `json:"repo"`
			} `json:"head"`
			Base struct {
				Ref  string `json:"ref"`
				Repo *repo  `json:"repo"`
			} `json:"base"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return p, fmt.Errorf("webhook: %w", err)
	}
	pr := ev.PullRequest
	n := ev.Number
	if n == 0 {
		n = pr.Number
	}
	if n <= 0 {
		return p, errors.New("webhook: no pull request number")
	}
	out := &PullRequest{Number: n, BaseBranch: pr.Base.Ref, HeadRef: fmt.Sprintf("refs/pull/%d/head", n), HeadSHA: pr.Head.SHA,
		Title: pr.Title, URL: pr.HTMLURL, Author: pr.User.Login}
	out.Fork = pr.Head.Repo == nil || pr.Base.Repo == nil || !strings.EqualFold(pr.Head.Repo.FullName, pr.Base.Repo.FullName)
	switch ev.Action {
	case "opened", "reopened", "ready_for_review":
		out.Action = PROpen
	case "synchronize", "synchronized":
		out.Action = PRUpdate
	case "closed":
		out.Action = PRClose
	}
	p.PR = out
	p.Commit, p.Message = pr.Head.SHA, pr.Title
	return p, nil
}

// parseGitLabMR decodes a GitLab merge request event.
func parseGitLabMR(p Push, body []byte) (Push, error) {
	var ev struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Attrs struct {
			IID             int    `json:"iid"`
			Title           string `json:"title"`
			URL             string `json:"url"`
			Action          string `json:"action"`
			TargetBranch    string `json:"target_branch"`
			SourceProjectID int64  `json:"source_project_id"`
			TargetProjectID int64  `json:"target_project_id"`
			OldRev          string `json:"oldrev"`
			LastCommit      struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"object_attributes"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return p, fmt.Errorf("webhook: %w", err)
	}
	a := ev.Attrs
	if a.IID <= 0 {
		return p, errors.New("webhook: no merge request number")
	}
	out := &PullRequest{Number: a.IID, BaseBranch: a.TargetBranch, HeadRef: fmt.Sprintf("refs/merge-requests/%d/head", a.IID),
		HeadSHA: a.LastCommit.ID, Title: a.Title, URL: a.URL, Author: ev.User.Username, Fork: a.SourceProjectID != a.TargetProjectID}
	switch a.Action {
	case "open", "reopen":
		out.Action = PROpen
	case "update":
		if a.OldRev != "" { // new commits (without oldrev only the title, labels … changed)
			out.Action = PRUpdate
		}
	case "close", "merge":
		out.Action = PRClose
	}
	p.PR = out
	p.Commit, p.Message = a.LastCommit.ID, a.Title
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
