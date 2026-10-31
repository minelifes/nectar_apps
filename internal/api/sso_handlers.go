package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/sso"
	"nectar_apps/internal/store"
)

// Single sign-on (Settings → Sign-in): GitHub, Google or an OpenID Connect provider. The provider proves
// who the user is (a verified email); the panel finds the user by email, or creates one when the provider
// allows it (allowed domains + auto create, roles from default_role / OIDC groups). Sessions are the
// normal web sessions; the panel's own 2FA is not asked — the provider's sign-in (and its MFA) counts.

const (
	ssoSettingsKey = "sso"
	ssoSecretsKey  = "sso_secrets" // provider id → sealed client secret (re-sealed by master key rotation)
	ssoCookie      = "nectar_sso"
)

type ssoFlows struct {
	mu    sync.Mutex
	flows map[string]sso.Flow // state → flow
}

func (f *ssoFlows) put(fl sso.Flow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.flows == nil {
		f.flows = map[string]sso.Flow{}
	}
	now := time.Now().Unix()
	for k, v := range f.flows {
		if v.Expires < now {
			delete(f.flows, k)
		}
	}
	f.flows[fl.State] = fl
}

func (f *ssoFlows) take(state string) (sso.Flow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl, ok := f.flows[state]
	delete(f.flows, state)
	return fl, ok && fl.Expires >= time.Now().Unix()
}

func (s *Server) ssoProviders(ctx context.Context, withSecrets bool) []sso.Provider {
	var ps []sso.Provider
	_ = s.store.GetSetting(ctx, ssoSettingsKey, &ps)
	if !withSecrets {
		return ps
	}
	var secrets map[string]string
	_ = s.store.GetSetting(ctx, ssoSecretsKey, &secrets)
	for i := range ps {
		if enc := secrets[ps[i].ID]; enc != "" && s.ssoBox != nil {
			if v, err := s.ssoBox.Open(enc); err == nil {
				ps[i].ClientSecret = v
			}
		}
	}
	return ps
}

// panelBase is the panel's external URL as the browser sees it.
func panelBase(r *http.Request) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func ssoCallback(r *http.Request, id string) string {
	return panelBase(r) + "/api/auth/sso/" + id + "/callback"
}

// getSSO: GET /api/settings/sso — providers without secrets, with the callback URL to register.
func (s *Server) getSSO(w http.ResponseWriter, r *http.Request, _ *principal) {
	ps := s.ssoProviders(r.Context(), false)
	var secrets map[string]string
	_ = s.store.GetSetting(r.Context(), ssoSecretsKey, &secrets)
	out := []map[string]any{}
	for _, p := range ps {
		out = append(out, map[string]any{"provider": p, "has_secret": secrets[p.ID] != "", "callback_url": ssoCallback(r, p.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out, "callback_base": panelBase(r) + "/api/auth/sso/"})
}

// putSSO: PUT /api/settings/sso [providers] — an empty client_secret keeps the stored one.
func (s *Server) putSSO(w http.ResponseWriter, r *http.Request, p *principal) {
	var in []sso.Provider
	if !decode(w, r, &in) {
		return
	}
	if len(in) > 5 {
		writeError(w, http.StatusBadRequest, "at most 5 providers")
		return
	}
	var secrets map[string]string
	_ = s.store.GetSetting(r.Context(), ssoSecretsKey, &secrets)
	next := map[string]string{}
	seen := map[string]bool{}
	for i := range in {
		pr := &in[i]
		if err := pr.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, pr.ID+": "+err.Error())
			return
		}
		if seen[pr.ID] {
			writeError(w, http.StatusBadRequest, "duplicate provider id "+pr.ID)
			return
		}
		seen[pr.ID] = true
		if r := s.resolveRole(r.Context(), pr.DefaultRole); pr.DefaultRole != "" && r == 0 {
			writeError(w, http.StatusBadRequest, pr.ID+": unknown default_role "+pr.DefaultRole)
			return
		}
		for g, role := range pr.GroupRoles {
			if s.resolveRole(r.Context(), role) == 0 {
				writeError(w, http.StatusBadRequest, pr.ID+": unknown role "+role+" for group "+g)
				return
			}
		}
		switch {
		case pr.ClientSecret != "":
			enc, err := s.ssoBox.Seal(pr.ClientSecret)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			next[pr.ID] = enc
		case secrets[pr.ID] != "":
			next[pr.ID] = secrets[pr.ID]
		case pr.Enabled:
			writeError(w, http.StatusBadRequest, pr.ID+": client_secret is required")
			return
		}
		pr.ClientSecret = ""
	}
	if err := s.store.SetSetting(r.Context(), ssoSecretsKey, next); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.SetSetting(r.Context(), ssoSettingsKey, in); err != nil {
		s.serverError(w, r, err)
		return
	}
	ids := []string{}
	for _, pr := range in {
		ids = append(ids, pr.ID)
	}
	s.audit(r, p, "settings.sso", "", map[string]any{"providers": ids})
	s.getSSO(w, r, p)
}

