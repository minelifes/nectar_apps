package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Artifact is an uploaded file (content-addressed by SHA-256).
type Artifact struct {
	ID        int64           `json:"id"`
	AppID     int64           `json:"app_id"`
	SHA256    string          `json:"sha256"`
	Filename  string          `json:"filename"`
	Size      int64           `json:"size"`
	Detect    json.RawMessage `json:"detect"`
	CreatedBy *int64          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}

const artifactCols = `id, app_id, sha256, filename, size, detect, created_by, created_at`

func scanArtifact(r scanner) (Artifact, error) {
	var a Artifact
	var det string
	var by sql.NullInt64
	var created int64
	err := r.Scan(&a.ID, &a.AppID, &a.SHA256, &a.Filename, &a.Size, &det, &by, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.Detect, a.CreatedBy, a.CreatedAt = json.RawMessage(det), intPtr(by), ts(created)
	return a, err
}

// CreateArtifact records an uploaded file.
func (s *Store) CreateArtifact(ctx context.Context, appID int64, sha, filename string, size int64, detect any, by *int64) (Artifact, error) {
	det, err := json.Marshal(detect)
	if err != nil {
		return Artifact{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO artifacts (app_id, sha256, filename, size, detect, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		appID, sha, filename, size, string(det), nullInt(by), s.unix())
	if err != nil {
		return Artifact{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetArtifact(ctx, id)
}

// GetArtifact returns an artifact by id.
func (s *Store) GetArtifact(ctx context.Context, id int64) (Artifact, error) {
	return scanArtifact(s.db.QueryRowContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE id = ?`, id))
}

// ListArtifacts returns the artifacts of an app, newest first.
func (s *Store) ListArtifacts(ctx context.Context, appID int64, limit int) ([]Artifact, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE app_id = ? ORDER BY id DESC LIMIT ?`, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ArtifactSHAExists reports whether any artifact row references a content hash.
func (s *Store) ArtifactSHAExists(ctx context.Context, sha string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE sha256 = ?`, sha).Scan(&n)
	return n > 0, err
}

// PruneArtifacts deletes all but the newest keep artifacts of every app, never deleting artifacts that are
// referenced by a live, deploying or pending release. It returns content hashes that are no longer
// referenced by any artifact (their files can be removed).
func (s *Store) PruneArtifacts(ctx context.Context, keep int) ([]string, error) {
	var orphans []string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT a.id, a.sha256 FROM artifacts a
			WHERE (SELECT COUNT(*) FROM artifacts b WHERE b.app_id = a.app_id AND b.id > a.id) >= ?
			AND NOT EXISTS (SELECT 1 FROM releases r WHERE r.artifact_id = a.id AND r.status IN ('live', 'deploying', 'pending'))`, keep)
		if err != nil {
			return err
		}
		type victim struct {
			id  int64
			sha string
		}
		var vs []victim
		for rows.Next() {
			var v victim
			if err := rows.Scan(&v.id, &v.sha); err != nil {
				rows.Close()
				return err
			}
			vs = append(vs, v)
		}
		rows.Close()
		for _, v := range vs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE id = ?`, v.id); err != nil {
				return err
			}
		}
		seen := map[string]bool{}
		for _, v := range vs {
			if seen[v.sha] {
				continue
			}
			seen[v.sha] = true
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE sha256 = ?`, v.sha).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				orphans = append(orphans, v.sha)
			}
		}
		return nil
	})
	return orphans, err
}

// --------------------------------------------------------------- uploads --

// Upload is an in-progress resumable upload.
type Upload struct {
	ID        string
	AppID     int64
	Filename  string
	Size      int64
	Offset    int64
	CreatedBy *int64
	ExpiresAt time.Time
}

// CreateUpload starts a resumable upload.
func (s *Store) CreateUpload(ctx context.Context, u Upload, ttl time.Duration) error {
	now := s.unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO uploads (id, app_id, filename, size, offset, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		u.ID, u.AppID, u.Filename, u.Size, nullInt(u.CreatedBy), now, now+int64(ttl.Seconds()))
	return err
}

// GetUpload returns an unexpired upload.
func (s *Store) GetUpload(ctx context.Context, id string) (Upload, error) {
	var u Upload
	var by sql.NullInt64
	var exp int64
	err := s.db.QueryRowContext(ctx, `SELECT id, app_id, filename, size, offset, created_by, expires_at FROM uploads WHERE id = ? AND expires_at > ?`, id, s.unix()).
		Scan(&u.ID, &u.AppID, &u.Filename, &u.Size, &u.Offset, &by, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	u.CreatedBy, u.ExpiresAt = intPtr(by), ts(exp)
	return u, err
}

// SetUploadOffset records progress. It only advances from the expected previous offset.
func (s *Store) SetUploadOffset(ctx context.Context, id string, from, to int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE uploads SET offset = ? WHERE id = ? AND offset = ?`, to, id, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}
	return nil
}

// DeleteUpload removes an upload record.
func (s *Store) DeleteUpload(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM uploads WHERE id = ?`, id)
	return err
}

// ExpiredUploads returns and deletes expired upload records (callers remove the temp files).
func (s *Store) ExpiredUploads(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM uploads WHERE expires_at <= ?`, s.unix())
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.DeleteUpload(ctx, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}
