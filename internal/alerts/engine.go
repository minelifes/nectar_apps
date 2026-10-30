package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// Kube is the part of the Kubernetes client the engine needs.
type Kube interface {
	ListNodes(ctx context.Context) ([]kube.Node, error)
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
}

// Gateway is the part of the gateway client the engine needs.
type Gateway interface {
	Version(ctx context.Context) (gateway.Version, error)
	ListManaged(ctx context.Context) ([]gateway.Route, error)
}

// CertificateLister is implemented by gateways with standalone (wildcard) certificates.
type CertificateLister interface {
	ListCertificates(ctx context.Context) ([]gateway.Certificate, error)
}

const labelApp = "nectar.dev/app"

// Engine evaluates the rules every Interval, records incidents and notifies the enabled channels.
type Engine struct {
	Store    *store.Store
	Kube     Kube    // nil without Kubernetes
	Gateway  Gateway // nil without a gateway
	Box      *secretbox.Box
	Sender   *Sender
	Log      *slog.Logger
	Interval time.Duration // default 30 s
	Now      func() time.Time
	// PanelSMTP returns the panel's SMTP server for email channels that use it.
	PanelSMTP func(ctx context.Context) (ChannelConfig, error)
	// Volumes returns the usage of every volume (nil = no volume_full checks).
	Volumes func(ctx context.Context) ([]VolumeUsage, error)

	mu       sync.Mutex
	pending  map[string]time.Time // key → since when the condition holds
	lastSent map[int64]time.Time  // alert id → last notification (for reminders)
}

// cond is a rule condition that currently holds.
type cond struct {
	Rule     string
	Key      string
	Subject  string
	Severity string
	Message  string
	AppID    *int64
	Path     string // panel path for the link, e.g. /apps/12
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// LoadSettings returns the alert settings with every rule filled in.
func LoadSettings(ctx context.Context, st *store.Store) (Settings, error) {
	var s Settings
	if err := st.GetSetting(ctx, SettingsKey, &s); err != nil && !errors.Is(err, store.ErrNotFound) {
		return Settings{}.Normalize(), err
	}
	return s.Normalize(), nil
}

// Run evaluates until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	if e.Interval <= 0 {
		e.Interval = 30 * time.Second
	}
	t := time.NewTicker(e.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := e.Evaluate(ctx); err != nil && ctx.Err() == nil {
			e.Log.Warn("alerts: evaluate", "err", err)
		}
	}
}

// Evaluate runs one round: checks every enabled rule, opens incidents whose condition held long enough and
// resolves incidents whose condition cleared.
func (e *Engine) Evaluate(ctx context.Context) error {
	set, err := LoadSettings(ctx, e.Store)
	if err != nil {
		return err
	}
	now := e.now()
	conds, checked := e.check(ctx, set, now)

	e.mu.Lock()
	if e.pending == nil {
		e.pending, e.lastSent = map[string]time.Time{}, map[int64]time.Time{}
	}
	active := map[string]cond{}
	var due []cond
	for _, c := range conds {
		active[c.Key] = c
		since, ok := e.pending[c.Key]
		if !ok {
			since = now
			e.pending[c.Key] = now
		}
		if now.Sub(since) >= time.Duration(set.Rule(c.Rule).For)*time.Second {
			due = append(due, c)
		}
	}
	for k := range e.pending {
		if _, ok := active[k]; !ok {
			delete(e.pending, k)
		}
	}
	e.mu.Unlock()

	open, err := e.Store.FiringAlerts(ctx)
	if err != nil {
		return err
	}
	openByKey := map[string]store.Alert{}
	for _, a := range open {
		openByKey[a.Key] = a
	}

	for _, c := range due {
		if a, ok := openByKey[c.Key]; ok {
			if a.Message != c.Message {
				_ = e.Store.UpdateAlertMessage(ctx, a.ID, c.Message)
				a.Message = c.Message
			}
			e.remind(ctx, set, a, c, now)
			continue
		}
		a, err := e.Store.OpenAlert(ctx, store.Alert{Rule: c.Rule, Key: c.Key, Subject: c.Subject, Severity: c.Severity,
			Message: c.Message, AppID: c.AppID, StartedAt: now})
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		e.Log.Info("alert firing", "rule", c.Rule, "subject", c.Subject, "message", c.Message)
		n := e.notifyAll(ctx, set, Notification{Title: title(c.Rule, c.Subject, ""), Message: c.Message, Severity: c.Severity,
			State: store.AlertFiring, Rule: c.Rule, Subject: c.Subject, Link: link(set, c.Path), Time: now})
		_ = e.Store.SetAlertNotified(ctx, a.ID, n)
		e.mu.Lock()
		e.lastSent[a.ID] = now
		e.mu.Unlock()
	}

	for _, a := range open {
		rule := set.Rule(a.Rule)
		_, still := active[a.Key]
		switch {
		case !rule.Enabled: // the rule was turned off: close quietly
		case !checked[a.Rule] || still:
			continue // unknown this round (source failed) or still a problem
		}
		if err := e.Store.ResolveAlert(ctx, a.ID, now); err != nil {
			return err
		}
		e.mu.Lock()
		delete(e.lastSent, a.ID)
		e.mu.Unlock()
		if !rule.Enabled {
			continue
		}
		e.Log.Info("alert resolved", "rule", a.Rule, "subject", a.Subject)
		path := ""
		if a.AppID != nil {
			path = "/apps/" + strconv.FormatInt(*a.AppID, 10)
		}
		e.notifyAll(ctx, set, Notification{Title: title(a.Rule, a.Subject, "Resolved: "),
			Message:  "Back to normal after " + humanDuration(now.Sub(a.StartedAt)) + ". Was: " + a.Message,
			Severity: a.Severity, State: store.AlertResolved, Rule: a.Rule, Subject: a.Subject, Link: link(set, path), Time: now})
	}
	return nil
}

