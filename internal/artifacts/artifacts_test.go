package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestReceiveCommitDedup(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tmp, sha, n, err := s.Receive(strings.NewReader("hello"), 10)
	if err != nil || n != 5 {
		t.Fatal(err, n)
	}
	want := sha256.Sum256([]byte("hello"))
	if sha != hex.EncodeToString(want[:]) {
		t.Fatalf("sha = %s", sha)
	}
	if err := s.Commit(tmp, sha); err != nil || !s.Exists(sha) {
		t.Fatal(err)
	}
	tmp2, sha2, _, _ := s.Receive(strings.NewReader("hello"), 10)
	if err := s.Commit(tmp2, sha2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp2); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("duplicate temp file not removed")
	}
	if _, _, _, err := s.Receive(strings.NewReader("hello world"), 5); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	if err := s.Remove(sha); err != nil || s.Exists(sha) {
		t.Fatal("remove failed")
	}
	if s.Exists("../../etc/passwd") {
		t.Fatal("path traversal")
	}
}

func TestResumableUpload(t *testing.T) {
	s, _ := New(t.TempDir())
	id := "abcdefghijklmnop1234"
	if err := s.StartUpload(id); err != nil {
		t.Fatal(err)
	}
	if err := s.StartUpload(id); err == nil {
		t.Fatal("duplicate upload id accepted")
	}
	n, err := s.AppendUpload(id, 0, strings.NewReader("hello "), 100)
	if err != nil || n != 6 {
		t.Fatal(n, err)
	}
	if _, err := s.AppendUpload(id, 3, strings.NewReader("xx"), 100); err != nil {
		t.Fatal("offset behind the file must truncate and continue:", err)
	}
	if _, err := s.AppendUpload(id, 99, strings.NewReader("x"), 100); err == nil {
		t.Fatal("offset beyond the file accepted")
	}
	s.AppendUpload(id, 5, strings.NewReader(" world"), 100)
	tmp, sha, err := s.FinishUpload(id)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(tmp)
	want := sha256.Sum256([]byte("helxx world"))
	if string(b) != "helxx world" || sha != hex.EncodeToString(want[:]) {
		t.Fatalf("content %q sha %s", b, sha)
	}
	if _, err := s.AppendUpload("../x", 0, strings.NewReader(""), 1); err == nil {
		t.Fatal("bad id accepted")
	}
}
