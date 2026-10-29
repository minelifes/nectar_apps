package update

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Policies.
const (
	PolicyOff    = "off"    // never check
	PolicyNotify = "notify" // check and notify, install by hand
	PolicyPatch  = "patch"  // install patch releases (and security fixes) automatically
	PolicyMinor  = "minor"  // install patch and minor releases automatically
	PolicyAuto   = "auto"   // k3s: follow the channel automatically
)

// SettingsKey is the settings row of the update configuration.
const SettingsKey = "updates"

// ComponentSettings configures one component.
type ComponentSettings struct {
	Policy  string `json:"policy"`
	Channel string `json:"channel"`            // stable | beta (k3s: stable | latest | v1.31 …)
	FeedURL string `json:"feed_url,omitempty"` // "" = the official feed
}

// Window is the maintenance window for automatic updates ("" start and end = any time).
type Window struct {
	Days     []string `json:"days,omitempty"` // mon … sun, empty = every day
	Start    string   `json:"start,omitempty"`
	End      string   `json:"end,omitempty"`
	TimeZone string   `json:"timezone,omitempty"` // IANA name, default UTC
}

// Settings is the update configuration (Settings → Updates).
type Settings struct {
	Panel         ComponentSettings `json:"panel"`
	Gateway       ComponentSettings `json:"gateway"`
	K3s           ComponentSettings `json:"k3s"`
	Window        Window            `json:"window"`
	IntervalHours int               `json:"interval_hours"`
}

// Defaults: notify only for panel and gateway, k3s off, check every 6 hours.
var Defaults = Settings{
	Panel:         ComponentSettings{Policy: PolicyNotify, Channel: Stable},
	Gateway:       ComponentSettings{Policy: PolicyNotify, Channel: Stable},
	K3s:           ComponentSettings{Policy: PolicyOff, Channel: Stable},
	IntervalHours: 6,
}

// Component returns the settings of a component.
func (s Settings) Component(c string) ComponentSettings {
	switch c {
	case Panel:
		return s.Panel
	case Gateway:
		return s.Gateway
	}
	return s.K3s
}

// Normalize fills defaults.
func (s Settings) Normalize() Settings {
	fill := func(c *ComponentSettings, d ComponentSettings) {
		if c.Policy == "" {
			c.Policy = d.Policy
		}
		if c.Channel == "" {
			c.Channel = d.Channel
		}
		c.FeedURL = strings.TrimSpace(c.FeedURL)
	}
	fill(&s.Panel, Defaults.Panel)
	fill(&s.Gateway, Defaults.Gateway)
	fill(&s.K3s, Defaults.K3s)
	if s.IntervalHours == 0 {
		s.IntervalHours = Defaults.IntervalHours
	}
	for i, d := range s.Window.Days {
		s.Window.Days[i] = strings.ToLower(strings.TrimSpace(d))[:min(3, len(strings.TrimSpace(d)))]
	}
	return s
}

var weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Validate checks the settings.
func (s Settings) Validate() error {
	for _, c := range []struct {
		name string
		cs   ComponentSettings
	}{{Panel, s.Panel}, {Gateway, s.Gateway}} {
		if !slices.Contains([]string{PolicyOff, PolicyNotify, PolicyPatch, PolicyMinor}, c.cs.Policy) {
			return fmt.Errorf("%s: policy must be off, notify, patch or minor", c.name)
		}
		if c.cs.Channel != Stable && c.cs.Channel != Beta {
			return fmt.Errorf("%s: channel must be stable or beta", c.name)
		}
		if c.cs.FeedURL != "" {
			if u, err := url.Parse(c.cs.FeedURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return fmt.Errorf("%s: feed URL must be an http(s) URL", c.name)
			}
		}
	}
	if !slices.Contains([]string{PolicyOff, PolicyNotify, PolicyAuto}, s.K3s.Policy) {
		return errors.New("k3s: policy must be off, notify or auto")
	}
	if !validK3sChannel(s.K3s.Channel) {
		return errors.New("k3s: channel must be stable, latest or a minor version like v1.31")
	}
	if s.IntervalHours < 1 || s.IntervalHours > 24 {
		return errors.New("the check interval must be 1–24 hours")
	}
	return s.Window.Validate()
}

func validK3sChannel(c string) bool {
	if c == "stable" || c == "latest" || c == "testing" {
		return true
	}
	p := strings.Split(strings.TrimPrefix(c, "v"), ".")
	if len(p) != 2 {
		return false
	}
	for _, x := range p {
		if _, err := strconv.Atoi(x); err != nil {
			return false
		}
	}
	return true
}

// Validate checks the window.
func (w Window) Validate() error {
	for _, d := range w.Days {
		if !slices.Contains(weekdays, d) {
			return fmt.Errorf("window: unknown day %q (mon … sun)", d)
		}
	}
	if (w.Start == "") != (w.End == "") {
		return errors.New("window: set both start and end, or neither")
	}
	if w.Start != "" {
		if _, err := clock(w.Start); err != nil {
			return err
		}
		if _, err := clock(w.End); err != nil {
			return err
		}
	}
	if w.TimeZone != "" {
		if _, err := time.LoadLocation(w.TimeZone); err != nil {
			return fmt.Errorf("window: unknown time zone %q", w.TimeZone)
		}
	}
	return nil
}

func clock(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("window: %q is not a time like 03:00", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func (w Window) loc() *time.Location {
	if w.TimeZone != "" {
		if l, err := time.LoadLocation(w.TimeZone); err == nil {
			return l
		}
	}
	return time.UTC
}

// Allows reports whether t is inside the window. A window that ends before it starts spans midnight
// (22:00–02:00); the day is the day it started.
func (w Window) Allows(t time.Time) bool {
	t = t.In(w.loc())
	day := t
	if w.Start != "" {
		start, _ := clock(w.Start)
		end, _ := clock(w.End)
		now := t.Hour()*60 + t.Minute()
		switch {
		case start == end: // whole day
		case start < end:
			if now < start || now >= end {
				return false
			}
		default:
			if now < start && now >= end {
				return false
			}
			if now < end {
				day = t.AddDate(0, 0, -1)
			}
		}
	}
	return len(w.Days) == 0 || slices.Contains(w.Days, weekdays[day.Weekday()])
}

// Next returns the next moment at or after t inside the window (t itself when it is inside); zero when the
// window never opens.
func (w Window) Next(t time.Time) time.Time {
	t = t.Truncate(time.Minute)
	for i := 0; i < 8*24*60; i += 15 {
		c := t.Add(time.Duration(i) * time.Minute)
		if i > 0 {
			c = c.Truncate(15 * time.Minute)
		}
		if w.Allows(c) {
			return c
		}
	}
	return time.Time{}
}

// AutoInstall reports whether a policy installs release r over version cur without asking. Major and
// breaking releases always need a person.
func AutoInstall(policy string, cur Semver, r Release) bool {
	bump := cur.Bump(r.Semver())
	if bump == "" || bump == "major" || r.Breaking || r.Kind == "major" {
		return false
	}
	switch policy {
	case PolicyPatch:
		return bump == "patch" || r.Security
	case PolicyMinor:
		return true
	}
	return false
}
