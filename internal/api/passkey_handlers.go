package api

import (
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/store"
	"nectar_apps/internal/webauthn"
)

// Passkeys (WebAuthn): a signed-in user adds passkeys under Settings → Security; the login page signs in
// with one (no email, no password, no second factor — the passkey is both). A passkey belongs to the
// panel's host name: one created at panel.example.com does not work at another address of the panel.

type passkeyFlow struct {
	challenge string
	userID    int64 // 0 = a sign-in
	rp        webauthn.RP
	expires   time.Time
}

type passkeyFlows struct {
	mu    sync.Mutex
	flows map[string]passkeyFlow
}

func (f *passkeyFlows) put(fl passkeyFlow) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.flows == nil {
		f.flows = map[string]passkeyFlow{}
	}
	now := time.Now()
	for k, v := range f.flows {
		if now.After(v.expires) {
			delete(f.flows, k)
		}
	}
	if len(f.flows) > 10000 { // never grows without bound (anonymous sign-in starts)
		for k := range f.flows {
			delete(f.flows, k)
			break
		}
	}
	id := webauthn.NewChallenge()
	f.flows[id] = fl
	return id
}

func (f *passkeyFlows) take(id string) (passkeyFlow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl, ok := f.flows[id]
	delete(f.flows, id)
	return fl, ok && time.Now().Before(fl.expires)
}

// passkeyRP is the relying party for this request: the host the browser uses.
func passkeyRP(r *http.Request) webauthn.RP {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return webauthn.RP{ID: strings.ToLower(host), Name: "Nectar Apps", Origin: panelBase(r)}
}

func userHandle(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

// passkeyOptions: POST /api/me/passkeys/options — options to create a passkey for the signed-in user.
func (s *Server) passkeyOptions(w http.ResponseWriter, r *http.Request, p *principal) {
	rp := passkeyRP(r)
	existing, err := s.store.ListPasskeys(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var exclude [][]byte
	for _, k := range existing {
		exclude = append(exclude, k.CredentialID)
	}
	ch := webauthn.NewChallenge()
	flow := s.passkeys.put(passkeyFlow{challenge: ch, userID: p.user.ID, rp: rp, expires: time.Now().Add(5 * time.Minute)})
	name := p.user.Name
	if name == "" {
		name = p.user.Email
	}
	writeJSON(w, http.StatusOK, map[string]any{"flow": flow,
		"options": webauthn.CreationOptions(rp, webauthn.User{ID: userHandle(p.user.ID), Name: p.user.Email, DisplayName: name}, ch, exclude)})
}

// addPasskey: POST /api/me/passkeys {flow, name, credential} — stores the passkey created with the options.
func (s *Server) addPasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Flow       string                        `json:"flow"`
		Name       string                        `json:"name"`
		Credential webauthn.RegistrationResponse `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	fl, ok := s.passkeys.take(in.Flow)
	if !ok || fl.userID != p.user.ID {
		writeError(w, http.StatusBadRequest, "the passkey request expired — try again")
		return
	}
	cred, err := webauthn.FinishRegistration(fl.rp, fl.challenge, in.Credential)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Passkey"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	pk, err := s.store.AddPasskey(r.Context(), store.Passkey{UserID: p.user.ID, CredentialID: cred.ID, PublicKey: cred.PublicKey, SignCount: cred.SignCount,
		AAGUID: cred.AAGUID, RPID: fl.rp.ID, Name: name, BackupEligible: cred.BackupEligible, Transports: cred.Transports})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this passkey is already registered")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "user.passkey.add", p.user.Email, map[string]any{"name": name, "host": fl.rp.ID})
	writeJSON(w, http.StatusCreated, pk)
}

// listPasskeys: GET /api/me/passkeys.
func (s *Server) listPasskeys(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.store.ListPasskeys(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// renamePasskey: PATCH /api/me/passkeys/{pk} {name}.
func (s *Server) renamePasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	id, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
	var in struct {
		Name string `json:"name"`
	}
	if err != nil || !decode(w, r, &in) {
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
		}
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 60 {
		writeError(w, http.StatusBadRequest, "name: 1–60 characters")
		return
	}
	if err := s.store.RenamePasskey(r.Context(), p.user.ID, id, name); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletePasskey: DELETE /api/me/passkeys/{pk}.
func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	id, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	n, err := s.store.DeletePasskeys(r.Context(), p.user.ID, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	s.audit(r, p, "user.passkey.delete", p.user.Email, map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

// passkeyLoginOptions: POST /api/auth/passkey/options (public) — options to sign in with any passkey of
// this host.
func (s *Server) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	rp := passkeyRP(r)
	ch := webauthn.NewChallenge()
	flow := s.passkeys.put(passkeyFlow{challenge: ch, rp: rp, expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, http.StatusOK, map[string]any{"flow": flow, "options": webauthn.RequestOptions(rp, ch)})
}

// passkeyLogin: POST /api/auth/passkey {flow, credential} (public) — starts a web session.
func (s *Server) passkeyLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	var in struct {
		Flow       string                     `json:"flow"`
		Credential webauthn.AssertionResponse `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	fail := func(code int, msg string) {
		s.limiter.fail(ip)
		writeError(w, code, msg)
	}
	fl, ok := s.passkeys.take(in.Flow)
	if !ok || fl.userID != 0 {
		writeError(w, http.StatusBadRequest, "the sign-in expired — try again")
		return
	}
	credID, handle, err := in.Credential.CredentialID()
	if err != nil {
		fail(http.StatusUnauthorized, err.Error())
		return
	}
	pk, err := s.store.PasskeyByCredential(r.Context(), credID)
	if errors.Is(err, store.ErrNotFound) {
		fail(http.StatusUnauthorized, "this passkey is not registered here (it may have been removed)")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(handle) > 0 && string(handle) != string(userHandle(pk.UserID)) {
		fail(http.StatusUnauthorized, webauthn.ErrVerify.Error())
		return
	}
	count, err := webauthn.FinishLogin(fl.rp, fl.challenge, in.Credential, webauthn.Credential{ID: pk.CredentialID, PublicKey: pk.PublicKey, SignCount: pk.SignCount})
	if err != nil {
		fail(http.StatusUnauthorized, err.Error())
		return
	}
	u, err := s.store.GetUser(r.Context(), pk.UserID)
	if err != nil {
		fail(http.StatusUnauthorized, "the account no longer exists")
		return
	}
	if u.Disabled {
		fail(http.StatusForbidden, "your account is disabled")
		return
	}
	_ = s.store.TouchPasskey(r.Context(), pk.ID, count)
	s.limiter.reset(ip)
	s.audit(r, &principal{user: u}, "auth.login", "passkey:"+pk.Name, nil)
	s.startSession(w, r, u, "web")
}
