package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ------------------------------------------------------- registry logins --

// RegistryCredential is a login for a private registry; Password is sealed.
type RegistryCredential struct {
	ID        int64     `json:"id"`
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListRegistryCredentials returns every login, by host.
func (s *Store) ListRegistryCredentials(ctx context.Context) ([]RegistryCredential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, host, username, password, created_at, updated_at FROM registry_credentials ORDER BY host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegistryCredential{}
	for rows.Next() {
		var c RegistryCredential
		var created, updated int64
		if err := rows.Scan(&c.ID, &c.Host, &c.Username, &c.Password, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt, c.UpdatedAt = ts(created), ts(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutRegistryCredential creates or replaces the login of a host.
func (s *Store) PutRegistryCredential(ctx context.Context, host, username, sealedPassword string) error {
	now := s.unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO registry_credentials (host, username, password, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (host) DO UPDATE SET username = excluded.username, password = excluded.password, updated_at = excluded.updated_at`,
		host, username, sealedPassword, now, now)
	return err
}

// DeleteRegistryCredential removes a login.
func (s *Store) DeleteRegistryCredential(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registry_credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// -------------------------------------------------------------- sources --

// AppSource is the Git repository an image app is built from. Token and WebhookSecret are sealed.
type AppSource struct {
	AppID         int64  `json:"app_id"`
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch"`
	ContextDir    string `json:"context_dir"`
	Dockerfile    string `json:"dockerfile"`
	Token         string `json:"-"`
	WebhookSecret string `json:"-"`
	AutoDeploy    bool   `json:"auto_deploy"`
	BuildCPU      int64  `json:"build_cpu_millicores"`
	BuildMemory   int64  `json:"build_memory_mb"`
	// Previews: every pull request into Branch gets its own copy of the app (see previews.go).
	Previews         bool      `json:"previews"`
	PreviewDatabases bool      `json:"preview_databases"` // previews keep the parent's database links
	PreviewForks     bool      `json:"preview_forks"`     // also pull requests from forks (their code runs with the app's variables)
	PreviewTTLDays   int       `json:"preview_ttl_days"`  // 0 = never expire
	UpdatedAt        time.Time `json:"updated_at"`
}

const sourceCols = `app_id, repo_url, branch, context_dir, dockerfile, token, webhook_secret, auto_deploy, build_cpu, build_memory,
	previews, preview_databases, preview_forks, preview_ttl_days, updated_at`

// GetAppSource returns the source of an app (ErrNotFound when the app is not built from Git).
func (s *Store) GetAppSource(ctx context.Context, appID int64) (AppSource, error) {
	var a AppSource
	var updated int64
	err := s.db.QueryRowContext(ctx, `SELECT `+sourceCols+` FROM app_sources WHERE app_id = ?`, appID).Scan(
		&a.AppID, &a.RepoURL, &a.Branch, &a.ContextDir, &a.Dockerfile, &a.Token, &a.WebhookSecret, &a.AutoDeploy, &a.BuildCPU, &a.BuildMemory,
		&a.Previews, &a.PreviewDatabases, &a.PreviewForks, &a.PreviewTTLDays, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.UpdatedAt = ts(updated)
	return a, err
}

// SaveAppSource creates or replaces the source of an app.
func (s *Store) SaveAppSource(ctx context.Context, a AppSource) (AppSource, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_sources (`+sourceCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (app_id) DO UPDATE SET repo_url = excluded.repo_url, branch = excluded.branch, context_dir = excluded.context_dir,
		dockerfile = excluded.dockerfile, token = excluded.token, webhook_secret = excluded.webhook_secret, auto_deploy = excluded.auto_deploy,
		build_cpu = excluded.build_cpu, build_memory = excluded.build_memory, previews = excluded.previews,
		preview_databases = excluded.preview_databases, preview_forks = excluded.preview_forks, preview_ttl_days = excluded.preview_ttl_days,
		updated_at = excluded.updated_at`,
		a.AppID, a.RepoURL, a.Branch, a.ContextDir, a.Dockerfile, a.Token, a.WebhookSecret, b2i(a.AutoDeploy), a.BuildCPU, a.BuildMemory,
		b2i(a.Previews), b2i(a.PreviewDatabases), b2i(a.PreviewForks), a.PreviewTTLDays, s.unix())
	if isForeignKey(err) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	return s.GetAppSource(ctx, a.AppID)
}

// DeleteAppSource stops building an app from Git.
func (s *Store) DeleteAppSource(ctx context.Context, appID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_sources WHERE app_id = ?`, appID)
	return err
}

// --------------------------------------------------------------- builds --

// Build statuses.
const (
	BuildRunning   = "running"
	BuildSucceeded = "succeeded"
	BuildFailed    = "failed"
	BuildCanceled  = "canceled"
)

// Build is one image build of an app.
type Build struct {
	ID         int64      `json:"id"`
	AppID      int64      `json:"app_id"`
	Number     int        `json:"number"`
	Trigger    string     `json:"trigger"`
	Branch     string     `json:"branch"`
	CommitSHA  string     `json:"commit_sha"`
	CommitMsg  string     `json:"commit_message"`
	Status     string     `json:"status"`
	Image      string     `json:"image"`
	Deploy     bool       `json:"deploy"`
	Release    *int64     `json:"release,omitempty"`
	Message    string     `json:"message,omitempty"`
	Log        string     `json:"-"`
	CreatedBy  *int64     `json:"created_by,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

const buildCols = `id, app_id, number, trigger, branch, commit_sha, commit_msg, status, image, deploy, release, message, log, created_by, started_at, finished_at`

func scanBuild(r scanner) (Build, error) {
	var b Build
	var rel, by, fin sql.NullInt64
	var started int64
	err := r.Scan(&b.ID, &b.AppID, &b.Number, &b.Trigger, &b.Branch, &b.CommitSHA, &b.CommitMsg, &b.Status, &b.Image, &b.Deploy, &rel, &b.Message, &b.Log, &by, &started, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	b.Release, b.CreatedBy, b.StartedAt, b.FinishedAt = intPtr(rel), intPtr(by), ts(started), nullTime(fin)
	return b, err
}

// ErrBuildRunning is returned when an app already has a running build.
var ErrBuildRunning = errors.New("a build of this app is already running")

// CreateBuild records a started build (number = next per app). Only one build per app runs at a time.
func (s *Store) CreateBuild(ctx context.Context, b Build) (Build, error) {
	var id int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var running int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM builds WHERE app_id = ? AND status = 'running'`, b.AppID).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return ErrBuildRunning
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO builds (app_id, number, trigger, branch, commit_sha, commit_msg, status, image, deploy, created_by, started_at)
			VALUES (?, (SELECT IFNULL(MAX(number), 0) + 1 FROM builds WHERE app_id = ?), ?, ?, ?, ?, 'running', ?, ?, ?, ?)`,
			b.AppID, b.AppID, b.Trigger, b.Branch, b.CommitSHA, b.CommitMsg, b.Image, b2i(b.Deploy), nullInt(b.CreatedBy), s.unix())
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return b, err
	}
	return s.GetBuild(ctx, id)
}

// SetBuildImage records the image a running build pushes to (known once the number is assigned).
func (s *Store) SetBuildImage(ctx context.Context, id int64, image string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE builds SET image = ? WHERE id = ?`, image, id)
	return err
}

// FinishBuild records the outcome and the log tail.
func (s *Store) FinishBuild(ctx context.Context, id int64, status, message, log string, release *int64) error {
	if len(log) > 256<<10 {
		log = log[len(log)-256<<10:]
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE builds SET status = ?, message = ?, log = ?, release = ?, finished_at = ? WHERE id = ?`,
		status, message, log, nullInt(release), s.unix(), id)
	return err
}

// SetBuildRelease records the release a build started.
func (s *Store) SetBuildRelease(ctx context.Context, id int64, release int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE builds SET release = ? WHERE id = ?`, release, id)
	return err
}

// GetBuild returns a build.
func (s *Store) GetBuild(ctx context.Context, id int64) (Build, error) {
	return scanBuild(s.db.QueryRowContext(ctx, `SELECT `+buildCols+` FROM builds WHERE id = ?`, id))
}

// GetBuildByNumber returns build n of an app.
func (s *Store) GetBuildByNumber(ctx context.Context, appID int64, n int) (Build, error) {
	return scanBuild(s.db.QueryRowContext(ctx, `SELECT `+buildCols+` FROM builds WHERE app_id = ? AND number = ?`, appID, n))
}

// ListBuilds returns the builds of an app, newest first (without logs).
func (s *Store) ListBuilds(ctx context.Context, appID int64, limit int) ([]Build, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+buildCols+` FROM builds WHERE app_id = ? ORDER BY number DESC LIMIT ?`, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		b.Log = ""
		out = append(out, b)
	}
	return out, rows.Err()
}

// RunningBuilds returns builds that have not finished (to resume tracking after a restart).
func (s *Store) RunningBuilds(ctx context.Context) ([]Build, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+buildCols+` FROM builds WHERE status = 'running' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Build
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
