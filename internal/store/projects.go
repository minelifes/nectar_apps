package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Quota limits a project. Zero fields mean "no limit" for that resource.
type Quota struct {
	CPUMillicores int64 `json:"cpu_millicores,omitempty"`
	MemoryMB      int64 `json:"memory_mb,omitempty"`
	StorageGB     int64 `json:"storage_gb,omitempty"`
	Pods          int64 `json:"pods,omitempty"`
	Apps          int64 `json:"apps,omitempty"`
}

// IsZero reports whether the quota sets no limits.
func (q Quota) IsZero() bool { return q == Quota{} }

// Project is a group of apps sharing one Kubernetes namespace.
type Project struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Quota     *Quota `json:"quota"` // nil = unlimited
	Apps      int    `json:"apps"`
	// Adopted projects were created from an existing namespace: the panel only manages its own objects
	// there (no isolation policy, no Pod Security labels) and never deletes the namespace.
	Adopted   bool      `json:"adopted"`
	CreatedAt time.Time `json:"created_at"`
	// Environment labels copies of a project ("staging", "production", …); OriginID is the project it was
	// copied from (apps of the same name in both can be promoted).
	Environment string `json:"environment,omitempty"`
	OriginID    *int64 `json:"origin_id,omitempty"`
}

const projectCols = `p.id, p.slug, p.name, p.namespace, p.quota_json, p.created_at, (SELECT COUNT(*) FROM apps a WHERE a.project_id = p.id), p.adopted,
	p.environment, p.origin_id`

func scanProject(r scanner) (Project, error) {
	var p Project
	var quota sql.NullString
	var created int64
	var origin sql.NullInt64
	err := r.Scan(&p.ID, &p.Slug, &p.Name, &p.Namespace, &quota, &created, &p.Apps, &p.Adopted, &p.Environment, &origin)
	p.OriginID = intPtr(origin)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.CreatedAt = ts(created)
	if quota.Valid && quota.String != "" {
		var q Quota
		if err := json.Unmarshal([]byte(quota.String), &q); err != nil {
			return p, err
		}
		if !q.IsZero() {
			p.Quota = &q
		}
	}
	return p, nil
}

func quotaJSON(q *Quota) any {
	if q == nil || q.IsZero() {
		return nil
	}
	b, _ := json.Marshal(q)
	return string(b)
}

// CreateProject creates a project. The namespace is "prj-<slug>".
func (s *Store) CreateProject(ctx context.Context, slug, name string, quota *Quota) (Project, error) {
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (slug, name, namespace, quota_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		slug, strings.TrimSpace(name), "prj-"+slug, quotaJSON(quota), now, now)
	if isUnique(err) {
		return Project{}, ErrConflict
	}
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetProject(ctx, id)
}

// CreateAdoptedProject creates a project for an existing namespace (adopt mode).
func (s *Store) CreateAdoptedProject(ctx context.Context, slug, name, namespace string) (Project, error) {
	now := s.unix()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE namespace = ?`, namespace).Scan(&n); err != nil {
		return Project{}, err
	}
	if n > 0 {
		return Project{}, ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (slug, name, namespace, quota_json, adopted, created_at, updated_at) VALUES (?, ?, ?, NULL, 1, ?, ?)`,
		slug, strings.TrimSpace(name), namespace, now, now)
	if isUnique(err) {
		return Project{}, ErrConflict
	}
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetProject(ctx, id)
}

// GetProject returns a project by id.
func (s *Store) GetProject(ctx context.Context, id int64) (Project, error) {
	return scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects p WHERE p.id = ?`, id))
}

// ListProjects returns all projects by name.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectCols+` FROM projects p ORDER BY p.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProject changes name and/or quota. setQuota=false keeps the quota unchanged.
func (s *Store) UpdateProject(ctx context.Context, id int64, name *string, setQuota bool, quota *Quota) (Project, error) {
	p, err := s.GetProject(ctx, id)
	if err != nil {
		return p, err
	}
	if name != nil {
		p.Name = strings.TrimSpace(*name)
	}
	if setQuota {
		p.Quota = quota
	}
	_, err = s.db.ExecContext(ctx, `UPDATE projects SET name = ?, quota_json = ?, updated_at = ? WHERE id = ?`,
		p.Name, quotaJSON(p.Quota), s.unix(), id)
	if err != nil {
		return p, err
	}
	return s.GetProject(ctx, id)
}

// DeleteProject deletes a project and (by cascade) its apps, releases, artifacts and bindings.
func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
