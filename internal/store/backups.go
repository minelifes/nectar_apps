package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// BackupTarget is a restic repository location. Config is sealed by the caller.
type BackupTarget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Config    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BackupPlan says what is backed up where and when.
type BackupPlan struct {
	ID          int64     `json:"id"`
	TargetID    int64     `json:"target_id"`
	Scope       string    `json:"scope"`
	SubjectID   int64     `json:"subject_id"`
	Schedule    string    `json:"schedule"`
	KeepDaily   int       `json:"keep_daily"`
	KeepWeekly  int       `json:"keep_weekly"`
	KeepMonthly int       `json:"keep_monthly"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BackupRun is one backup or restore Job.
type BackupRun struct {
	ID         int64      `json:"id"`
	PlanID     *int64     `json:"plan_id,omitempty"`
	Kind       string     `json:"kind"`
	Scope      string     `json:"scope"`
	SubjectID  int64      `json:"subject_id"`
	Subject    string     `json:"subject"`
	Namespace  string     `json:"namespace"`
	Job        string     `json:"job"`
	Snapshot   string     `json:"snapshot,omitempty"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	BytesAdded int64      `json:"bytes_added"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Run statuses.
const (
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
)

// ------------------------------------------------------------------ targets --

const targetCols = `id, name, kind, config, created_at, updated_at`

func scanTarget(r scanner) (BackupTarget, error) {
	var t BackupTarget
	var c, u int64
	err := r.Scan(&t.ID, &t.Name, &t.Kind, &t.Config, &c, &u)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	t.CreatedAt, t.UpdatedAt = ts(c), ts(u)
	return t, err
}

// ListBackupTargets returns all targets.
func (s *Store) ListBackupTargets(ctx context.Context) ([]BackupTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+targetCols+` FROM backup_targets ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupTarget{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetBackupTarget returns one target.
func (s *Store) GetBackupTarget(ctx context.Context, id int64) (BackupTarget, error) {
	return scanTarget(s.db.QueryRowContext(ctx, `SELECT `+targetCols+` FROM backup_targets WHERE id = ?`, id))
}

// SaveBackupTarget creates (ID 0) or updates a target.
func (s *Store) SaveBackupTarget(ctx context.Context, t BackupTarget) (BackupTarget, error) {
	now := s.unix()
	if t.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO backup_targets (name, kind, config, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, t.Name, t.Kind, t.Config, now, now)
		if err != nil {
			return t, err
		}
		t.ID, _ = res.LastInsertId()
	} else if _, err := s.db.ExecContext(ctx, `UPDATE backup_targets SET name = ?, config = ?, updated_at = ? WHERE id = ?`, t.Name, t.Config, now, t.ID); err != nil {
		return t, err
	}
	return s.GetBackupTarget(ctx, t.ID)
}

// DeleteBackupTarget deletes a target without plans (ErrConflict otherwise).
func (s *Store) DeleteBackupTarget(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_targets WHERE id = ?`, id)
	if isForeignKey(err) {
		return ErrConflict
	}
	return err
}

// -------------------------------------------------------------------- plans --

const planCols = `id, target_id, scope, subject_id, schedule, keep_daily, keep_weekly, keep_monthly, enabled, created_at, updated_at`

func scanPlan(r scanner) (BackupPlan, error) {
	var p BackupPlan
	var c, u int64
	err := r.Scan(&p.ID, &p.TargetID, &p.Scope, &p.SubjectID, &p.Schedule, &p.KeepDaily, &p.KeepWeekly, &p.KeepMonthly, &p.Enabled, &c, &u)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	p.CreatedAt, p.UpdatedAt = ts(c), ts(u)
	return p, err
}

// ListBackupPlans returns all plans.
func (s *Store) ListBackupPlans(ctx context.Context) ([]BackupPlan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+planCols+` FROM backup_plans ORDER BY scope, subject_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupPlan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetBackupPlan returns one plan.
func (s *Store) GetBackupPlan(ctx context.Context, id int64) (BackupPlan, error) {
	return scanPlan(s.db.QueryRowContext(ctx, `SELECT `+planCols+` FROM backup_plans WHERE id = ?`, id))
}

// SaveBackupPlan creates (ID 0) or updates a plan. ErrConflict for a duplicate subject on the same target.
func (s *Store) SaveBackupPlan(ctx context.Context, p BackupPlan) (BackupPlan, error) {
	now := s.unix()
	var err error
	if p.ID == 0 {
		var res sql.Result
		res, err = s.db.ExecContext(ctx, `INSERT INTO backup_plans (target_id, scope, subject_id, schedule, keep_daily, keep_weekly, keep_monthly, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, p.TargetID, p.Scope, p.SubjectID, p.Schedule, p.KeepDaily, p.KeepWeekly, p.KeepMonthly, b2i(p.Enabled), now, now)
		if err == nil {
			p.ID, _ = res.LastInsertId()
		}
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE backup_plans SET schedule = ?, keep_daily = ?, keep_weekly = ?, keep_monthly = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			p.Schedule, p.KeepDaily, p.KeepWeekly, p.KeepMonthly, b2i(p.Enabled), now, p.ID)
	}
	if isUnique(err) {
		return p, ErrConflict
	}
	if err != nil {
		return p, err
	}
	return s.GetBackupPlan(ctx, p.ID)
}

// DeleteBackupPlan deletes a plan (its runs stay in the history).
func (s *Store) DeleteBackupPlan(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM backup_plans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlansFor deletes the plans of a deleted app or add-on.
func (s *Store) DeletePlansFor(ctx context.Context, scope string, subjectID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_plans WHERE scope = ? AND subject_id = ?`, scope, subjectID)
	return err
}

// --------------------------------------------------------------------- runs --

const runCols = `id, plan_id, kind, scope, subject_id, subject, namespace, job, snapshot, status, message, bytes_added, started_at, finished_at`

func scanRun(r scanner) (BackupRun, error) {
	var x BackupRun
	var plan, fin sql.NullInt64
	var started int64
	err := r.Scan(&x.ID, &plan, &x.Kind, &x.Scope, &x.SubjectID, &x.Subject, &x.Namespace, &x.Job, &x.Snapshot, &x.Status, &x.Message, &x.BytesAdded, &started, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	x.PlanID, x.StartedAt, x.FinishedAt = intPtr(plan), ts(started), nullTime(fin)
	return x, err
}

// CreateBackupRun records a started Job.
func (s *Store) CreateBackupRun(ctx context.Context, r BackupRun) (BackupRun, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO backup_runs (plan_id, kind, scope, subject_id, subject, namespace, job, snapshot, status, started_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', ?)`,
		nullInt(r.PlanID), r.Kind, r.Scope, r.SubjectID, r.Subject, r.Namespace, r.Job, r.Snapshot, r.StartedAt.Unix())
	if err != nil {
		return r, err
	}
	id, _ := res.LastInsertId()
	return scanRun(s.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM backup_runs WHERE id = ?`, id))
}

// FinishBackupRun records the outcome.
func (s *Store) FinishBackupRun(ctx context.Context, id int64, status, message, snapshot string, bytesAdded int64, at time.Time) error {
	if len(message) > 2000 {
		message = message[len(message)-2000:]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE backup_runs SET status = ?, message = ?, snapshot = CASE WHEN ? = '' THEN snapshot ELSE ? END, bytes_added = ?, finished_at = ? WHERE id = ?`,
		status, message, snapshot, snapshot, bytesAdded, at.Unix(), id)
	return err
}

// ListBackupRuns returns runs newest first (planID 0 = all; status "" = any).
func (s *Store) ListBackupRuns(ctx context.Context, planID int64, status string, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q, args := `WHERE 1 = 1`, []any{}
	if planID > 0 {
		q, args = q+` AND plan_id = ?`, append(args, planID)
	}
	if status != "" {
		q, args = q+` AND status = ?`, append(args, status)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+runCols+` FROM backup_runs `+q+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupRun{}
	for rows.Next() {
		x, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LastBackupRuns returns the newest finished backup run of every plan.
func (s *Store) LastBackupRuns(ctx context.Context) (map[int64]BackupRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runCols+` FROM backup_runs WHERE id IN (
		SELECT MAX(id) FROM backup_runs WHERE kind = 'backup' AND status != 'running' AND plan_id IS NOT NULL GROUP BY plan_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]BackupRun{}
	for rows.Next() {
		x, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out[*x.PlanID] = x
	}
	return out, rows.Err()
}

// SnapshotDB writes a consistent copy of the database to path (VACUUM INTO).
func (s *Store) SnapshotDB(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
