// Package store is the SQLite persistence layer of the panel. It only depends on database/sql;
// the driver (modernc.org/sqlite, pure Go) is registered by the binary.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Common errors.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

// Store wraps the database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// DSN returns a modernc.org/sqlite connection string for path with the pragmas the panel needs.
func DSN(path string) string {
	return "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
}

// Open opens the database with the given driver ("sqlite") and DSN and applies migrations.
func Open(ctx context.Context, driver, dsn string) (*Store, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; a single connection keeps things simple and avoids
	// SQLITE_BUSY between our own goroutines. The panel's write volume is tiny.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle (for backups and tests).
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) unix() int64 { return s.now().Unix() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT IFNULL(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		base := strings.TrimPrefix(f, "migrations/")
		v, err := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("bad migration name %q", base)
		}
		if v <= current {
			continue
		}
		body, err := migrationsFS.ReadFile(f)
		if err != nil {
			return err
		}
		err = s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("%s: %w", base, err)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, v, s.unix())
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// isUnique reports whether err is a UNIQUE constraint violation (driver-independent check).
func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKey reports whether err is a FOREIGN KEY constraint violation.
func isForeignKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

func ts(unix int64) time.Time { return time.Unix(unix, 0).UTC() }

func nullTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := ts(v.Int64)
	return &t
}

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