// remind re-sends a still-firing incident every RepeatHours.
func (e *Engine) remind(ctx context.Context, set Settings, a store.Alert, c cond, now time.Time) {
	if set.RepeatHours <= 0 {
		return
	}
	e.mu.Lock()
	last, ok := e.lastSent[a.ID]
	if !ok {
		last = a.StartedAt
	}
	if now.Sub(last) < time.Duration(set.RepeatHours)*time.Hour {
		e.mu.Unlock()
		return
	}
	e.lastSent[a.ID] = now
	e.mu.Unlock()
	e.notifyAll(ctx, set, Notification{Title: title(c.Rule, c.Subject, "Still firing: "),
		Message: c.Message + " (for " + humanDuration(now.Sub(a.StartedAt)) + ")", Severity: c.Severity,
		State: store.AlertFiring, Rule: c.Rule, Subject: c.Subject, Link: link(set, c.Path), Time: now})
}

func title(rule, subject, prefix string) string {
	d, _ := Def(rule)
	return prefix + d.Title + " · " + subject
}

func link(set Settings, path string) string {
	if set.PanelURL == "" {
		return ""
	}
	return strings.TrimRight(set.PanelURL, "/") + path
}

// notifyAll sends n to every enabled channel and returns how many accepted it.
func (e *Engine) notifyAll(ctx context.Context, _ Settings, n Notification) int {
	chans, err := e.Store.ListAlertChannels(ctx)
	if err != nil {
		e.Log.Warn("alerts: channels", "err", err)
		return 0
	}
	ok := 0
	for _, ch := range chans {
		if !ch.Enabled {
			continue
		}
		cfg, err := OpenConfig(e.Box, ch.Config)
		if err != nil {
			e.Log.Warn("alerts: channel config", "channel", ch.Name, "err", err)
			continue
		}
		if ch.Kind == Email && cfg.UsePanelSMTP {
			if cfg, err = e.panelSMTP(ctx, cfg); err != nil {
				e.Log.Warn("alerts: panel SMTP", "channel", ch.Name, "err", err)
				continue
			}
		}
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = e.sender().Send(sctx, ch.Kind, cfg, n)
		cancel()
		if err != nil {
			e.Log.Warn("alerts: send", "channel", ch.Name, "kind", ch.Kind, "err", err)
			continue
		}
		ok++
	}
	return ok
}

