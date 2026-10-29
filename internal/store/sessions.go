package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Session is one login (web cookie or mobile token pair).
type Session struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"-"`
	Kind             string     `json:"kind"`
	TokenExpiresAt   time.Time  `json:"expires_at"`
	RefreshExpiresAt *time.Time `json:"-"`
	UserAgent        string     `json:"user_agent"`
	IP               string     `json:"ip"`
	CreatedAt        time.Time  `json:"created_at"`
	LastSeenAt       time.Time  `json:"last_seen_at"`
}

// NewSession describes a session to create.
type NewSession struct {
	UserID      int64
	Kind        string // "web" | "mobile"
	TokenHash   string
	TokenTTL    time.Duration
	RefreshHash string // mobile only
	RefreshTTL  time.Duration
	UserAgent   string
	IP          string
}

const sessionCols = `id, user_id, kind, token_expires_at, refresh_expires_at, user_agent, ip, created_at, last_seen_at`

func scanSession(r scanner) (Session, error) {
	var se Session
	var exp, created, seen int64
	var rexp sql.NullInt64
	err := r.Scan(&se.ID, &se.UserID, &se.Kind, &exp, &rexp, &se.UserAgent, &se.IP, &created, &seen)
	if errors.Is(err, sql.ErrNoRows) {
		return se, ErrNotFound
	}
	se.TokenExpiresAt, se.CreatedAt, se.LastSeenAt = ts(exp), ts(created), ts(seen)
	se.RefreshExpiresAt = nullTime(rexp)
	return se, err
}

// CreateSession stores a new session.
func (s *Store) CreateSession(ctx context.Context, n NewSession) (Session, error) {
	now := s.unix()
	var refresh, refreshExp any
	if n.RefreshHash != "" {
		refresh, refreshExp = n.RefreshHash, now+int64(n.RefreshTTL.Seconds())
	}
	if len(n.UserAgent) > 300 {
		n.UserAgent = n.UserAgent[:300]
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO sessions
		(user_id, kind, token_hash, token_expires_at, refresh_hash, refresh_expires_at, user_agent, ip, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Kind, n.TokenHash, now+int64(n.TokenTTL.Seconds()), refresh, refreshExp, n.UserAgent, n.IP, now, now)
	if err != nil {
		return Session{}, err
	}
	id, _ := res.LastInsertId()
	return scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE id = ?`, id))
}

// SessionByToken returns the valid (unexpired) session for a token hash, together with its user.
// If the session was last seen more than slideAfter ago, its expiry is extended by slideTTL (web sessions);
// pass slideTTL = 0 to disable sliding.
func (s *Store) SessionByToken(ctx context.Context, tokenHash string, slideAfter, slideTTL time.Duration) (Session, User, error) {
	now := s.unix()
	se, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE token_hash = ? AND token_expires_at > ?`, tokenHash, now))
	if err != nil {
		return Session{}, User{}, err
	}
	u, err := s.GetUser(ctx, se.UserID)
	if err != nil {
		return Session{}, User{}, err
	}
	if now-se.LastSeenAt.Unix() > int64(slideAfter.Seconds()) {
		exp := se.TokenExpiresAt.Unix()
		if slideTTL > 0 {
			exp = now + int64(slideTTL.Seconds())
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, token_expires_at = ? WHERE id = ?`, now, exp, se.ID); err == nil {
			se.LastSeenAt, se.TokenExpiresAt = ts(now), ts(exp)
		}
	}
	return se, u, nil
}

// RotateMobile validates a refresh token, replaces the access token and slides the refresh expiry.
func (s *Store) RotateMobile(ctx context.Context, refreshHash, newTokenHash string, tokenTTL, refreshTTL time.Duration) (Session, User, error) {
	now := s.unix()
	var id, userID int64
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id FROM sessions WHERE refresh_hash = ? AND refresh_expires_at > ?`, refreshHash, now).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, ErrNotFound
	}
	if err != nil {
		return Session{}, User{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET token_hash = ?, token_expires_at = ?, refresh_expires_at = ?, last_seen_at = ? WHERE id = ?`,
		newTokenHash, now+int64(tokenTTL.Seconds()), now+int64(refreshTTL.Seconds()), now, id)
	if err != nil {
		return Session{}, User{}, err
	}
	se, err := scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionCols+` FROM sessions WHERE id = ?`, id))
	if err != nil {
		return Session{}, User{}, err
	}
	u, err := s.GetUser(ctx, userID)
	return se, u, err
}

// ListSessions returns the unexpired sessions of a user, newest first.
func (s *Store) ListSessions(ctx context.Context, userID int64) ([]Session, error) {
	now := s.unix()
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionCols+` FROM sessions
		WHERE user_id = ? AND (token_expires_at > ? OR refresh_expires_at > ?) ORDER BY last_seen_at DESC`, userID, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		se, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// DeleteSession deletes one session of a user.
func (s *Store) DeleteSession(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserSessions revokes all sessions of a user (e.g. after a password change), except keepID.
func (s *Store) DeleteUserSessions(ctx context.Context, userID, keepID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepID)
	return err
}

// PurgeExpiredSessions removes sessions whose tokens are all expired.
func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	now := s.unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_expires_at <= ? AND (refresh_expires_at IS NULL OR refresh_expires_at <= ?)`, now, now)
	return err
}
