// Package alerts evaluates the built-in alert rules and notifies channels (ntfy, Telegram, email, webhook)
// when an incident starts and when it clears (PLAN.md §7 "Alerts").
package alerts

import (
	"fmt"
	"slices"
)

// Rule kinds.
const (
	AppDown      = "app_down"
	AddonDown    = "addon_down"
	AppCrashLoop = "app_crashloop"
	AppOOM       = "app_oom"
	NodeNotReady = "node_not_ready"
	NodeCPU      = "node_cpu"
	NodeMemory   = "node_memory"
	NodeDisk     = "node_disk"
	VolumeFull   = "volume_full"
	CertExpiry   = "cert_expiry"
	GatewayDown  = "gateway_down"
	BackupFailed = "backup_failed"
	UpdateAvail  = "update_available"
	UpdateFailed = "update_failed"
	// events (one notification each, no incident)
	DeployOK     = "deploy_succeeded"
	DeployFailed = "deploy_failed"
)

// Severities.
const (
	Warning  = "warning"
	Critical = "critical"
)

// RuleDef describes a built-in rule.
type RuleDef struct {
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Severity    string  `json:"severity"`
	Unit        string  `json:"unit,omitempty"` // threshold unit: "%" or "days"; "" = no threshold
	Threshold   float64 `json:"default_threshold,omitempty"`
	For         int     `json:"default_for"` // seconds the condition must hold before it fires
	// Event rules notify once per occurrence (e.g. a finished deploy) instead of opening an incident.
	Event bool `json:"event,omitempty"`
	// Off rules start disabled.
	Off bool `json:"default_off,omitempty"`
}

// Catalog lists the built-in rules in display order.
var Catalog = []RuleDef{
	{AppDown, "App is down", "A web app or worker has no ready replica although it should run.", Critical, "", 0, 120, false, false},
	{AddonDown, "Database is down", "A database add-on is not ready although it should run.", Critical, "", 0, 120, false, false},
	{AppCrashLoop, "Crash loop", "A container keeps crashing and Kubernetes backs off restarting it.", Critical, "", 0, 0, false, false},
	{AppOOM, "Out of memory", "A container was killed because it used more memory than its limit.", Warning, "", 0, 0, false, false},
	{NodeNotReady, "Node not ready", "A server node stopped reporting as ready.", Critical, "", 0, 60, false, false},
	{NodeCPU, "High CPU", "CPU usage of a node is above the threshold.", Warning, "%", 90, 600, false, false},
	{NodeMemory, "High memory", "Memory usage of a node is above the threshold.", Warning, "%", 90, 600, false, false},
	{NodeDisk, "Disk almost full", "Disk usage of a node is above the threshold.", Critical, "%", 85, 0, false, false},
	{VolumeFull, "Volume almost full", "An app volume or a database uses more than the threshold of its size. On local-path storage the size is not enforced and the node's disk fills up instead.", Warning, "%", 90, 300, false, false},
	{CertExpiry, "Certificate expiring", "A TLS certificate expires within the threshold and has not been renewed.", Warning, "days", 14, 0, false, false},
	{BackupFailed, "Backup failed", "The last backup of an app, database or the panel did not succeed.", Critical, "", 0, 0, false, false},
	{UpdateAvail, "Update available", "A new version of the panel, the gateway or k3s waits to be installed by hand.", Warning, "", 0, 0, false, false},
	{UpdateFailed, "Update failed", "The last update failed its checks and was rolled back, or the rollback needs attention.", Critical, "", 0, 0, false, false},
	{Kind: DeployFailed, Title: "Deploy failed", Description: "A deploy did not become healthy (the previous release keeps serving).", Severity: Critical, Event: true},
	{Kind: DeployOK, Title: "Deploy finished", Description: "A new release of an app is live.", Severity: Warning, Event: true, Off: true},
	{GatewayDown, "Gateway unreachable", "The panel cannot reach nectar-gateway, so domains may not be served.", Critical, "", 0, 120, false, false},
}

// Def returns the definition of a rule kind.
func Def(kind string) (RuleDef, bool) {
	i := slices.IndexFunc(Catalog, func(d RuleDef) bool { return d.Kind == kind })
	if i < 0 {
		return RuleDef{}, false
	}
	return Catalog[i], true
}

// Rule is the configuration of one built-in rule.
type Rule struct {
	Kind      string  `json:"kind"`
	Enabled   bool    `json:"enabled"`
	Threshold float64 `json:"threshold,omitempty"`
	For       int     `json:"for"` // seconds
}

// Settings is the stored alert configuration (settings key "alerts").
type Settings struct {
	Rules []Rule `json:"rules"`
	// PanelURL is put into notifications as a link (e.g. https://panel.example.com); empty = no link.
	PanelURL string `json:"panel_url"`
	// RepeatHours re-sends still-firing alerts after this many hours; 0 = never.
	RepeatHours int `json:"repeat_hours"`
}

// SettingsKey is the key of Settings in the settings table.
const SettingsKey = "alerts"

// Normalize fills in every catalog rule (enabled with defaults when missing) and drops unknown kinds.
func (s Settings) Normalize() Settings {
	byKind := map[string]Rule{}
	for _, r := range s.Rules {
		byKind[r.Kind] = r
	}
	out := Settings{PanelURL: s.PanelURL, RepeatHours: s.RepeatHours, Rules: make([]Rule, 0, len(Catalog))}
	for _, d := range Catalog {
		r, ok := byKind[d.Kind]
		if !ok {
			r = Rule{Kind: d.Kind, Enabled: !d.Off, Threshold: d.Threshold, For: d.For}
		}
		if d.Unit == "" {
			r.Threshold = 0
		} else if r.Threshold <= 0 {
			r.Threshold = d.Threshold
		}
		if r.For < 0 {
			r.For = 0
		}
		out.Rules = append(out.Rules, r)
	}
	if out.RepeatHours < 0 {
		out.RepeatHours = 0
	}
	return out
}

// Validate checks user input.
func (s Settings) Validate() error {
	for _, r := range s.Rules {
		d, ok := Def(r.Kind)
		if !ok {
			return fmt.Errorf("unknown rule %q", r.Kind)
		}
		if d.Unit == "%" && (r.Threshold < 1 || r.Threshold > 100) {
			return fmt.Errorf("%s: threshold must be between 1 and 100 %%", r.Kind)
		}
		if d.Unit == "days" && (r.Threshold < 1 || r.Threshold > 90) {
			return fmt.Errorf("%s: threshold must be between 1 and 90 days", r.Kind)
		}
		if r.For < 0 || r.For > 86400 {
			return fmt.Errorf("%s: duration must be between 0 and 86400 seconds", r.Kind)
		}
	}
	if s.RepeatHours < 0 || s.RepeatHours > 168 {
		return fmt.Errorf("repeat_hours must be between 0 and 168")
	}
	return nil
}

// Rule returns the configuration of a kind (after Normalize).
func (s Settings) Rule(kind string) Rule {
	for _, r := range s.Rules {
		if r.Kind == kind {
			return r
		}
	}
	return Rule{Kind: kind}
}