// updateState is the part of the updater's last check the rules read (settings row updates_state).
type updateState struct {
	Components map[string]*struct {
		Installed struct {
			Version string `json:"version"`
		} `json:"installed"`
		Available []struct {
			Version string `json:"version"`
		} `json:"available"`
		Auto *struct {
			Version string `json:"version"`
		} `json:"auto"`
	} `json:"components"`
}

// Announce sends a one-off message (e.g. a finished update) to every enabled channel.
func (e *Engine) Announce(ctx context.Context, title, message, path string, ok bool) {
	set, _ := LoadSettings(ctx, e.Store)
	n := Notification{Title: title, Message: message, Severity: Warning, State: "resolved", Time: e.now(), Link: link(set, path)}
	if !ok {
		n.Severity, n.State = Critical, "firing"
	}
	e.notifyAll(ctx, set, n)
}

// Event notifies an event rule (e.g. deploy_failed) once, when the rule is enabled.
func (e *Engine) Event(ctx context.Context, kind, subject, message, path string) {
	d, ok := Def(kind)
	if !ok || !d.Event {
		return
	}
	set, err := LoadSettings(ctx, e.Store)
	if err != nil || !set.Rule(kind).Enabled {
		return
	}
	n := Notification{Title: d.Title + " · " + subject, Message: message, Severity: d.Severity, State: "firing", Rule: kind,
		Subject: subject, Time: e.now(), Link: link(set, path)}
	if kind == DeployOK {
		n.State = "resolved"
	}
	e.notifyAll(ctx, set, n)
}

// panelSMTP fills in the panel's SMTP server, keeping the channel's recipients.
func (e *Engine) panelSMTP(ctx context.Context, c ChannelConfig) (ChannelConfig, error) {
	return ResolvePanelSMTP(ctx, e.PanelSMTP, c)
}

// ResolvePanelSMTP replaces the server settings of an email channel that uses the panel's SMTP server.
func ResolvePanelSMTP(ctx context.Context, smtp func(context.Context) (ChannelConfig, error), c ChannelConfig) (ChannelConfig, error) {
	if smtp == nil {
		return c, errors.New("no panel SMTP server")
	}
	base, err := smtp(ctx)
	if err != nil {
		return c, err
	}
	if base.Host == "" {
		return c, errors.New("no SMTP server is configured (Settings → Email)")
	}
	base.To, base.UsePanelSMTP = c.To, false
	return base, nil
}

func (e *Engine) sender() *Sender {
	if e.Sender == nil {
		e.Sender = &Sender{}
	}
	return e.Sender
}

// SealConfig encrypts a channel config for storage.
func SealConfig(box *secretbox.Box, c ChannelConfig) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return box.Seal(string(b))
}

// OpenConfig decrypts a stored channel config.
func OpenConfig(box *secretbox.Box, sealed string) (ChannelConfig, error) {
	var c ChannelConfig
	pt, err := box.Open(sealed)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(pt), &c)
	return c, err
}

// ------------------------------------------------------------------- checks --

