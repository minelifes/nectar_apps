package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Release statuses.
const (
	ReleasePending    = "pending"
	ReleaseDeploying  = "deploying"
	ReleaseLive       = "live"
	ReleaseFailed     = "failed"
	ReleaseSuperseded = "superseded"
)

// Release is one deploy of an app: an artifact (or image) plus a config snapshot.
type Release struct {
	ID         int64      `json:"id"`
	AppID      int64      `json:"app_id"`
	Number     int        `json:"number"`
	ArtifactID *int64     `json:"artifact_id"`
	Image      string     `json:"image"`
	Config     AppConfig  `json:"config"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	CreatedBy  *int64     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

const releaseCols = `id, app_id, number, artifact_id, image, config, status, message, created_by, created_at, finished_at`

func scanRelease(r scanner) (Release, error) {
	var rel Release
	var art, by, fin sql.NullInt64
	var cfg string
	var created int64
	err := r.Scan(&rel.ID, &rel.AppID, &rel.Number, &art, &rel.Image, &cfg, &rel.Status, &rel.Message, &by, &created, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return rel, ErrNotFound
	}
	if err != nil {
		return rel, err
	}
	rel.ArtifactID, rel.CreatedBy, rel.CreatedAt, rel.FinishedAt = intPtr(art), intPtr(by), ts(created), nullTime(fin)
	return rel, json.Unmarshal([]byte(cfg), &rel.Config)
}

// CreateRelease creates the next release (number = previous + 1) in status pending.
func (s *Store) CreateRelease(ctx context.Context, appID int64, artifactID *int64, image string, cfg AppConfig, by *int64) (Release, error) {
	cj, err := json.Marshal(cfg)
	if err != nil {
		return Release{}, err
	}
	var id int64
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT IFNULL(MAX(number), 0) + 1 FROM releases WHERE app_id = ?`, appID).Scan(&next); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO releases (app_id, number, artifact_id, image, config, status, created_by, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`,
			appID, next, nullInt(artifactID), image, string(cj), nullInt(by), s.unix())
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return Release{}, err
	}
	return s.GetRelease(ctx, id)
}

// GetRelease returns a release by id.
func (s *Store) GetRelease(ctx context.Context, id int64) (Release, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases WHERE id = ?`, id))
}

// GetReleaseByNumber returns release #number of an app.
func (s *Store) GetReleaseByNumber(ctx context.Context, appID int64, number int) (Release, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases WHERE app_id = ? AND number = ?`, appID, number))
}

// ListReleases returns the releases of an app, newest first.
func (s *Store) ListReleases(ctx context.Context, appID int64, limit int) ([]Release, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.queryReleases(ctx, `SELECT `+releaseCols+` FROM releases WHERE app_id = ? ORDER BY number DESC LIMIT ?`, appID, limit)
}

// ReleasesInStatus returns all releases with the given status (e.g. to resume after a restart).
func (s *Store) ReleasesInStatus(ctx context.Context, status string) ([]Release, error) {
	return s.queryReleases(ctx, `SELECT `+releaseCols+` FROM releases WHERE status = ? ORDER BY id`, status)
}

// LiveRelease returns the current live release of an app.
func (s *Store) LiveRelease(ctx context.Context, appID int64) (Release, error) {
	return scanRelease(s.db.QueryRowContext(ctx, `SELECT `+releaseCols+` FROM releases WHERE app_id = ? AND status = 'live' ORDER BY number DESC LIMIT 1`, appID))
}

func (s *Store) queryReleases(ctx context.Context, q string, args ...any) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetReleaseStatus updates a release. Terminal statuses set finished_at. When a release becomes
// live, the previous live release of the same app becomes superseded.
func (s *Store) SetReleaseStatus(ctx context.Context, id int64, status, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var fin any
		if status == ReleaseLive || status == ReleaseFailed || status == ReleaseSuperseded {
			fin = s.unix()
		}
		res, err := tx.ExecContext(ctx, `UPDATE releases SET status = ?, message = ?, finished_at = IFNULL(?, finished_at) WHERE id = ?`, status, message, fin, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if status == ReleaseLive {
			_, err = tx.ExecContext(ctx, `UPDATE releases SET status = 'superseded' WHERE status = 'live' AND id <> ? AND app_id = (SELECT app_id FROM releases WHERE id = ?)`, id, id)
		}
		return err
	})
}

// HasActiveRelease reports whether the app has a pending or deploying release.
func (s *Store) HasActiveRelease(ctx context.Context, appID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM releases WHERE app_id = ? AND status IN ('pending', 'deploying')`, appID).Scan(&n)
	return n > 0, err
}
