package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/store"
)

// API tokens for CI and scripts: "nat_…", sent as Authorization: Bearer. A token acts as its user, limited to
// a role and/or a project when given. Only signed-in people (not tokens) create tokens.

func (s *Server) listMyTokens(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.store.ListAPITokens(r.Context(), p.user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createMyToken(w http.ResponseWriter, r *http.Request, p *principal) {
	if p.token != nil {
		writeError(w, http.StatusForbidden, "API tokens cannot create tokens: sign in to the panel")
		return
	}
	var in struct {
		Name          string `json:"name"`
		RoleID        *int64 `json:"role_id"`
		ProjectID     *int64 `json:"project_id"`
		ExpiresInDays int    `json:"expires_in_days"` // 0 = never
	}
	if !decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		writeError(w, http.StatusBadRequest, "a name of 1–80 characters is required (e.g. \"GitHub Actions: shop\")")
		return
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > 3650 {
		writeError(w, http.StatusBadRequest, "expires_in_days must be 0 (never) to 3650")
		return
	}
	ctx := r.Context()
	if in.RoleID != nil {
		if _, err := s.store.GetRole(ctx, *in.RoleID); errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "role not found")
			return
		} else if err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	if in.ProjectID != nil {
		if !p.eff.Can("project.view", in.ProjectID) {
			writeError(w, http.StatusBadRequest, "project not found")
			return
		}
		if ok, err := s.store.ProjectExists(ctx, *in.ProjectID); err != nil {
			s.serverError(w, r, err)
			return
		} else if !ok {
			writeError(w, http.StatusBadRequest, "project not found")
			return
		}
	}
	t := store.APIToken{UserID: p.user.ID, Name: name, RoleID: in.RoleID, ProjectID: in.ProjectID}
	if in.ExpiresInDays > 0 {
		exp := time.Now().Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour)
		t.ExpiresAt = &exp
	}
	plain, hash := auth.NewToken("nat_")
	t, err := s.store.CreateAPIToken(ctx, t, hash)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "unknown role or project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "token.create", name, map[string]any{"id": t.ID, "role_id": in.RoleID, "project_id": in.ProjectID, "expires_in_days": in.ExpiresInDays})
	writeJSON(w, http.StatusCreated, map[string]any{"token": plain, "api_token": t})
}

func (s *Server) deleteMyToken(w http.ResponseWriter, r *http.Request, p *principal) {
	s.revokeToken(w, r, p, true)
}

func (s *Server) listAllTokens(w http.ResponseWriter, r *http.Request, _ *principal) {
	list, err := s.store.ListAPITokens(r.Context(), 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) deleteAnyToken(w http.ResponseWriter, r *http.Request, p *principal) {
	s.revokeToken(w, r, p, false)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request, p *principal, ownOnly bool) {
	id, err := strconv.ParseInt(r.PathValue("tid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	t, err := s.store.GetAPIToken(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && ownOnly && t.UserID != p.user.ID) {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.DeleteAPIToken(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, p, "token.revoke", t.Name, map[string]any{"id": t.ID, "user": t.UserEmail})
	w.WriteHeader(http.StatusNoContent)
}
