// Package artifacts stores uploaded files on disk, content-addressed by SHA-256:
//
//	<dir>/blobs/ab/abcdef…   committed artifacts
//	<dir>/uploads/<id>       in-progress resumable uploads
//	<dir>/tmp/               single-request uploads being received
package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// ErrTooLarge is returned when an upload exceeds the size limit.
var ErrTooLarge = errors.New("file is too large")

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSHA reports whether s is a lowercase hex SHA-256.
func ValidSHA(s string) bool { return shaRe.MatchString(s) }

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// Store manages artifact files.
type Store struct{ dir string }

// New creates the directory layout under dir.
func New(dir string) (*Store, error) {
	for _, d := range []string{"blobs", "uploads", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir}, nil
}

// Path returns the file path of a committed artifact.
func (s *Store) Path(sha string) string {
	return filepath.Join(s.dir, "blobs", sha[:2], sha)
}

// Exists reports whether a committed artifact file exists.
func (s *Store) Exists(sha string) bool {
	if !ValidSHA(sha) {
		return false
	}
	_, err := os.Stat(s.Path(sha))
	return err == nil
}

// Receive streams r into a temp file (at most max bytes), returning the temp path, SHA-256 and size.
// The caller must Commit or remove the temp file.
func (s *Store) Receive(r io.Reader, max int64) (tmp, sha string, size int64, err error) {
	f, err := os.CreateTemp(filepath.Join(s.dir, "tmp"), "upload-*")
	if err != nil {
		return "", "", 0, err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, max+1))
	if err != nil {
		return "", "", 0, err
	}
	if n > max {
		return "", "", 0, ErrTooLarge
	}
	if err := f.Sync(); err != nil {
		return "", "", 0, err
	}
	return f.Name(), hex.EncodeToString(h.Sum(nil)), n, nil
}

// Commit moves a received file into the blob store under its hash (deduplicated).
func (s *Store) Commit(tmp, sha string) error {
	if !ValidSHA(sha) {
		return errors.New("invalid sha256")
	}
	dst := s.Path(sha)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return os.Remove(tmp) // identical content already stored
	}
	return os.Rename(tmp, dst)
}

// Remove deletes a committed artifact file.
func (s *Store) Remove(sha string) error {
	if !ValidSHA(sha) {
		return errors.New("invalid sha256")
	}
	err := os.Remove(s.Path(sha))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ------------------------------------------------------- resumable (tus) --

func (s *Store) uploadPath(id string) (string, error) {
	if !idRe.MatchString(id) {
		return "", errors.New("invalid upload id")
	}
	return filepath.Join(s.dir, "uploads", id), nil
}

// StartUpload creates an empty file for a resumable upload.
func (s *Store) StartUpload(id string) error {
	p, err := s.uploadPath(id)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// AppendUpload appends at most limit bytes from r at offset and returns the number of bytes written.
// It fails if the file on disk does not have exactly offset bytes (client and server out of sync).
func (s *Store) AppendUpload(id string, offset int64, r io.Reader, limit int64) (int64, error) {
	p, err := s.uploadPath(id)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if st.Size() != offset {
		// a previous PATCH may have been cut off after writing part of its body: truncate back
		if st.Size() > offset {
			if err := f.Truncate(offset); err != nil {
				return 0, err
			}
		} else {
			return 0, fmt.Errorf("upload offset mismatch: have %d, got %d", st.Size(), offset)
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, limit))
	if err != nil && n == 0 {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return n, err
	}
	return n, nil // a partial body is fine: the client resumes from the new offset
}

// FinishUpload hashes a completed upload and moves it to the tmp area, returning (tmpPath, sha).
func (s *Store) FinishUpload(id string) (string, string, error) {
	p, err := s.uploadPath(id)
	if err != nil {
		return "", "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", "", err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return "", "", err
	}
	tmp := filepath.Join(s.dir, "tmp", "tus-"+id)
	if err := os.Rename(p, tmp); err != nil {
		return "", "", err
	}
	return tmp, hex.EncodeToString(h.Sum(nil)), nil
}

// RemoveUpload deletes an in-progress upload file.
func (s *Store) RemoveUpload(id string) error {
	p, err := s.uploadPath(id)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Usage reports the disk space used by committed artifacts and by unfinished uploads.
func (s *Store) Usage() (blobBytes int64, blobs int, uploadBytes int64) {
	size := func(dir string, count *int) int64 {
		var total int64
		_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil {
				total += info.Size()
				if count != nil {
					*count++
				}
			}
			return nil
		})
		return total
	}
	blobBytes = size(filepath.Join(s.dir, "blobs"), &blobs)
	uploadBytes = size(filepath.Join(s.dir, "uploads"), nil) + size(filepath.Join(s.dir, "tmp"), nil)
	return blobBytes, blobs, uploadBytes
}

// Size returns the size of a committed artifact file (0 if missing).
func (s *Store) Size(sha string) int64 {
	if !ValidSHA(sha) {
		return 0
	}
	if info, err := os.Stat(s.Path(sha)); err == nil {
		return info.Size()
	}
	return 0
}