// resolveRole finds a role by builtin key, name or id (0 = none).
func (s *Server) resolveRole(ctx context.Context, ref string) int64 {
	if ref == "" {
		return 0
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return 0
	}
	for _, r := range roles {
		if r.BuiltinKey == ref || strings.EqualFold(r.Name, ref) || strconv.FormatInt(r.ID, 10) == ref {
			return r.ID
		}
	}
	return 0
}

// listSSO: GET /api/auth/sso (public) — the sign-in buttons.
func (s *Server) listSSO(w http.ResponseWriter, r *http.Request) {
	out := []map[string]string{}
	for _, p := range s.ssoProviders(r.Context(), false) {
		if p.Enabled {
			out = append(out, map[string]string{"id": p.ID, "name": p.Name, "kind": p.Kind})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ssoProvider(r *http.Request) (sso.Provider, bool) {
	for _, p := range s.ssoProviders(r.Context(), true) {
		if p.ID == r.PathValue("id") && p.Enabled {
			return p, true
		}
	}
	return sso.Provider{}, false
}

// safeNext keeps redirects inside the panel.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, "\\") || strings.HasPrefix(next, "/api/") {
		return "/"
	}
	return next
}

// startSSO: GET /api/auth/sso/{id}/start?next=/path — redirects to the provider.
func (s *Server) startSSO(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ssoProvider(r)
	if !ok {
		http.Redirect(w, r, "/login?sso_error="+url.QueryEscape("this sign-in method is not set up"), http.StatusFound)
		return
	}
	authURL, flow, err := s.ssoClient.Start(r.Context(), p, ssoCallback(r, p.ID), safeNext(r.URL.Query().Get("next")))
	if err != nil {
		http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	s.sso.put(flow)
	// the state is bound to this browser: a callback with someone else's state is refused (login CSRF)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: flow.State, Path: "/api/auth/sso/", MaxAge: 600, HttpOnly: true,
		Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// callbackSSO: GET /api/auth/sso/{id}/callback — finishes the sign-in and opens the panel.
func (s *Server) callbackSSO(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string) {
		http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/api/auth/sso/", MaxAge: -1})
		http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(msg), http.StatusFound)
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		fail("the provider cancelled the sign-in: " + strings.TrimSpace(e+" "+q.Get("error_description")))
		return
	}
	c, err := r.Cookie(ssoCookie)
	if err != nil || c.Value == "" || c.Value != q.Get("state") {
		fail("the sign-in expired or was started in another browser — try again")
		return
	}
	flow, ok := s.sso.take(c.Value)
	if !ok || flow.Provider != r.PathValue("id") {
		fail("the sign-in expired — try again")
		return
	}
	p, ok := s.ssoProvider(r)
	if !ok {
		fail("this sign-in method is not set up")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		fail("too many failed attempts, try again later")
		return
	}
	id, err := s.ssoClient.Finish(r.Context(), p, flow, ssoCallback(r, p.ID), q.Get("code"))
	if err != nil {
		s.limiter.fail(ip)
		fail(err.Error())
		return
	}
	if !p.AllowsEmail(id.Email) {
		s.limiter.fail(ip)
		fail(id.Email + " is not allowed to sign in here")
		return
	}
	u, err := s.store.GetUserByEmail(r.Context(), id.Email)
	switch {
	case errors.Is(err, store.ErrNotFound) && p.AutoCreate:
		u, err = s.createSSOUser(r.Context(), p, id)
		if err != nil {
			fail("could not create your account: " + err.Error())
			return
		}
		s.audit(r, &principal{user: u}, "user.create_sso", u.Email, map[string]string{"provider": p.ID})
	case errors.Is(err, store.ErrNotFound):
		s.limiter.fail(ip)
		fail("there is no account for " + id.Email + " — ask an admin to invite you")
		return
	case err != nil:
		fail("sign-in failed")
		return
	}
	if u.Disabled {
		fail("your account is disabled")
		return
	}
	if p.AutoCreate && len(p.GroupRoles) > 0 && !u.IsOwner {
		s.applyGroupRoles(r.Context(), u, p, id.Groups)
	}
	s.limiter.reset(ip)
	ns := store.NewSession{UserID: u.ID, Kind: "web", UserAgent: r.UserAgent(), IP: ip}
	var plain string
	plain, ns.TokenHash = auth.NewToken(auth.PrefixSession)
	ns.TokenTTL = webSessionTTL
	se, err := s.store.CreateSession(r.Context(), ns)
	if err != nil {
		fail("sign-in failed")
		return
	}
	s.setSessionCookie(w, r, plain, se.TokenExpiresAt)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/api/auth/sso/", MaxAge: -1})
	s.audit(r, &principal{user: u}, "auth.login", "sso:"+p.ID, nil)
	http.Redirect(w, r, flow.Next, http.StatusFound)
}

func (s *Server) createSSOUser(ctx context.Context, p sso.Provider, id sso.Identity) (store.User, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	hash, err := auth.HashPassword(base64.RawURLEncoding.EncodeToString(b)) // unusable: the user signs in with SSO (or resets it)
	if err != nil {
		return store.User{}, err
	}
	name := strings.TrimSpace(id.Name)
	if name == "" {
		name, _, _ = strings.Cut(id.Email, "@")
	}
	u, err := s.store.CreateUser(ctx, id.Email, name, hash, false)
	if err != nil {
		return u, err
	}
	var bindings []store.BindingInput
	if role := s.resolveRole(ctx, p.DefaultRole); role != 0 {
		bindings = append(bindings, store.BindingInput{RoleID: role})
	}
	for _, g := range id.Groups {
		if role := s.resolveRole(ctx, p.GroupRoles[g]); role != 0 {
			bindings = append(bindings, store.BindingInput{RoleID: role})
		}
	}
	if len(bindings) > 0 {
		if _, err := s.store.ReplaceBindings(ctx, u.ID, bindings); err != nil {
			return u, err
		}
	}
	return u, nil
}

// applyGroupRoles adds the roles of the user's current groups (never removes any).
func (s *Server) applyGroupRoles(ctx context.Context, u store.User, p sso.Provider, groups []string) {
	current, err := s.store.UserBindings(ctx, u.ID)
	if err != nil {
		return
	}
	var in []store.BindingInput
	have := map[int64]bool{}
	for _, b := range current {
		in = append(in, store.BindingInput{RoleID: b.RoleID, ProjectID: b.ProjectID})
		if b.ProjectID == nil {
			have[b.RoleID] = true
		}
	}
	added := false
	for _, g := range groups {
		if role := s.resolveRole(ctx, p.GroupRoles[g]); role != 0 && !have[role] {
			in, have[role], added = append(in, store.BindingInput{RoleID: role}), true, true
		}
	}
	if added {
		_, _ = s.store.ReplaceBindings(ctx, u.ID, in)
	}
}
