package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"nectar_apps/internal/store"
)

// memberRole is a role a user holds in a project (or everywhere).
type memberRole struct {
	RoleID   int64  `json:"role_id"`
	RoleName string `json:"role_name"`
}

// member is a user with access to a project.
type member struct {
	UserID      int64        `json:"user_id"`
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	IsOwner     bool         `json:"is_owner"`
	Disabled    bool         `json:"disabled"`
	Roles       []memberRole `json:"roles"`        // bound to this project (editable on the Members tab)
	GlobalRoles []memberRole `json:"global_roles"` // bound everywhere (edited on Settings → Users)
}

// projectMembers lists everyone with access to the project: owners, global bindings and project bindings.
func (s *Server) projectMembers(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	out := []member{}
	for _, u := range users {
		bs, err := s.store.UserBindings(r.Context(), u.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		m := member{UserID: u.ID, Name: u.Name, Email: u.Email, IsOwner: u.IsOwner, Disabled: u.Disabled, Roles: []memberRole{}, GlobalRoles: []memberRole{}}
		for _, b := range bs {
			switch {
			case b.ProjectID == nil:
				m.GlobalRoles = append(m.GlobalRoles, memberRole{b.RoleID, b.RoleName})
			case *b.ProjectID == proj.ID:
				m.Roles = append(m.Roles, memberRole{b.RoleID, b.RoleName})
			}
		}
		if u.IsOwner || len(m.Roles) > 0 || len(m.GlobalRoles) > 0 {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsOwner != out[j].IsOwner {
			return out[i].IsOwner
		}
		return out[i].Email < out[j].Email
	})
	writeJSON(w, http.StatusOK, out)
}

// grantCheck refuses changes to bindings whose permissions the caller does not hold in that scope.
func (s *Server) grantCheck(ctx context.Context, p *principal, roleID int64, project *int64) (string, int) {
	role, err := s.store.GetRole(ctx, roleID)
	if errors.Is(err, store.ErrNotFound) {
		return "role " + strconv.FormatInt(roleID, 10) + " not found", http.StatusBadRequest
	}
	if err != nil {
		return err.Error(), http.StatusInternalServerError
	}
	if !p.eff.CanGrant(role.Permissions, project) {
		return "you cannot grant or revoke role " + role.Name + " — it has permissions you do not hold", http.StatusForbidden
	}
	return "", 0
}

// putProjectMember replaces the roles a user holds in this project (global bindings stay as they are).
func (s *Server) putProjectMember(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	var in struct {
		RoleIDs []int64 `json:"role_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.setProjectRoles(w, r, p, proj, in.RoleIDs)
}

func (s *Server) deleteProjectMember(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	s.setProjectRoles(w, r, p, proj, nil)
}

func (s *Server) setProjectRoles(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project, roleIDs []int64) {
	ctx := r.Context()
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	u, err := s.store.GetUser(ctx, uid)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if len(roleIDs) > 20 {
		writeError(w, http.StatusBadRequest, "too many roles")
		return
	}
	current, err := s.store.UserBindings(ctx, u.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	pid := proj.ID
	want := map[int64]bool{}
	for _, id := range roleIDs {
		want[id] = true
	}
	var next []store.BindingInput
	have := map[int64]bool{}
	for _, b := range current {
		if b.ProjectID == nil || *b.ProjectID != pid {
			next = append(next, store.BindingInput{RoleID: b.RoleID, ProjectID: b.ProjectID})
			continue
		}
		have[b.RoleID] = true
		if !want[b.RoleID] {
			if msg, code := s.grantCheck(ctx, p, b.RoleID, &pid); code != 0 {
				writeError(w, code, msg)
				return
			}
		}
	}
	for id := range want {
		if !have[id] {
			if msg, code := s.grantCheck(ctx, p, id, &pid); code != 0 {
				writeError(w, code, msg)
				return
			}
		}
		next = append(next, store.BindingInput{RoleID: id, ProjectID: &pid})
	}
	if _, err := s.store.ReplaceBindings(ctx, u.ID, next); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "unknown role")
		return
	} else if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "project.members", proj.Slug, map[string]any{"user": u.Email, "role_ids": roleIDs})
	w.WriteHeader(http.StatusNoContent)
}

// userDirectory lists active users (id, name, email) for people who add members to projects.
func (s *Server) userDirectory(w http.ResponseWriter, r *http.Request, p *principal) {
	if !p.eff.CanAnywhere("project.members") && !p.eff.Can("user.manage", nil) {
		writeError(w, http.StatusForbidden, "missing permission: project.members")
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	type entry struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	out := []entry{}
	for _, u := range users {
		if !u.Disabled {
			out = append(out, entry{u.ID, u.Name, u.Email})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
