package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Components.
const (
	Panel   = "panel"
	Gateway = "gateway"
	K3s     = "k3s"
)

// Channels.
const (
	Stable = "stable"
	Beta   = "beta"
)

// Release is one release of a component, as published in release.json (scripts/release_meta.py).
type Release struct {
	Component   string    `json:"component"`
	Version     string    `json:"version"`
	Channel     string    `json:"channel"`
	Image       string    `json:"image"` // pinned digest: ghcr.io/minelifes/nectar-apps@sha256:…
	Kind        string    `json:"kind"`  // patch | minor | major
	Security    bool      `json:"security"`
	Breaking    bool      `json:"breaking"`
	APIVersion  *int      `json:"api_version,omitempty"`
	Requires    Requires  `json:"requires"`
	Migrations  bool      `json:"migrations"`
	Notes       string    `json:"notes"`
	Previous    string    `json:"previous,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// Requires holds the compatibility constraints of a release (.github/release-requires.json).
type Requires struct {
	GatewayAPI []int  `json:"gateway_api,omitempty"` // panel: gateway admin API versions it can talk to
	MinGateway string `json:"min_gateway,omitempty"` // panel: oldest gateway it works with
	PanelAPI   []int  `json:"panel_api,omitempty"`   // gateway: panel API versions it works with
	Panel      string `json:"panel,omitempty"`       // gateway: constraint on the panel version, e.g. ">=0.6.0"
}

// Semver returns the parsed version (zero when invalid).
func (r Release) Semver() Semver { v, _ := ParseSemver(r.Version); return v }

// Feed is updates.json: the releases of every component per channel, newest first.
type Feed struct {
	Schema     int                             `json:"schema"`
	Components map[string]map[string][]Release `json:"components"`
	UpdatedAt  string                          `json:"updated_at,omitempty"`
}

// Source says where the releases of a component come from.
type Source struct {
	FeedURL string // updates.json (GitHub Pages)
	Repo    string // owner/name for the GitHub Releases fallback
}

// DefaultSources are the official feeds.
var DefaultSources = map[string]Source{
	Panel:   {FeedURL: "https://minelifes.github.io/nectar_apps/updates.json", Repo: "minelifes/nectar_apps"},
	Gateway: {FeedURL: "https://minelifes.github.io/nectar_gateway/updates.json", Repo: "minelifes/nectar_gateway"},
}

// Fetcher downloads release lists.
type Fetcher struct {
	HTTP      *http.Client
	GitHubAPI string // default https://api.github.com
}

func (f *Fetcher) client() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (f *Fetcher) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nectar-apps-updater")
	resp, err := f.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

// Releases returns the releases of a component in a channel (newest first). The beta channel includes
// stable releases. It reads the feed and falls back to the GitHub Releases API; the error of the feed is
// returned together with the fallback result so the UI can show it.
func (f *Fetcher) Releases(ctx context.Context, component, channel string, src Source) ([]Release, string, error) {
	var feedErr error
	if src.FeedURL != "" {
		var feed Feed
		if err := f.getJSON(ctx, src.FeedURL, &feed); err == nil {
			comp := feed.Components[component]
			if comp == nil {
				feedErr = fmt.Errorf("the feed has no %s releases", component)
			} else {
				list := append([]Release(nil), comp[channel]...)
				if channel == Beta && len(comp[Beta]) == 0 {
					list = append(list, comp[Stable]...)
				}
				return clean(component, channel, list), "feed", nil
			}
		} else {
			feedErr = err
		}
	}
	if src.Repo == "" {
		if feedErr == nil {
			feedErr = errors.New("no update source")
		}
		return nil, "", feedErr
	}
	list, err := f.githubReleases(ctx, component, src.Repo)
	if err != nil {
		if feedErr != nil {
			return nil, "", fmt.Errorf("feed: %v; GitHub: %w", feedErr, err)
		}
		return nil, "", err
	}
	return clean(component, channel, list), "github", nil
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Body       string    `json:"body"`
	Published  time.Time `json:"published_at"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (f *Fetcher) githubReleases(ctx context.Context, component, repo string) ([]Release, error) {
	api := f.GitHubAPI
	if api == "" {
		api = "https://api.github.com"
	}
	var list []ghRelease
	if err := f.getJSON(ctx, strings.TrimRight(api, "/")+"/repos/"+repo+"/releases?per_page=30", &list); err != nil {
		return nil, err
	}
	var out []Release
	for _, gr := range list {
		if gr.Draft || len(out) >= 15 {
			continue
		}
		for _, a := range gr.Assets {
			if a.Name != "release.json" {
				continue
			}
			var r Release
			if err := f.getJSON(ctx, a.URL, &r); err != nil {
				continue
			}
			if r.Notes == "" {
				r.Notes = gr.Body
			}
			if r.PublishedAt.IsZero() {
				r.PublishedAt = gr.Published
			}
			if r.Channel == "" {
				r.Channel = Stable
				if gr.Prerelease {
					r.Channel = Beta
				}
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// clean drops invalid entries and releases of other components or channels and sorts newest first.
func clean(component, channel string, in []Release) []Release {
	seen := map[string]bool{}
	var out []Release
	for _, r := range in {
		v, ok := ParseSemver(r.Version)
		if !ok || seen[r.Version] || (r.Component != "" && r.Component != component) || r.Image == "" {
			continue
		}
		if r.Component == "" {
			r.Component = component
		}
		if r.Channel == "" {
			r.Channel = Stable
			if v.Pre != "" {
				r.Channel = Beta
			}
		}
		if channel == Stable && (r.Channel != Stable || v.Pre != "") {
			continue
		}
		seen[r.Version] = true
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[j].Semver().Less(out[i].Semver()) })
	return out
}
