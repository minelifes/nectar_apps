package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/store"
	"nectar_apps/internal/update"
	"nectar_apps/internal/version"
)

func (s *Server) needUpdates(w http.ResponseWriter) bool {
	if s.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "updates are not available on this server")
		return false
	}
	return true
}

// getUpdates returns the settings, the last check, the history and the running update.
func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needUpdates(w) {
		return
	}
	ctx := r.Context()
	set, err := s.updates.Settings(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	runs, err := s.store.ListUpdateRuns(ctx, 20)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var active *store.UpdateRun
	for i := range runs {
		if runs[i].FinishedAt == nil {
			active = &runs[i]
			break
		}
	}
	st := s.updates.State(ctx)
	var next *time.Time
	if !st.CheckedAt.IsZero() {
		n := st.CheckedAt.Add(time.Duration(set.IntervalHours) * time.Hour)
		next = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": set, "state": st, "bad": s.updates.Bad(ctx), "runs": runs, "active": active,
		"next_check": next, "window_open": set.Window.Allows(time.Now()),
		"panel": map[string]any{"version": version.Version, "commit": version.Commit, "api_version": version.APIVersion},
	})
}

func (s *Server) putUpdateSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needUpdates(w) {
		return
	}
	var in update.Settings
	if !decode(w, r, &in) {
		return
	}
	set, err := s.updates.SaveSettings(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, p, "updates.settings", "", map[string]any{"panel": set.Panel.Policy, "gateway": set.Gateway.Policy, "k3s": set.K3s.Policy})
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needUpdates(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	st, err := s.updates.Check(ctx)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type updateRequest struct {
	Panel   string `json:"panel"`
	Gateway string `json:"gateway"`
}

// planUpdate shows what "Update now" would do (with release notes and warnings).
func (s *Server) planUpdate(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needUpdates(w) {
		return
	}
	var in updateRequest
	if !decode(w, r, &in) {
		return
	}
	plan, err := s.updates.Plan(r.Context(), update.PlanRequest{Panel: in.Panel, Gateway: in.Gateway}, true)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "summary": update.Summary(plan), "releases": s.planReleases(r.Context(), plan),
		"warnings": planWarnings(plan)})
}

// planReleases returns the release entries (notes) of every version a plan passes.
func (s *Server) planReleases(ctx context.Context, plan update.Plan) []update.Release {
	st := s.updates.State(ctx)
	var out []update.Release
	for _, step := range plan.Steps {
		cs := st.Components[step.Component]
		if cs == nil {
			continue
		}
		from, _ := update.ParseSemver(step.From)
		to, _ := update.ParseSemver(step.To)
		for _, rel := range cs.Available {
			v := rel.Semver()
			if from.Less(v) && v.Compare(to) <= 0 {
				out = append(out, rel)
			}
		}
	}
	return out
}

func planWarnings(plan update.Plan) []string {
	var out []string
	for _, st := range plan.Steps {
		name := st.Component
		if st.Breaking {
			out = append(out, name+" "+st.To+" is a breaking release: read its notes first.")
		}
		if st.Migrations {
			out = append(out, name+" "+st.To+" changes its database. A backup is taken first; rolling back later restores it and loses changes made after the update.")
		}
		if st.Component == update.Panel {
			out = append(out, "The panel restarts during its update (a few seconds; apps and traffic are not affected).")
		}
	}
	return out
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needUpdates(w) {
		return
	}
	var in updateRequest
	if !decode(w, r, &in) {
		return
	}
	plan, err := s.updates.Plan(r.Context(), update.PlanRequest{Panel: in.Panel, Gateway: in.Gateway}, true)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	run, err := s.updates.Start(r.Context(), plan, "manual", nil, &p.user.ID)
	if errors.Is(err, update.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "update.start", run.Summary, map[string]any{"run": run.ID})
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) rollbackUpdate(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needUpdates(w) {
		return
	}
	var in struct {
		RunID int64 `json:"run_id"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	run, err := s.updates.Rollback(r.Context(), in.RunID, &p.user.ID)
	switch {
	case errors.Is(err, update.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "update run not found")
		return
	case err != nil:
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r, p, "update.rollback", run.Summary, map[string]any{"run": run.ID, "of": in.RunID})
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listUpdateRuns(w http.ResponseWriter, r *http.Request, _ *principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.ListUpdateRuns(r.Context(), limit)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getUpdateRun(w http.ResponseWriter, r *http.Request, _ *principal) {
	id, err := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "update run not found")
		return
	}
	run, err := s.store.GetUpdateRun(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "update run not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// clearBadRelease lets a release that failed be installed automatically again.
func (s *Server) clearBadRelease(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needUpdates(w) {
		return
	}
	key := r.PathValue("key")
	if !strings.Contains(key, "@") {
		writeError(w, http.StatusBadRequest, "expected component@version")
		return
	}
	if err := s.updates.ClearBad(r.Context(), key); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "update.allow_again", key, nil)
	w.WriteHeader(http.StatusNoContent)
}

// applyK3s pins the k3s upgrade plans to a version (the system-upgrade-controller upgrades the nodes now).
func (s *Server) applyK3s(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needUpdates(w) {
		return
	}
	var in struct {
		Version string `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, ok := update.ParseSemver(in.Version); !ok || !strings.HasPrefix(in.Version, "v") {
		writeError(w, http.StatusBadRequest, "version must look like v1.31.4+k3s1")
		return
	}
	set, err := s.updates.Settings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.updates.SyncK3s(r.Context(), set, in.Version); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r, p, "update.k3s", in.Version, nil)
	writeJSON(w, http.StatusAccepted, map[string]any{"version": in.Version})
}
