package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// k3s upgrades run through Rancher's system-upgrade-controller (installed with install.sh --k3s-upgrades).
const (
	SUCNamespace      = "system-upgrade"
	SUCServiceAccount = "system-upgrade"
	SUCCRD            = "plans.upgrade.cattle.io"
	K3sUpgradeImage   = "rancher/k3s-upgrade"
	K3sChannelURL     = "https://update.k3s.io/v1-release/channels/"
	serverPlan        = "nectar-k3s-server"
	agentPlan         = "nectar-k3s-agent"
)

// K3sInfo is the extra state of the k3s component.
type K3sInfo struct {
	Controller bool          `json:"controller"` // system-upgrade-controller is installed
	Nodes      []K3sNode     `json:"nodes"`
	Plans      []K3sPlanInfo `json:"plans,omitempty"`
}

// K3sNode is the k3s version of a node.
type K3sNode struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	ControlPlane  bool   `json:"control_plane"`
	Unschedulable bool   `json:"unschedulable,omitempty"`
}

// K3sPlanInfo is one of our upgrade plans.
type K3sPlanInfo struct {
	Name    string `json:"name"`
	Channel string `json:"channel,omitempty"`
	Version string `json:"version,omitempty"`
	Latest  string `json:"latest,omitempty"` // status.latestVersion
}

// K3sLatest resolves a k3s channel ("stable", "latest", "v1.31") to its version.
func (f *Fetcher) K3sLatest(ctx context.Context, channel string) (string, error) {
	c := *f.client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, K3sChannelURL+url.PathEscape(channel), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nectar-apps-updater")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("k3s channel %s: HTTP %d without a redirect", channel, resp.StatusCode)
	}
	v, err := url.PathUnescape(path.Base(loc))
	if err != nil || !strings.HasPrefix(v, "v") {
		return "", fmt.Errorf("k3s channel %s: unexpected redirect %s", channel, loc)
	}
	return v, nil
}

// k3sLess compares versions like v1.31.4+k3s1 (the k3s revision counts too).
func k3sLess(a, b string) bool {
	va, oka := ParseSemver(a)
	vb, okb := ParseSemver(b)
	if !oka || !okb {
		return false
	}
	if c := va.Compare(vb); c != 0 {
		return c < 0
	}
	return k3sRev(a) < k3sRev(b)
}

func k3sRev(v string) int {
	i := strings.Index(v, "+k3s")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(v[i+4:])
	return n
}

