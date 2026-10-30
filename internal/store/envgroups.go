package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// EnvGroup is a named set of variables of a project; apps attach groups by name (AppConfig.EnvGroups).
type EnvGroup struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Name      string    `json:"name"`
	Vars      []EnvVar  `json:"vars"`
	Apps      []string  `json:"apps"` // slugs of the apps that use it
	UpdatedAt time.Time `json:"updated_at"`
}

// ListEnvGroups returns a project's groups with their variables (values sealed) and the apps using them.
func (s *Store) ListEnvGroups(ctx context.Context, projectID int64) ([]EnvGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, updated_at FROM env_groups WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	var out []EnvGroup
	for rows.Next() {
		var g EnvGroup
		var upd int64
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.Name, &upd); err != nil {
			rows.Close()
			return nil, err
		}
		g.UpdatedAt = ts(upd)
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	apps, err := s.ListApps(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		g := &out[i]
		if g.Vars, err = s.envGroupVars(ctx, g.ID); err != nil {
			return nil, err
		}
		g.Apps = []string{}
		for _, a := range apps {
			for _, n := range a.Config.EnvGroups {
				if n == g.Name {
					g.Apps = append(g.Apps, a.Slug)
				}
			}
		}
	}
	if out == nil {
		out = []EnvGroup{}
	}
	return out, nil
}

func (s *Store) envGroupVars(ctx context.Context, groupID int64) ([]EnvVar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_enc, is_secret FROM env_group_vars WHERE group_id = ? ORDER BY key`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnvVar{}
	for rows.Next() {
		var e EnvVar
		if err := rows.Scan(&e.Key, &e.ValueEnc, &e.IsSecret); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEnvGroup returns one group by name.
func (s *Store) GetEnvGroup(ctx context.Context, projectID int64, name string) (EnvGroup, error) {
	gs, err := s.ListEnvGroups(ctx, projectID)
	if err != nil {
		return EnvGroup{}, err
	}
	for _, g := range gs {
		if g.Name == name {
			return g, nil
		}
	}
	return EnvGroup{}, ErrNotFound
}

// SaveEnvGroup creates a group or replaces its variables, and marks the apps that use it as changed.
func (s *Store) SaveEnvGroup(ctx context.Context, projectID int64, name string, vars []EnvVar) (EnvGroup, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.unix()
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM env_groups WHERE project_id = ? AND name = ?`, projectID, name).Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res, err := tx.ExecContext(ctx, `INSERT INTO env_groups (project_id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, projectID, name, now, now)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE env_groups SET updated_at = ? WHERE id = ?`, now, id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM env_group_vars WHERE group_id = ?`, id); err != nil {
			return err
		}
		for _, v := range vars {
			if _, err := tx.ExecContext(ctx, `INSERT INTO env_group_vars (group_id, key, value_enc, is_secret) VALUES (?, ?, ?, ?)`,
				id, v.Key, v.ValueEnc, b2i(v.IsSecret)); err != nil {
				if isUnique(err) {
					return ErrConflict
				}
				return err
			}
		}
		// apps using the group have changes that are not live yet
		_, err = tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE project_id = ? AND EXISTS
			(SELECT 1 FROM json_each(apps.config, '$.env_groups') WHERE json_each.value = ?)`, now, projectID, name)
		return err
	})
	if err != nil {
		return EnvGroup{}, err
	}
	return s.GetEnvGroup(ctx, projectID, name)
}

// DeleteEnvGroup removes a group (refused while apps use it — the caller checks).
func (s *Store) DeleteEnvGroup(ctx context.Context, projectID int64, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM env_groups WHERE project_id = ? AND name = ?`, projectID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectEnvironment records the environment label of a project and the project it was copied from.
func (s *Store) SetProjectEnvironment(ctx context.Context, id int64, environment string, originID *int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET environment = ?, origin_id = ?, updated_at = ? WHERE id = ?`,
		environment, nullInt(originID), s.unix(), id)
	return err
}

// TouchEnv marks an app's environment as changed (not live until the next deploy).
func (s *Store) TouchEnv(ctx context.Context, appID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id = ?`, s.unix(), appID)
	return err
}
