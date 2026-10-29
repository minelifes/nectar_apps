package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AlertChannel is a notification target. Config is sealed by the caller (it holds tokens and passwords).
type AlertChannel struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Config    string    `json:"-"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Alert is one incident.
type Alert struct {
	ID         int64      `json:"id"`
	Rule       string     `json:"rule"`
	Key        string     `json:"key"`
	Subject    string     `json:"subject"`
	Severity   string     `json:"severity"`
	Message    string     `json:"message"`
	AppID      *int64     `json:"app_id,omitempty"`
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Notified   int        `json:"notified"`
}

// Alert states.
const (
	AlertFiring   = "firing"
	AlertResolved = "resolved"
)

func scanChannel(r scanner) (AlertChannel, error) {
	var c AlertChannel
	var enabled int
	var created, updated int64
	err := r.Scan(&c.ID, &c.Kind, &c.Name, &c.Config, &enabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	c.Enabled, c.CreatedAt, c.UpdatedAt = enabled == 1, ts(created), ts(updated)
	return c, err
}

const channelCols = `id, kind, name, config, enabled, created_at, updated_at`

// ListAlertChannels returns all channels by name.
func (s *Store) ListAlertChannels(ctx context.Context) ([]AlertChannel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+channelCols+` FROM alert_channels ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertChannel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetAlertChannel returns one channel.
func (s *Store) GetAlertChannel(ctx context.Context, id int64) (AlertChannel, error) {
	return scanChannel(s.db.QueryRowContext(ctx, `SELECT `+channelCols+` FROM alert_channels WHERE id = ?`, id))
}

// CreateAlertChannel adds a channel.
func (s *Store) CreateAlertChannel(ctx context.Context, c AlertChannel) (AlertChannel, error) {
	now := s.unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO alert_channels (kind, name, config, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Kind, c.Name, c.Config, b2i(c.Enabled), now, now)
	if err != nil {
		return c, err
	}
	id, _ := res.LastInsertId()
	return s.GetAlertChannel(ctx, id)
}

// UpdateAlertChannel saves name, config and enabled.
func (s *Store) UpdateAlertChannel(ctx context.Context, c AlertChannel) (AlertChannel, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE alert_channels SET name = ?, config = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Config, b2i(c.Enabled), s.unix(), c.ID)
	if err != nil {
		return c, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return c, ErrNotFound
	}
	return s.GetAlertChannel(ctx, c.ID)
}

// DeleteAlertChannel removes a channel.
func (s *Store) DeleteAlertChannel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alert_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const alertCols = `id, rule, key, subject, severity, message, app_id, state, started_at, resolved_at, notified`

func scanAlert(r scanner) (Alert, error) {
	var a Alert
	var appID, resolved sql.NullInt64
	var started int64
	err := r.Scan(&a.ID, &a.Rule, &a.Key, &a.Subject, &a.Severity, &a.Message, &appID, &a.State, &started, &resolved, &a.Notified)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.AppID, a.StartedAt, a.ResolvedAt = intPtr(appID), ts(started), nullTime(resolved)
	return a, err
}

func (s *Store) queryAlerts(ctx context.Context, q string, args ...any) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+alertCols+` FROM alerts `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FiringAlerts returns open incidents, newest first.
func (s *Store) FiringAlerts(ctx context.Context) ([]Alert, error) {
	return s.queryAlerts(ctx, `WHERE state = 'firing' ORDER BY started_at DESC, id DESC`)
}

// ListAlerts returns incidents newest first; state "" = all, appID nil = all apps. before = 0 → newest.
func (s *Store) ListAlerts(ctx context.Context, state string, appID *int64, limit int, before int64) ([]Alert, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q, args := `WHERE 1 = 1`, []any{}
	if state != "" {
		q, args = q+` AND state = ?`, append(args, state)
	}
	if appID != nil {
		q, args = q+` AND app_id = ?`, append(args, *appID)
	}
	if before > 0 {
		q, args = q+` AND id < ?`, append(args, before)
	}
	return s.queryAlerts(ctx, q+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
}

// OpenAlert records a new firing incident. It returns ErrConflict when one is already open for the key.
func (s *Store) OpenAlert(ctx context.Context, a Alert) (Alert, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO alerts (rule, key, subject, severity, message, app_id, state, started_at) VALUES (?, ?, ?, ?, ?, ?, 'firing', ?)`,
		a.Rule, a.Key, a.Subject, a.Severity, a.Message, nullInt(a.AppID), a.StartedAt.Unix())
	if isUnique(err) {
		return a, ErrConflict
	}
	if isForeignKey(err) { // the app was deleted meanwhile
		a.AppID = nil
		return s.OpenAlert(ctx, a)
	}
	if err != nil {
		return a, err
	}
	id, _ := res.LastInsertId()
	return scanAlert(s.db.QueryRowContext(ctx, `SELECT `+alertCols+` FROM alerts WHERE id = ?`, id))
}

// SetAlertNotified stores how many channels were reached.
func (s *Store) SetAlertNotified(ctx context.Context, id int64, n int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET notified = ? WHERE id = ?`, n, id)
	return err
}

// UpdateAlertMessage refreshes the message of an open incident (e.g. a growing restart count).
func (s *Store) UpdateAlertMessage(ctx context.Context, id int64, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET message = ? WHERE id = ? AND state = 'firing'`, message, id)
	return err
}

// ResolveAlert closes an incident.
func (s *Store) ResolveAlert(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET state = 'resolved', resolved_at = ? WHERE id = ? AND state = 'firing'`, at.Unix(), id)
	return err
}

// PruneAlerts deletes resolved incidents older than before.
func (s *Store) PruneAlerts(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE state = 'resolved' AND resolved_at < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneAudit deletes audit entries older than before.
func (s *Store) PruneAudit(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecentMetrics returns the raw points of a series since from, oldest first.
func (s *Store) RecentMetrics(ctx context.Context, series string, from time.Time) ([]MetricPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, cpu_m, mem_b, disk_used, disk_total, rx_bps, tx_bps FROM metric_points
		WHERE series = ? AND res = ? AND ts >= ? ORDER BY ts`, series, MetricRaw, from.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetricPoint
	for rows.Next() {
		var p MetricPoint
		var t int64
		if err := rows.Scan(&t, &p.CPUm, &p.MemB, &p.DiskUsed, &p.DiskTotal, &p.RxBps, &p.TxBps); err != nil {
			return nil, err
		}
		p.T = ts(t)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountArtifacts returns the number of artifact records.
func (s *Store) CountArtifacts(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts`).Scan(&n)
	return n, err
}
