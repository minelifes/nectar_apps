package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Addon is a managed database of a project.
type Addon struct {
	ID            int64     `json:"id"`
	ProjectID     int64     `json:"project_id"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Version       string    `json:"version"`
	SizeMB        int64     `json:"size_mb"`
	CPUMillicores int64     `json:"cpu_millicores"`
	MemoryMB      int64     `json:"memory_mb"`
	Stopped       bool      `json:"stopped"`
	Credentials   string    `json:"-"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// AddonLink connects an app to an add-on.
type AddonLink struct {
	AddonID   int64     `json:"addon_id"`
	AppID     int64     `json:"app_id"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

const addonCols = `id, project_id, slug, name, kind, version, size_mb, cpu_millicores, memory_mb, stopped, credentials, status, status_message, created_at, updated_at`

func scanAddon(r scanner) (Addon, error) {
	var a Addon
	var created, updated int64
	err := r.Scan(&a.ID, &a.ProjectID, &a.Slug, &a.Name, &a.Kind, &a.Version, &a.SizeMB, &a.CPUMillicores, &a.MemoryMB, &a.Stopped,
		&a.Credentials, &a.Status, &a.StatusMessage, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.CreatedAt, a.UpdatedAt = ts(created), ts(updated)
	return a, err
}

// CreateAddon inserts an add-on. ErrConflict when the slug is taken in the project.
func (s *Store) CreateAddon(ctx context.Context, a Addon) (Addon, error) {
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO addons (id, project_id, slug, name, kind, version, size_mb, cpu_millicores, memory_mb, credentials, created_at, updated_at)
		VALUES (`+nextID("addons")+`, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.ProjectID, a.Slug, a.Name, a.Kind, a.Version, a.SizeMB, a.CPUMillicores, a.MemoryMB, a.Credentials, now, now)
	if isUnique(err) {
		return a, ErrConflict
	}
	if err != nil {
		return a, err
	}
	id, _ := res.LastInsertId()
	return s.GetAddon(ctx, id)
}

// GetAddon returns one add-on.
func (s *Store) GetAddon(ctx context.Context, id int64) (Addon, error) {
	return scanAddon(s.db.QueryRowContext(ctx, `SELECT `+addonCols+` FROM addons WHERE id = ?`, id))
}

// ListAddons returns the add-ons of a project (projectID 0 = all) by slug.
func (s *Store) ListAddons(ctx context.Context, projectID int64) ([]Addon, error) {
	q, args := `SELECT `+addonCols+` FROM addons ORDER BY project_id, slug`, []any{}
	if projectID > 0 {
		q, args = `SELECT `+addonCols+` FROM addons WHERE project_id = ? ORDER BY slug`, []any{projectID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Addon{}
	for rows.Next() {
		a, err := scanAddon(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAddon updates the editable fields.
func (s *Store) SaveAddon(ctx context.Context, a Addon) (Addon, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE addons SET name = ?, size_mb = ?, cpu_millicores = ?, memory_mb = ?, stopped = ?, updated_at = ? WHERE id = ?`,
		a.Name, a.SizeMB, a.CPUMillicores, a.MemoryMB, b2i(a.Stopped), s.unix(), a.ID)
	if err != nil {
		return a, err
	}
	return s.GetAddon(ctx, a.ID)
}

// SetAddonStatus records the last known status.
func (s *Store) SetAddonStatus(ctx context.Context, id int64, status, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE addons SET status = ?, status_message = ? WHERE id = ?`, status, message, id)
	return err
}

// DeleteAddon deletes an add-on and its links. Linked apps are marked as changed (their env shrinks).
func (s *Store) DeleteAddon(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id IN (SELECT app_id FROM addon_links WHERE addon_id = ?)`, s.unix(), id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM addons WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SlugTaken reports whether an app or an add-on of the project uses slug (they share the namespace).
func (s *Store) SlugTaken(ctx context.Context, projectID int64, slug string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM apps WHERE project_id = ? AND slug = ?) + (SELECT COUNT(*) FROM addons WHERE project_id = ? AND slug = ?)`,
		projectID, slug, projectID, slug).Scan(&n)
	return n > 0, err
}

// LinkAddon links (or re-links with a new prefix) an app to an add-on and marks the app as changed.
func (s *Store) LinkAddon(ctx context.Context, addonID, appID int64, prefix string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO addon_links (addon_id, app_id, prefix, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(addon_id, app_id) DO UPDATE SET prefix = excluded.prefix`, addonID, appID, prefix, s.unix()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id = ?`, s.unix(), appID)
		return err
	})
}

// UnlinkAddon removes a link.
func (s *Store) UnlinkAddon(ctx context.Context, addonID, appID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM addon_links WHERE addon_id = ? AND app_id = ?`, addonID, appID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id = ?`, s.unix(), appID)
		return err
	})
}

// AddonLinks returns the links of an add-on (addonID > 0) or of an app (appID > 0).
func (s *Store) AddonLinks(ctx context.Context, addonID, appID int64) ([]AddonLink, error) {
	q, arg := `SELECT addon_id, app_id, prefix, created_at FROM addon_links WHERE addon_id = ? ORDER BY app_id`, addonID
	if appID > 0 {
		q, arg = `SELECT addon_id, app_id, prefix, created_at FROM addon_links WHERE app_id = ? ORDER BY addon_id`, appID
	}
	rows, err := s.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AddonLink{}
	for rows.Next() {
		var l AddonLink
		var created int64
		if err := rows.Scan(&l.AddonID, &l.AppID, &l.Prefix, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = ts(created)
		out = append(out, l)
	}
	return out, rows.Err()
}