// check returns the conditions that hold now and which rule kinds could be evaluated (a failed source
// must not resolve its incidents).
func (e *Engine) check(ctx context.Context, set Settings, now time.Time) ([]cond, map[string]bool) {
	var out []cond
	checked := map[string]bool{}
	on := func(kind string) bool { return set.Rule(kind).Enabled }

	if e.Kube != nil && (on(NodeNotReady) || on(NodeCPU) || on(NodeMemory) || on(NodeDisk)) {
		if cs, err := e.checkNodes(ctx, set, now); err == nil {
			out = append(out, cs...)
			for _, k := range []string{NodeNotReady, NodeCPU, NodeMemory, NodeDisk} {
				checked[k] = true
			}
		} else {
			e.Log.Debug("alerts: nodes", "err", err)
		}
	}
	if e.Kube != nil && (on(AppDown) || on(AddonDown) || on(AppCrashLoop) || on(AppOOM)) {
		if cs, err := e.checkApps(ctx, set, now); err == nil {
			out = append(out, cs...)
			for _, k := range []string{AppDown, AddonDown, AppCrashLoop, AppOOM} {
				checked[k] = true
			}
		} else {
			e.Log.Debug("alerts: apps", "err", err)
		}
	}
	if e.Gateway != nil {
		vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		gv, verr := e.Gateway.Version(vctx)
		cancel()
		checked[GatewayDown] = true
		if verr != nil && on(GatewayDown) {
			out = append(out, cond{Rule: GatewayDown, Key: GatewayDown, Subject: "nectar-gateway", Severity: Critical,
				Message: "The gateway does not answer: " + verr.Error(), Path: "/settings"})
		}
		if verr == nil && on(CertExpiry) {
			lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			routes, err := e.Gateway.ListManaged(lctx)
			cancel()
			if err == nil {
				checked[CertExpiry] = true
				out = append(out, certConds(routes, set.Rule(CertExpiry).Threshold, now)...)
			}
			if cl, ok := e.Gateway.(CertificateLister); ok && gv.Has(gateway.FeatureCertificates) {
				lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				certs, err := cl.ListCertificates(lctx)
				cancel()
				if err == nil {
					out = append(out, wildcardConds(certs, set.Rule(CertExpiry).Threshold, now)...)
				} else {
					delete(checked, CertExpiry) // keep open incidents rather than resolving them blind
				}
			}
		}
	}
	if on(VolumeFull) && e.Volumes != nil {
		if vols, err := e.Volumes(ctx); err == nil {
			checked[VolumeFull] = true
			out = append(out, volumeConds(vols, set.Rule(VolumeFull).Threshold)...)
		}
	}
	if on(BackupFailed) {
		if last, err := e.Store.LastBackupRuns(ctx); err == nil {
			checked[BackupFailed] = true
			for plan, r := range last {
				if r.Status != store.RunFailed {
					continue
				}
				msg := r.Message
				if i := strings.IndexByte(msg, '\n'); i > 0 {
					msg = msg[:i]
				}
				out = append(out, cond{Rule: BackupFailed, Key: BackupFailed + ":" + strconv.FormatInt(plan, 10), Subject: r.Subject, Severity: Critical,
					Path: "/backups", Message: "The backup of " + r.Subject + " failed: " + msg})
			}
		}
	}
	if on(UpdateAvail) {
		var st updateState
		if err := e.Store.GetSetting(ctx, "updates_state", &st); err == nil {
			checked[UpdateAvail] = true
			for _, comp := range []string{"panel", "gateway", "k3s"} {
				cs := st.Components[comp]
				if cs == nil || len(cs.Available) == 0 || cs.Auto != nil {
					continue
				}
				v := cs.Available[0].Version
				name := map[string]string{"panel": "Nectar Apps panel", "gateway": "nectar-gateway", "k3s": "k3s"}[comp]
				out = append(out, cond{Rule: UpdateAvail, Key: UpdateAvail + ":" + comp + ":" + v, Subject: name, Severity: Warning, Path: "/updates",
					Message: name + " " + v + " is available (installed: " + cs.Installed.Version + ")."})
			}
		} else if errors.Is(err, store.ErrNotFound) {
			checked[UpdateAvail] = true
		}
	}
	if on(UpdateFailed) {
		if runs, err := e.Store.ListUpdateRuns(ctx, 10); err == nil {
			checked[UpdateFailed] = true
			for _, r := range runs {
				if r.FinishedAt == nil {
					continue
				}
				if r.State == "rolled_back" || r.State == "rollback_failed" {
					sev, what := Critical, "was rolled back"
					if r.State == "rollback_failed" {
						what = "failed and the rollback needs attention"
					}
					msg := r.Error
					if i := strings.IndexByte(msg, '\n'); i > 0 {
						msg = msg[:i]
					}
					out = append(out, cond{Rule: UpdateFailed, Key: UpdateFailed + ":" + strconv.FormatInt(r.ID, 10), Subject: r.Summary, Severity: sev,
						Path: "/updates", Message: "The update " + r.Summary + " " + what + ": " + msg})
				}
				break // only the newest finished run counts
			}
		}
	}
	// disabled rules never produce conditions
	kept := out[:0]
	for _, c := range out {
		if on(c.Rule) {
			kept = append(kept, c)
		}
	}
	return kept, checked
}

