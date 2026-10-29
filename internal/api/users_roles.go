package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/authz"
	"nectar_apps/internal/store"
)

func contextTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// ------------------------------------------------------------------- users --

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, _ *principal) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		// Invite emails a link to choose the password (no password needed); without email the link is returned.
		Invite bool `json:"invite"`
	}
	if !decode(w, r, &in) {
		return
	}
	pw := in.Password
	if in.Invite && pw == "" {
		pw = "placeholder-not-used" // validated below like a real one; replaced by an unusable hash
	}
	if msg := validateUserInput(in.Email, in.Name, pw); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var hash string
	var err error
	if in.Invite && in.Password == "" {
		hash, err = unusablePassword()
	} else {
		hash, err = auth.HashPassword(in.Password)
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	u, err := s.store.CreateUser(r.Context(), in.Email, in.Name, hash, false)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a user with this email already exists")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "user.create", u.Email, nil)
	if !in.Invite {
		writeJSON(w, http.StatusCreated, u)
		return
	}
	link, sent, err := s.sendUserLink(r.Context(), r, u, "invite")
	out := struct {
		store.User
		Emailed   bool   `json:"emailed"`
		InviteURL string `json:"invite_url,omitempty"`
		Warning   string `json:"warning,omitempty"`
	}{User: u, Emailed: sent}
	if !sent {
		out.InviteURL = link
	}
	if err != nil {
		out.Warning = "the invitation email failed: " + err.Error()
	}
	s.audit(r, p, "user.invite", u.Email, map[string]bool{"emailed": sent})
	writeJSON(w, http.StatusCreated, out)
}

