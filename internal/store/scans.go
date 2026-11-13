package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Image scan states.
const (
	ScanRunning = "running"
	ScanDone    = "done"
	ScanFailed  = "failed"
)

// ImageScan is the last vulnerability scan of an app's image. Findings is the scanner's JSON list.
type ImageScan struct {
	AppID      int64           `json:"app_id"`
	Image      string          `json:"image"`
	Status     string          `json:"status"`
	Message    string          `json:"message,omitempty"`
	Total      int             `json:"total"`
	Critical   int             `json:"critical"`
	High       int             `json:"high"`
	Medium     int             `json:"medium"`
	Low        int             `json:"low"`
	Unknown    int             `json:"unknown"`
	Fixable    int             `json:"fixable"`
	Findings   json.RawMessage `json:"findings"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// StartImageScan records a running scan of the app's image, replacing the previous one. ErrConflict when
// one is already running.
func (s *Store) StartImageScan(ctx context.Context, appID int64, image string) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO image_scans (app_id, image, status, started_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET image = excluded.image, status = excluded.status, message = '', total = 0, critical = 0,
			high = 0, medium = 0, low = 0, unknown = 0, fixable = 0, findings = '[]', started_at = excluded.started_at, finished_at = 0
		WHERE image_scans.status <> ?`, appID, image, ScanRunning, s.unix(), ScanRunning)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}
	return nil
}

// FinishImageScan stores the result of the running scan (sc.Status done or failed).
func (s *Store) FinishImageScan(ctx context.Context, sc ImageScan) error {
	if len(sc.Findings) == 0 {
		sc.Findings = json.RawMessage("[]")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE image_scans SET status = ?, message = ?, total = ?, critical = ?, high = ?, medium = ?,
		low = ?, unknown = ?, fixable = ?, findings = ?, finished_at = ? WHERE app_id = ?`,
		sc.Status, sc.Message, sc.Total, sc.Critical, sc.High, sc.Medium, sc.Low, sc.Unknown, sc.Fixable, string(sc.Findings), s.unix(), sc.AppID)
	return err
}

const scanCols = `app_id, image, status, message, total, critical, high, medium, low, unknown, fixable, findings, started_at, finished_at`

func scanImageScan(r interface{ Scan(...any) error }) (ImageScan, error) {
	var sc ImageScan
	var findings string
	var st, fin int64
	err := r.Scan(&sc.AppID, &sc.Image, &sc.Status, &sc.Message, &sc.Total, &sc.Critical, &sc.High, &sc.Medium, &sc.Low,
		&sc.Unknown, &sc.Fixable, &findings, &st, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return sc, ErrNotFound
	}
	sc.Findings = json.RawMessage(findings)
	sc.StartedAt = time.Unix(st, 0).UTC()
	if fin > 0 {
		t := time.Unix(fin, 0).UTC()
		sc.FinishedAt = &t
	}
	return sc, err
}

// GetImageScan returns the app's last scan (ErrNotFound when it was never scanned).
func (s *Store) GetImageScan(ctx context.Context, appID int64) (ImageScan, error) {
	return scanImageScan(s.db.QueryRowContext(ctx, `SELECT `+scanCols+` FROM image_scans WHERE app_id = ?`, appID))
}

// ListImageScans returns every app's last scan without the findings (for lists and the re-scan loop).
func (s *Store) ListImageScans(ctx context.Context) ([]ImageScan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT app_id, image, status, message, total, critical, high, medium, low, unknown, fixable,
		'[]', started_at, finished_at FROM image_scans ORDER BY app_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageScan
	for rows.Next() {
		sc, err := scanImageScan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// FailInterruptedImageScans: at startup, scans that were running when the panel stopped.
func (s *Store) FailInterruptedImageScans(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE image_scans SET status = ?, message = 'the panel restarted during the scan: run it again', finished_at = ?
		WHERE status = ?`, ScanFailed, s.unix(), ScanRunning)
	return err
}
