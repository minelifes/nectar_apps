package store

import (
	"context"
	"time"
)

// PushDevice is an installation of the Nectar app that receives push notifications.
type PushDevice struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"-"`
	Platform  string    `json:"platform"` // android | ios | macos
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

// RegisterPushDevice stores (or moves to this user) a device token.
func (s *Store) RegisterPushDevice(ctx context.Context, userID int64, token, platform, name string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO push_devices (user_id, token, platform, name, created_at, last_seen) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET user_id = excluded.user_id, platform = excluded.platform, name = excluded.name, last_seen = excluded.last_seen`,
		userID, token, platform, name, now, now)
	return err
}

// DeletePushDevice forgets a token (userID 0 = whoever registered it).
func (s *Store) DeletePushDevice(ctx context.Context, userID int64, token string) error {
	if userID == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM push_devices WHERE token = ?`, token)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_devices WHERE token = ? AND user_id = ?`, token, userID)
	return err
}

// DeletePushDeviceID removes one of a user's devices.
func (s *Store) DeletePushDeviceID(ctx context.Context, userID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_devices WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// ListPushDevices returns the devices of a user (0 = every user).
func (s *Store) ListPushDevices(ctx context.Context, userID int64) ([]PushDevice, error) {
	q, args := `SELECT id, user_id, token, platform, name, created_at, last_seen FROM push_devices`, []any{}
	if userID != 0 {
		q, args = q+` WHERE user_id = ?`, append(args, userID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushDevice{}
	for rows.Next() {
		var d PushDevice
		var c, l int64
		if err := rows.Scan(&d.ID, &d.UserID, &d.Token, &d.Platform, &d.Name, &c, &l); err != nil {
			return nil, err
		}
		d.CreatedAt, d.LastSeen = time.Unix(c, 0).UTC(), time.Unix(l, 0).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}