func (e *Engine) checkNodes(ctx context.Context, set Settings, now time.Time) ([]cond, error) {
	nodes, err := e.Kube.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	var out []cond
	for _, n := range nodes {
		path := "/server"
		if !n.Ready {
			out = append(out, cond{Rule: NodeNotReady, Key: NodeNotReady + ":" + n.Name, Subject: n.Name, Severity: Critical,
				Message: "Node " + n.Name + " is not ready: apps on it may be unavailable.", Path: path})
			continue
		}
		if u := n.Usage; u != nil {
			if a := n.Allocatable.CPUMillicores; a > 0 {
				if pct := float64(u.CPUMillicores) * 100 / float64(a); pct >= set.Rule(NodeCPU).Threshold {
					out = append(out, cond{Rule: NodeCPU, Key: NodeCPU + ":" + n.Name, Subject: n.Name, Severity: Warning,
						Message: fmt.Sprintf("CPU at %.0f%% of %s cores (threshold %.0f%%).", pct, trimFloat(float64(a)/1000), set.Rule(NodeCPU).Threshold), Path: path})
				}
			}
			if a := n.Allocatable.MemoryBytes; a > 0 {
				if pct := float64(u.MemoryBytes) * 100 / float64(a); pct >= set.Rule(NodeMemory).Threshold {
					out = append(out, cond{Rule: NodeMemory, Key: NodeMemory + ":" + n.Name, Subject: n.Name, Severity: Warning,
						Message: fmt.Sprintf("Memory at %.0f%%: %s of %s (threshold %.0f%%).", pct, bytesHuman(u.MemoryBytes), bytesHuman(a), set.Rule(NodeMemory).Threshold), Path: path})
				}
			}
		}
		pts, err := e.Store.RecentMetrics(ctx, "node:"+n.Name, now.Add(-2*time.Minute))
		if err == nil && len(pts) > 0 {
			p := pts[len(pts)-1]
			if p.DiskTotal > 0 {
				if pct := float64(p.DiskUsed) * 100 / float64(p.DiskTotal); pct >= set.Rule(NodeDisk).Threshold {
					out = append(out, cond{Rule: NodeDisk, Key: NodeDisk + ":" + n.Name, Subject: n.Name, Severity: Critical,
						Message: fmt.Sprintf("Disk at %.0f%%: %s free of %s (threshold %.0f%%). Clean up unused images and old releases on the Server page.",
							pct, bytesHuman(p.DiskTotal-p.DiskUsed), bytesHuman(p.DiskTotal), set.Rule(NodeDisk).Threshold), Path: path})
				}
			}
		}
	}
	return out, nil
}

