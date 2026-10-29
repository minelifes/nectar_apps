package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/backup"
	"nectar_apps/internal/store"
)

func (s *Server) needBackups(w http.ResponseWriter) bool {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backups need Kubernetes")
		return false
	}
	return true
}

// ------------------------------------------------------------------ targets --

type targetView struct {
	store.BackupTarget
	Repository string              `json:"repository"`
	Config     backup.TargetConfig `json:"config"`
	Plans      int                 `json:"plans"`
}

func (s *Server) targetView(t store.BackupTarget, plans []store.BackupPlan) (targetView, error) {
	cfg, err := backup.Open(s.backups.Box, t.Config)
	if err != nil {
		return targetView{}, err
	}
	v := targetView{BackupTarget: t, Repository: cfg.Display(t.Kind), Config: cfg.Mask()}
	for _, p := range plans {
		if p.TargetID == t.ID {
			v.Plans++
		}
	}
	return v, nil
}

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needBackups(w) {
		return
	}
	list, err := s.store.ListBackupTargets(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	plans, _ := s.store.ListBackupPlans(r.Context())
	out := make([]targetView, 0, len(list))
	for _, t := range list {
		v, err := s.targetView(t, plans)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

type targetInput struct {
	Name   string              `json:"name"`
	Kind   string              `json:"kind"`
	Config backup.TargetConfig `json:"config"`
}

func (s *Server) createTarget(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	var in targetInput
	if !decode(w, r, &in) {
		return
	}
	if in.Config.Password == backup.Masked {
		in.Config.Password = ""
	}
	s.saveTarget(w, r, p, store.BackupTarget{Kind: in.Kind}, backup.TargetConfig{}, in)
}

func (s *Server) targetFromPath(w http.ResponseWriter, r *http.Request) (store.BackupTarget, bool) {
	id, err := strconv.ParseInt(r.PathValue("tid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "target not found")
		return store.BackupTarget{}, false
	}
	t, err := s.store.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "target not found")
		return t, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return t, false
	}
	return t, true
}

func (s *Server) updateTarget(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	t, ok := s.targetFromPath(w, r)
	if !ok {
		return
	}
	old, err := backup.Open(s.backups.Box, t.Config)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var in targetInput
	if !decode(w, r, &in) {
		return
	}
	s.saveTarget(w, r, p, t, old, in)
}

func (s *Server) saveTarget(w http.ResponseWriter, r *http.Request, p *principal, t store.BackupTarget, old backup.TargetConfig, in targetInput) {
	if !slices.Contains([]string{backup.KindS3, backup.KindSFTP, backup.KindLocal}, t.Kind) {
		writeError(w, http.StatusBadRequest, "kind must be s3, sftp or local")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 60 {
		writeError(w, http.StatusBadRequest, "a name of 1–60 characters is required")
		return
	}
	cfg := in.Config.KeepSecrets(old)
	if cfg.SecretKey == backup.Masked || cfg.PrivateKey == backup.Masked {
		writeError(w, http.StatusBadRequest, "enter the secret again")
		return
	}
	if err := cfg.Validate(t.Kind); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sealed, err := backup.Seal(s.backups.Box, cfg)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	created := t.ID == 0
	t.Name, t.Config = name, sealed
	t, err = s.store.SaveBackupTarget(r.Context(), t)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	action, status := "backup.target.update", http.StatusOK
	if created {
		action, status = "backup.target.create", http.StatusCreated
	}
	s.audit(r, p, action, strconv.FormatInt(t.ID, 10), map[string]string{"kind": t.Kind, "name": t.Name})
	plans, _ := s.store.ListBackupPlans(r.Context())
	v, err := s.targetView(t, plans)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request, p *principal) {
	t, ok := s.targetFromPath(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteBackupTarget(r.Context(), t.ID); errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "delete the plans that use this target first")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	if s.backups != nil {
		s.backups.RemoveRestServer(r.Context(), t)
	}
	s.audit(r, p, "backup.target.delete", strconv.FormatInt(t.ID, 10), map[string]string{"name": t.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkTarget(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needBackups(w) {
		return
	}
	t, ok := s.targetFromPath(w, r)
	if !ok {
		return
	}
	n, err := s.backups.CheckTarget(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "snapshots": n})
}

// recoveryKit returns what install.sh --restore needs to rebuild the server from this target (with the
// master key, which the panel never hands out).
func (s *Server) recoveryKit(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	t, ok := s.targetFromPath(w, r)
	if !ok {
		return
	}
	cfg, err := backup.Open(s.backups.Box, t.Config)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "backup.recovery_kit", strconv.FormatInt(t.ID, 10), nil)
	w.Header().Set("Content-Disposition", `attachment; filename="nectar-recovery-kit.json"`)
	writeJSON(w, http.StatusOK, cfg.Kit(t.Kind))
}

// -------------------------------------------------------------------- plans --

type planView struct {
	store.BackupPlan
	Subject    string           `json:"subject"`
	TargetName string           `json:"target_name"`
	NextRun    *time.Time       `json:"next_run,omitempty"`
	LastRun    *store.BackupRun `json:"last_run,omitempty"`
}

func (s *Server) planView(ctx context.Context, p store.BackupPlan, last map[int64]store.BackupRun) planView {
	v := planView{BackupPlan: p}
	v.Subject, _ = s.backups.Subject(ctx, p.Scope, p.SubjectID)
	if t, err := s.store.GetBackupTarget(ctx, p.TargetID); err == nil {
		v.TargetName = t.Name
	}
	if p.Enabled && p.Schedule != "" {
		if sch, err := backup.ParseSchedule(p.Schedule); err == nil {
			if n := sch.Next(time.Now()); !n.IsZero() {
				v.NextRun = &n
			}
		}
	}
	if r, ok := last[p.ID]; ok {
		v.LastRun = &r
	}
	return v
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needBackups(w) {
		return
	}
	plans, err := s.store.ListBackupPlans(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	last, _ := s.store.LastBackupRuns(r.Context())
	out := make([]planView, 0, len(plans))
	for _, p := range plans {
		out = append(out, s.planView(r.Context(), p, last))
	}
	writeJSON(w, http.StatusOK, out)
}

type planInput struct {
	TargetID    int64  `json:"target_id"`
	Scope       string `json:"scope"`
	SubjectID   int64  `json:"subject_id"`
	Schedule    string `json:"schedule"`
	KeepDaily   *int   `json:"keep_daily"`
	KeepWeekly  *int   `json:"keep_weekly"`
	KeepMonthly *int   `json:"keep_monthly"`
	Enabled     *bool  `json:"enabled"`
}

func (in planInput) apply(p *store.BackupPlan) string {
	p.Schedule = strings.TrimSpace(in.Schedule)
	if p.Schedule != "" {
		if _, err := backup.ParseSchedule(p.Schedule); err != nil {
			return "schedule: " + err.Error()
		}
	}
	for _, kv := range []struct {
		v   *int
		dst *int
	}{{in.KeepDaily, &p.KeepDaily}, {in.KeepWeekly, &p.KeepWeekly}, {in.KeepMonthly, &p.KeepMonthly}} {
		if kv.v != nil {
			if *kv.v < 0 || *kv.v > 365 {
				return "keep values must be 0–365"
			}
			*kv.dst = *kv.v
		}
	}
	if p.KeepDaily+p.KeepWeekly+p.KeepMonthly == 0 {
		return "keep at least one snapshot"
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	return ""
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	var in planInput
	if !decode(w, r, &in) {
		return
	}
	if _, err := s.store.GetBackupTarget(r.Context(), in.TargetID); err != nil {
		writeError(w, http.StatusBadRequest, "unknown target")
		return
	}
	if !slices.Contains([]string{backup.ScopeApp, backup.ScopeAddon, backup.ScopePanel}, in.Scope) {
		writeError(w, http.StatusBadRequest, "scope must be app, addon or panel")
		return
	}
	if in.Scope == backup.ScopePanel {
		in.SubjectID = 0
	}
	if _, err := s.backups.Subject(r.Context(), in.Scope, in.SubjectID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Scope == backup.ScopeApp {
		if a, _ := s.store.GetApp(r.Context(), in.SubjectID); len(a.Config.Volumes) == 0 {
			writeError(w, http.StatusBadRequest, "the app has no volumes: add one in its settings first")
			return
		}
	}
	plan := store.BackupPlan{TargetID: in.TargetID, Scope: in.Scope, SubjectID: in.SubjectID, KeepDaily: 7, KeepWeekly: 4, KeepMonthly: 6, Enabled: true}
	if msg := in.apply(&plan); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	plan, err := s.store.SaveBackupPlan(r.Context(), plan)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this is backed up to that target already")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "backup.plan.create", strconv.FormatInt(plan.ID, 10), map[string]any{"scope": plan.Scope, "subject": plan.SubjectID})
	writeJSON(w, http.StatusCreated, s.planView(r.Context(), plan, nil))
}

func (s *Server) planFromPath(w http.ResponseWriter, r *http.Request) (store.BackupPlan, bool) {
	id, err := strconv.ParseInt(r.PathValue("bid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "plan not found")
		return store.BackupPlan{}, false
	}
	plan, err := s.store.GetBackupPlan(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "plan not found")
		return plan, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return plan, false
	}
	return plan, true
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	plan, ok := s.planFromPath(w, r)
	if !ok {
		return
	}
	var in planInput
	if !decode(w, r, &in) {
		return
	}
	if msg := in.apply(&plan); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	plan, err := s.store.SaveBackupPlan(r.Context(), plan)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "backup.plan.update", strconv.FormatInt(plan.ID, 10), nil)
	last, _ := s.store.LastBackupRuns(r.Context())
	writeJSON(w, http.StatusOK, s.planView(r.Context(), plan, last))
}

func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request, p *principal) {
	plan, ok := s.planFromPath(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteBackupPlan(r.Context(), plan.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "backup.plan.delete", strconv.FormatInt(plan.ID, 10), map[string]any{"scope": plan.Scope, "subject": plan.SubjectID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runPlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	plan, ok := s.planFromPath(w, r)
	if !ok {
		return
	}
	run, err := s.backups.StartBackup(r.Context(), plan)
	if errors.Is(err, backup.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "backup.run", strconv.FormatInt(plan.ID, 10), nil)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) planSnapshots(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needBackups(w) {
		return
	}
	plan, ok := s.planFromPath(w, r)
	if !ok {
		return
	}
	list, err := s.backups.Snapshots(r.Context(), plan)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) restorePlan(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needBackups(w) {
		return
	}
	plan, ok := s.planFromPath(w, r)
	if !ok {
		return
	}
	var in struct {
		Snapshot string `json:"snapshot"`
	}
	if !decode(w, r, &in) {
		return
	}
	run, err := s.backups.StartRestore(r.Context(), plan, in.Snapshot)
	if errors.Is(err, backup.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, backup.ErrNoSnapshot) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, p, "backup.restore", strconv.FormatInt(plan.ID, 10), map[string]string{"snapshot": in.Snapshot})
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request, _ *principal) {
	planID, _ := strconv.ParseInt(r.URL.Query().Get("plan"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.ListBackupRuns(r.Context(), planID, r.URL.Query().Get("status"), limit)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}