// targetUser loads the {id} user and enforces that only owners can modify owners.
func (s *Server) targetUser(w http.ResponseWriter, r *http.Request, p *principal) (store.User, bool) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return store.User{}, false
	}
	u, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return u, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return u, false
	}
	if u.IsOwner && !p.eff.Owner {
		writeError(w, http.StatusForbidden, "only owners can change owners")
		return u, false
	}
	return u, true
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r, p)
	if !ok {
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Disabled *bool   `json:"disabled"`
		Password *string `json:"password"`
		IsOwner  *bool   `json:"is_owner"`
	}
	if !decode(w, r, &in) {
		return
	}
	var upd store.UserUpdate
	changed := []string{}
	if in.Name != nil {
		if n := len(strings.TrimSpace(*in.Name)); n == 0 || n > 100 {
			writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
			return
		}
		upd.Name, changed = in.Name, append(changed, "name")
	}
	if in.Password != nil {
		if err := auth.ValidatePassword(*in.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h, err := auth.HashPassword(*in.Password)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		upd.PasswordHash, changed = &h, append(changed, "password")
	}
	if in.Disabled != nil {
		if *in.Disabled && u.ID == p.user.ID {
			writeError(w, http.StatusBadRequest, "you cannot disable yourself")
			return
		}
		upd.Disabled, changed = in.Disabled, append(changed, "disabled")
	}
	if in.IsOwner != nil {
		if !p.eff.Owner {
			writeError(w, http.StatusForbidden, "only owners can grant or revoke owner")
			return
		}
		if err := s.store.SetOwner(r.Context(), u.ID, *in.IsOwner); errors.Is(err, store.ErrLastOwner) {
			writeError(w, http.StatusConflict, err.Error())
			return
		} else if err != nil {
			s.serverError(w, r, err)
			return
		}
		changed = append(changed, "is_owner")
	}
	u, err := s.store.UpdateUser(r.Context(), u.ID, upd)
	if errors.Is(err, store.ErrLastOwner) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// a new password or a disabled account ends all other sessions
	if in.Password != nil || (in.Disabled != nil && *in.Disabled) {
		keep := int64(0)
		if u.ID == p.user.ID && p.session != nil {
			keep = p.session.ID
		}
		_ = s.store.DeleteUserSessions(r.Context(), u.ID, keep)
	}
	s.audit(r, p, "user.update", u.Email, map[string]any{"changed": changed})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r, p)
	if !ok {
		return
	}
	if u.ID == p.user.ID {
		writeError(w, http.StatusBadRequest, "you cannot delete yourself")
		return
	}
	if err := s.store.DeleteUser(r.Context(), u.ID); errors.Is(err, store.ErrLastOwner) {
		writeError(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "user.delete", u.Email, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getBindings(w http.ResponseWriter, r *http.Request, p *principal) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	bs, err := s.store.UserBindings(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

func (s *Server) putBindings(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r, p)
	if !ok {
		return
	}
	var in []store.BindingInput
	if !decode(w, r, &in) {
		return
	}
	if len(in) > 200 {
		writeError(w, http.StatusBadRequest, "too many bindings")
		return
	}
	ctx := r.Context()
	// You can only add or remove bindings whose permissions you hold yourself in that scope.
	check := func(roleID int64, project *int64) (string, int) {
		role, err := s.store.GetRole(ctx, roleID)
		if errors.Is(err, store.ErrNotFound) {
			return "role " + strconv.FormatInt(roleID, 10) + " not found", http.StatusBadRequest
		}
		if err != nil {
			return err.Error(), http.StatusInternalServerError
		}
		if project != nil {
			exists, err := s.store.ProjectExists(ctx, *project)
			if err != nil {
				return err.Error(), http.StatusInternalServerError
			}
			if !exists {
				return "project " + strconv.FormatInt(*project, 10) + " not found", http.StatusBadRequest
			}
		}
		if !p.eff.CanGrant(role.Permissions, project) {
			return "you cannot grant or revoke role " + role.Name + " — it has permissions you do not hold", http.StatusForbidden
		}
		return "", 0
	}
	current, err := s.store.UserBindings(ctx, u.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	key := func(role int64, project *int64) string {
		if project == nil {
			return strconv.FormatInt(role, 10) + "/*"
		}
		return strconv.FormatInt(role, 10) + "/" + strconv.FormatInt(*project, 10)
	}
	desired := map[string]bool{}
	for _, b := range in {
		desired[key(b.RoleID, b.ProjectID)] = true
	}
	have := map[string]bool{}
	for _, b := range current {
		have[key(b.RoleID, b.ProjectID)] = true
		if !desired[key(b.RoleID, b.ProjectID)] { // removal
			if msg, code := check(b.RoleID, b.ProjectID); code != 0 {
				writeError(w, code, msg)
				return
			}
		}
	}
	for _, b := range in {
		if have[key(b.RoleID, b.ProjectID)] {
			continue
		}
		if msg, code := check(b.RoleID, b.ProjectID); code != 0 { // addition
			writeError(w, code, msg)
			return
		}
	}
	out, err := s.store.ReplaceBindings(ctx, u.ID, in)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "unknown role or project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "user.bindings", u.Email, in)
	writeJSON(w, http.StatusOK, out)
}

// ------------------------------------------------------------------- roles --

func (s *Server) permissionCatalog(w http.ResponseWriter, r *http.Request, _ *principal) {
	writeJSON(w, http.StatusOK, authz.Catalog)
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request, _ *principal) {
	roles, err := s.store.ListRoles(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if roles == nil {
		roles = []store.Role{}
	}
	writeJSON(w, http.StatusOK, roles)
}

type roleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) readRole(w http.ResponseWriter, r *http.Request) (roleInput, bool) {
	var in roleInput
	if !decode(w, r, &in) {
		return in, false
	}
	if n := len(strings.TrimSpace(in.Name)); n == 0 || n > 64 {
		writeError(w, http.StatusBadRequest, "name must be 1–64 characters")
		return in, false
	}
	if len(in.Description) > 300 {
		writeError(w, http.StatusBadRequest, "description too long")
		return in, false
	}
	if unknown := authz.Validate(in.Permissions); unknown != nil {
		writeError(w, http.StatusBadRequest, "unknown permissions: "+strings.Join(unknown, ", "))
		return in, false
	}
	if in.Permissions == nil {
		in.Permissions = []string{}
	}
	return in, true
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request, p *principal) {
	in, ok := s.readRole(w, r)
	if !ok {
		return
	}
	role, err := s.store.CreateRole(r.Context(), in.Name, in.Description, in.Permissions)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a role with this name already exists")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "role.create", role.Name, in)
	writeJSON(w, http.StatusCreated, role)
}

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request, p *principal) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	in, ok := s.readRole(w, r)
	if !ok {
		return
	}
	role, err := s.store.UpdateRole(r.Context(), id, in.Name, in.Description, in.Permissions)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "role not found")
	case errors.Is(err, store.ErrBuiltinRole):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "a role with this name already exists")
	case err != nil:
		s.serverError(w, r, err)
	default:
		s.audit(r, p, "role.update", role.Name, in)
		writeJSON(w, http.StatusOK, role)
	}
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request, p *principal) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	err := s.store.DeleteRole(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "role not found")
	case errors.Is(err, store.ErrBuiltinRole):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrRoleInUse):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		s.serverError(w, r, err)
	default:
		s.audit(r, p, "role.delete", strconv.FormatInt(id, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// ------------------------------------------------------------------- audit --

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request, _ *principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	entries, err := s.store.ListAudit(r.Context(), limit, before)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