func (e *Engine) checkApps(ctx context.Context, set Settings, now time.Time) ([]cond, error) {
	projects, err := e.Store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	var out []cond
	for _, p := range projects {
		apps, err := e.Store.ListApps(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if on := set.Rule(AddonDown).Enabled; on {
			cs, err := e.checkAddons(ctx, p)
			if err != nil {
				return nil, err
			}
			out = append(out, cs...)
		}
		if len(apps) == 0 {
			continue
		}
		bySlug := map[string]store.App{}
		for _, a := range apps {
			bySlug[a.Slug] = a
		}
		deps, err := e.Kube.List(ctx, "Deployment", p.Namespace, labelApp)
		if err != nil && !kube.IsNotFound(err) {
			return nil, err
		}
		pods, err := e.Kube.List(ctx, "Pod", p.Namespace, labelApp)
		if err != nil && !kube.IsNotFound(err) {
			return nil, err
		}
		for _, d := range deps {
			a, ok := bySlug[kube.Str(d, "metadata", "labels", labelApp)]
			if !ok || kube.Str(d, "metadata", "name") != a.Slug || a.Status == "deploying" {
				continue
			}
			want, ready := kube.Int(d, "spec", "replicas"), kube.Int(d, "status", "readyReplicas")
			if want > 0 && ready == 0 {
				id := a.ID
				out = append(out, cond{Rule: AppDown, Key: AppDown + ":" + strconv.FormatInt(a.ID, 10), Subject: p.Slug + "/" + a.Slug,
					Severity: Critical, AppID: &id, Path: "/apps/" + strconv.FormatInt(a.ID, 10),
					Message: fmt.Sprintf("%s has 0 of %d replicas ready.", a.Name, want)})
			}
		}
		crash := map[int64][]string{}
		oom := map[int64][]string{}
		for _, pod := range pods {
			a, ok := bySlug[kube.Str(pod, "metadata", "labels", labelApp)]
			if !ok {
				continue
			}
			name := kube.Str(pod, "metadata", "name")
			limits := map[string]string{}
			for _, c := range kube.Slice(pod, "spec", "containers") {
				limits[kube.Str(c, "name")] = kube.Str(c, "resources", "limits", "memory")
			}
			for _, cs := range kube.Slice(pod, "status", "containerStatuses") {
				cname := kube.Str(cs, "name")
				if kube.Str(cs, "state", "waiting", "reason") == "CrashLoopBackOff" {
					msg := fmt.Sprintf("%s restarted %d times", name, kube.Int(cs, "restartCount"))
					if r := kube.Str(cs, "lastState", "terminated", "reason"); r != "" {
						msg += fmt.Sprintf(" (last exit: %s, code %d)", r, kube.Int(cs, "lastState", "terminated", "exitCode"))
					}
					crash[a.ID] = append(crash[a.ID], msg)
				}
				if kube.Str(cs, "lastState", "terminated", "reason") == "OOMKilled" {
					fin, err := time.Parse(time.RFC3339, kube.Str(cs, "lastState", "terminated", "finishedAt"))
					if err == nil && now.Sub(fin) <= 15*time.Minute {
						msg := name + " was killed for running out of memory"
						if l := limits[cname]; l != "" {
							if q, err := kube.ParseQuantity(l); err == nil {
								msg += " (limit " + bytesHuman(int64(q)) + ")"
							}
						}
						oom[a.ID] = append(oom[a.ID], msg)
					}
				}
			}
		}
		for _, a := range apps {
			id := a.ID
			sid := strconv.FormatInt(a.ID, 10)
			if m := crash[a.ID]; len(m) > 0 {
				out = append(out, cond{Rule: AppCrashLoop, Key: AppCrashLoop + ":" + sid, Subject: p.Slug + "/" + a.Slug, Severity: Critical,
					AppID: &id, Path: "/apps/" + sid, Message: "Crash loop: " + strings.Join(m, "; ") + ". Open the app's crash log to see why."})
			}
			if m := oom[a.ID]; len(m) > 0 {
				out = append(out, cond{Rule: AppOOM, Key: AppOOM + ":" + sid, Subject: p.Slug + "/" + a.Slug, Severity: Warning,
					AppID: &id, Path: "/apps/" + sid, Message: strings.Join(m, "; ") + ". Give it more memory in the app's resources."})
			}
		}
	}
	return out, nil
}

// checkAddons reports database add-ons whose StatefulSet should run but has no ready replica.
func (e *Engine) checkAddons(ctx context.Context, p store.Project) ([]cond, error) {
	addons, err := e.Store.ListAddons(ctx, p.ID)
	if err != nil || len(addons) == 0 {
		return nil, err
	}
	sets, err := e.Kube.List(ctx, "StatefulSet", p.Namespace, "nectar.dev/addon")
	if err != nil && !kube.IsNotFound(err) {
		return nil, err
	}
	ready := map[string]bool{}
	for _, s := range sets {
		ready[kube.Str(s, "metadata", "name")] = kube.Int(s, "spec", "replicas") == 0 || kube.Int(s, "status", "readyReplicas") > 0
	}
	var out []cond
	for _, a := range addons {
		if a.Stopped || ready[a.Slug] {
			continue
		}
		if _, exists := ready[a.Slug]; !exists {
			continue // not created (yet)
		}
		out = append(out, cond{Rule: AddonDown, Key: AddonDown + ":" + strconv.FormatInt(a.ID, 10), Subject: p.Slug + "/" + a.Slug, Severity: Critical,
			Path: "/addons/" + strconv.FormatInt(a.ID, 10), Message: a.Name + " is not ready: apps using it cannot reach their data."})
	}
	return out, nil
}

// wildcardConds: standalone certificates (e.g. *.<base domain>) that expire soon or could not be issued.
// Hosts they serve have no certificate of their own, so certConds does not see them.
func wildcardConds(certs []gateway.Certificate, days float64, now time.Time) []cond {
	var out []cond
	for _, c := range certs {
		key := CertExpiry + ":" + c.Domain
		if c.NotAfter <= 0 {
			if c.LastError != "" && c.LastAttempt > 0 && now.Sub(time.Unix(c.LastAttempt, 0)) < 48*time.Hour {
				out = append(out, cond{Rule: CertExpiry, Key: key, Subject: c.Domain, Severity: Warning,
					Message: "The certificate " + c.Domain + " could not be issued: " + c.LastError, Path: "/gateway"})
			}
			continue
		}
		left := time.Unix(c.NotAfter, 0).Sub(now)
		if left.Hours()/24 >= days {
			continue
		}
		sev := Warning
		if left < 3*24*time.Hour {
			sev = Critical
		}
		msg := "The certificate " + c.Domain + " expires in " + humanDuration(left) + " (" + time.Unix(c.NotAfter, 0).UTC().Format("2006-01-02") + ")."
		if left <= 0 {
			msg = "The certificate " + c.Domain + " expired " + humanDuration(-left) + " ago."
		}
		if len(c.Serves) > 0 {
			msg += fmt.Sprintf(" It serves %d host(s).", len(c.Serves))
		}
		if c.ACME {
			msg += " Automatic renewal has not succeeded yet"
			if c.LastError != "" {
				msg += ": " + c.LastError
			}
			msg += "."
		}
		out = append(out, cond{Rule: CertExpiry, Key: key, Subject: c.Domain, Severity: sev, Message: msg, Path: "/gateway"})
	}
	return out
}

func certConds(routes []gateway.Route, days float64, now time.Time) []cond {
	var out []cond
	seen := map[string]bool{}
	for _, r := range routes {
		if r.CertNotAfter <= 0 || seen[r.Domain] {
			continue
		}
		seen[r.Domain] = true
		left := time.Unix(r.CertNotAfter, 0).Sub(now)
		if left.Hours()/24 >= days {
			continue
		}
		sev, msg := Warning, "The certificate of "+r.Domain+" expires in "+humanDuration(left)+" ("+time.Unix(r.CertNotAfter, 0).UTC().Format("2006-01-02")+")."
		if left < 3*24*time.Hour {
			sev = Critical
		}
		if left <= 0 {
			msg = "The certificate of " + r.Domain + " expired " + humanDuration(-left) + " ago."
		}
		if r.ACME {
			msg += " Automatic renewal has not succeeded yet: check that the domain still points to this server."
		} else {
			msg += " It was uploaded manually: upload a renewed certificate."
		}
		out = append(out, cond{Rule: CertExpiry, Key: CertExpiry + ":" + r.Domain, Subject: r.Domain, Severity: sev, Message: msg, Path: "/settings"})
	}
	return out
}

// ------------------------------------------------------------------ helpers --

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(math.Max(1, math.Round(d.Seconds())))) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) % 60; m > 0 && h < 10 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return strconv.Itoa(h) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + " days"
}