func (m *Manager) k3sNodes(ctx context.Context) ([]K3sNode, error) {
	list, err := m.Kube.List(ctx, "Node", "", "")
	if err != nil {
		return nil, err
	}
	var out []K3sNode
	for _, n := range list {
		_, _, name := kube.Meta(n)
		labels, _ := kube.Field(n, "metadata", "labels").(map[string]any)
		cp := labels["node-role.kubernetes.io/control-plane"] == "true" || labels["node-role.kubernetes.io/master"] == "true"
		unsched, _ := kube.Field(n, "spec", "unschedulable").(bool)
		out = append(out, K3sNode{Name: name, Version: kube.Str(n, "status", "nodeInfo", "kubeletVersion"), ControlPlane: cp, Unschedulable: unsched})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Manager) sucInstalled(ctx context.Context) bool {
	var crd kube.Object
	return m.Kube.GetObject(ctx, "CustomResourceDefinition", "", SUCCRD, &crd) == nil
}

func (m *Manager) checkK3s(ctx context.Context, set Settings, now time.Time) *ComponentState {
	cs := &ComponentState{CheckedAt: now, Source: "k3s channel " + set.K3s.Channel}
	if m.Kube == nil {
		cs.Error = "kubernetes is not configured"
		return cs
	}
	info := K3sInfo{}
	nodes, err := m.k3sNodes(ctx)
	if err != nil {
		cs.Error = err.Error()
		return cs
	}
	info.Nodes = nodes
	for _, n := range nodes {
		if cs.Installed.Version == "" || k3sLess(n.Version, cs.Installed.Version) {
			cs.Installed.Version = n.Version
		}
	}
	info.Controller = m.sucInstalled(ctx)
	if info.Controller {
		for _, name := range []string{serverPlan, agentPlan} {
			var p kube.Object
			if err := m.Kube.GetObject(ctx, "Plan", SUCNamespace, name, &p); err == nil {
				info.Plans = append(info.Plans, K3sPlanInfo{Name: name, Channel: kube.Str(p, "spec", "channel"),
					Version: kube.Str(p, "spec", "version"), Latest: kube.Str(p, "status", "latestVersion")})
			}
		}
	}
	cs.Extra = info
	if set.K3s.Policy == PolicyOff {
		cs.Waiting = "k3s updates are off"
		return cs
	}
	fctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	latest, err := m.Fetcher.K3sLatest(fctx, set.K3s.Channel)
	cancel()
	if err != nil {
		cs.Error = err.Error()
		return cs
	}
	r := Release{Component: K3s, Version: latest, Channel: set.K3s.Channel, Image: K3sUpgradeImage + ":" + strings.ReplaceAll(latest, "+", "-"),
		Notes: "https://github.com/k3s-io/k3s/releases/tag/" + url.PathEscape(latest)}
	r.Kind = mustSemver(cs.Installed.Version).Bump(mustSemver(latest))
	if r.Kind == "" {
		r.Kind = "patch" // a new k3s revision of the same Kubernetes version
	}
	cs.Latest = &r
	if k3sLess(cs.Installed.Version, latest) {
		cs.Available = []Release{r}
	}
	switch {
	case !info.Controller && len(cs.Available) > 0:
		cs.Waiting = "install system-upgrade-controller to update k3s from the panel (install.sh --k3s-upgrades)"
	case set.K3s.Policy == PolicyAuto && len(cs.Available) > 0:
		cs.Auto = &r
		cs.Waiting = "system-upgrade-controller installs it in the maintenance window"
	}
	return cs
}

func mustSemver(s string) Semver { v, _ := ParseSemver(s); return v }

var weekdayNames = map[string]string{"mon": "monday", "tue": "tuesday", "wed": "wednesday", "thu": "thursday", "fri": "friday", "sat": "saturday", "sun": "sunday"}

// SyncK3s makes our upgrade plans match the settings: removed for off/notify, following the channel for auto,
// or pinned to version ("update now", which ignores the window).
func (m *Manager) SyncK3s(ctx context.Context, set Settings, version string) error {
	if m.Kube == nil {
		return errors.New("kubernetes is not configured")
	}
	installed := m.sucInstalled(ctx)
	if version == "" && set.K3s.Policy != PolicyAuto {
		if installed {
			for _, n := range []string{serverPlan, agentPlan} {
				if err := m.Kube.Delete(ctx, "Plan", SUCNamespace, n); err != nil && !kube.IsNotFound(err) {
					return err
				}
			}
		}
		return nil
	}
	if !installed {
		return errors.New("system-upgrade-controller is not installed: re-run install.sh with --k3s-upgrades")
	}
	for _, p := range k3sPlans(set, version) {
		if _, err := m.Kube.Apply(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func k3sPlans(set Settings, version string) []kube.Object {
	spec := func(extra map[string]any) map[string]any {
		s := map[string]any{
			"concurrency":        1,
			"cordon":             true,
			"serviceAccountName": SUCServiceAccount,
			"upgrade":            map[string]any{"image": K3sUpgradeImage},
		}
		if version != "" {
			s["version"] = version
		} else {
			s["channel"] = K3sChannelURL + set.K3s.Channel
			if w := set.Window; w.Start != "" || len(w.Days) > 0 {
				win := map[string]any{"timeZone": "UTC"}
				if w.TimeZone != "" {
					win["timeZone"] = w.TimeZone
				}
				var days []any
				for _, d := range w.Days {
					days = append(days, weekdayNames[d])
				}
				if len(days) == 0 {
					for _, d := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
						days = append(days, weekdayNames[d])
					}
				}
				win["days"] = days
				start, end := w.Start, w.End
				if start == "" {
					start, end = "00:00", "23:59"
				}
				win["startTime"], win["endTime"] = start, end
				s["window"] = win
			}
		}
		for k, v := range extra {
			s[k] = v
		}
		return s
	}
	meta := func(name string) map[string]any {
		return map[string]any{"name": name, "namespace": SUCNamespace,
			"labels": map[string]any{"app.kubernetes.io/managed-by": "nectar-apps", "app.kubernetes.io/part-of": "nectar"}}
	}
	server := kube.Object{"apiVersion": "upgrade.cattle.io/v1", "kind": "Plan", "metadata": meta(serverPlan),
		"spec": spec(map[string]any{"nodeSelector": map[string]any{"matchExpressions": []any{
			map[string]any{"key": "node-role.kubernetes.io/control-plane", "operator": "In", "values": []any{"true"}}}}})}
	agent := kube.Object{"apiVersion": "upgrade.cattle.io/v1", "kind": "Plan", "metadata": meta(agentPlan),
		"spec": spec(map[string]any{
			"nodeSelector": map[string]any{"matchExpressions": []any{
				map[string]any{"key": "node-role.kubernetes.io/control-plane", "operator": "DoesNotExist"}}},
			"prepare": map[string]any{"image": K3sUpgradeImage, "args": []any{"prepare", serverPlan}},
		})}
	return []kube.Object{server, agent}
}
