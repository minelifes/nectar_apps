package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// AuditEntry is one recorded action.
type AuditEntry struct {
	ID        int64           `json:"id"`
	UserID    *int64          `json:"user_id"`
	UserEmail string          `json:"user_email"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	Payload   json.RawMessage `json:"payload"`
	IP        string          `json:"ip"`
	CreatedAt time.Time       `json:"created_at"`
}

// Audit records an action. payload may be nil. Never put secrets into payloads.
func (s *Store) Audit(ctx context.Context, userID *int64, userEmail, action, target, ip string, payload any) error {
	pj := []byte("{}")
	if payload != nil {
		var err error
		if pj, err = json.Marshal(payload); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_log (user_id, user_email, action, target, payload, ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullInt(userID), userEmail, action, target, string(pj), ip, s.unix())
	return err
}

// ListAudit returns entries newest first. before = 0 means from the newest.
func (s *Store) ListAudit(ctx context.Context, limit int, before int64) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, user_email, action, target, payload, ip, created_at
		FROM audit_log WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var uid sql.NullInt64
		var payload string
		var created int64
		if err := rows.Scan(&e.ID, &uid, &e.UserEmail, &e.Action, &e.Target, &payload, &e.IP, &created); err != nil {
			return nil, err
		}
		e.UserID, e.Payload, e.CreatedAt = intPtr(uid), json.RawMessage(payload), ts(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListAuditForTarget returns entries about one project: target == slug or target starting with "slug/"
// (its apps), not older than since (so a re-created project does not inherit an old project's history).
func (s *Store) ListAuditForTarget(ctx context.Context, slug string, since time.Time, limit int, before int64) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, user_email, action, target, payload, ip, created_at
		FROM audit_log WHERE id < ? AND (target = ? OR substr(target, 1, ?) = ?) AND created_at >= ?
		ORDER BY id DESC LIMIT ?`, before, slug, len(slug)+1, slug+"/", since.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var uid sql.NullInt64
		var payload string
		var created int64
		if err := rows.Scan(&e.ID, &uid, &e.UserEmail, &e.Action, &e.Target, &payload, &e.IP, &created); err != nil {
			return nil, err
		}
		e.UserID, e.Payload, e.CreatedAt = intPtr(uid), json.RawMessage(payload), ts(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ProjectExists reports whether a project id exists.
func (s *Store) ProjectExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// GetSetting loads a JSON setting into out. Returns ErrNotFound if unset.
func (s *Store) GetSetting(ctx context.Context, key string, out any) error {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(v), out)
}

// SetSetting stores a JSON setting.
func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, string(b), s.unix())
	return err
}
