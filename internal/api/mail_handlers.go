package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"time"

	"nectar_apps/internal/alerts"
	"nectar_apps/internal/auth"
	mailer "nectar_apps/internal/mail"
	"nectar_apps/internal/store"
)

const (
	resetTTL  = time.Hour
	inviteTTL = 7 * 24 * time.Hour
)

// ------------------------------------------------------------ SMTP settings --

func (s *Server) getSMTP(w http.ResponseWriter, r *http.Request, _ *principal) {
	st, err := s.mailer.Load(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if st.Password != "" {
		st.Password = alerts.Masked
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": st, "configured": st.Configured()})
}

func (s *Server) putSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	var in mailer.Settings
	if !decode(w, r, &in) {
		return
	}
	old, err := s.mailer.Load(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if in.Password == alerts.Masked {
		in.Password = old.Password
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.mailer.Save(r.Context(), in); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "settings.smtp", "", map[string]any{"host": in.Host, "from": in.From})
	if in.Password != "" {
		in.Password = alerts.Masked
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": in, "configured": in.Configured()})
}

func (s *Server) testSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		To string `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.To == "" {
		in.To = p.user.Email
	}
	if _, err := mail.ParseAddress(in.To); err != nil {
		writeError(w, http.StatusBadRequest, "invalid address")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	err := s.mailer.Send(ctx, in.To, "Test from Nectar Apps", "Email works. Password resets, invitations and alert emails will be sent like this.")
	if errors.Is(err, mailer.ErrNotConfigured) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "smtp: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// panelSMTP is the panel's SMTP server as an alert channel config (for "use the panel's SMTP" channels).
func (s *Server) panelSMTP(ctx context.Context) (alerts.ChannelConfig, error) {
	st, err := s.mailer.Load(ctx)
	if err != nil {
		return alerts.ChannelConfig{}, err
	}
	return st.Channel(), nil
}

// panelURL is the base of links in emails: the configured address, else the address of this request.
func (s *Server) panelURL(ctx context.Context, r *http.Request) string {
	if st, err := s.mailer.Load(ctx); err == nil && st.PanelURL != "" {
		return st.PanelURL
	}
	if set, err := alerts.LoadSettings(ctx, s.store); err == nil && set.PanelURL != "" {
		return set.PanelURL
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host
}

// -------------------------------------------------- password reset, invites --

// sendUserLink creates a one-time token and emails the link (or returns it when email is off).
func (s *Server) sendUserLink(ctx context.Context, r *http.Request, u store.User, purpose string) (link string, sent bool, err error) {
	plain, hash := auth.NewToken("nrst_")
	ttl := resetTTL
	if purpose == "invite" {
		ttl = inviteTTL
	}
	if err := s.store.CreateUserToken(ctx, u.ID, hash, purpose, ttl); err != nil {
		return "", false, err
	}
	link = s.panelURL(ctx, r) + "/reset?token=" + plain
	subject, body := "Reset your Nectar Apps password", "Someone (hopefully you) asked to reset the password of "+u.Email+
		".\n\nChoose a new password here within the next hour:\n"+link+"\n\nIf it was not you, ignore this email: your password stays as it is."
	if purpose == "invite" {
		subject = "You are invited to Nectar Apps"
		body = "Hello " + u.Name + ",\n\nyou have an account on the Nectar Apps panel at " + s.panelURL(ctx, r) +
			".\n\nChoose your password here (the link works for 7 days):\n" + link
	}
	sctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	err = s.mailer.Send(sctx, u.Email, subject, body)
	if errors.Is(err, mailer.ErrNotConfigured) {
		return link, false, nil
	}
	return link, err == nil, err
}

// forgotPassword emails a reset link. It answers the same whether the account exists or not.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	s.limiter.fail(ip) // every request counts: no flooding of inboxes
	st, _ := s.mailer.Load(r.Context())
	if !st.Configured() {
		writeError(w, http.StatusConflict, "this panel cannot send emails: ask an administrator, or reset it on the server with nectar-apps admin reset-password")
		return
	}
	if u, err := s.store.GetUserByEmail(r.Context(), in.Email); err == nil && !u.Disabled {
		if _, _, err := s.sendUserLink(r.Context(), r, u, "reset"); err != nil {
			s.log.Warn("password reset email failed", "err", err)
		} else {
			s.audit(r, &principal{user: u}, "auth.reset_requested", u.Email, nil)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkUserToken tells the reset page whose password it sets.
func (s *Server) checkUserToken(w http.ResponseWriter, r *http.Request) {
	u, purpose, err := s.store.PeekUserToken(r.Context(), auth.HashToken(r.URL.Query().Get("token")))
	if err != nil {
		writeError(w, http.StatusNotFound, "this link is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": u.Email, "name": u.Name, "purpose": purpose})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, purpose, err := s.store.ConsumeUserToken(r.Context(), auth.HashToken(in.Token))
	if err != nil {
		writeError(w, http.StatusNotFound, "this link is invalid or expired")
		return
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if _, err := s.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{PasswordHash: &h}); err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), u.ID, 0)
	action := "auth.password_reset"
	if purpose == "invite" {
		action = "auth.invite_accepted"
	}
	s.audit(r, &principal{user: u}, action, u.Email, nil)
	writeJSON(w, http.StatusOK, map[string]any{"email": u.Email})
}

// inviteUser (re)sends the invitation of a user; without email the link is returned to share by hand.
func (s *Server) inviteUser(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r, p)
	if !ok {
		return
	}
	link, sent, err := s.sendUserLink(r.Context(), r, u, "invite")
	if err != nil && !sent {
		writeError(w, http.StatusBadGateway, "the invitation email failed: "+err.Error())
		return
	}
	s.audit(r, p, "user.invite", u.Email, map[string]bool{"emailed": sent})
	out := map[string]any{"emailed": sent}
	if !sent {
		out["invite_url"] = link
	}
	writeJSON(w, http.StatusOK, out)
}

// unusablePassword is the hash of invited users until they choose a password.
func unusablePassword() (string, error) {
	plain, _ := auth.NewToken("x")
	return auth.HashPassword(plain)
}
