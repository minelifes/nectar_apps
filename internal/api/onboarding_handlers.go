package api

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/store"
)

// First-run onboarding (after the Owner exists): base domain + DNS check, Let's Encrypt email, a sample app.
// The panel only remembers whether it was finished or skipped; every step uses the regular settings APIs.

const onboardingKey = "onboarding"

type onboardingState struct {
	Done   bool       `json:"done"`
	DoneAt *time.Time `json:"done_at,omitempty"`
}

func (s *Server) getOnboarding(w http.ResponseWriter, r *http.Request, _ *principal) {
	ctx := r.Context()
	var st onboardingState
	if err := s.store.GetSetting(ctx, onboardingKey, &st); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, r, err)
		return
	}
	set := s.loadSettings(ctx)
	resp := map[string]any{
		"done":        st.Done,
		"done_at":     st.DoneAt,
		"base_domain": set.BaseDomain,
		"public_ips":  set.PublicIPs,
		"server_ips":  s.serverIPs(ctx, set),
		"gateway":     map[string]any{"configured": s.gateway != nil, "routing": s.cfg.Routing},
		"acme_email":  "",
	}
	if s.gateway != nil {
		gctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if raw, err := s.gateway.Send(gctx, http.MethodGet, "/api/settings", nil); err == nil {
			var gs struct {
				ACMEEmail string `json:"acme_email"`
			}
			if json.Unmarshal(raw, &gs) == nil {
				resp["acme_email"] = gs.ACMEEmail
			}
			resp["gateway"] = map[string]any{"configured": true, "reachable": true, "routing": s.cfg.Routing}
		} else {
			resp["gateway"] = map[string]any{"configured": true, "reachable": false, "error": err.Error(), "routing": s.cfg.Routing}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) putOnboarding(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Done bool `json:"done"`
	}
	if !decode(w, r, &in) {
		return
	}
	st := onboardingState{Done: in.Done}
	if in.Done {
		now := time.Now().UTC()
		st.DoneAt = &now
	}
	if err := s.store.SetSetting(r.Context(), onboardingKey, st); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "onboarding.update", "", st)
	writeJSON(w, http.StatusOK, st)
}

// dnsCheckBase checks that *.<base_domain> points to this server (apps get <app>-<project>.<base>).
func (s *Server) dnsCheckBase(w http.ResponseWriter, r *http.Request, _ *principal) {
	var in struct {
		BaseDomain string `json:"base_domain"`
	}
	if !decode(w, r, &in) {
		return
	}
	base := store.NormalizeHost(in.BaseDomain)
	if base == "" || strings.HasPrefix(base, "*.") || !hostRe.MatchString(base) {
		writeError(w, http.StatusBadRequest, "base_domain must be a domain like apps.example.com")
		return
	}
	expected := s.serverIPs(r.Context(), s.loadSettings(r.Context()))
	// a random name: only a wildcard record (*.base) answers it
	probe := "nectar-check-" + strconv.FormatUint(rand.Uint64()%1e9, 36) + "." + base
	res := dnsCheck(r.Context(), probe, expected)
	res["host"] = "*." + base
	res["probe"] = probe
	writeJSON(w, http.StatusOK, res)
}

// projectActivity is the audit trail of one project and its apps (readable by anyone who can view the
// project, unlike the global audit log).
func (s *Server) projectActivity(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	entries, err := s.store.ListAuditForTarget(r.Context(), proj.Slug, proj.CreatedAt, limit, before)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for i := range entries {
		entries[i].IP = "" // not needed for the feed
	}
	writeJSON(w, http.StatusOK, entries)
}
