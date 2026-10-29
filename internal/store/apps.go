package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// AppConfig is the user-editable configuration of an app (stored as JSON).
type AppConfig struct {
	// Image is the container image for the "image" runtime (ignored otherwise).
	Image string `json:"image,omitempty"`
	// Port the app listens on (web apps).
	Port int `json:"port,omitempty"`
	// Command overrides the runtime command (rarely needed).
	Command []string `json:"command,omitempty"`
	// Args is passed to the app as $APP_ARGS.
	Args string `json:"args,omitempty"`
	// Replicas for web/worker apps (0 = stopped).
	Replicas int `json:"replicas"`
	// Resources per replica.
	Resources Resources `json:"resources"`
	// Health check.
	Health HealthConfig `json:"health"`
	// Schedule (cron syntax) for cron apps, e.g. "*/15 * * * *".
	Schedule string `json:"schedule,omitempty"`
	// ReleaseCommand runs once per deploy before the new version goes live (e.g. DB migrations).
	ReleaseCommand string `json:"release_command,omitempty"`
	// Expose opens TCP/UDP ports on the server (NodePort), e.g. MQTT 1883.
	Expose []ExposedPort `json:"expose,omitempty"`
	// Options are runtime-specific settings (e.g. spa_fallback for static sites).
	Options map[string]any `json:"options,omitempty"`
	// Volumes are persistent disks (PVCs) mounted into every container of the app.
	Volumes []Volume `json:"volumes,omitempty"`
	// ResumeReplicas remembers the replicas before a stop, for start.
	ResumeReplicas int `json:"resume_replicas,omitempty"`
}

// Volume is a persistent disk of an app (PersistentVolumeClaim "<app>-<name>").
type Volume struct {
	Name      string `json:"name"`       // a-z0-9-, unique per app
	MountPath string `json:"mount_path"` // absolute path in the container
	SizeMB    int64  `json:"size_mb"`
}

// Resources per replica. NoLimit = requests only (the app may use any free capacity).
type Resources struct {
	CPUMillicores int64 `json:"cpu_millicores"`
	MemoryMB      int64 `json:"memory_mb"`
	NoLimit       bool  `json:"no_limit,omitempty"`
}

// HealthConfig configures readiness/liveness probes.
type HealthConfig struct {
	Type                string `json:"type"` // tcp | http | none
	Path                string `json:"path,omitempty"`
	InitialDelaySeconds int    `json:"initial_delay_seconds,omitempty"`
}

// ExposedPort is a port opened on the server.
type ExposedPort struct {
	Port     int    `json:"port"`                // container port
	Protocol string `json:"protocol"`            // TCP | UDP
	NodePort int    `json:"node_port,omitempty"` // server port (30000-32767); 0 = assigned automatically
}

