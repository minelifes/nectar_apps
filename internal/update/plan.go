package update

import (
	"fmt"
	"slices"
)

// Installed is what runs now.
type Installed struct {
	Version    string `json:"version"`
	Image      string `json:"image,omitempty"`
	APIVersion int    `json:"api_version,omitempty"`
	// Requires of the running build (the panel knows its own; the gateway's comes from its release entry).
	Requires Requires `json:"-"`
}

// Step updates one component.
type Step struct {
	Component  string `json:"component"`
	From       string `json:"from"`
	To         string `json:"to"`
	FromImage  string `json:"from_image"`
	ToImage    string `json:"to_image"`
	Migrations bool   `json:"migrations"`
	Breaking   bool   `json:"breaking,omitempty"`
}

// Plan is an ordered list of steps that keeps the panel and the gateway compatible after every step.
type Plan struct {
	Steps []Step   `json:"steps"`
	Notes []string `json:"notes,omitempty"` // why steps were added or ordered
}

// side is a panel or gateway build as the planner sees it.
type side struct {
	version  Semver
	api      int
	requires Requires
	label    string
}

func panelSide(v string, api int, req Requires) side {
	s, _ := ParseSemver(v)
	return side{s, api, req, "panel " + v}
}

func gatewaySide(v string, api int, req Requires) side {
	s, _ := ParseSemver(v)
	return side{s, api, req, "gateway " + v}
}

func releaseAPI(r Release, def int) int {
	if r.APIVersion != nil {
		return *r.APIVersion
	}
	return def
}

// Compatible explains why a panel and a gateway cannot work together ("" = they can).
func compatible(p, g side) string {
	if len(p.requires.GatewayAPI) > 0 && g.api > 0 && !slices.Contains(p.requires.GatewayAPI, g.api) {
		return fmt.Sprintf("%s needs gateway API %v, %s serves API %d", p.label, p.requires.GatewayAPI, g.label, g.api)
	}
	if p.requires.MinGateway != "" && !g.version.Satisfies(">="+p.requires.MinGateway) {
		return fmt.Sprintf("%s needs gateway %s or newer", p.label, p.requires.MinGateway)
	}
	if g.requires.Panel != "" && !p.version.Satisfies(g.requires.Panel) {
		return fmt.Sprintf("%s needs panel %s", g.label, g.requires.Panel)
	}
	if len(g.requires.PanelAPI) > 0 && p.api > 0 && !slices.Contains(g.requires.PanelAPI, p.api) {
		return fmt.Sprintf("%s needs panel API %v, %s serves API %d", g.label, g.requires.PanelAPI, p.label, p.api)
	}
	return ""
}

// PlanInput describes a requested update. A nil target keeps that component as it is.
type PlanInput struct {
	Panel, Gateway       Installed
	PanelTo, GatewayTo   *Release
	PanelAPI, GatewayAPI int       // API versions of the running builds
	PanelReleases        []Release // newest first, to complete a plan
	GatewayReleases      []Release
	// Complete allows adding an update of the other component when the target needs it.
	Complete bool
	// Skip lists versions that must not be picked when completing (failed before).
	Skip map[string]bool
}

// MakePlan builds a plan or explains why there is none yet.
func MakePlan(in PlanInput) (Plan, error) {
	var plan Plan
	curP := panelSide(in.Panel.Version, in.PanelAPI, in.Panel.Requires)
	curG := gatewaySide(in.Gateway.Version, in.GatewayAPI, in.Gateway.Requires)
	newP, newG := curP, curG
	if in.PanelTo != nil {
		newP = panelSide(in.PanelTo.Version, releaseAPI(*in.PanelTo, in.PanelAPI), in.PanelTo.Requires)
	}
	if in.GatewayTo != nil {
		newG = gatewaySide(in.GatewayTo.Version, releaseAPI(*in.GatewayTo, in.GatewayAPI), in.GatewayTo.Requires)
	}
	pTo, gTo := in.PanelTo, in.GatewayTo

	if why := compatible(newP, newG); why != "" {
		if !in.Complete {
			return plan, fmt.Errorf("not compatible: %s", why)
		}
		// complete the plan with the newest release of the other component that fits
		switch {
		case gTo != nil && pTo == nil:
			for _, r := range in.PanelReleases {
				r := r
				if in.Skip[Panel+"@"+r.Version] || !curP.version.Less(r.Semver()) {
					continue
				}
				cand := panelSide(r.Version, releaseAPI(r, in.PanelAPI), r.Requires)
				if compatible(cand, newG) == "" && (compatible(cand, curG) == "" || compatible(curP, newG) == "") {
					pTo, newP = &r, panelSide(r.Version, releaseAPI(r, in.PanelAPI), r.Requires)
					plan.Notes = append(plan.Notes, fmt.Sprintf("The panel is updated to %s too: %s.", r.Version, why))
					break
				}
			}
		case pTo != nil && gTo == nil:
			for _, r := range in.GatewayReleases {
				r := r
				if in.Skip[Gateway+"@"+r.Version] || !curG.version.Less(r.Semver()) {
					continue
				}
				cand := gatewaySide(r.Version, releaseAPI(r, in.GatewayAPI), r.Requires)
				if compatible(newP, cand) == "" && (compatible(newP, curG) == "" || compatible(curP, cand) == "") {
					gTo, newG = &r, gatewaySide(r.Version, releaseAPI(r, in.GatewayAPI), r.Requires)
					plan.Notes = append(plan.Notes, fmt.Sprintf("The gateway is updated to %s too: %s.", r.Version, why))
					break
				}
			}
		}
		if why := compatible(newP, newG); why != "" {
			return Plan{}, fmt.Errorf("waiting for a compatible release: %s", why)
		}
	}

	pStep := func() Step {
		return Step{Component: Panel, From: in.Panel.Version, To: pTo.Version, FromImage: in.Panel.Image, ToImage: pTo.Image,
			Migrations: pTo.Migrations, Breaking: pTo.Breaking}
	}
	gStep := func() Step {
		return Step{Component: Gateway, From: in.Gateway.Version, To: gTo.Version, FromImage: in.Gateway.Image, ToImage: gTo.Image,
			Migrations: gTo.Migrations, Breaking: gTo.Breaking}
	}
	switch {
	case pTo != nil && gTo != nil:
		// the panel goes first when the pair stays compatible meanwhile (its updater then runs the gateway step)
		if compatible(newP, curG) == "" {
			plan.Steps = []Step{pStep(), gStep()}
		} else if compatible(curP, newG) == "" {
			plan.Steps = []Step{gStep(), pStep()}
			plan.Notes = append(plan.Notes, "The gateway goes first: "+compatible(newP, curG)+".")
		} else {
			return Plan{}, fmt.Errorf("no safe order: %s, and %s", compatible(newP, curG), compatible(curP, newG))
		}
	case pTo != nil:
		plan.Steps = []Step{pStep()}
	case gTo != nil:
		plan.Steps = []Step{gStep()}
	default:
		return Plan{}, fmt.Errorf("nothing to update")
	}
	for _, s := range plan.Steps {
		if s.ToImage == "" {
			return Plan{}, fmt.Errorf("%s %s has no image", s.Component, s.To)
		}
		if s.FromImage == s.ToImage {
			return Plan{}, fmt.Errorf("%s already runs %s", s.Component, s.To)
		}
	}
	return plan, nil
}
