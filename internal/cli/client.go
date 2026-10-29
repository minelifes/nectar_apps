// Package cli is the `nectar` command-line client: deploy and operate apps on a Nectar Apps panel with an
// API token (Settings → API tokens).
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is ~/.config/nectar/config.json (NECTAR_URL / NECTAR_TOKEN override it).
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// ConfigPath is where `nectar login` stores the panel address and token.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nectar", "config.json"), nil
}

// LoadConfig reads the config file and applies the environment.
func LoadConfig() (Config, error) {
	var c Config
	if p, err := ConfigPath(); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(b, &c); err != nil {
				return c, fmt.Errorf("%s: %w", p, err)
			}
		}
	}
	if v := os.Getenv("NECTAR_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("NECTAR_TOKEN"); v != "" {
		c.Token = v
	}
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	return c, nil
}

// SaveConfig writes the config file (mode 0600: it holds the token).
func SaveConfig(c Config) (string, error) {
	p, err := ConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return p, os.WriteFile(p, append(b, '\n'), 0o600)
}

// Client talks to the panel API.
type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

// APIError is an error answer of the panel.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "the token is not valid (expired or revoked?): " + e.Message
	case http.StatusForbidden:
		return "the token may not do this: " + e.Message
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 0} // uploads can be long; requests use contexts
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	if c.URL == "" {
		return errors.New("no panel address: run `nectar login` or set NECTAR_URL")
	}
	if c.Token == "" {
		return errors.New("no token: run `nectar login` or set NECTAR_TOKEN")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+"/api"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nectar-cli")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if f, ok := body.(*os.File); ok {
		if st, err := f.Stat(); err == nil {
			req.ContentLength = st.Size()
		}
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(b))
		}
		if e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if w, ok := out.(io.Writer); ok {
		_, err := io.Copy(w, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, "", out)
}

func (c *Client) postJSON(ctx context.Context, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	return c.do(ctx, http.MethodPost, path, body, "application/json", out)
}

// API types (the fields the CLI uses).
type (
	User struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	Project struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	App struct {
		ID        int64  `json:"id"`
		ProjectID int64  `json:"project_id"`
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		Runtime   string `json:"runtime"`
		Status    string `json:"status"`
		Message   string `json:"status_message"`
		Config    struct {
			Image string `json:"image"`
		} `json:"config"`
	}
	Release struct {
		ID         int64      `json:"id"`
		Number     int        `json:"number"`
		Status     string     `json:"status"`
		Message    string     `json:"message"`
		Image      string     `json:"image"`
		ArtifactID *int64     `json:"artifact_id"`
		CreatedAt  time.Time  `json:"created_at"`
		FinishedAt *time.Time `json:"finished_at"`
	}
	Artifact struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		SHA256   string `json:"sha256"`
		Detect   struct {
			Runtime  string   `json:"runtime"`
			Warnings []string `json:"warnings"`
		} `json:"detect"`
	}
)

// Me returns the user of the token.
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	return u, c.getJSON(ctx, "/me", &u)
}

// Projects lists the projects the token sees.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var ps []Project
	return ps, c.getJSON(ctx, "/projects", &ps)
}

// Apps lists the apps of a project.
func (c *Client) Apps(ctx context.Context, projectID int64) ([]App, error) {
	var as []App
	return as, c.getJSON(ctx, "/projects/"+strconv.FormatInt(projectID, 10)+"/apps", &as)
}

// App returns an app.
func (c *Client) App(ctx context.Context, id int64) (App, error) {
	var a App
	return a, c.getJSON(ctx, "/apps/"+strconv.FormatInt(id, 10), &a)
}

// ResolveApp finds an app by id ("12") or by "project/app" slugs.
func (c *Client) ResolveApp(ctx context.Context, ref string) (App, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return c.App(ctx, id)
	}
	proj, slug, ok := strings.Cut(ref, "/")
	if !ok || proj == "" || slug == "" {
		return App{}, fmt.Errorf("app %q: use the app id or project/app (e.g. shop/api)", ref)
	}
	ps, err := c.Projects(ctx)
	if err != nil {
		return App{}, err
	}
	for _, p := range ps {
		if p.Slug != proj {
			continue
		}
		as, err := c.Apps(ctx, p.ID)
		if err != nil {
			return App{}, err
		}
		for _, a := range as {
			if a.Slug == slug {
				return a, nil
			}
		}
		return App{}, fmt.Errorf("project %s has no app %q", proj, slug)
	}
	return App{}, fmt.Errorf("project %q not found (or the token may not see it)", proj)
}

// Upload sends an artifact file.
func (c *Client) Upload(ctx context.Context, appID int64, path string) (Artifact, error) {
	f, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	defer f.Close()
	var a Artifact
	q := url.Values{"filename": {filepath.Base(path)}}
	err = c.do(ctx, http.MethodPost, "/apps/"+strconv.FormatInt(appID, 10)+"/artifacts?"+q.Encode(), f, "application/octet-stream", &a)
	return a, err
}

// DeployRequest starts a release.
type DeployRequest struct {
	ArtifactID *int64  `json:"artifact_id,omitempty"`
	Image      *string `json:"image,omitempty"`
	Force      bool    `json:"force,omitempty"`
}

// Deploy starts a release.
func (c *Client) Deploy(ctx context.Context, appID int64, in DeployRequest) (Release, error) {
	var r Release
	return r, c.postJSON(ctx, "/apps/"+strconv.FormatInt(appID, 10)+"/deploy", in, &r)
}

// Releases lists the releases of an app, newest first.
func (c *Client) Releases(ctx context.Context, appID int64) ([]Release, error) {
	var rs []Release
	return rs, c.getJSON(ctx, "/apps/"+strconv.FormatInt(appID, 10)+"/releases", &rs)
}

// Rollback redeploys release n.
func (c *Client) Rollback(ctx context.Context, appID int64, n int) (Release, error) {
	var r Release
	return r, c.postJSON(ctx, fmt.Sprintf("/apps/%d/releases/%d/rollback", appID, n), nil, &r)
}

// Status returns the app status answer.
func (c *Client) Status(ctx context.Context, appID int64) (map[string]any, error) {
	var m map[string]any
	return m, c.getJSON(ctx, "/apps/"+strconv.FormatInt(appID, 10)+"/status", &m)
}

// Logs writes the last lines of the app's logs.
func (c *Client) Logs(ctx context.Context, appID int64, tail int, w io.Writer) error {
	return c.do(ctx, http.MethodGet, fmt.Sprintf("/apps/%d/logs?format=text&tail=%d", appID, tail), nil, "", w)
}

// WaitRelease polls until release n of the app is live or failed.
func (c *Client) WaitRelease(ctx context.Context, appID int64, n int, poll time.Duration, progress func(Release)) (Release, error) {
	last := ""
	for {
		rs, err := c.Releases(ctx, appID)
		if err != nil {
			return Release{}, err
		}
		for _, r := range rs {
			if r.Number != n {
				continue
			}
			if r.Status != last && progress != nil {
				progress(r)
				last = r.Status
			}
			switch r.Status {
			case "live", "superseded":
				return r, nil
			case "failed":
				return r, fmt.Errorf("release #%d failed: %s", n, r.Message)
			}
		}
		select {
		case <-ctx.Done():
			return Release{}, fmt.Errorf("gave up waiting for release #%d: %w", n, ctx.Err())
		case <-time.After(poll):
		}
	}
}
