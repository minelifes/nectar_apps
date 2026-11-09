package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// HelmRelease is a Helm chart installed into a project. ValuesEnc and RepoPassword are sealed by the
// API (env box); the live state (revisions, status) is read from Helm's own release Secrets.
type HelmRelease struct {
	ID           int64     `json:"id"`
	ProjectID    int64     `json:"project_id"`
	Name         string    `json:"name"`
	Repo         string    `json:"repo"`
	Chart        string    `json:"chart"`
	Version      string    `json:"version"`
	ValuesEnc    string    `json:"-"`
	RepoUsername string    `json:"repo_username,omitempty"`
	RepoPassword string    `json:"-"`
	Status       string    `json:"status"`
	Message      string    `json:"message,omitempty"`
	Output       string    `json:"output,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Helm release states.
const (
	HelmInstalling   = "installing"
	HelmUpgrading    = "upgrading"
	HelmRollingBack  = "rolling_back"
	HelmUninstalling = "uninstalling"
	HelmDeployed     = "deployed"
	HelmFailed       = "failed"
)

// HelmBusy: an operation is running.
func HelmBusy(status string) bool {
	switch status {
	case HelmInstalling, HelmUpgrading, HelmRollingBack, HelmUninstalling:
		return true
	}
	return false
}

const helmCols = `id, project_id, name, repo, chart, version, values_enc, repo_username, repo_password, status, message, output, created_at, updated_at`

func scanHelm(r interface{ Scan(...any) error }) (HelmRelease, error) {
	var h HelmRelease
	var c, u int64
	err := r.Scan(&h.ID, &h.ProjectID, &h.Name, &h.Repo, &h.Chart, &h.Version, &h.ValuesEnc, &h.RepoUsername, &h.RepoPassword,
		&h.Status, &h.Message, &h.Output, &c, &u)
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	h.CreatedAt, h.UpdatedAt = time.Unix(c, 0).UTC(), time.Unix(u, 0).UTC()
	return h, err
}

// CreateHelmRelease stores a new release (status installing). ErrConflict when the name is taken.
func (s *Store) CreateHelmRelease(ctx context.Context, h HelmRelease, userID *int64) (HelmRelease, error) {
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO helm_releases (project_id, name, repo, chart, version, values_enc, repo_username,
		repo_password, status, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ProjectID, h.Name, h.Repo, h.Chart, h.Version, h.ValuesEnc, h.RepoUsername, h.RepoPassword, HelmInstalling, userID, now, now)
	if isUnique(err) {
		return HelmRelease{}, ErrConflict
	}
	if err != nil {
		return HelmRelease{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetHelmRelease(ctx, id)
}

func (s *Store) GetHelmRelease(ctx context.Context, id int64) (HelmRelease, error) {
	return scanHelm(s.db.QueryRowContext(ctx, `SELECT `+helmCols+` FROM helm_releases WHERE id = ?`, id))
}

func (s *Store) GetHelmReleaseByName(ctx context.Context, projectID int64, name string) (HelmRelease, error) {
	return scanHelm(s.db.QueryRowContext(ctx, `SELECT `+helmCols+` FROM helm_releases WHERE project_id = ? AND name = ?`, projectID, name))
}

// ListHelmReleases: projectID 0 = every project.
func (s *Store) ListHelmReleases(ctx context.Context, projectID int64) ([]HelmRelease, error) {
	q, args := `SELECT `+helmCols+` FROM helm_releases ORDER BY name`, []any{}
	if projectID > 0 {
		q, args = `SELECT `+helmCols+` FROM helm_releases WHERE project_id = ? ORDER BY name`, []any{projectID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HelmRelease{}
	for rows.Next() {
		h, err := scanHelm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// UpdateHelmChart stores a new chart reference and values before an upgrade.
func (s *Store) UpdateHelmChart(ctx context.Context, h HelmRelease) error {
	_, err := s.db.ExecContext(ctx, `UPDATE helm_releases SET repo = ?, chart = ?, version = ?, values_enc = ?, repo_username = ?,
		repo_password = ?, updated_at = ? WHERE id = ?`, h.Repo, h.Chart, h.Version, h.ValuesEnc, h.RepoUsername, h.RepoPassword, s.unix(), h.ID)
	return err
}

// StartHelmOp marks an operation as running unless one is already: false when busy.
func (s *Store) StartHelmOp(ctx context.Context, id int64, status string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE helm_releases SET status = ?, message = '', updated_at = ? WHERE id = ?
		AND status NOT IN (?, ?, ?, ?)`, status, s.unix(), id, HelmInstalling, HelmUpgrading, HelmRollingBack, HelmUninstalling)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishHelmOp records the result of an operation.
func (s *Store) FinishHelmOp(ctx context.Context, id int64, status, message, output string) error {
	if len(output) > 64<<10 {
		output = output[len(output)-64<<10:]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE helm_releases SET status = ?, message = ?, output = ?, updated_at = ? WHERE id = ?`,
		status, message, output, s.unix(), id)
	return err
}

// FailInterruptedHelmOps: at startup, operations that were running when the panel stopped.
func (s *Store) FailInterruptedHelmOps(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE helm_releases SET status = ?, message = 'the panel restarted during the operation: check the history, then try again', updated_at = ?
		WHERE status IN (?, ?, ?, ?)`, HelmFailed, s.unix(), HelmInstalling, HelmUpgrading, HelmRollingBack, HelmUninstalling)
	return err
}

func (s *Store) DeleteHelmRelease(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM helm_releases WHERE id = ?`, id)
	return err
}
