package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// UpdateRun is one run of the updater (PLAN.md §5a).
type UpdateRun struct {
	ID         int64           `json:"id"`
	Trigger    string          `json:"trigger"`
	State      string          `json:"state"`
	Summary    string          `json:"summary"`
	Plan       json.RawMessage `json:"plan"`
	Status     json.RawMessage `json:"status"`
	Error      string          `json:"error,omitempty"`
	UserID     *int64          `json:"user_id,omitempty"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

const updateCols = `id, trigger, state, summary, plan, status, error, user_id, started_at, finished_at`

func scanUpdateRun(r scanner) (UpdateRun, error) {
	var x UpdateRun
	var plan, status string
	var user, fin sql.NullInt64
	var started int64
	err := r.Scan(&x.ID, &x.Trigger, &x.State, &x.Summary, &plan, &status, &x.Error, &user, &started, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	x.Plan, x.Status = json.RawMessage(plan), json.RawMessage(status)
	x.UserID, x.StartedAt, x.FinishedAt = intPtr(user), ts(started), nullTime(fin)
	return x, err
}

// CreateUpdateRun records a new run.
func (s *Store) CreateUpdateRun(ctx context.Context, r UpdateRun) (UpdateRun, error) {
	if len(r.Status) == 0 {
		r.Status = json.RawMessage("{}")
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO update_runs (trigger, state, summary, plan, status, user_id, started_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Trigger, r.State, r.Summary, string(r.Plan), string(r.Status), nullInt(r.UserID), s.unix())
	if err != nil {
		return r, err
	}
	id, _ := res.LastInsertId()
	return s.GetUpdateRun(ctx, id)
}

// GetUpdateRun returns a run.
func (s *Store) GetUpdateRun(ctx context.Context, id int64) (UpdateRun, error) {
	return scanUpdateRun(s.db.QueryRowContext(ctx, `SELECT `+updateCols+` FROM update_runs WHERE id = ?`, id))
}

// SaveUpdateRun stores the state, status and error of a run (finished when final is true).
func (s *Store) SaveUpdateRun(ctx context.Context, id int64, state string, status json.RawMessage, errMsg string, final bool) error {
	if len(errMsg) > 4000 {
		errMsg = errMsg[:4000]
	}
	var fin any
	if final {
		fin = s.unix()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE update_runs SET state = ?, status = ?, error = ?, finished_at = COALESCE(finished_at, ?) WHERE id = ?`,
		state, string(status), errMsg, fin, id)
	return err
}

// ListUpdateRuns returns runs newest first.
func (s *Store) ListUpdateRuns(ctx context.Context, limit int) ([]UpdateRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+updateCols+` FROM update_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UpdateRun{}
	for rows.Next() {
		x, err := scanUpdateRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ActiveUpdateRuns returns the runs that have not finished.
func (s *Store) ActiveUpdateRuns(ctx context.Context) ([]UpdateRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+updateCols+` FROM update_runs WHERE finished_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpdateRun
	for rows.Next() {
		x, err := scanUpdateRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
