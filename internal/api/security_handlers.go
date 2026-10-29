package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/store"
)

// ------------------------------------------------------- two-factor login --

// mfaState holds pending second-factor logins and the last used TOTP step per user (no code twice).
type mfaState struct {
	mu        sync.Mutex
	pending   map[string]mfaPending // token hash → login waiting for its code
	usedSteps map[int64]int64
}

type mfaPending struct {
	userID  int64
	client  string
	expires time.Time
	tries   int
}

func (m *mfaState) start(userID int64, client string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		m.pending = map[string]mfaPending{}
	}
	now := time.Now()
	for k, p := range m.pending {
		if now.After(p.expires) {
			delete(m.pending, k)
		}
	}
	plain, hash := auth.NewToken("nmfa_")
	m.pending[hash] = mfaPending{userID: userID, client: client, expires: now.Add(5 * time.Minute)}
	return plain
}

// get returns the pending login of a token and counts the attempt (5 per token).
func (m *mfaState) get(token string) (mfaPending, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := auth.HashToken(token)
	p, ok := m.pending[h]
	if !ok || time.Now().After(p.expires) || p.tries >= 5 {
		delete(m.pending, h)
		return p, false
	}
	p.tries++
	m.pending[h] = p
	return p, true
}

func (m *mfaState) done(token string) {
	m.mu.Lock()
	delete(m.pending, auth.HashToken(token))
	m.mu.Unlock()
}

// useStep records a TOTP step; false when the code was used already (replay).
func (m *mfaState) useStep(userID, step int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.usedSteps == nil {
		m.usedSteps = map[int64]int64{}
	}
	if m.usedSteps[userID] >= step {
		return false
	}
	m.usedSteps[userID] = step
	return true
}

// checkSecondFactor accepts a TOTP code or an unused recovery code.
func (s *Server) checkSecondFactor(ctx context.Context, u store.User, code string) (ok, recovery bool, err error) {
	sealed, enabled, _, err := s.store.MFA(ctx, u.ID)
	if err != nil || !enabled {
		return false, false, err
	}
	secret, err := s.mfaBox.Open(sealed)
	if err != nil {
		return false, false, err
	}
	if step := auth.VerifyTOTP(secret, code, time.Now()); step != 0 {
		return s.mfa.useStep(u.ID, step), false, nil
	}
	if len(strings.TrimSpace(code)) >= 8 {
		used, err := s.store.UseRecoveryCode(ctx, u.ID, auth.HashRecoveryCode(code))
		return used, used, err
	}
	return false, false, nil
}

