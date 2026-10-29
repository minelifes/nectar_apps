package api

import (
	"context"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), "sqlite", store.DSN(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
