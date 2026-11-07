package volfiles

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name, body string
	mode       os.FileMode
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	return buf.Bytes()
}

func planOf(t *testing.T, data []byte) (*ZipPlan, error) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return PlanZip(zr)
}

func TestPlanZipRejectsUnsafe(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "/etc/passwd", "C:/x", "a\\..\\..\\x"} {
		if _, err := planOf(t, makeZip(t, zipEntry{name: name, body: "x"})); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
	// symlinks are skipped, Finder litter ignored
	p, err := planOf(t, makeZip(t, zipEntry{name: "link", body: "/etc", mode: os.ModeSymlink | 0o777},
		zipEntry{name: "__MACOSX/._a", body: "x"}, zipEntry{name: "a/.DS_Store", body: "x"}, zipEntry{name: "ok.txt", body: "y"}))
	if err != nil || p.Files != 1 || p.Dirs != 0 || p.Skipped != 1 {
		t.Fatalf("plan: %+v %v", p, err)
	}
}

// TarSize must be exact: the pod reads exactly that many bytes ("head -c").
func TestPlanZipTarSize(t *testing.T) {
	long := strings.Repeat("very-long-folder-name/", 8) + "file-with-a-long-name.txt" // > 100 chars: PAX records
	p, err := planOf(t, makeZip(t, zipEntry{name: "a.txt", body: "hello"}, zipEntry{name: long, body: strings.Repeat("x", 1000)},
		zipEntry{name: "empty/", mode: os.ModeDir | 0o755}, zipEntry{name: "ü/ñ.txt", body: ""}))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := p.WriteTar(&buf); err != nil {
		t.Fatal(err)
	}
	if int64(buf.Len()) != p.TarSize {
		t.Fatalf("tar is %d bytes, plan says %d", buf.Len(), p.TarSize)
	}
}

func TestUnzipWithBusybox(t *testing.T) {
	bb, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("busybox not installed")
	}
	root := t.TempDir()
	s := &Session{b: &Browser{Kube: localExec{root: root, busybox: bb}}, namespace: "ns", pod: "p"}
	ctx := context.Background()
	os.MkdirAll(filepath.Join(root, "site/css"), 0o755)
	os.WriteFile(filepath.Join(root, "site/index.html"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(root, "site/keep.txt"), []byte("keep"), 0o644)
	os.WriteFile(filepath.Join(root, "site/css/old.css"), []byte("o"), 0o644)

	big := strings.Repeat("0123456789", 300_000) // 3 MB: several pipe/exec chunks
	p, err := planOf(t, makeZip(t,
		zipEntry{name: "index.html", body: "new"},
		zipEntry{name: "css/app.css", body: "body{}"},
		zipEntry{name: "img/", mode: os.ModeDir | 0o755},
		zipEntry{name: "deep/a/b/run.sh", body: "#!/bin/sh", mode: 0o755},
		zipEntry{name: ".env", body: "A=1"},
		zipEntry{name: "data/big.bin", body: big},
	))
	if err != nil {
		t.Fatal(err)
	}
	var sent int64
	if err := s.Unzip(ctx, Clean("site"), p, func(n int64) { sent = n }); err != nil {
		t.Fatal(err)
	}
	if sent != p.TarSize {
		t.Fatalf("progress ended at %d of %d", sent, p.TarSize)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(root, p)); return string(b) }
	if read("site/index.html") != "new" || read("site/keep.txt") != "keep" || read("site/css/old.css") != "o" ||
		read("site/css/app.css") != "body{}" || read("site/.env") != "A=1" || read("site/deep/a/b/run.sh") != "#!/bin/sh" ||
		read("site/data/big.bin") != big {
		t.Fatal("unexpected content after unzip")
	}
	if st, err := os.Stat(filepath.Join(root, "site/img")); err != nil || !st.IsDir() {
		t.Fatal("empty folder missing")
	}
	if st, _ := os.Stat(filepath.Join(root, "site/deep/a/b/run.sh")); st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("script lost its x bit: %v", st.Mode())
	}
	entries, _ := os.ReadDir(filepath.Join(root, "site"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nectar-unzip") {
			t.Fatalf("temporary folder left: %s", e.Name())
		}
	}

	// a file in the archive where the volume has a folder (deep down): refused before anything moves
	p, _ = planOf(t, makeZip(t, zipEntry{name: "index.html", body: "newer"}, zipEntry{name: "deep/a/b", body: "x"}))
	if err := s.Unzip(ctx, Clean("site"), p, nil); err == nil || !strings.Contains(err.Error(), "deep/a/b") {
		t.Fatalf("file over folder: %v", err)
	}
	if read("site/index.html") != "new" || read("site/deep/a/b/run.sh") != "#!/bin/sh" {
		t.Fatal("changed despite the clash")
	}
	if err := s.Unzip(ctx, Clean("missing"), p, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing folder: %v", err)
	}
	if free, err := s.FreeBytes(ctx); err != nil || free <= 0 {
		t.Fatalf("free space: %d %v", free, err)
	}
}
