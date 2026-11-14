package api

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"nectar_apps/internal/sizing"
	"nectar_apps/internal/store"
)

// Right-sizing: what an app reserves per replica vs. what its pods use (internal/sizing).

func (s *Server) appSizing(r *http.Request, app store.App) (sizing.Report, error) {
	pts, err := s.store.UsageSince(r.Context(), "app:"+strconv.FormatInt(app.ID, 10), time.Now().Add(-sizing.Window))
	if err != nil {
		return sizing.Report{}, err
	}
	return sizing.Analyze(app.Config.Resources, max(app.Config.Replicas, app.Config.ResumeReplicas, 1), pts), nil
}

func (s *Server) getAppSizing(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	rep, err := s.appSizing(r, app)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// serverSizing lists the apps with a hint, the biggest savings first, and what the server would free.
func (s *Server) serverSizing(w http.ResponseWriter, r *http.Request, _ *principal) {
	apps, err := s.store.ListApps(r.Context(), 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	type row struct {
		AppID   int64  `json:"app_id"`
		App     string `json:"app"`
		Project string `json:"project"`
		sizing.Report
	}
	projects := map[int64]string{}
	out := struct {
		Apps        []row `json:"apps"`
		SavingCPU   int64 `json:"saving_cpu_m"`     // freed by the suggestions that lower a reservation
		SavingMemMB int64 `json:"saving_memory_mb"` //
		Waiting     int   `json:"collecting"`       // apps without enough data yet
		ReqCPU      int64 `json:"requested_cpu_m"`  // all apps, all replicas
		ReqMemMB    int64 `json:"requested_memory_mb"`
	}{Apps: []row{}}
	for _, a := range apps {
		if a.Config.Replicas == 0 && a.Config.ResumeReplicas == 0 {
			continue // stopped long ago, or a task/cron app without replicas
		}
		rep, err := s.appSizing(r, a)
		if err != nil {
			continue
		}
		n := int64(max(a.Config.Replicas, 1))
		out.ReqCPU += a.Config.Resources.CPUMillicores * n
		out.ReqMemMB += a.Config.Resources.MemoryMB * n
		if !rep.Ready {
			out.Waiting++
			continue
		}
		if len(rep.Hints) == 0 {
			continue
		}
		name, ok := projects[a.ProjectID]
		if !ok {
			if p, err := s.store.GetProject(r.Context(), a.ProjectID); err == nil {
				name = p.Slug
			}
			projects[a.ProjectID] = name
		}
		out.Apps = append(out.Apps, row{AppID: a.ID, App: a.Name, Project: name, Report: rep})
		out.SavingCPU += max(rep.SavingCPU, 0)
		out.SavingMemMB += max(rep.SavingMemMB, 0)
	}
	slices.SortFunc(out.Apps, func(a, b row) int {
		// the ones at risk (needing more) first, then the biggest memory savings
		ra, rb := a.SavingMemMB < 0 || a.SavingCPU < 0, b.SavingMemMB < 0 || b.SavingCPU < 0
		switch {
		case ra != rb:
			if ra {
				return -1
			}
			return 1
		case a.SavingMemMB != b.SavingMemMB:
			return int(b.SavingMemMB - a.SavingMemMB)
		}
		return int(b.SavingCPU - a.SavingCPU)
	})
	writeJSON(w, http.StatusOK, out)
}
