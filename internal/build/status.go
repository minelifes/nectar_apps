package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Commit states reported to the Git host.
const (
	StatePending = "pending"
	StateSuccess = "success"
	StateFailure = "failure"
)

// StatusContext is the name of the commit status previews report.
const StatusContext = "nectar/preview"

// CommitStatus is a commit status for the pull request page (GitHub, Gitea/Forgejo, GitLab).
type CommitStatus struct {
	Provider    string // github | gitea | gitlab
	RepoURL     string // https://github.com/acme/api(.git)
	Token       string // needs "Commit statuses: write" (GitHub), write:repository (Gitea), api (GitLab)
	SHA         string
	State       string // pending | success | failure
	TargetURL   string
	Description string
}

// ReportStatus sets a commit status. Without a token it does nothing (public repositories still build).
func ReportStatus(ctx context.Context, c *http.Client, st CommitStatus) error {
	if st.Token == "" || len(st.SHA) < 7 {
		return nil
	}
	u, err := url.Parse(st.RepoURL)
	if err != nil {
		return err
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if len(st.Description) > 140 {
		st.Description = st.Description[:140]
	}
	var req *http.Request
	switch st.Provider {
	case "github", "gitea":
		api := "https://" + u.Host + "/api/v3" // GitHub Enterprise
		auth := "Bearer " + st.Token
		switch {
		case st.Provider == "gitea":
			api, auth = u.Scheme+"://"+u.Host+"/api/v1", "token "+st.Token
		case u.Host == "github.com":
			api = "https://api.github.com"
		}
		body, _ := json.Marshal(map[string]string{"state": st.State, "target_url": st.TargetURL, "description": st.Description, "context": StatusContext})
		req, _ = http.NewRequestWithContext(ctx, http.MethodPost, api+"/repos/"+path+"/statuses/"+st.SHA, bytes.NewReader(body))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
	case "gitlab":
		state := st.State
		if state == StateFailure {
			state = "failed"
		}
		q := url.Values{"state": {state}, "name": {StatusContext}, "target_url": {st.TargetURL}, "description": {st.Description}}
		req, _ = http.NewRequestWithContext(ctx, http.MethodPost,
			u.Scheme+"://"+u.Host+"/api/v4/projects/"+url.PathEscape(path)+"/statuses/"+st.SHA+"?"+q.Encode(), nil)
		req.Header.Set("PRIVATE-TOKEN", st.Token)
	default:
		return nil
	}
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("commit status: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}
