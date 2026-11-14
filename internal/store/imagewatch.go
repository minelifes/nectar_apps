package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Image watch states.
const (
	WatchCurrent = "current" // the app runs what its tag points to
	WatchUpdate  = "update"  // the tag points to a newer image
	WatchError   = "error"   // the registry could not be asked
	WatchUnknown = "unknown" // not known what the app runs (not running)
)

// ImageWatch is the last image update check of an app.
type ImageWatch struct {
	AppID         int64     `json:"app_id"`
	Image         string    `json:"image"`
	RunningDigest string    `json:"running_digest,omitempty"`
	LatestDigest  string    `json:"latest_digest,omitempty"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	SeenAt        time.Time `json:"seen_at,omitzero"` // when the newer image was first seen
	HandledDigest string    `json:"-"`
	CheckedAt     time.Time `json:"checked_at"`
}

// SaveImageWatch stores the result of a check.
func (s *Store) SaveImageWatch(ctx context.Context, w ImageWatch) error {
	var seen int64
	if !w.SeenAt.IsZero() {
		seen = w.SeenAt.Unix()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO image_watch (app_id, image, running_digest, latest_digest, status, message, seen_at, handled_digest, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET image = excluded.image, running_digest = excluded.running_digest, latest_digest = excluded.latest_digest,
			status = excluded.status, message = excluded.message, seen_at = excluded.seen_at, handled_digest = excluded.handled_digest,
			checked_at = excluded.checked_at`,
		w.AppID, w.Image, w.RunningDigest, w.LatestDigest, w.Status, w.Message, seen, w.HandledDigest, w.CheckedAt.Unix())
	return err
}

// GetImageWatch returns the app's last check (ErrNotFound: never checked).
func (s *Store) GetImageWatch(ctx context.Context, appID int64) (ImageWatch, error) {
	var w ImageWatch
	var seen, checked int64
	err := s.db.QueryRowContext(ctx, `SELECT app_id, image, running_digest, latest_digest, status, message, seen_at, handled_digest, checked_at
		FROM image_watch WHERE app_id = ?`, appID).Scan(&w.AppID, &w.Image, &w.RunningDigest, &w.LatestDigest, &w.Status, &w.Message, &seen, &w.HandledDigest, &checked)
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrNotFound
	}
	if seen > 0 {
		w.SeenAt = time.Unix(seen, 0).UTC()
	}
	w.CheckedAt = time.Unix(checked, 0).UTC()
	return w, err
}
