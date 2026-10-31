package api

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
	"nectar_apps/internal/version"
)

func hashToken(t string) string { return auth.HashToken(t) }

// ------------------------------------------------------------------ system --

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"status": "ok"}
	if s.kube != nil {
		ctx, cancel := contextTimeout(r, 2*time.Second)
		defer cancel()
		if _, err := s.kube.ServerVersion(ctx); err != nil {
			resp["kubernetes"] = "unavailable"
		} else {
			resp["kubernetes"] = "ok"
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":            version.Version,
		"commit":             version.Commit,
		"api_version":        version.APIVersion,
		"min_client_version": version.MinClientVersion,
		// lets clients notice a wrong server clock (times shown hours off, certificates, 2FA codes)
		"server_time": time.Now().UTC(),
	})
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request, _ *principal) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, kube.ErrNotConfigured.Error())
		return
	}
	nodes, err := s.kube.ListNodes(r.Context())
	if err != nil {
		s.log.Warn("list nodes", "err", err)
		writeError(w, http.StatusServiceUnavailable, "kubernetes API unavailable: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// ------------------------------------------------------------------- setup --

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"required": n == 0})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Client   string `json:"client"` // web (cookie, default) | mobile (token pair, for the native apps)
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Client == "" {
		in.Client = "web"
	}
	if in.Client != "web" && in.Client != "mobile" {
		writeError(w, http.StatusBadRequest, "client must be web or mobile")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	if s.cfg.SetupToken == "" || subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.cfg.SetupToken)) != 1 {
		s.limiter.fail(ip)
		writeError(w, http.StatusForbidden, "invalid setup token")
		return
	}
	if msg := validateUserInput(in.Email, in.Name, in.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	u, err := s.store.CreateFirstOwner(r.Context(), in.Email, in.Name, hash)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, &principal{user: u}, "setup.owner_created", u.Email, nil)
	s.startSession(w, r, u, in.Client)
}

// ------------------------------------------------------------------- login --

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Client   string `json:"client"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Client == "" {
		in.Client = "web"
	}
	if in.Client != "web" && in.Client != "mobile" {
		writeError(w, http.StatusBadRequest, "client must be web or mobile")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	const bad = "invalid email or password"
	u, err := s.store.GetUserByEmail(r.Context(), in.Email)
	if errors.Is(err, store.ErrNotFound) {
		auth.BurnTime(in.Password)
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, bad)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	ok, rehash, err := auth.VerifyPassword(in.Password, u.PasswordHash)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok || u.Disabled {
		s.limiter.fail(ip)
		s.audit(r, &principal{user: u}, "auth.login_failed", u.Email, nil)
		writeError(w, http.StatusUnauthorized, bad)
		return
	}
	s.limiter.reset(ip)
	if rehash {
		if h, err := auth.HashPassword(in.Password); err == nil {
			_, _ = s.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{PasswordHash: &h})
		}
	}
	if u.MFAEnabled {
		// the password was right: the session starts once POST /api/auth/mfa gets the code
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": s.mfa.start(u.ID, in.Client)})
		return
	}
	s.audit(r, &principal{user: u}, "auth.login", in.Client, nil)
	s.startSession(w, r, u, in.Client)
}

type loginResponse struct {
	User            store.User `json:"user"`
	AccessToken     string     `json:"access_token,omitempty"`
	AccessExpiresAt *time.Time `json:"access_expires_at,omitempty"`
	RefreshToken    string     `json:"refresh_token,omitempty"`
}

// startSession creates a web session (cookie) or a mobile token pair and writes the login response.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.User, client string) {
	ns := store.NewSession{UserID: u.ID, Kind: client, UserAgent: r.UserAgent(), IP: clientIP(r)}
	resp := loginResponse{User: u}
	var plain string
	if client == "mobile" {
		var refresh string
		plain, ns.TokenHash = auth.NewToken(auth.PrefixAccess)
		refresh, ns.RefreshHash = auth.NewToken(auth.PrefixRefresh)
		ns.TokenTTL, ns.RefreshTTL = mobileAccessTTL, mobileRefreshTTL
		resp.AccessToken, resp.RefreshToken = plain, refresh
	} else {
		plain, ns.TokenHash = auth.NewToken(auth.PrefixSession)
		ns.TokenTTL = webSessionTTL
	}
	se, err := s.store.CreateSession(r.Context(), ns)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if client == "mobile" {
		resp.AccessExpiresAt = &se.TokenExpiresAt
	} else {
		s.setSessionCookie(w, r, plain, se.TokenExpiresAt)
	}
	status := http.StatusOK
	if r.URL.Path == "/api/setup" {
		status = http.StatusCreated
	}
	writeJSON(w, status, resp)
}

// setSessionCookie marks the cookie Secure only when the browser reached the panel over HTTPS (directly or
// through the gateway). Browsers drop Secure cookies on plain-HTTP pages, so a Secure cookie on
// http://<server-ip>:30480 (the first-run address) would log the owner out right after setup.
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !auth.HasPrefix(in.RefreshToken, auth.PrefixRefresh) {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	access, accessHash := auth.NewToken(auth.PrefixAccess)
	se, u, err := s.store.RotateMobile(r.Context(), auth.HashToken(in.RefreshToken), accessHash, mobileAccessTTL, mobileRefreshTTL)
	if errors.Is(err, store.ErrNotFound) || (err == nil && u.Disabled) {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{User: u, AccessToken: access, AccessExpiresAt: &se.TokenExpiresAt, RefreshToken: in.RefreshToken})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, p *principal) {
	if p.session != nil {
		_ = s.store.DeleteSession(r.Context(), p.user.ID, p.session.ID)
	}
	s.setSessionCookie(w, r, "", time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------- me --

func (s *Server) me(w http.ResponseWriter, r *http.Request, p *principal) {
	writeJSON(w, http.StatusOK, p.user)
}

func (s *Server) myPermissions(w http.ResponseWriter, r *http.Request, p *principal) {
	global, projects := p.eff.Lists()
	writeJSON(w, http.StatusOK, map[string]any{"owner": p.eff.Owner, "global": global, "projects": projects})
}

func (s *Server) mySessions(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.store.ListSessions(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	type item struct {
		store.Session
		Current bool `json:"current"`
	}
	out := make([]item, len(list))
	for i, se := range list {
		out[i] = item{Session: se, Current: p.session != nil && se.ID == p.session.ID}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteMySession(w http.ResponseWriter, r *http.Request, p *principal) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.store.DeleteSession(r.Context(), p.user.ID, id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "auth.session_revoked", "", map[string]int64{"session_id": id})
	w.WriteHeader(http.StatusNoContent)
}

// validateUserInput returns an error message or "".
func validateUserInput(email, name, password string) string {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 || !strings.Contains(email, "@") || strings.ContainsAny(email, " \t\r\n") {
		return "invalid email"
	}
	if n := len(strings.TrimSpace(name)); n == 0 || n > 100 {
		return "name must be 1–100 characters"
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err.Error()
	}
	return ""
}

// requestIsHTTPS reports whether the browser used HTTPS: TLS on this connection, or X-Forwarded-Proto
// "https" from a proxy inside the cluster (the gateway).
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && (addr.IsPrivate() || addr.IsLoopback())
}