// App is a deployable unit inside a project.
type App struct {
	ID             int64     `json:"id"`
	ProjectID      int64     `json:"project_id"`
	Slug           string    `json:"slug"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	RuntimeID      string    `json:"runtime"`
	RuntimeVersion string    `json:"runtime_version"`
	Config         AppConfig `json:"config"`
	Status         string    `json:"status"`
	StatusMessage  string    `json:"status_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const appCols = `id, project_id, slug, name, kind, runtime_id, runtime_version, config, status, status_message, created_at, updated_at`

func scanApp(r scanner) (App, error) {
	var a App
	var cfg string
	var created, updated int64
	err := r.Scan(&a.ID, &a.ProjectID, &a.Slug, &a.Name, &a.Kind, &a.RuntimeID, &a.RuntimeVersion, &cfg, &a.Status, &a.StatusMessage, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.CreatedAt, a.UpdatedAt = ts(created), ts(updated)
	return a, json.Unmarshal([]byte(cfg), &a.Config)
}

// CreateApp inserts an app. Returns ErrConflict if the slug is taken in the project.
func (s *Store) CreateApp(ctx context.Context, a App) (App, error) {
	cfg, err := json.Marshal(a.Config)
	if err != nil {
		return App{}, err
	}
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO apps (id, project_id, slug, name, kind, runtime_id, runtime_version, config, status, created_at, updated_at)
		VALUES (`+nextID("apps")+`, ?, ?, ?, ?, ?, ?, ?, 'new', ?, ?)`,
		a.ProjectID, a.Slug, strings.TrimSpace(a.Name), a.Kind, a.RuntimeID, a.RuntimeVersion, string(cfg), now, now)
	if isUnique(err) {
		return App{}, ErrConflict
	}
	if err != nil {
		return App{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetApp(ctx, id)
}

// GetApp returns an app by id.
func (s *Store) GetApp(ctx context.Context, id int64) (App, error) {
	return scanApp(s.db.QueryRowContext(ctx, `SELECT `+appCols+` FROM apps WHERE id = ?`, id))
}

// ListApps returns the apps of a project (projectID 0 = all apps).
func (s *Store) ListApps(ctx context.Context, projectID int64) ([]App, error) {
	q, args := `SELECT `+appCols+` FROM apps ORDER BY name COLLATE NOCASE`, []any{}
	if projectID != 0 {
		q, args = `SELECT `+appCols+` FROM apps WHERE project_id = ? ORDER BY name COLLATE NOCASE`, []any{projectID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveApp updates name, runtime version and config of an app.
func (s *Store) SaveApp(ctx context.Context, a App) (App, error) {
	cfg, err := json.Marshal(a.Config)
	if err != nil {
		return App{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE apps SET name = ?, runtime_version = ?, config = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(a.Name), a.RuntimeVersion, string(cfg), s.unix(), a.ID)
	if err != nil {
		return App{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return App{}, ErrNotFound
	}
	return s.GetApp(ctx, a.ID)
}

// SetAppStatus records the last known status of an app.
func (s *Store) SetAppStatus(ctx context.Context, id int64, status, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE apps SET status = ?, status_message = ? WHERE id = ?`, status, message, id)
	return err
}

// DeleteApp deletes an app and its env vars, artifacts, uploads and releases.
func (s *Store) DeleteApp(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------- env --

// EnvVar is an environment variable; ValueEnc is encrypted with the master key.
type EnvVar struct {
	Key      string `json:"key"`
	ValueEnc string `json:"-"`
	IsSecret bool   `json:"secret"`
}

// ListEnv returns the env vars of an app sorted by key.
func (s *Store) ListEnv(ctx context.Context, appID int64) ([]EnvVar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_enc, is_secret FROM env_vars WHERE app_id = ? ORDER BY key`, appID)
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

// ReplaceEnv atomically replaces all env vars of an app.
func (s *Store) ReplaceEnv(ctx context.Context, appID int64, vars []EnvVar) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM env_vars WHERE app_id = ?`, appID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id = ?`, s.unix(), appID); err != nil {
			return err
		}
		for _, v := range vars {
			if _, err := tx.ExecContext(ctx, `INSERT INTO env_vars (app_id, key, value_enc, is_secret) VALUES (?, ?, ?, ?)`,
				appID, v.Key, v.ValueEnc, b2i(v.IsSecret)); err != nil {
				if isUnique(err) {
					return ErrConflict
				}
				return err
			}
		}
		return nil
	})
}

// AppFile is a config file mounted into the containers of an app.
type AppFile struct {
	Path       string `json:"path"`
	ContentEnc string `json:"-"`
	IsSecret   bool   `json:"secret"`
}

// ListFiles returns the config files of an app sorted by path.
func (s *Store) ListFiles(ctx context.Context, appID int64) ([]AppFile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, content_enc, is_secret FROM app_files WHERE app_id = ? ORDER BY path`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AppFile{}
	for rows.Next() {
		var f AppFile
		if err := rows.Scan(&f.Path, &f.ContentEnc, &f.IsSecret); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReplaceFiles atomically replaces all config files of an app.
func (s *Store) ReplaceFiles(ctx context.Context, appID int64, files []AppFile) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_files WHERE app_id = ?`, appID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE apps SET env_changed_at = ? WHERE id = ?`, s.unix(), appID); err != nil {
			return err
		}
		for _, f := range files {
			if _, err := tx.ExecContext(ctx, `INSERT INTO app_files (app_id, path, content_enc, is_secret) VALUES (?, ?, ?, ?)`,
				appID, f.Path, f.ContentEnc, b2i(f.IsSecret)); err != nil {
				if isUnique(err) {
					return ErrConflict
				}
				return err
			}
		}
		return nil
	})
}

// EnvChangedAt returns when env vars or config files of an app last changed (zero if never).
func (s *Store) EnvChangedAt(ctx context.Context, appID int64) (time.Time, error) {
	var t int64
	err := s.db.QueryRowContext(ctx, `SELECT env_changed_at FROM apps WHERE id = ?`, appID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if t == 0 {
		return time.Time{}, err
	}
	return ts(t), err
}

// SetAppReplicas stores the replicas (and the value to resume with) without touching updated_at: scaling
// takes effect immediately and is not a pending change.
func (s *Store) SetAppReplicas(ctx context.Context, id int64, replicas, resume int) error {
	a, err := s.GetApp(ctx, id)
	if err != nil {
		return err
	}
	a.Config.Replicas, a.Config.ResumeReplicas = replicas, resume
	cfg, err := json.Marshal(a.Config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE apps SET config = ? WHERE id = ?`, string(cfg), id)
	return err
}

// nextID is the id expression for a new row of tbl (apps, addons): one above both the highest id and
// the highest deleted id (table id_floor), so ids are never reused (they tag backups and metrics).
func nextID(tbl string) string {
	return "max(coalesce((SELECT max(id) FROM " + tbl + "), 0), coalesce((SELECT seq FROM id_floor WHERE tbl = '" + tbl + "'), 0)) + 1"
}
