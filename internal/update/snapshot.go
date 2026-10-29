package update

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SnapshotPrefix starts the file names of pre-update snapshots.
const SnapshotPrefix = "pre-update-"

// Snapshot copies a SQLite database with VACUUM INTO (consistent while the owner keeps writing) into dir
// and keeps the newest keep snapshots. It refuses to start when the disk is too full.
func Snapshot(ctx context.Context, driver, dbPath, dir, name string, keep int) (string, error) {
	st, err := os.Stat(dbPath)
	if err != nil {
		return "", fmt.Errorf("database: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if free, ok := freeSpace(dir); ok && free < 2*st.Size()+32<<20 {
		return "", fmt.Errorf("not enough free space for a backup: %d MB free, the database has %d MB", free>>20, st.Size()>>20)
	}
	out := filepath.Join(dir, SnapshotPrefix+safeName(name)+".db")
	_ = os.Remove(out)
	db, err := sql.Open(driver, "file:"+dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, out); err != nil {
		return "", fmt.Errorf("vacuum into: %w", err)
	}
	prune(dir, keep)
	return out, nil
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

func prune(dir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		name string
		mod  int64
	}
	var list []f
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), SnapshotPrefix) || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			list = append(list, f{e.Name(), info.ModTime().UnixNano()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod > list[j].mod })
	for _, x := range list[min(keep, len(list)):] {
		_ = os.Remove(filepath.Join(dir, x.name))
	}
}

// RestoreFile puts a snapshot back in place of the database (its owner must be stopped).
func RestoreFile(snapshot, dbPath string) error {
	if !strings.HasPrefix(filepath.Base(snapshot), SnapshotPrefix) {
		return fmt.Errorf("%s is not a pre-update snapshot", snapshot)
	}
	in, err := os.Open(snapshot)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dbPath + ".restore"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(tmp, dbPath)
}
