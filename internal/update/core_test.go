package update

import (
	"strings"
	"testing"
	"time"
)

func TestSemver(t *testing.T) {
	order := []string{"0.9.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.2.0", "2.0.0"}
	for i := 1; i < len(order); i++ {
		a, _ := ParseSemver(order[i-1])
		b, ok := ParseSemver(order[i])
		if !ok || !a.Less(b) || b.Less(a) {
			t.Errorf("%s < %s failed", order[i-1], order[i])
		}
	}
	for _, s := range []string{"v1.2.3", "nectar-gateway-v1.2.3", "1.2.3+build.5"} {
		if v, ok := ParseSemver(s); !ok || v.String() != "1.2.3" {
			t.Errorf("parse %q = %v %v", s, v, ok)
		}
	}
	for _, s := range []string{"dev", "1.2", "1.2.x", "1.2.3-", ""} {
		if _, ok := ParseSemver(s); ok {
			t.Errorf("%q should not parse", s)
		}
	}
	v, _ := ParseSemver("1.4.2")
	cases := map[string]bool{">=1.4.0": true, ">=1.5.0": false, ">=1.0.0 <2.0.0": true, "<1.4.2": false, "^1.2": true, "^2.0.0": false,
		"~1.4.0": true, "~1.3.0": false, "1.4.x": true, "1.x": true, "1.3.x": false, "1": true, "2": false, "": true, "=1.4.2": true, ">=1.4.0,<1.4.2": false}
	for c, want := range cases {
		if got := v.Satisfies(c); got != want {
			t.Errorf("1.4.2 satisfies %q = %v, want %v", c, got, want)
		}
	}
	z, _ := ParseSemver("0.3.1")
	if !z.Satisfies("^0.3.0") || z.Satisfies("^0.2.0") {
		t.Error("caret on 0.x")
	}
	if b := v.Bump(Semver{1, 4, 3, ""}); b != "patch" {
		t.Error(b)
	}
	if b := v.Bump(Semver{1, 5, 0, ""}); b != "minor" {
		t.Error(b)
	}
	if b := v.Bump(Semver{2, 0, 0, ""}); b != "major" {
		t.Error(b)
	}
	if b := v.Bump(Semver{1, 4, 2, ""}); b != "" {
		t.Error(b)
	}
}

func TestAutoInstall(t *testing.T) {
	cur, _ := ParseSemver("1.4.2")
	r := func(v string, sec, brk bool) Release { return Release{Version: v, Security: sec, Breaking: brk} }
	cases := []struct {
		policy string
		rel    Release
		want   bool
	}{
		{PolicyPatch, r("1.4.3", false, false), true},
		{PolicyPatch, r("1.5.0", false, false), false},
		{PolicyPatch, r("1.5.0", true, false), true},
		{PolicyPatch, r("2.0.0", true, false), false},
		{PolicyMinor, r("1.5.0", false, false), true},
		{PolicyMinor, r("1.5.0", false, true), false},
		{PolicyMinor, r("2.0.0", false, false), false},
		{PolicyNotify, r("1.4.3", false, false), false},
		{PolicyMinor, r("1.4.1", false, false), false},
	}
	for _, c := range cases {
		if got := AutoInstall(c.policy, cur, c.rel); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.policy, c.rel.Version, got, c.want)
		}
	}
}

func TestWindow(t *testing.T) {
	at := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339, s); return tm }
	any := Window{}
	if !any.Allows(at("2026-10-28T13:00:00Z")) {
		t.Error("empty window")
	}
	w := Window{Start: "03:00", End: "05:00", Days: []string{"sat", "sun"}}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	if !w.Allows(at("2026-10-31T03:30:00Z")) || w.Allows(at("2026-10-31T05:00:00Z")) || w.Allows(at("2026-10-30T03:30:00Z")) {
		t.Error("sat 03–05")
	}
	night := Window{Start: "22:00", End: "02:00", Days: []string{"fri"}}
	if !night.Allows(at("2026-10-30T23:00:00Z")) || !night.Allows(at("2026-10-31T01:00:00Z")) || night.Allows(at("2026-10-31T23:00:00Z")) {
		t.Error("overnight window belongs to the day it starts")
	}
	ny := Window{Start: "03:00", End: "04:00", TimeZone: "America/New_York"}
	if !ny.Allows(at("2026-10-28T07:30:00Z")) || ny.Allows(at("2026-10-28T03:30:00Z")) {
		t.Error("time zone")
	}
	if n := w.Next(at("2026-10-28T12:00:00Z")); !n.Equal(at("2026-10-31T03:00:00Z")) {
		t.Errorf("next = %v", n)
	}
	for _, bad := range []Window{{Start: "03:00"}, {Start: "25:00", End: "01:00"}, {Days: []string{"monday"}}, {TimeZone: "Mars/Base"}} {
		if bad.Validate() == nil {
			t.Errorf("%+v should be invalid", bad)
		}
	}
}

