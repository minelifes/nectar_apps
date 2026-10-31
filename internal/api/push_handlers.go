package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/alerts"
	"nectar_apps/internal/push"
)

// Native push notifications (optional): Settings → Push notifications holds a Firebase project (its service
// account key, sealed, and the app ids the Nectar app needs). Signed-in apps register their device token;
// alert channels of kind "push" then reach them.

const pushSettingsKey = "push"

// pushApp is the Firebase app of one platform (public values: they are also inside every app build).
type pushApp struct {
	APIKey   string `json:"api_key"`
	AppID    string `json:"app_id"`
	BundleID string `json:"bundle_id,omitempty"` // apple
}

type pushSettings struct {
	Enabled   bool    `json:"enabled"`
	ProjectID string  `json:"project_id"`
	SenderID  string  `json:"sender_id"`
	Android   pushApp `json:"android"`
	Apple     pushApp `json:"apple"`
	// ServiceAccount is the Firebase service account key (JSON), sealed in storage, never returned.
	ServiceAccount string `json:"service_account,omitempty"`
}

func (s *Server) loadPush(ctx context.Context) (pushSettings, string) {
	var ps pushSettings
	_ = s.store.GetSetting(ctx, pushSettingsKey, &ps)
	sa := ""
	if ps.ServiceAccount != "" && s.pushBox != nil {
		if v, err := s.pushBox.Open(ps.ServiceAccount); err == nil {
			sa = v
		}
	}
	ps.ServiceAccount = ""
	return ps, sa
}

// getPushSettings: GET /api/settings/push (settings.manage).
func (s *Server) getPushSettings(w http.ResponseWriter, r *http.Request, _ *principal) {
	ps, sa := s.loadPush(r.Context())
	email := ""
	if sa != "" {
		if acct, _, err := push.ParseServiceAccount([]byte(sa)); err == nil {
			email = acct.ClientEmail
		}
	}
	devices, _ := s.store.ListPushDevices(r.Context(), 0)
	writeJSON(w, http.StatusOK, map[string]any{"settings": ps, "has_service_account": sa != "", "service_account_email": email, "devices": len(devices)})
}

// putPushSettings: PUT /api/settings/push — an empty service_account keeps the stored key.
func (s *Server) putPushSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	var in pushSettings
	if !decode(w, r, &in) {
		return
	}
	_, oldSA := s.loadPush(r.Context())
	sa := strings.TrimSpace(in.ServiceAccount)
	if sa == "" {
		sa = oldSA
	}
	trim := func(a *pushApp) {
		a.APIKey, a.AppID, a.BundleID = strings.TrimSpace(a.APIKey), strings.TrimSpace(a.AppID), strings.TrimSpace(a.BundleID)
	}
	trim(&in.Android)
	trim(&in.Apple)
	in.ProjectID, in.SenderID = strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.SenderID)
	if sa != "" {
		acct, _, err := push.ParseServiceAccount([]byte(sa))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.ProjectID == "" {
			in.ProjectID = acct.ProjectID
		}
		if in.ProjectID != acct.ProjectID {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("the service account belongs to project %s, not %s", acct.ProjectID, in.ProjectID))
			return
		}
	}
	if in.Enabled {
		switch {
		case sa == "":
			writeError(w, http.StatusBadRequest, "paste the service account key (Firebase console → Project settings → Service accounts → Generate new private key)")
			return
		case in.SenderID == "":
			writeError(w, http.StatusBadRequest, "the sender id is required (Project settings → Cloud Messaging)")
			return
		case in.Android.AppID == "" && in.Apple.AppID == "":
			writeError(w, http.StatusBadRequest, "add the Android or the Apple app (Project settings → Your apps)")
			return
		case (in.Android.AppID != "" && in.Android.APIKey == "") || (in.Apple.AppID != "" && in.Apple.APIKey == ""):
			writeError(w, http.StatusBadRequest, "each app needs its API key and app id")
			return
		}
	}
	stored := in
	stored.ServiceAccount = ""
	if sa != "" {
		sealed, err := s.pushBox.Seal(sa)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		stored.ServiceAccount = sealed
	}
	if err := s.store.SetSetting(r.Context(), pushSettingsKey, stored); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "settings.push", "", map[string]any{"enabled": in.Enabled, "project": in.ProjectID})
	s.getPushSettings(w, r, p)
}

