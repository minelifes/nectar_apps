package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// APIToken is a long-lived token for CI and scripts (the plain token is shown once).
type APIToken struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	UserEmail   string     `json:"user_email,omitempty"`
	Name        string     `json:"name"`
	RoleID      *int64     `json:"role_id,omitempty"` // nil = the user's own rights
	RoleName    string     `json:"role_name,omitempty"`
	ProjectID   *int64     `json:"project_id,omitempty"` // nil = all projects
	ProjectName string     `json:"project_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	// Permissions of the role (nil without a role); not serialized.
	RolePermissions []string `json:"-"`
}

const tokenCols = `t.id, t.user_id, u.email, t.name, t.role_id, IFNULL(r.name, ''), IFNULL(r.permissions, ''), t.project_id, IFNULL(p.name, ''),
	t.created_at, t.last_used_at, t.expires_at`

const tokenFrom = ` FROM api_tokens t JOIN users u ON u.id = t.user_id LEFT JOIN roles r ON r.id = t.role_id LEFT JOIN projects p ON p.id = t.project_id`

func scanToken(r scanner) (APIToken, error) {
	var t APIToken
	var role, project, used, exp sql.NullInt64
	var perms string
	var created int64
	err := r.Scan(&t.ID, &t.UserID, &t.UserEmail, &t.Name, &role, &t.RoleName, &perms, &project, &t.ProjectName, &created, &used, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	t.RoleID, t.ProjectID, t.CreatedAt, t.LastUsedAt, t.ExpiresAt = intPtr(role), intPtr(project), ts(created), nullTime(used), nullTime(exp)
	if t.RoleID != nil {
		t.RolePermissions = []string{}
		if perms != "" {
			if err := json.Unmarshal([]byte(perms), &t.RolePermissions); err != nil {
				return t, err
			}
		}
	}
	return t, nil
}

// CreateAPIToken stores a token by its hash.
func (s *Store) CreateAPIToken(ctx context.Context, t APIToken, hash string) (APIToken, error) {
	var exp any
	if t.ExpiresAt != nil {
		exp = t.ExpiresAt.Unix()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, role_id, project_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, hash, nullInt(t.RoleID), nullInt(t.ProjectID), s.unix(), exp)
	if isForeignKey(err) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	id, _ := res.LastInsertId()
	return s.GetAPIToken(ctx, id)
}

// GetAPIToken returns a token.
func (s *Store) GetAPIToken(ctx context.Context, id int64) (APIToken, error) {
	return scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+tokenFrom+` WHERE t.id = ?`, id))
}

// ListAPITokens returns the tokens of a user (userID 0 = every user), newest first.
func (s *Store) ListAPITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	q, args := `SELECT `+tokenCols+tokenFrom, []any{}
	if userID > 0 {
		q += ` WHERE t.user_id = ?`
		args = append(args, userID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY t.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteAPIToken revokes a token.
func (s *Store) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// APITokenByHash resolves a token (expired tokens are not found) and records its use at most once a minute.
func (s *Store) APITokenByHash(ctx context.Context, hash string) (APIToken, User, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM api_tokens WHERE token_hash = ? AND (expires_at IS NULL OR expires_at > ?)`, hash, s.unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, User{}, ErrNotFound
	}
	if err != nil {
		return APIToken{}, User{}, err
	}
	t, err := s.GetAPIToken(ctx, id)
	if err != nil {
		return t, User{}, err
	}
	now := s.unix()
	if t.LastUsedAt == nil || now-t.LastUsedAt.Unix() >= 60 {
		_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	}
	u, err := s.GetUser(ctx, t.UserID)
	return t, u, err
}