func TestSettings(t *testing.T) {
	s := Settings{}.Normalize()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Panel.Policy != PolicyNotify || s.K3s.Policy != PolicyOff || s.IntervalHours != 6 {
		t.Errorf("defaults %+v", s)
	}
	s.K3s.Channel = "v1.31"
	s.Gateway.Policy = PolicyMinor
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.K3s.Policy = PolicyMinor
	if s.Validate() == nil {
		t.Error("k3s minor")
	}
}

func intp(i int) *int { return &i }

func TestMakePlan(t *testing.T) {
	panel := Installed{Version: "0.6.0", Image: "p@sha256:0", Requires: Requires{GatewayAPI: []int{1}, MinGateway: "1.0.0"}}
	gw := Installed{Version: "1.2.1", Image: "g@sha256:0"}
	p7 := Release{Component: Panel, Version: "0.7.0", Image: "p@sha256:7", Requires: Requires{GatewayAPI: []int{1, 2}, MinGateway: "1.2.0"}, Migrations: true}
	p8 := Release{Component: Panel, Version: "0.8.0", Image: "p@sha256:8", Requires: Requires{GatewayAPI: []int{2}}}
	g2 := Release{Component: Gateway, Version: "2.0.0", Image: "g@sha256:2", APIVersion: intp(2), Requires: Requires{Panel: ">=0.7.0"}, Breaking: true}
	g13 := Release{Component: Gateway, Version: "1.3.0", Image: "g@sha256:13", APIVersion: intp(1)}

	// simple panel update
	pl, err := MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, PanelTo: &p7})
	if err != nil || len(pl.Steps) != 1 || pl.Steps[0].To != "0.7.0" || !pl.Steps[0].Migrations {
		t.Fatalf("panel: %+v %v", pl, err)
	}
	// gateway 2.0 needs panel >= 0.7.0 → completed with the newest panel that fits (0.8.0 needs API 2 too: fits)
	_, err = MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, GatewayTo: &g2})
	if err == nil || !strings.Contains(err.Error(), "needs") {
		t.Fatalf("incomplete plan should fail: %v", err)
	}
	// 0.8.0 is newer but cannot be ordered safely (it only speaks API 2, gateway 2.0 needs panel 0.7+):
	// 0.7.0 is picked, it goes first (it speaks API 1 and 2), then the gateway
	pl, err = MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, GatewayTo: &g2, Complete: true,
		PanelReleases: []Release{p8, p7}})
	if err != nil || len(pl.Steps) != 2 || pl.Steps[0].Component != Panel || pl.Steps[0].To != "0.7.0" || pl.Steps[1].To != "2.0.0" || len(pl.Notes) == 0 {
		t.Fatalf("%+v %v", pl, err)
	}
	if _, err := MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, PanelTo: &p8, GatewayTo: &g2}); err == nil {
		t.Fatal("unsafe order accepted")
	}
	// panel 0.9 needs gateway 1.3+ → gateway first
	p9 := Release{Component: Panel, Version: "0.9.0", Image: "p@sha256:9", Requires: Requires{GatewayAPI: []int{1}, MinGateway: "1.3.0"}}
	pl, err = MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, PanelTo: &p9, GatewayTo: &g13})
	if err != nil || pl.Steps[0].Component != Gateway || len(pl.Notes) == 0 {
		t.Fatalf("gateway first: %+v %v", pl, err)
	}
	// 0.7 needs gateway >= 1.2.0: fine with 1.2.1; panel first
	pl, err = MakePlan(PlanInput{Panel: panel, Gateway: gw, PanelAPI: 1, GatewayAPI: 1, PanelTo: &p7, GatewayTo: &g13})
	if err != nil || pl.Steps[0].Component != Panel {
		t.Fatalf("panel first: %+v %v", pl, err)
	}
	if _, err := MakePlan(PlanInput{Panel: panel, Gateway: gw}); err == nil {
		t.Error("empty plan")
	}
}

func TestClean(t *testing.T) {
	in := []Release{{Version: "1.0.0", Image: "x"}, {Version: "1.2.0-beta.1", Image: "x"}, {Version: "1.1.0", Image: "x"},
		{Version: "bad", Image: "x"}, {Version: "1.1.0", Image: "x"}, {Version: "0.9.0"}, {Version: "1.3.0", Image: "x", Component: Gateway}}
	st := clean(Panel, Stable, in)
	if len(st) != 2 || st[0].Version != "1.1.0" {
		t.Errorf("stable %+v", st)
	}
	bt := clean(Panel, Beta, in)
	if len(bt) != 3 || bt[0].Version != "1.2.0-beta.1" {
		t.Errorf("beta %+v", bt)
	}
}