// pushConfig: GET /api/push/config (signed in) — what the app needs to register (Firebase options).
func (s *Server) pushConfig(w http.ResponseWriter, r *http.Request, _ *principal) {
	ps, sa := s.loadPush(r.Context())
	if !ps.Enabled || sa == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "project_id": ps.ProjectID, "sender_id": ps.SenderID, "android": ps.Android, "apple": ps.Apple})
}

// listMyPushDevices: GET /api/me/push-devices.
func (s *Server) listMyPushDevices(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.store.ListPushDevices(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// registerPushDevice: PUT /api/me/push-devices {token, platform, name}.
func (s *Server) registerPushDevice(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
		Name     string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if len(in.Token) < 20 || len(in.Token) > 4096 || strings.ContainsAny(in.Token, " \n") {
		writeError(w, http.StatusBadRequest, "invalid device token")
		return
	}
	if !slices.Contains([]string{"android", "ios", "macos"}, in.Platform) {
		writeError(w, http.StatusBadRequest, "platform must be android, ios or macos")
		return
	}
	if len(in.Name) > 100 {
		in.Name = in.Name[:100]
	}
	if err := s.store.RegisterPushDevice(r.Context(), p.user.ID, in.Token, in.Platform, in.Name); err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// unregisterPushDevice: POST /api/me/push-devices/unregister {token} (the app signs out).
func (s *Server) unregisterPushDevice(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.store.DeletePushDevice(r.Context(), p.user.ID, in.Token); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMyPushDevice: DELETE /api/me/push-devices/{id}.
func (s *Server) deleteMyPushDevice(w http.ResponseWriter, r *http.Request, p *principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.store.DeletePushDeviceID(r.Context(), p.user.ID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testPush: POST /api/settings/push/test — sends a test to the caller's own devices.
func (s *Server) testPush(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	n := alerts.Notification{Title: "Test from Nectar Apps", Message: "Push notifications work. Add a Mobile app channel under Alerts to get alerts here.",
		Severity: alerts.Warning, State: "test", Time: time.Now()}
	sent, err := s.sendPush(ctx, []int64{p.user.ID}, n)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
}

// SendPush delivers an alert to the recipients of a push channel: the users listed in To (emails), or every
// active user who can see the server. Used by the alert engine (alerts.Sender.Push).
func (s *Server) SendPush(ctx context.Context, c alerts.ChannelConfig, n alerts.Notification) error {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, e := range c.Recipients() {
		want[strings.ToLower(e)] = true
	}
	var ids []int64
	for _, u := range users {
		if u.Disabled {
			continue
		}
		if len(want) > 0 {
			if want[strings.ToLower(u.Email)] {
				ids = append(ids, u.ID)
			}
			continue
		}
		if eff, err := s.effective(ctx, u); err == nil && eff.Can("server.view", nil) {
			ids = append(ids, u.ID)
		}
	}
	_, err = s.sendPush(ctx, ids, n)
	return err
}

func (s *Server) sendPush(ctx context.Context, userIDs []int64, n alerts.Notification) (int, error) {
	ps, sa := s.loadPush(ctx)
	if !ps.Enabled || sa == "" {
		return 0, errors.New("push notifications are off (Settings → Push notifications)")
	}
	acct, key, err := push.ParseServiceAccount([]byte(sa))
	if err != nil {
		return 0, err
	}
	devices, err := s.store.ListPushDevices(ctx, 0)
	if err != nil {
		return 0, err
	}
	m := push.Message{Title: n.Title, Body: n.Message, Data: map[string]string{"state": n.State, "severity": n.Severity}}
	if n.Subject != "" {
		m.Title = n.Title + " · " + n.Subject
	}
	if n.Link != "" {
		if u, err := url.Parse(n.Link); err == nil && u.Path != "" {
			m.Data["path"] = u.Path // the app opens this page when the notification is tapped
		}
	}
	sent, failed := 0, []string{}
	targets := 0
	for _, d := range devices {
		if !slices.Contains(userIDs, d.UserID) {
			continue
		}
		targets++
		err := s.pushClient.Send(ctx, acct, key, d.Token, m)
		switch {
		case err == nil:
			sent++
		case errors.Is(err, push.ErrUnregistered):
			_ = s.store.DeletePushDevice(ctx, 0, d.Token)
		default:
			failed = append(failed, err.Error())
		}
	}
	if targets == 0 {
		return 0, errors.New("no device has registered: sign in to the Nectar app on your phone (with push allowed)")
	}
	if sent == 0 && len(failed) > 0 {
		return 0, errors.New(failed[0])
	}
	return sent, nil
}
