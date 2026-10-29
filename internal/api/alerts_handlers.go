package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/alerts"
	"nectar_apps/internal/housekeep"
	"nectar_apps/internal/store"
)

// ------------------------------------------------------------------ alerts --

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request, _ *principal) {
	q := r.URL.Query()
	state := q.Get("state")
	if state != "" && state != store.AlertFiring && state != store.AlertResolved {
		writeError(w, http.StatusBadRequest, "state must be firing or resolved")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	list, err := s.store.ListAlerts(r.Context(), state, nil, limit, before)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) appAlerts(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.store.ListAlerts(r.Context(), r.URL.Query().Get("state"), &app.ID, limit, 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getAlertRules(w http.ResponseWriter, r *http.Request, _ *principal) {
	set, err := alerts.LoadSettings(r.Context(), s.store)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": alerts.Catalog, "settings": set})
}

func (s *Server) putAlertRules(w http.ResponseWriter, r *http.Request, p *principal) {
	var in alerts.Settings
	if !decode(w, r, &in) {
		return
	}
	in.PanelURL = strings.TrimRight(strings.TrimSpace(in.PanelURL), "/")
	if in.PanelURL != "" {
		if err := (&alerts.ChannelConfig{URL: in.PanelURL}).Validate(alerts.Webhook); err != nil {
			writeError(w, http.StatusBadRequest, "panel_url: "+err.Error())
			return
		}
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	set := in.Normalize()
	if err := s.store.SetSetting(r.Context(), alerts.SettingsKey, set); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "alerts.rules.update", "", nil)
	writeJSON(w, http.StatusOK, map[string]any{"catalog": alerts.Catalog, "settings": set})
}

// channelView is a channel as returned by the API (secrets masked).
type channelView struct {
	store.AlertChannel
	Config alerts.ChannelConfig `json:"config"`
}

type channelInput struct {
	Kind    string               `json:"kind"`
	Name    string               `json:"name"`
	Enabled *bool                `json:"enabled"`
	Config  alerts.ChannelConfig `json:"config"`
}

func (s *Server) channelView(c store.AlertChannel) (channelView, error) {
	cfg, err := alerts.OpenConfig(s.alertBox, c.Config)
	if err != nil {
		return channelView{}, err
	}
	return channelView{AlertChannel: c, Config: cfg.Mask()}, nil
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request, _ *principal) {
	list, err := s.store.ListAlertChannels(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := make([]channelView, 0, len(list))
	for _, c := range list {
		v, err := s.channelView(c)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out, "kinds": alerts.Kinds})
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	if !slices.Contains(alerts.Kinds, in.Kind) {
		writeError(w, http.StatusBadRequest, "kind must be one of "+strings.Join(alerts.Kinds, ", "))
		return
	}
	s.saveChannel(w, r, p, store.AlertChannel{Kind: in.Kind, Enabled: true}, alerts.ChannelConfig{}, in)
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	c, ok := s.channelFromPath(w, r)
	if !ok {
		return
	}
	old, err := alerts.OpenConfig(s.alertBox, c.Config)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	if in.Kind != "" && in.Kind != c.Kind {
		writeError(w, http.StatusBadRequest, "the kind of a channel cannot change")
		return
	}
	s.saveChannel(w, r, p, c, old, in)
}

func (s *Server) saveChannel(w http.ResponseWriter, r *http.Request, p *principal, c store.AlertChannel, old alerts.ChannelConfig, in channelInput) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 60 {
		writeError(w, http.StatusBadRequest, "a name of 1–60 characters is required")
		return
	}
	cfg := in.Config.KeepSecrets(old)
	for _, v := range []string{cfg.Token, cfg.BotToken, cfg.Password, cfg.Secret} {
		if v == alerts.Masked {
			writeError(w, http.StatusBadRequest, "enter the secret again")
			return
		}
	}
	if err := cfg.Validate(c.Kind); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sealed, err := alerts.SealConfig(s.alertBox, cfg)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	c.Name, c.Config = name, sealed
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	status, action := http.StatusOK, "alerts.channel.update"
	if c.ID == 0 {
		c, err = s.store.CreateAlertChannel(r.Context(), c)
		status, action = http.StatusCreated, "alerts.channel.create"
	} else {
		c, err = s.store.UpdateAlertChannel(r.Context(), c)
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, action, strconv.FormatInt(c.ID, 10), map[string]any{"kind": c.Kind, "name": c.Name, "enabled": c.Enabled})
	v, err := s.channelView(c)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) channelFromPath(w http.ResponseWriter, r *http.Request) (store.AlertChannel, bool) {
	id, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "channel not found")
		return store.AlertChannel{}, false
	}
	c, err := s.store.GetAlertChannel(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "channel not found")
		return c, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return c, false
	}
	return c, true
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	c, ok := s.channelFromPath(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAlertChannel(r.Context(), c.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "alerts.channel.delete", strconv.FormatInt(c.ID, 10), map[string]any{"kind": c.Kind, "name": c.Name})
	w.WriteHeader(http.StatusNoContent)
}

// testChannel sends a test message; the error of the channel is returned as is (it helps fixing tokens).
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request, _ *principal) {
	c, ok := s.channelFromPath(w, r)
	if !ok {
		return
	}
	cfg, err := alerts.OpenConfig(s.alertBox, c.Config)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if c.Kind == alerts.Email && cfg.UsePanelSMTP {
		if cfg, err = alerts.ResolvePanelSMTP(r.Context(), s.panelSMTP, cfg); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	set, _ := alerts.LoadSettings(r.Context(), s.store)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	n := alerts.Notification{Title: "Test from Nectar Apps", Message: "Alerts will arrive here when an app goes down, a disk fills up or a certificate is about to expire.",
		Severity: alerts.Warning, State: "test", Time: time.Now(), Link: set.PanelURL}
	if err := s.alertSender.Send(ctx, c.Kind, cfg, n); err != nil {
		writeError(w, http.StatusBadGateway, c.Kind+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------------------------------------- housekeeping --

func (s *Server) serverDisk(w http.ResponseWriter, r *http.Request, _ *principal) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.keeper.Disk(ctx))
}

func (s *Server) putHousekeeping(w http.ResponseWriter, r *http.Request, p *principal) {
	var in housekeep.Settings
	if !decode(w, r, &in) {
		return
	}
	if in.K3sDataDir == "" {
		in.K3sDataDir = housekeep.Defaults.K3sDataDir
	}
	in.K3sDataDir = strings.TrimRight(in.K3sDataDir, "/")
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetSetting(r.Context(), housekeep.SettingsKey, in); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "housekeeping.update", "", in)
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) serverCleanup(w http.ResponseWriter, r *http.Request, p *principal) {
	res, err := s.keeper.Clean(r.Context())
	if err != nil {
		s.log.Warn("cleanup", "err", err)
	}
	s.audit(r, p, "server.cleanup", "", res)
	out := map[string]any{"result": res}
	if err != nil {
		out["warning"] = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pruneImages(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	var in struct {
		Node string `json:"node"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	jobs, err := s.keeper.PruneImages(r.Context(), in.Node)
	if err != nil && len(jobs) == 0 {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "server.image_cleanup", in.Node, map[string]any{"jobs": len(jobs)})
	writeJSON(w, http.StatusAccepted, map[string]any{"jobs": jobs})
}
