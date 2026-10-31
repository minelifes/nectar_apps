package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Preview is a preview environment: an app copied from Parent for one pull request.
type Preview struct {
	AppID     int64     `json:"app_id"`
	ParentID  int64     `json:"parent_id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Author    string    `json:"author"`
	HeadRef   string    `json:"head_ref"`
	CommitSHA string    `json:"commit_sha"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const previewCols = `app_id, parent_id, number, title, url, author, head_ref, commit_sha, provider, created_at, updated_at`

func scanPreview(sc interface{ Scan(...any) error }) (Preview, error) {
	var p Preview
	var c, u int64
	err := sc.Scan(&p.AppID, &p.ParentID, &p.Number, &p.Title, &p.URL, &p.Author, &p.HeadRef, &p.CommitSHA, &p.Provider, &c, &u)
	p.CreatedAt, p.UpdatedAt = ts(c), ts(u)
	return p, err
}

// SavePreview creates or updates a preview (UpdatedAt = now).
func (s *Store) SavePreview(ctx context.Context, p Preview) error {
	now := s.unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO previews (`+previewCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (app_id) DO UPDATE SET title = excluded.title, url = excluded.url, author = excluded.author, head_ref = excluded.head_ref,
		commit_sha = excluded.commit_sha, provider = excluded.provider, updated_at = excluded.updated_at`,
		p.AppID, p.ParentID, p.Number, p.Title, p.URL, p.Author, p.HeadRef, p.CommitSHA, p.Provider, now, now)
	return err
}

// GetPreview returns the preview of an app (ErrNotFound when the app is not a preview).
func (s *Store) GetPreview(ctx context.Context, appID int64) (Preview, error) {
	p, err := scanPreview(s.db.QueryRowContext(ctx, `SELECT `+previewCols+` FROM previews WHERE app_id = ?`, appID))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// FindPreview returns the preview of a pull request of a parent app.
func (s *Store) FindPreview(ctx context.Context, parentID int64, number int) (Preview, error) {
	p, err := scanPreview(s.db.QueryRowContext(ctx, `SELECT `+previewCols+` FROM previews WHERE parent_id = ? AND number = ?`, parentID, number))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ListPreviews returns the previews of a parent app (0 = all), newest pull request first.
func (s *Store) ListPreviews(ctx context.Context, parentID int64) ([]Preview, error) {
	q, args := `SELECT `+previewCols+` FROM previews`, []any{}
	if parentID != 0 {
		q, args = q+` WHERE parent_id = ?`, append(args, parentID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY number DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Preview{}
	for rows.Next() {
		p, err := scanPreview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