// loginMFA completes a login with the second factor.
func (s *Server) loginMFA(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"mfa_token"`
		Code  string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	pend, ok := s.mfa.get(in.Token)
	if !ok {
		writeError(w, http.StatusUnauthorized, "the sign-in expired: enter your password again")
		return
	}
	u, err := s.store.GetUser(r.Context(), pend.userID)
	if err != nil || u.Disabled {
		writeError(w, http.StatusUnauthorized, "the sign-in expired: enter your password again")
		return
	}
	ok, recovery, err := s.checkSecondFactor(r.Context(), u, in.Code)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		s.limiter.fail(ip)
		s.audit(r, &principal{user: u}, "auth.mfa_failed", u.Email, nil)
		writeError(w, http.StatusUnauthorized, "wrong code")
		return
	}
	s.limiter.reset(ip)
	s.mfa.done(in.Token)
	if recovery {
		s.audit(r, &principal{user: u}, "auth.recovery_code_used", u.Email, nil)
	}
	s.audit(r, &principal{user: u}, "auth.login", pend.client, map[string]bool{"mfa": true})
	s.startSession(w, r, u, pend.client)
}

// ---------------------------------------------------------- 2FA settings --

func (s *Server) mfaStatus(w http.ResponseWriter, r *http.Request, p *principal) {
	_, enabled, codes, err := s.store.MFA(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "recovery_codes_left": len(codes)})
}

// mfaSetup creates a new secret (not active until confirmed with a code).
func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request, p *principal) {
	_, enabled, _, err := s.store.MFA(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is on already: turn it off first to use another device")
		return
	}
	secret := auth.NewTOTPSecret()
	sealed, err := s.mfaBox.Seal(secret)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.SetMFA(r.Context(), p.user.ID, sealed, false, nil); err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauth_url": auth.TOTPURL("Nectar Apps", p.user.Email, secret)})
}

func (s *Server) mfaEnable(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	sealed, enabled, _, err := s.store.MFA(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if enabled || sealed == "" {
		writeError(w, http.StatusConflict, "start the setup first")
		return
	}
	secret, err := s.mfaBox.Open(sealed)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	step := auth.VerifyTOTP(secret, in.Code, time.Now())
	if step == 0 {
		writeError(w, http.StatusBadRequest, "wrong code: check that the phone's clock is right")
		return
	}
	s.mfa.useStep(p.user.ID, step)
	plain, hashes := auth.NewRecoveryCodes(10)
	if err := s.store.SetMFA(r.Context(), p.user.ID, sealed, true, hashes); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "auth.mfa_enabled", p.user.Email, nil)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": plain})
}

// verifyPassword re-checks the password for sensitive changes.
func (s *Server) verifyPassword(w http.ResponseWriter, r *http.Request, p *principal, password string) bool {
	u, err := s.store.GetUser(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return false
	}
	if ok, _, err := auth.VerifyPassword(password, u.PasswordHash); err != nil || !ok {
		s.limiter.fail(clientIP(r))
		writeError(w, http.StatusForbidden, "wrong password")
		return false
	}
	return true
}

func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) || !s.verifyPassword(w, r, p, in.Password) {
		return
	}
	if err := s.store.SetMFA(r.Context(), p.user.ID, "", false, nil); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "auth.mfa_disabled", p.user.Email, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mfaRecoveryCodes(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) || !s.verifyPassword(w, r, p, in.Password) {
		return
	}
	sealed, enabled, _, err := s.store.MFA(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is off")
		return
	}
	plain, hashes := auth.NewRecoveryCodes(10)
	if err := s.store.SetMFA(r.Context(), p.user.ID, sealed, true, hashes); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "auth.recovery_codes_renewed", p.user.Email, nil)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": plain})
}

// resetUserMFA turns 2FA off for another user who lost their device (only owners can do it for owners).
func (s *Server) resetUserMFA(w http.ResponseWriter, r *http.Request, p *principal) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	u, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if u.IsOwner && !p.user.IsOwner {
		writeError(w, http.StatusForbidden, "only owners can reset the two-factor authentication of an owner")
		return
	}
	if err := s.store.SetMFA(r.Context(), u.ID, "", false, nil); err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), u.ID, 0)
	s.audit(r, p, "user.mfa_reset", u.Email, nil)
	w.WriteHeader(http.StatusNoContent)
}

// --------------------------------------------------------- IP allowlist --

// AccessSettings restricts who can reach the panel (settings key "access").
type AccessSettings struct {
	IPAllowlist []string `json:"ip_allowlist"` // IPs or CIDRs; empty = everyone
}

const accessKey = "access"

type allowlist struct {
	mu       sync.Mutex
	prefixes []netip.Prefix
	loaded   time.Time
}

func parseAllowlist(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range list {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "/") {
			a, err := netip.ParseAddr(v)
			if err != nil {
				return nil, errors.New("not an IP or CIDR: " + v)
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		pf, err := netip.ParsePrefix(v)
		if err != nil {
			return nil, errors.New("not an IP or CIDR: " + v)
		}
		out = append(out, pf.Masked())
	}
	return out, nil
}

func (s *Server) allowed(ctx context.Context) []netip.Prefix {
	a := &s.access
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.loaded) > 30*time.Second {
		var st AccessSettings
		_ = s.store.GetSetting(ctx, accessKey, &st)
		a.prefixes, _ = parseAllowlist(st.IPAllowlist)
		a.loaded = time.Now()
	}
	return a.prefixes
}

// directLocal reports a request that did not pass a proxy and comes from a private or loopback address
// (an SSH tunnel to the node port, kubectl port-forward, pods): it always passes the allowlist, so a
// wrong list can be fixed through the tunnel.
func directLocal(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	return err == nil && (a.IsLoopback() || a.IsPrivate())
}

// ipAllowlist blocks requests from addresses outside the allowlist (health checks and artifact downloads
// by pods excepted).
func (s *Server) ipAllowlist(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// push webhooks come from GitHub/GitLab and are verified by their signature
		if r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/internal/") || strings.HasPrefix(r.URL.Path, "/api/hooks/") || directLocal(r) {
			next.ServeHTTP(w, r)
			return
		}
		list := s.allowed(r.Context())
		if len(list) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		if ip, err := netip.ParseAddr(clientIP(r)); err == nil {
			ip = ip.Unmap()
			for _, pf := range list {
				if pf.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		writeError(w, http.StatusForbidden, "this address may not use the panel")
	})
}

func (s *Server) getAccess(w http.ResponseWriter, r *http.Request, _ *principal) {
	var st AccessSettings
	_ = s.store.GetSetting(r.Context(), accessKey, &st)
	if st.IPAllowlist == nil {
		st.IPAllowlist = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ip_allowlist": st.IPAllowlist, "your_ip": clientIP(r), "direct": directLocal(r)})
}

func (s *Server) putAccess(w http.ResponseWriter, r *http.Request, p *principal) {
	var in AccessSettings
	if !decode(w, r, &in) {
		return
	}
	if len(in.IPAllowlist) > 100 {
		writeError(w, http.StatusBadRequest, "at most 100 entries")
		return
	}
	prefixes, err := parseAllowlist(in.IPAllowlist)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(prefixes) > 0 && !directLocal(r) {
		ip, err := netip.ParseAddr(clientIP(r))
		inside := false
		for _, pf := range prefixes {
			if err == nil && pf.Contains(ip.Unmap()) {
				inside = true
			}
		}
		if !inside {
			writeError(w, http.StatusBadRequest, "your address "+clientIP(r)+" is not in the list: you would lock yourself out")
			return
		}
	}
	clean := make([]string, 0, len(prefixes))
	for _, pf := range prefixes {
		if pf.IsSingleIP() {
			clean = append(clean, pf.Addr().String())
		} else {
			clean = append(clean, pf.String())
		}
	}
	if err := s.store.SetSetting(r.Context(), accessKey, AccessSettings{IPAllowlist: clean}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.access.mu.Lock()
	s.access.loaded = time.Time{}
	s.access.mu.Unlock()
	s.audit(r, p, "settings.ip_allowlist", "", map[string]any{"entries": clean})
	writeJSON(w, http.StatusOK, map[string]any{"ip_allowlist": clean, "your_ip": clientIP(r), "direct": directLocal(r)})
}
