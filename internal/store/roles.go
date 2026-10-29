package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Role is a named set of permissions.
type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	BuiltinKey  string    `json:"builtin_key,omitempty"`
	Builtin     bool      `json:"builtin"`
	Permissions []string  `json:"permissions"`
	Bindings    int       `json:"bindings"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RoleBinding binds a role to a user, globally (ProjectID nil) or for one project.
type RoleBinding struct {
	ID          int64    `json:"id"`
	UserID      int64    `json:"-"`
	RoleID      int64    `json:"role_id"`
	RoleName    string   `json:"role_name"`
	ProjectID   *int64   `json:"project_id"`
	Permissions []string `json:"-"`
}

// ErrBuiltinRole is returned when trying to modify a built-in role.
var ErrBuiltinRole = errors.New("built-in roles cannot be changed — clone it instead")

// ErrRoleInUse is returned when deleting a role that is still bound.
var ErrRoleInUse = errors.New("role is still assigned to users")

const roleCols = `r.id, r.name, r.description, IFNULL(r.builtin_key, ''), r.permissions, r.updated_at,
	(SELECT COUNT(*) FROM role_bindings b WHERE b.role_id = r.id)`

func scanRole(r scanner) (Role, error) {
	var ro Role
	var perms string
	var updated int64
	err := r.Scan(&ro.ID, &ro.Name, &ro.Description, &ro.BuiltinKey, &perms, &updated, &ro.Bindings)
	if errors.Is(err, sql.ErrNoRows) {
		return ro, ErrNotFound
	}
	if err != nil {
		return ro, err
	}
	ro.Builtin = ro.BuiltinKey != ""
	ro.UpdatedAt = ts(updated)
	if err := json.Unmarshal([]byte(perms), &ro.Permissions); err != nil {
		return ro, err
	}
	if ro.Permissions == nil {
		ro.Permissions = []string{}
	}
	return ro, nil
}

// SyncBuiltinRole creates or updates a built-in role identified by key.
func (s *Store) SyncBuiltinRole(ctx context.Context, key, name, description string, perms []string) error {
	pj, _ := json.Marshal(perms)
	now := s.unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO roles (name, description, builtin_key, permissions, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(builtin_key) DO UPDATE SET name = excluded.name, description = excluded.description,
			permissions = excluded.permissions, updated_at = excluded.updated_at
		WHERE roles.permissions <> excluded.permissions OR roles.name <> excluded.name OR roles.description <> excluded.description`,
		name, description, key, string(pj), now, now)
	return err
}

// ListRoles returns all roles: built-in first, then custom by name.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+roleCols+` FROM roles r ORDER BY r.builtin_key IS NULL, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		ro, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ro)
	}
	return out, rows.Err()
}

// GetRole returns a role by id.
func (s *Store) GetRole(ctx context.Context, id int64) (Role, error) {
	return scanRole(s.db.QueryRowContext(ctx, `SELECT `+roleCols+` FROM roles r WHERE r.id = ?`, id))
}

// CreateRole creates a custom role.
func (s *Store) CreateRole(ctx context.Context, name, description string, perms []string) (Role, error) {
	pj, _ := json.Marshal(perms)
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO roles (name, description, permissions, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		strings.TrimSpace(name), strings.TrimSpace(description), string(pj), now, now)
	if isUnique(err) {
		return Role{}, ErrConflict
	}
	if err != nil {
		return Role{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetRole(ctx, id)
}

// UpdateRole replaces a custom role's name, description and permissions.
func (s *Store) UpdateRole(ctx context.Context, id int64, name, description string, perms []string) (Role, error) {
	ro, err := s.GetRole(ctx, id)
	if err != nil {
		return Role{}, err
	}
	if ro.Builtin {
		return Role{}, ErrBuiltinRole
	}
	pj, _ := json.Marshal(perms)
	_, err = s.db.ExecContext(ctx, `UPDATE roles SET name = ?, description = ?, permissions = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(name), strings.TrimSpace(description), string(pj), s.unix(), id)
	if isUnique(err) {
		return Role{}, ErrConflict
	}
	if err != nil {
		return Role{}, err
	}
	return s.GetRole(ctx, id)
}

// DeleteRole deletes a custom role that has no bindings.
func (s *Store) DeleteRole(ctx context.Context, id int64) error {
	ro, err := s.GetRole(ctx, id)
	if err != nil {
		return err
	}
	if ro.Builtin {
		return ErrBuiltinRole
	}
	if ro.Bindings > 0 {
		return ErrRoleInUse
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id)
	if isForeignKey(err) {
		return ErrRoleInUse
	}
	return err
}

// UserBindings returns the bindings of a user, including each role's permissions.
func (s *Store) UserBindings(ctx context.Context, userID int64) ([]RoleBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, b.user_id, b.role_id, r.name, b.project_id, r.permissions
		FROM role_bindings b JOIN roles r ON r.id = b.role_id WHERE b.user_id = ? ORDER BY b.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleBinding{}
	for rows.Next() {
		var b RoleBinding
		var pid sql.NullInt64
		var perms string
		if err := rows.Scan(&b.ID, &b.UserID, &b.RoleID, &b.RoleName, &pid, &perms); err != nil {
			return nil, err
		}
		b.ProjectID = intPtr(pid)
		if err := json.Unmarshal([]byte(perms), &b.Permissions); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BindingInput is one desired binding.
type BindingInput struct {
	RoleID    int64  `json:"role_id"`
	ProjectID *int64 `json:"project_id"`
}

// ReplaceBindings atomically replaces all bindings of a user. Duplicates are collapsed.
// Unknown roles or projects return ErrNotFound.
func (s *Store) ReplaceBindings(ctx context.Context, userID int64, in []BindingInput) ([]RoleBinding, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM role_bindings WHERE user_id = ?`, userID); err != nil {
			return err
		}
		now := s.unix()
		for _, b := range in {
			_, err := tx.ExecContext(ctx, `INSERT INTO role_bindings (user_id, role_id, project_id, created_at) VALUES (?, ?, ?, ?)
				ON CONFLICT DO NOTHING`, userID, b.RoleID, nullInt(b.ProjectID), now)
			if isForeignKey(err) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.UserBindings(ctx, userID)
}