func bytesHuman(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return trimFloat(float64(b)/float64(div)) + " " + string("KMGTP"[exp]) + "iB"
}

func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

// VolumeUsage is one volume for the volume_full rule.
type VolumeUsage struct {
	Key     string // stable id, e.g. prj-shop/web-data
	Subject string // "shop/web: data" or "shop/db (database)"
	Path    string // panel link
	UsedB   int64  // -1 = unknown
	SizeB   int64  // the size in the settings
}

func volumeConds(vols []VolumeUsage, threshold float64) []cond {
	var out []cond
	for _, v := range vols {
		if v.UsedB < 0 || v.SizeB <= 0 {
			continue // unknown while no pod mounts it
		}
		pct := float64(v.UsedB) * 100 / float64(v.SizeB)
		if pct < threshold {
			continue
		}
		out = append(out, cond{Rule: VolumeFull, Key: VolumeFull + ":" + v.Key, Subject: v.Subject, Severity: Warning, Path: v.Path,
			Message: fmt.Sprintf("Volume %s uses %s of %s (%.0f%%, threshold %.0f%%). Grow it in the app settings or clean up files (Volumes → Files).",
				v.Subject, bytesHuman(v.UsedB), bytesHuman(v.SizeB), pct, threshold)})
	}
	return out
}
