package store

import (
	"context"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestStore opens a fresh database in a temp dir.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), "sqlite", DSN(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
